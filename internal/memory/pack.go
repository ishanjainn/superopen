package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/session"
)

const (
	packBudget     = 800
	nextPackBudget = 350
	searchDumpEst  = 8000
	packTimeout    = 1500 * time.Millisecond
)

type Pack struct {
	Text           string `json:"text"`
	Tokens         int    `json:"tokens"`
	PendingSession string `json:"pending_session,omitempty"`
	AskDistill     bool   `json:"ask_distill,omitempty"`
}

func PackForRoot(root, cue, currentSession string) (Pack, error) {
	ctx, cancel := context.WithTimeout(context.Background(), packTimeout)
	defer cancel()
	store, err := OpenQuick(paths.Resolve(root).Database)
	if err != nil {
		return Pack{}, err
	}
	defer store.Close()
	if err := store.Ping(ctx); err != nil {
		return Pack{}, err
	}
	return store.BuildPack(cue, currentSession)
}

func PackNextForRoot(root, cue, currentSession string) (Pack, error) {
	ctx, cancel := context.WithTimeout(context.Background(), packTimeout)
	defer cancel()
	store, err := OpenQuick(paths.Resolve(root).Database)
	if err != nil {
		return Pack{}, err
	}
	defer store.Close()
	if err := store.Ping(ctx); err != nil {
		return Pack{}, err
	}
	return store.BuildNextPack(cue, currentSession)
}

func (s *Store) BuildNextPack(cue, currentSession string) (Pack, error) {
	var b strings.Builder
	budget := nextPackBudget
	res, _ := s.Recall(cue, nextPackBudget)
	hits := res.Hits
	if len(hits) == 0 {
		hits, _ = s.Search(SearchFilter{Query: cue, Limit: 6})
	}
	if len(hits) == 0 {
		return Pack{}, nil
	}
	writeBudget(&b, &budget, "Next:")
	for _, hit := range hits {
		if hit.Kind == KindTool {
			continue
		}
		line := fmt.Sprintf("  #%d %s", hit.ID, compactLine(hit.Episode))
		if !writeBudget(&b, &budget, line) {
			break
		}
	}
	text := strings.TrimSpace(b.String())
	return Pack{Text: text, Tokens: EstimateTokens(text)}, nil
}

func (s *Store) BuildPack(cue, currentSession string) (Pack, error) {
	var b strings.Builder
	budget := packBudget

	working, err := s.currentWorking(currentSession)
	if err == nil && working.ID != 0 {
		line := fmt.Sprintf("Working: %s", compactLine(working))
		writeBudget(&b, &budget, line)
		if len(working.Files) > 0 {
			writeBudget(&b, &budget, "Files: "+strings.Join(clipSlice(working.Files, 6), ", "))
		}
	}

	moments, _ := s.Search(SearchFilter{Query: cue, Kind: KindPrompt, Limit: 8})
	if len(moments) == 0 {
		moments, _ = s.Search(SearchFilter{Kind: KindPrompt, Limit: 8})
	}
	if len(moments) > 0 {
		writeBudget(&b, &budget, "Moments:")
		for _, hit := range moments {
			if hit.Faded {
				continue
			}
			line := fmt.Sprintf("  #%d %s %s ~%d", hit.ID, hit.Kind, compactLine(hit.Episode), hit.Tokens)
			if !writeBudget(&b, &budget, line) {
				break
			}
		}
	}

	ltm, _ := s.Search(SearchFilter{Query: cue, Limit: 12})
	wroteLTM := false
	for _, hit := range ltm {
		if hit.Kind != KindSession && hit.Kind != KindPin && hit.Kind != KindTeaching {
			continue
		}
		if !wroteLTM {
			if !writeBudget(&b, &budget, "Long-term:") {
				break
			}
			wroteLTM = true
		}
		line := fmt.Sprintf("  #%d %s %s ~%d", hit.ID, hit.Kind, compactLine(hit.Episode), hit.Tokens)
		if !writeBudget(&b, &budget, line) {
			break
		}
	}

	pending := ""
	for _, id := range s.PendingDistill() {
		if id != currentSession {
			pending = id
			break
		}
	}
	if pending == "" && len(s.PendingDistill()) > 0 {
		pending = s.PendingDistill()[0]
	}
	ask := pending != "" && !s.HasSessionRollup(pending)
	if ask {
		writeBudget(&b, &budget, fmt.Sprintf("If continuing last session #%s: memory_capture once with request/learned/next. Skip if unrelated.", pending))
	}
	writeBudget(&b, &budget, "Fetch: memory_get / so memory get. Hints, not authority.")

	text := strings.TrimSpace(b.String())
	tokens := EstimateTokens(text)
	if tokens > packBudget {
		runes := []rune(text)
		// chars/4 ≈ tokens; trim to budget.
		keep := packBudget * 4
		if keep > len(runes) {
			keep = len(runes)
		}
		text = strings.TrimSpace(string(runes[:keep]))
		tokens = EstimateTokens(text)
	}
	if text != "" {
		saved := searchDumpEst - tokens
		if saved < 0 {
			saved = 0
		}
		_ = s.RecordPack(tokens, saved)
	}
	return Pack{Text: text, Tokens: tokens, PendingSession: pending, AskDistill: ask}, nil
}

func (s *Store) currentWorking(sessionID string) (Episode, error) {
	if sessionID != "" {
		ep, err := s.scanOne(`SELECT `+episodeCols+` FROM memory_episodes WHERE session_id=? AND kind=? ORDER BY updated_at DESC LIMIT 1`, sessionID, KindWorking)
		if err == nil {
			return ep, nil
		}
	}
	return s.LatestSessionKind(KindWorking)
}

func (s *Store) WorkingSnapshot(sessionID string) string {
	ep, err := s.currentWorking(sessionID)
	if err != nil || ep.ID == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Working snapshot: %s\n", compactLine(ep))
	if ep.Text != "" {
		fmt.Fprintf(&b, "%s\n", firstLine(ep.Text, 240))
	}
	if len(ep.Files) > 0 {
		fmt.Fprintf(&b, "Files: %s\n", strings.Join(clipSlice(ep.Files, 8), ", "))
	}
	b.WriteString("Memory is hints, not authority.")
	text := strings.TrimSpace(b.String())
	if EstimateTokens(text) > 200 {
		runes := []rune(text)
		if len(runes) > 800 {
			text = string(runes[:800])
		}
	}
	return text
}

func compactLine(ep Episode) string {
	t := firstLine(ep.Title, 72)
	if t == "" {
		t = firstLine(ep.Text, 72)
	}
	return t
}

func writeBudget(b *strings.Builder, budget *int, line string) bool {
	cost := EstimateTokens(line + "\n")
	if *budget-cost < 0 && b.Len() > 0 {
		return false
	}
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	b.WriteString(line)
	*budget -= cost
	return *budget > 0
}

func clipSlice(in []string, n int) []string {
	if len(in) <= n {
		return in
	}
	return in[:n]
}

// CompactSnapshot is the Cursor preCompact inject: this-session working set,
// fail-open, never a full JSONL dump.
func CompactSnapshot(root, sessionID string) string {
	store, err := OpenQuick(paths.Resolve(root).Database)
	if err == nil {
		defer store.Close()
		if text := store.WorkingSnapshot(sessionID); text != "" {
			return text
		}
	}
	meta, err := session.NewStore(paths.Resolve(root)).Get(sessionID)
	if err != nil {
		return ""
	}
	title := strings.TrimSpace(meta.Title)
	if title == "" {
		title = strings.TrimSpace(meta.PromptPreview)
	}
	if title == "" {
		return ""
	}
	return strings.TrimSpace("Working snapshot: " + firstLine(title, 120) + "\nMemory is hints, not authority.")
}
