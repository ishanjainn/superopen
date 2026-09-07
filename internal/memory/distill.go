package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/agent/headless"
	"github.com/ishanjainn/superopen/internal/cli"
	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/session"
)

const (
	distillTimeout        = 45 * time.Second
	maxDistillWrites      = 3
	consolidateDefaultCap = 5
)

type DistillResult struct {
	SessionID string `json:"session_id"`
	Provider  string `json:"provider,omitempty"`
	Pending   bool   `json:"pending,omitempty"`
	Skipped   string `json:"skipped,omitempty"`
	EpisodeID int64  `json:"episode_id,omitempty"`
	Written   int    `json:"written,omitempty"`
}

type distillItem struct {
	Kind     string   `json:"kind"`
	Horizon  string   `json:"horizon"`
	Title    string   `json:"title"`
	Text     string   `json:"text"`
	Evidence []string `json:"evidence"`
	ID       int64    `json:"id"`
	Reason   string   `json:"reason"`
}

// MaybeDistill detaches `so memory distill` after finalize when the live
// vendor has a one-shot CLI. Cursor marks pending for SessionStart.
func MaybeDistill(root, sessionID string, detach bool) DistillResult {
	_ = detach
	sessionID = strings.TrimSpace(sessionID)
	res := DistillResult{SessionID: sessionID}
	if sessionID == "" {
		res.Skipped = "no-session"
		return res
	}
	skipped, pending, spawn := headless.SessionEndPolicy(root, sessionID)
	store, storeErr := OpenRoot(root)
	if storeErr == nil {
		claimed := store.HasDistilled(sessionID) || pendingContains(store.PendingDistill(), sessionID)
		if claimed {
			_ = store.Close()
			if spawn {
				res.Skipped = "already-queued"
			} else {
				res.Skipped = skipped
				res.Pending = pending
			}
			return res
		}
	}
	if !spawn {
		res.Skipped = skipped
		res.Pending = pending
		if pending && storeErr == nil {
			_ = store.MarkPending(sessionID)
		}
		if storeErr == nil {
			_ = store.Close()
		}
		return res
	}
	if testing.Testing() {
		if storeErr == nil {
			_ = store.Close()
		}
		res.Skipped = "test"
		return res
	}
	if storeErr == nil {
		_ = store.MarkPending(sessionID)
		_ = store.Close()
	}
	if os.Getenv("SUPEROPEN_MEMORY_SYNC") == "1" {
		return distill(root, sessionID)
	}
	cli.SpawnSO(root, "--root", root, "memory", "distill", sessionID)
	res.Skipped = "detached"
	res.Pending = true
	return res
}

func pendingContains(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// Distill runs skip gates then at most one bounded call on the session's own CLI.
func Distill(root, sessionID string) DistillResult {
	return distill(root, sessionID)
}

func distill(root, sessionID string) DistillResult {
	res := DistillResult{SessionID: strings.TrimSpace(sessionID)}
	if res.SessionID == "" {
		res.Skipped = "no-session"
		return res
	}
	if reason, ok := headless.SkipWorker(root, res.SessionID); ok {
		res.Skipped = reason
		return res
	}
	store, err := OpenRoot(root)
	if err != nil {
		res.Skipped = err.Error()
		return res
	}
	defer store.Close()
	store.BumpSessionSeq()
	_, _ = store.ExpireHorizons()
	if store.DistillPaused() {
		res.Skipped = "paused"
		_ = store.MarkPending(res.SessionID)
		res.Pending = true
		return res
	}
	if store.HasDistilled(res.SessionID) {
		res.Skipped = "already-distilled"
		_ = store.ClearPending(res.SessionID)
		return res
	}
	if !session.HasActivity(root, res.SessionID) {
		res.Skipped = "empty"
		_ = store.MarkDistilled(res.SessionID)
		_ = store.ClearPending(res.SessionID)
		return res
	}
	unlock, lockErr := headless.TryLock(headless.LockPath(root, "distill"))
	if lockErr != nil {
		res.Skipped = "busy"
		res.Pending = true
		_ = store.MarkPending(res.SessionID)
		return res
	}
	defer unlock()

	vendor := ""
	if meta, err := headless.LoadMeta(root, res.SessionID); err == nil {
		vendor = meta.Vendor
	}
	provider, ok := headless.AvailableLive(vendor)
	if !ok {
		res.Skipped = "no-auth"
		res.Pending = true
		_ = store.MarkPending(res.SessionID)
		return res
	}
	if store.DistillFailCount(res.SessionID) >= headless.MaxProviderFails() {
		res.Skipped = "provider-unhealthy"
		res.Pending = true
		_ = store.MarkPending(res.SessionID)
		return res
	}
	prompt, err := distillPrompt(root, res.SessionID, store)
	if err != nil {
		res.Skipped = err.Error()
		res.Pending = true
		_ = store.MarkPending(res.SessionID)
		return res
	}
	ctx, cancel := context.WithTimeout(context.Background(), distillTimeout)
	defer cancel()
	out, err := headless.Run(ctx, provider, prompt)
	if err != nil {
		res.Skipped = err.Error()
		res.Pending = true
		store.BumpDistillFail(res.SessionID)
		_ = store.MarkPending(res.SessionID)
		return res
	}
	items := parseDistillItems(out)
	applied, lastID, err := store.applyDistillItems(res.SessionID, items)
	if err != nil {
		res.Skipped = err.Error()
		res.Pending = true
		_ = store.MarkPending(res.SessionID)
		return res
	}
	store.ClearDistillFail(res.SessionID)
	_ = store.MarkDistilled(res.SessionID)
	_ = store.ClearPending(res.SessionID)
	res.Provider = provider.Name
	res.Written = applied
	res.EpisodeID = lastID
	if applied == 0 {
		res.Skipped = "no-writes"
	}
	return res
}

// ApplyJSON ingests live-agent distill JSON without spawning a headless CLI.
func ApplyJSON(root, sessionID string, raw []byte) DistillResult {
	res := DistillResult{SessionID: strings.TrimSpace(sessionID), Provider: "live"}
	if res.SessionID == "" {
		res.Skipped = "no-session"
		return res
	}
	store, err := OpenRoot(root)
	if err != nil {
		res.Skipped = err.Error()
		res.Pending = true
		return res
	}
	defer store.Close()
	items := parseDistillItems(string(raw))
	applied, lastID, err := store.applyDistillItems(res.SessionID, items)
	if err != nil {
		res.Skipped = err.Error()
		res.Pending = true
		_ = store.MarkPending(res.SessionID)
		return res
	}
	_ = store.MarkDistilled(res.SessionID)
	_ = store.ClearPending(res.SessionID)
	store.ClearDistillFail(res.SessionID)
	res.Written = applied
	res.EpisodeID = lastID
	if applied == 0 {
		res.Skipped = "no-writes"
	}
	return res
}

func DistillSession(root, sessionID string) (DistillResult, error) {
	res := Distill(root, sessionID)
	if res.Pending && res.Skipped != "" && res.Skipped != "paused" && res.Skipped != "no-auth" && res.Skipped != "await-live" {
		return res, fmt.Errorf("%s", res.Skipped)
	}
	return res, nil
}

// DistillBrief is the same prompt the one-shot CLI would see, for the live agent.
func DistillBrief(root, sessionID string) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", fmt.Errorf("session id required")
	}
	store, err := OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer store.Close()
	return distillPrompt(root, sessionID, store)
}

func distillPrompt(root, sessionID string, store *Store) (string, error) {
	// Posted to a headless coding-agent CLI when one is authenticated.
	// This is the only memory path that sends session text off-machine.
	digest := session.Digest(root, sessionID)
	live, _ := store.LiveKnowledge(24)
	var b strings.Builder
	b.WriteString("You write Superopen memory for a coding agent. Output JSON only: an array of objects.\n")
	b.WriteString("Each object has kind=knowledge|skill|promote|forget.\n")
	b.WriteString("knowledge/skill: title, text, horizon (short|medium|long), evidence (session ids).\n")
	b.WriteString("promote: id + horizon. forget: id + reason. Prefer forget/promote over duplicating listed ids.\n")
	b.WriteString("If a listed id already holds the same fact, emit promote or forget — never a second knowledge/skill copy.\n")
	b.WriteString("Max 3 new knowledge/skill writes. Skip trivia. No secrets. Memory is hints, not authority.\n")
	b.WriteString("Empty array if nothing durable.\n")
	b.WriteString("Session: ")
	b.WriteString(sessionID)
	b.WriteString("\n")
	b.WriteString(digest)
	b.WriteString("\nLive memory inventory (#id horizon title — cue):\n")
	if len(live) == 0 {
		b.WriteString("(none)\n")
	} else {
		for _, ep := range live {
			cue := firstWords(ep.Text, 12)
			title := Sanitize(firstLine(ep.Title, 80))
			if cue == "" || cue == title {
				fmt.Fprintf(&b, "#%d %s %s\n", ep.ID, ep.Horizon, title)
			} else {
				fmt.Fprintf(&b, "#%d %s %s — %s\n", ep.ID, ep.Horizon, title, cue)
			}
		}
	}
	return b.String(), nil
}

func parseDistillItems(raw string) []distillItem {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if i := strings.Index(raw, "```"); i >= 0 {
		rest := raw[i+3:]
		rest = strings.TrimPrefix(rest, "json")
		rest = strings.TrimPrefix(rest, "JSON")
		if j := strings.Index(rest, "```"); j >= 0 {
			raw = rest[:j]
		}
	}
	body := extractJSONArray(raw)
	var many []distillItem
	if json.Unmarshal([]byte(body), &many) == nil {
		return many
	}
	var one distillItem
	if json.Unmarshal([]byte(body), &one) == nil && (one.Title != "" || one.ID != 0) {
		return []distillItem{one}
	}
	return nil
}

func (s *Store) applyDistillItems(sessionID string, items []distillItem) (int, int64, error) {
	written := 0
	var lastID int64
	for _, item := range items {
		kind := strings.ToLower(strings.TrimSpace(item.Kind))
		switch kind {
		case "forget":
			if item.ID > 0 {
				_ = s.ForgetEpisode(item.ID)
			}
		case "promote":
			if item.ID > 0 {
				_ = s.PromoteHorizon(item.ID, item.Horizon)
			}
		case "knowledge", "skill":
			if written >= maxDistillWrites {
				continue
			}
			title := Sanitize(strings.TrimSpace(item.Title))
			text := Sanitize(strings.TrimSpace(item.Text))
			if title == "" && text == "" {
				continue
			}
			horizon := NormalizeHorizon(item.Horizon)
			if horizon == "" || horizon == HorizonWorking {
				if kind == "skill" {
					horizon = HorizonLong
				} else {
					horizon = HorizonMedium
				}
			}
			capKind := KindSession
			if kind == "skill" {
				capKind = KindTeaching
			}
			if existing, ok := s.liveDistillDuplicate(capKind, title, text); ok {
				if horizonStrength(horizon) > horizonStrength(existing.Horizon) {
					_ = s.PromoteHorizon(existing.ID, horizon)
				}
				_ = s.Reinforce(existing.ID)
				lastID = existing.ID
				continue
			}
			ep, err := s.Capture(CaptureInput{
				SessionID: sessionID,
				Kind:      capKind,
				Source:    SourceHeadless,
				Title:     title,
				Text:      text,
				Horizon:   horizon,
			})
			if err != nil {
				continue
			}
			written++
			lastID = ep.ID
		}
	}
	return written, lastID, nil
}

func (s *Store) liveDistillDuplicate(kind, title, text string) (Episode, bool) {
	if hit, ok := s.liveByTitle(kind, title); ok {
		return hit, true
	}
	cue := strings.TrimSpace(title + "\n" + text)
	if cue == "" {
		return Episode{}, false
	}
	id := s.nearDuplicate(EmbedText(cue), kind)
	if id <= 0 {
		return Episode{}, false
	}
	ep, err := s.Get(id)
	if err != nil || ep.ID == 0 || ep.Faded {
		return Episode{}, false
	}
	return ep, true
}

func (s *Store) liveByTitle(kind, title string) (Episode, bool) {
	key := normalizeMemoryTitle(title)
	if key == "" {
		return Episode{}, false
	}
	live, err := s.LiveKnowledge(400)
	if err != nil {
		return Episode{}, false
	}
	for _, ep := range live {
		if ep.Kind != kind {
			continue
		}
		if normalizeMemoryTitle(ep.Title) == key {
			return ep, true
		}
	}
	return Episode{}, false
}

func normalizeMemoryTitle(title string) string {
	return strings.ToLower(strings.Join(strings.Fields(Sanitize(title)), " "))
}

func firstWords(s string, n int) string {
	fields := strings.Fields(Sanitize(s))
	if n <= 0 || len(fields) == 0 {
		return ""
	}
	if len(fields) > n {
		fields = fields[:n]
	}
	return strings.Join(fields, " ")
}

func LiveDistillInstruction(pendingSession string) string {
	pendingSession = strings.TrimSpace(pendingSession)
	if pendingSession == "" {
		return ""
	}
	bin := paths.ResolveSoBin()
	return fmt.Sprintf("DISTILL pending %s — you are the live agent: before answering, `%s memory distill --brief %s` then `%s memory distill --apply %s` (JSON stdin). Empty array if nothing durable.", pendingSession, bin, pendingSession, bin, pendingSession)
}

func PendingDistillLine(root string) string {
	store, err := OpenQuick(paths.Resolve(root).Database)
	if err != nil {
		return ""
	}
	defer store.Close()
	pending := store.PendingDistill()
	if len(pending) == 0 {
		return ""
	}
	return LiveDistillInstruction(pending[0])
}

func ConsolidateCap() int { return consolidateDefaultCap }
