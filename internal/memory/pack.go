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
	packBudget       = 800
	nextPackBudget   = 350
	promptPackBudget = 450
	searchDumpEst    = 8000
	packTimeout      = 1500 * time.Millisecond
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
			line := fmt.Sprintf("  #%d %s %s ~%dt", hit.ID, hit.Kind, compactLine(hit.Episode), hit.Tokens)
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
		line := fmt.Sprintf("  #%d %s %s ~%dt", hit.ID, hit.Kind, compactLine(hit.Episode), hit.Tokens)
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
		writeBudget(&b, &budget, LiveDistillInstruction(pending))
	}
	writeBudget(&b, &budget, "Fetch: so memory get. Then so graph query to verify. Hints, not authority.")

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

const sessionIndexBudget = 350

// LiveMemoryCount is faded non-tool, non-prompt rows on knowledge horizons
// (KindSession diary captures included). KindPrompt is never knowledge (M3).
func (s *Store) LiveMemoryCount() int {
	var n int
	_ = s.db.QueryRow(`SELECT count(*) FROM memory_episodes
WHERE faded=0 AND kind NOT IN (?, ?) AND horizon IN (?, ?, ?)`,
		KindTool, KindPrompt, HorizonShort, HorizonMedium, HorizonLong).Scan(&n)
	return n
}

// CountLiveMemories is fail-open for hooks.
func CountLiveMemories(root string) int {
	store, err := OpenQuick(paths.Resolve(root).Database)
	if err != nil {
		return 0
	}
	defer store.Close()
	return store.LiveMemoryCount()
}

// FTSDocCount is how many plaintext FTS rows exist (0 if missing).
func (s *Store) FTSDocCount() int {
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM memory_episodes_fts`).Scan(&n); err != nil {
		return 0
	}
	return n
}

// IndexSealed reports a leftover ciphertext FTS (search cannot see bodies).
func (s *Store) IndexSealed() bool {
	return s.ftsLooksSealed()
}

// EmptyHitHint distinguishes empty store vs no match vs sealed FTS (AXI hint:).
func EmptyHitHint(live, fts int, sealed bool) string {
	if live <= 0 {
		return "no saved memories in this store"
	}
	if sealed {
		return fmt.Sprintf("%d memories exist but the lexical index is sealed; run so memory recall", live)
	}
	if fts <= 0 {
		return fmt.Sprintf("%d memories exist but the lexical index is empty; run so memory recall", live)
	}
	return fmt.Sprintf("no match for this cue; %d memories exist — search is a title index; so memory recall returns bodies", live)
}

// EmptyRecallHint is the empty-hit line for so memory recall (which prints bodies).
func EmptyRecallHint(live, fts int, sealed bool) string {
	if live <= 0 {
		return "no saved memories in this store"
	}
	if sealed {
		return fmt.Sprintf("%d memories exist but the lexical index is sealed; try different terms", live)
	}
	if fts <= 0 {
		return fmt.Sprintf("%d memories exist but the lexical index is empty; try different terms", live)
	}
	return fmt.Sprintf("no match for this cue; %d memories exist — try different terms", live)
}

// SessionStartIndex is the SessionStart inject: when live memories exist,
// say how many and give the Bash recall command. Fail-open, no bodies and
// no uncued #id list (there is no question yet). Prompt-submit uses
// PromptRecallPack for matching bodies. Empty store stays silent except a
// pending distill one-liner.
func SessionStartIndex(root string) string {
	store, err := OpenQuick(paths.Resolve(root).Database)
	if err != nil {
		return ""
	}
	defer store.Close()
	text := store.BuildSessionIndex()
	pending := store.PendingDistill()
	if len(pending) == 0 {
		return text
	}
	line := LiveDistillInstruction(pending[0])
	if strings.TrimSpace(text) == "" {
		return line
	}
	return strings.TrimSpace(text + "\n" + line)
}

// PromptRecallPack is the UserPromptSubmit inject for a prior-work / personal
// cue: matching recalled bodies from this workspace store, framed as the user's
// own notes. Empty when recall has no hits (caller may fall back to the
// pointer-only index). Fail-open.
func PromptRecallPack(root, cue string) string {
	ctx, cancel := context.WithTimeout(context.Background(), packTimeout)
	defer cancel()
	store, err := OpenQuick(paths.Resolve(root).Database)
	if err != nil {
		return ""
	}
	defer store.Close()
	if err := store.Ping(ctx); err != nil {
		return ""
	}
	return store.BuildPromptRecall(cue)
}

func (s *Store) BuildPromptRecall(cue string) string {
	cue = strings.TrimSpace(cue)
	if cue == "" {
		return ""
	}
	res, err := s.Recall(cue, promptPackBudget)
	if err != nil || len(res.Hits) == 0 {
		return ""
	}
	bin := paths.ResolveSoBin()
	var b strings.Builder
	budget := promptPackBudget
	header := fmt.Sprintf("Superopen: matching notes from your past sessions in this workspace .so/ store (not built-in memory or MEMORY.md). Quote them and cite #id when answering.")
	if !writeBudget(&b, &budget, header) {
		return strings.TrimSpace(header)
	}
	wrote := false
	fetchID := int64(0)
	for _, hit := range res.Hits {
		if hit.Kind == KindTool || hit.Kind == KindPrompt || hit.Kind == KindWorking {
			continue
		}
		line := FormatIndexLine(hit.Episode)
		body := strings.TrimSpace(hit.Text)
		if body != "" {
			// Several short query windows beat one 350-token diary: the
			// answer is often in hit 2–3 (Sweden in a later session).
			body = clipAroundQuery(body, cue, 120)
			line = line + "\n" + body
		}
		if !writeBudget(&b, &budget, line) {
			break
		}
		if fetchID == 0 {
			fetchID = hit.ID
		}
		wrote = true
	}
	if !wrote {
		return ""
	}
	writeBudget(&b, &budget, fmt.Sprintf("Fetch more in your shell: `%s memory recall '<question>'` or `%s memory get %d --full`. Titles that look like import ids are still this workspace diary. If two notes conflict, cite both #ids and pick the most specific or recent. If this pack does not answer, run recall with a second cue. Quote the note and cite #id. Memory is hints, not authority.", bin, bin, fetchID))
	return strings.TrimSpace(b.String())
}

func (s *Store) BuildSessionIndex() string {
	n := s.LiveMemoryCount()
	if n <= 0 {
		return ""
	}
	bin := paths.ResolveSoBin()
	text := fmt.Sprintf("Superopen: %d memories in this workspace .so/ store - your own notes from past sessions (not built-in memory or MEMORY.md). Titles that look like import ids are still this workspace diary. If two notes conflict cite both #ids. If recall misses try a second cue. Run in your shell: `%s memory recall '<question>'`", n, bin)
	if EstimateTokens(text) > sessionIndexBudget {
		runes := []rune(text)
		keep := sessionIndexBudget * 4
		if keep > len(runes) {
			keep = len(runes)
		}
		text = strings.TrimSpace(string(runes[:keep]))
	}
	saved := searchDumpEst - EstimateTokens(text)
	if saved < 0 {
		saved = 0
	}
	_ = s.RecordPack(EstimateTokens(text), saved)
	return text
}

func compactLine(ep Episode) string {
	return displayTitle(ep, 72)
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
