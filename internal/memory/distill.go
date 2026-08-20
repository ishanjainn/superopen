package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/agent/headless"
	"github.com/ishanjainn/superopen/internal/cli"
	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/session"
)

const distillTimeout = 45 * time.Second

type DistillResult struct {
	SessionID string `json:"session_id"`
	Provider  string `json:"provider,omitempty"`
	Pending   bool   `json:"pending,omitempty"`
	Skipped   string `json:"skipped,omitempty"`
	EpisodeID int64  `json:"episode_id,omitempty"`
}

// MaybeDistill runs after local ingest. Headless distill detaches when a CLI
// is authenticated; otherwise the session is marked pending for a one-shot
// live-agent capture at the next SessionStart. Fail-open.
func MaybeDistill(root, sessionID string, detach bool) DistillResult {
	res := DistillResult{SessionID: sessionID}
	store, err := OpenRoot(root)
	if err != nil {
		res.Skipped = err.Error()
		return res
	}
	defer store.Close()
	if store.DistillPaused() {
		res.Skipped = "paused"
		_ = store.MarkPending(sessionID)
		res.Pending = true
		return res
	}
	if store.HasSessionRollup(sessionID) {
		res.Skipped = "already_rolled_up"
		_ = store.ClearPending(sessionID)
		return res
	}
	if _, ok := headless.Available(); ok {
		if detach {
			cli.SpawnSO(root, "memory", "distill", sessionID)
			res.Provider = "detach"
			return res
		}
		got, err := DistillSession(root, sessionID)
		if err != nil {
			_ = store.MarkPending(sessionID)
			res.Pending = true
			res.Skipped = err.Error()
			return res
		}
		return got
	}
	_ = store.MarkPending(sessionID)
	res.Pending = true
	return res
}

func DistillSession(root, sessionID string) (DistillResult, error) {
	res := DistillResult{SessionID: sessionID}
	provider, ok := headless.Available()
	if !ok {
		store, err := OpenRoot(root)
		if err != nil {
			return res, err
		}
		defer store.Close()
		_ = store.MarkPending(sessionID)
		res.Pending = true
		res.Skipped = "no_headless_cli"
		return res, nil
	}
	prompt, err := distillPrompt(root, sessionID)
	if err != nil {
		return res, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), distillTimeout)
	defer cancel()
	out, err := headless.Run(ctx, provider, prompt)
	if err != nil {
		store, openErr := OpenRoot(root)
		if openErr == nil {
			_ = store.MarkPending(sessionID)
			store.Close()
		}
		res.Pending = true
		return res, err
	}
	parsed := parseDistillJSON(out)
	text := parsed
	if text == "" {
		text = Sanitize(out)
	}
	ep, err := CaptureRoot(root, CaptureInput{
		SessionID: sessionID,
		Kind:      KindSession,
		Source:    SourceHeadless,
		Title:     "session rollup",
		Text:      text,
	})
	if err != nil {
		return res, err
	}
	res.Provider = provider.Name
	res.EpisodeID = ep.ID
	return res, nil
}

func distillPrompt(root, sessionID string) (string, error) {
	layout := paths.Resolve(root)
	store, err := OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer store.Close()
	hits, err := store.Search(SearchFilter{SessionID: sessionID, Limit: 24, IncludeFaded: false})
	if err != nil {
		return "", err
	}
	meta, _ := session.NewStore(layout).Get(sessionID)
	var b strings.Builder
	b.WriteString("Summarize this coding session as JSON only: {\"request\":\"...\",\"learned\":\"...\",\"next\":\"...\"}. ")
	b.WriteString("No secrets. Memory is hints, not authority. ~120 tokens total.\n")
	if meta.Title != "" {
		fmt.Fprintf(&b, "Title: %s\n", Sanitize(meta.Title))
	}
	if meta.PromptPreview != "" {
		fmt.Fprintf(&b, "Prompt: %s\n", Sanitize(meta.PromptPreview))
	}
	b.WriteString("Moments:\n")
	for _, hit := range hits {
		if hit.Kind == KindWorking {
			continue
		}
		fmt.Fprintf(&b, "- [%s] %s %s\n", hit.Kind, Sanitize(hit.Title), Sanitize(firstLine(hit.Text, 160)))
	}
	return b.String(), nil
}

func parseDistillJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return ""
	}
	var payload struct {
		Request string `json:"request"`
		Learned string `json:"learned"`
		Next    string `json:"next"`
	}
	if json.Unmarshal([]byte(raw[start:end+1]), &payload) != nil {
		return ""
	}
	parts := nonempty(payload.Request, payload.Learned, payload.Next)
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	if payload.Request != "" {
		fmt.Fprintf(&b, "request: %s\n", Sanitize(payload.Request))
	}
	if payload.Learned != "" {
		fmt.Fprintf(&b, "learned: %s\n", Sanitize(payload.Learned))
	}
	if payload.Next != "" {
		fmt.Fprintf(&b, "next: %s\n", Sanitize(payload.Next))
	}
	return strings.TrimSpace(b.String())
}

func LiveDistillInstruction(pendingSession string) string {
	pendingSession = strings.TrimSpace(pendingSession)
	if pendingSession == "" {
		return ""
	}
	return fmt.Sprintf("Last session #%s has moments but no rollup. If you will continue that work, call memory_capture once with {request, learned, next} (budget ~120 tokens). Skip if unrelated. Memory is hints, not authority.", pendingSession)
}
