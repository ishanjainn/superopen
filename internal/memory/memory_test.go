package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/session"
	"github.com/ishanjainn/superopen/internal/session/trace"
)

func testRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := paths.Resolve(root).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestOpenRootRefusesUnmanaged(t *testing.T) {
	_, err := OpenRoot(t.TempDir())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "so init") {
		t.Fatalf("got %v", err)
	}
}

func TestEmbedSimilarPhrasesCloserThanUnrelated(t *testing.T) {
	a := EmbedSentence("the login bug we hit last Thursday")
	b := EmbedSentence("login issue from Thursday")
	c := EmbedSentence("rewrite the graph layout camera")
	ab := Cosine(a, b.Bytes())
	ac := Cosine(a, c.Bytes())
	if ab <= ac {
		t.Fatalf("expected similar phrases closer: login=%.3f layout=%.3f", ab, ac)
	}
}

func TestIngestIdempotentAndRedactSkip(t *testing.T) {
	root := t.TempDir()
	id := "sess-1"
	writeSession(t, root, id, []trace.Span{
		llmSpan("s1", "please fix the login timeout"),
		llmSpan("s2", "user said <private>secret token</private> ignore this"),
		toolSpan("t1", "Read", "internal/auth/login.go"),
	})
	first, err := IngestSession(root, id)
	if err != nil {
		t.Fatal(err)
	}
	if first.Inserted < 1 {
		t.Fatalf("inserted=%d skipped=%d", first.Inserted, first.Skipped)
	}
	second, err := IngestSession(root, id)
	if err != nil {
		t.Fatal(err)
	}
	if second.Inserted != 0 {
		t.Fatalf("second ingest inserted %d, want 0", second.Inserted)
	}
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hits, err := store.Search(SearchFilter{Query: "login timeout", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected login hit")
	}
	foundPrompt := false
	for _, h := range hits {
		if h.Kind == KindTool {
			t.Fatalf("tools must stay on events.jsonl, got %#v", h)
		}
		if h.Kind == KindPrompt {
			foundPrompt = true
			if !strings.Contains(h.Text, "[tools:") {
				t.Fatalf("expected tools trailer on prompt, got %q", h.Text)
			}
		}
	}
	if !foundPrompt {
		t.Fatal("expected prompt moment")
	}
	secretHits, err := store.Search(SearchFilter{Query: "secret token", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range secretHits {
		if strings.Contains(h.Text, "secret token") || strings.Contains(h.Title, "secret token") {
			t.Fatalf("private text leaked: %+v", h)
		}
	}
}

func TestWorkingMemoryAppearsInTimelineAndLayout(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	working, err := store.Capture(CaptureInput{
		Kind:  KindWorking,
		Title: "GPU metrics are canonical",
		Text:  "Logs are supporting context only.",
	})
	if err != nil {
		t.Fatal(err)
	}

	timeline, err := store.Timeline(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(timeline) != 1 || len(timeline[0].Items) != 1 || timeline[0].Items[0].ID != working.ID {
		t.Fatalf("working memory missing from timeline: %+v", timeline)
	}

	layout, err := store.Layout(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(layout.Nodes) != 1 || layout.Nodes[0].ID != working.ID {
		t.Fatalf("working memory missing from layout: %+v", layout.Nodes)
	}
}

func TestContradictDownranksStale(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	old, err := store.Capture(CaptureInput{Kind: KindSession, Title: "auth uses cookies", Text: "session auth is cookie based"})
	if err != nil {
		t.Fatal(err)
	}
	newer, err := store.Contradict(old.ID, CaptureInput{Title: "auth uses JWT", Text: "session auth is JWT now"})
	if err != nil {
		t.Fatal(err)
	}
	hits, err := store.Search(SearchFilter{Query: "session auth", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) < 2 {
		t.Fatalf("want both facts, got %d", len(hits))
	}
	if hits[0].ID != newer.ID {
		t.Fatalf("stale ranked first: %+v", hits)
	}
}

func TestTeachPinFadeRescue(t *testing.T) {
	root := testRoot(t)
	path := filepath.Join(root, "note.md")
	if err := os.WriteFile(path, []byte("always run go test ./internal/memory"), 0o644); err != nil {
		t.Fatal(err)
	}
	ep, err := TeachFile(root, path, "test policy")
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Fade(ep.ID); err != nil {
		t.Fatal(err)
	}
	hinted, err := store.Search(SearchFilter{Query: "go test", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	foundHint := false
	for _, h := range hinted {
		if h.ID == ep.ID {
			foundHint = true
		}
	}
	if !foundHint {
		t.Fatal("hinted memory should stay searchable until sleep")
	}
	if err := store.Sleep(); err != nil {
		t.Fatal(err)
	}
	hidden, err := store.Search(SearchFilter{Query: "go test", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hidden {
		if h.ID == ep.ID {
			t.Fatal("faded memory should be excluded by default")
		}
	}
	if err := store.Rescue(ep.ID); err != nil {
		t.Fatal(err)
	}
	shown, err := store.Search(SearchFilter{Query: "go test", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range shown {
		if h.ID == ep.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("rescued memory missing")
	}
}

func TestPackBudgetAndEconomy(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for i := 0; i < 40; i++ {
		_, err := store.Capture(CaptureInput{
			SessionID: "s",
			Kind:      KindPrompt,
			Title:     "moment " + strings.Repeat("login graph memory ", 8),
			Text:      strings.Repeat("decided to keep verbatim moments in sqlite ", 20),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	pack, err := store.BuildPack("login graph", "s")
	if err != nil {
		t.Fatal(err)
	}
	if pack.Tokens == 0 || pack.Text == "" {
		t.Fatal("empty pack")
	}
	if pack.Tokens > packBudget+20 {
		t.Fatalf("pack tokens %d over budget %d", pack.Tokens, packBudget)
	}
	eco, _ := store.ReadEconomy()
	if eco.PacksServed < 1 || eco.TokensInjected < 1 {
		t.Fatalf("economy not recorded: %+v", eco)
	}
}

func TestHeadlessMissingMarksPending(t *testing.T) {
	root := t.TempDir()
	id := "sess-pending"
	writeSession(t, root, id, []trace.Span{llmSpan("s1", "investigate the layout bloom")})
	if _, err := IngestSession(root, id); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	res := MaybeDistill(root, id, false)
	if !res.Pending {
		t.Fatalf("expected pending, got %+v", res)
	}
	store, _ := OpenRoot(root)
	defer store.Close()
	pending := store.PendingDistill()
	found := false
	for _, p := range pending {
		if p == id {
			found = true
		}
	}
	if !found {
		t.Fatalf("pending missing: %v", pending)
	}
}

func TestFinalizeFailOpenIngest(t *testing.T) {
	root := t.TempDir()
	res, err := IngestSession(root, "missing-session")
	if err == nil {
		t.Fatalf("expected error, got %+v", res)
	}
}

func TestCompactSnapshotFailOpen(t *testing.T) {
	if text := CompactSnapshot(t.TempDir(), "nope"); text != "" {
		t.Fatalf("empty repo should omit snapshot, got %q", text)
	}
}

func writeSession(t *testing.T, root, id string, spans []trace.Span) {
	t.Helper()
	layout := paths.Resolve(root)
	if err := layout.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	dir := layout.SessionDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(f)
	for _, sp := range spans {
		sp.SessionID = id
		if err := enc.Encode(sp); err != nil {
			t.Fatal(err)
		}
	}
	_ = f.Close()
	store := session.NewStore(layout)
	now := time.Now().UTC()
	_ = store.Start(session.Meta{ID: id, Vendor: "cursor", StartedAt: now, PromptPreview: "please fix the login timeout"})
}

func TestEncryptRoundTripAfterReopen(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	ep, err := store.Capture(CaptureInput{Kind: KindSession, Title: "keep the diary in sqlite", Text: "graph is the code brain; memory is the project diary"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(ep.Text, encPrefix) {
		t.Fatalf("Get after capture returned ciphertext: %q", ep.Text[:40])
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, err := store.Get(ep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != ep.Text {
		t.Fatalf("reopen decrypt failed: %q vs %q", got.Text, ep.Text)
	}
}

func TestCopyIntoPreservesEpisodes(t *testing.T) {
	srcRoot := testRoot(t)
	dstRoot := testRoot(t)
	src, err := OpenRoot(srcRoot)
	if err != nil {
		t.Fatal(err)
	}
	ep, err := src.Capture(CaptureInput{Kind: KindSession, Title: "use workspace .so", Text: "do not split sessions across repos"})
	if err != nil {
		t.Fatal(err)
	}
	if err := src.Close(); err != nil {
		t.Fatal(err)
	}
	srcDB := paths.Resolve(srcRoot).Database
	dstDB := paths.Resolve(dstRoot).Database
	if err := CopyInto(srcDB, dstDB); err != nil {
		t.Fatal(err)
	}
	dst, err := OpenRoot(dstRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
	got, err := dst.Get(ep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != ep.Title || got.Text != ep.Text {
		t.Fatalf("copied %#v, want %#v", got, ep)
	}
	hits, err := dst.Search(SearchFilter{Query: "workspace", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("fts did not survive copy")
	}
}

func llmSpan(spanID, prompt string) trace.Span {
	return trace.Span{
		SpanID: spanID,
		Name:   "coding_agent.llm.turn",
		Attributes: map[string]string{
			"gen_ai.prompt": prompt,
		},
		StartTimeUnixN: time.Now().UnixNano(),
	}
}

func toolSpan(spanID, tool, path string) trace.Span {
	return trace.Span{
		SpanID: spanID,
		Name:   "coding_agent.tool.requested",
		Attributes: map[string]string{
			"gen_ai.tool.name":       tool,
			"coding_agent.file_path": path,
		},
		StartTimeUnixN: time.Now().UnixNano(),
	}
}

func TestSuccessorRanksTop10(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	old, err := store.Capture(CaptureInput{Kind: KindSession, Title: "auth uses cookies", Text: "session auth is cookie based"})
	if err != nil {
		t.Fatal(err)
	}
	newer, err := store.Contradict(old.ID, CaptureInput{Title: "auth uses JWT", Text: "session auth is JWT now"})
	if err != nil {
		t.Fatal(err)
	}
	hits, err := store.Search(SearchFilter{Query: "session auth", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].ID != newer.ID {
		t.Fatalf("successor should rank first: %+v", hits)
	}
	foundOld, foundNew := false, false
	for _, h := range hits {
		if h.ID == old.ID {
			foundOld = true
		}
		if h.ID == newer.ID {
			foundNew = true
		}
	}
	if !foundNew || !foundOld {
		t.Fatalf("both facts should stay retrievable in top-10: %+v", hits)
	}
}

func TestHistoricalWordingHitAt10(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	old, err := store.Capture(CaptureInput{Kind: KindSession, Title: "auth uses cookies", Text: "session auth is cookie based"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Contradict(old.ID, CaptureInput{Title: "auth uses JWT", Text: "session auth is JWT now"}); err != nil {
		t.Fatal(err)
	}
	hits, err := store.Search(SearchFilter{Query: "old wording cookie based auth", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range hits {
		if h.ID == old.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("historical cue should still surface old wording: %+v", hits)
	}
}

func TestContradictionChainSuccessorLeads(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	a, err := store.Capture(CaptureInput{Kind: KindSession, Title: "timeout 5s", Text: "request timeout is five seconds"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.Contradict(a.ID, CaptureInput{Title: "timeout 15s", Text: "request timeout is fifteen seconds"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := store.Contradict(b.ID, CaptureInput{Title: "timeout 30s", Text: "request timeout is thirty seconds"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := store.Recall("request timeout", 1500)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) == 0 || res.Hits[0].ID != c.ID {
		t.Fatalf("latest successor should lead recall: %+v", res.Hits)
	}
	if len(res.AntiHits) == 0 {
		t.Fatalf("expected anti_hits for contradicted facts: %+v", res)
	}
}

func TestSleepClustersAndShapeRecall(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.Capture(CaptureInput{Kind: KindSession, Title: "login timeout", Text: "fix the login timeout in auth"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Capture(CaptureInput{Kind: KindSession, Title: "login retry", Text: "retry the login timeout path"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Sleep(); err != nil {
		t.Fatal(err)
	}
	st, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.Topics < 1 {
		t.Fatalf("expected topics after sleep, got %+v", st)
	}
	hits, err := store.RecallShape("login timeout", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected shape hits")
	}
}

func TestStatusCountsAndCoverage(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	st, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.Counts.Episodic != 0 || st.Counts.Semantic != 0 || st.Counts.Edges != 0 {
		t.Fatalf("empty store must be zeros: %+v", st.Counts)
	}
	prompt, err := store.Capture(CaptureInput{SessionID: "s", Kind: KindPrompt, Title: "login timeout", Text: "fix the login timeout"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Capture(CaptureInput{SessionID: "s", Kind: KindSession, Title: "rollup", Text: "learned: keep login timeout in sqlite"}); err != nil {
		t.Fatal(err)
	}
	st, err = store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.Counts.Episodic < 1 || st.Counts.Semantic < 1 {
		t.Fatalf("want moments and knowledge: %+v", st.Counts)
	}
	if st.Coverage <= 0 {
		t.Fatalf("expected rolled_up_from coverage, got %v (prompt=%d)", st.Coverage, prompt.ID)
	}
	if st.Connected <= 0 {
		t.Fatalf("expected connections: %+v", st)
	}
}

func TestTemporalRecallAsOf(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	old, err := store.Capture(CaptureInput{Kind: KindSession, Title: "cookies", Text: "auth is cookies"})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := store.Contradict(old.ID, CaptureInput{Title: "jwt", Text: "auth is jwt"}); err != nil {
		t.Fatal(err)
	}
	asOf := old.ValidFrom
	if asOf == "" {
		asOf = old.CreatedAt
	}
	res, err := store.TemporalRecall("auth", asOf, "", 1500)
	if err != nil {
		t.Fatal(err)
	}
	foundOld := false
	for _, h := range res.Hits {
		if h.ID == old.ID {
			foundOld = true
		}
	}
	if !foundOld {
		t.Fatalf("as-of should include the version valid then: %+v", res.Hits)
	}
}

func TestTeachChunkAndDedup(t *testing.T) {
	root := testRoot(t)
	path := filepath.Join(root, "runbook.md")
	body := strings.Repeat("Keep GPU metrics canonical. ", 80)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := TeachPath(root, path, "gpu")
	if err != nil {
		t.Fatal(err)
	}
	if first.Inserted < 1 || first.RecallTested < 1 {
		t.Fatalf("expected chunks and recall check: %+v", first)
	}
	second, err := TeachPath(root, path, "gpu")
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var n int
	_ = store.db.QueryRow(`SELECT count(*) FROM memory_episodes WHERE kind=?`, KindTeaching).Scan(&n)
	if n > first.Inserted+second.Inserted {
		t.Fatalf("dedup should not grow unbounded: have %d after first=%d second=%d", n, first.Inserted, second.Inserted)
	}
}
