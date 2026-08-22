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

func TestHeadlessMissingWritesLocalRollup(t *testing.T) {
	root := t.TempDir()
	id := "sess-pending"
	writeSession(t, root, id, []trace.Span{llmSpan("s1", "investigate the layout bloom")})
	if _, err := IngestSession(root, id); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	res := MaybeDistill(root, id, false)
	if res.Pending || res.Provider != "local" {
		t.Fatalf("expected local rollup, got %+v", res)
	}
	store, _ := OpenRoot(root)
	defer store.Close()
	hits, err := store.Search(SearchFilter{Kind: KindSession, SessionID: id, Limit: 5})
	if err != nil || len(hits) == 0 {
		t.Fatalf("expected KindSession rollup: %+v %v", hits, err)
	}
	if strings.Contains(hits[0].Text, "learned:") {
		t.Fatalf("invented learned: %q", hits[0].Text)
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

func TestRescueAt10SemanticTarget(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	target, err := store.Capture(CaptureInput{
		Kind:  KindSession,
		Title: "JWT expiry is 15 minutes",
		Text:  "session tokens expire after fifteen minutes",
		Topic: ObservationDecision,
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 24; i++ {
		if _, err := store.Capture(CaptureInput{
			Kind:  KindSession,
			Title: "graph layout camera " + strings.Repeat("x", i+1),
			Text:  "rewrite the graph layout camera bloom and sqlite wal checkpoint",
		}); err != nil {
			t.Fatal(err)
		}
	}
	hits, err := store.Search(SearchFilter{Query: "jwt token expiry minutes", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range hits {
		if h.ID == target.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Rescue@10 missed target #%d in %+v", target.ID, titlesOf(hits))
	}
}

func TestHistoricalVerbatimRequired(t *testing.T) {
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
	hits, err := store.Search(SearchFilter{Query: "previously said cookie based auth", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range hits {
		if h.ID == old.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("historical verbatim must rank old wording in top-10: %+v", titlesOf(hits))
	}
}

func TestCompactSearchOmitsBodies(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	body := "SECRET_BODY_TEXT_NOT_FOR_INDEX"
	ep, err := store.Capture(CaptureInput{Kind: KindSession, Title: "JWT expiry is 15m", Text: body, Topic: ObservationDecision})
	if err != nil {
		t.Fatal(err)
	}
	hits, err := store.Search(SearchFilter{Query: "JWT expiry", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected hit")
	}
	idx := IndexFromHit(hits[0])
	raw, _ := json.Marshal(idx)
	if strings.Contains(string(raw), body) {
		t.Fatalf("index JSON leaked body: %s", raw)
	}
	line := FormatHit(hits[0].Episode)
	if strings.Contains(line, body) {
		t.Fatalf("compact line leaked body: %s", line)
	}
	got, err := store.Get(ep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Text, body) {
		t.Fatal("get must return the body")
	}
}

func TestGetManyAndTimelineAround(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var ids []int64
	for i := 0; i < 5; i++ {
		ep, err := store.Capture(CaptureInput{Kind: KindSession, Title: "step " + itoa(i), Text: "timeline neighbor " + itoa(i)})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, ep.ID)
		time.Sleep(2 * time.Millisecond)
	}
	many, err := store.GetMany([]int64{ids[1], ids[3]})
	if err != nil || len(many) != 2 {
		t.Fatalf("GetMany: %+v %v", many, err)
	}
	around, err := store.TimelineAround(ids[2], 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(around) != 3 || around[1].ID != ids[2] {
		t.Fatalf("around: %+v", around)
	}
}

func TestTypeAndFileFilters(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	win := `C:\Users\me\auth\login.go`
	posix := `/tmp/auth/login.go`
	if _, err := store.Capture(CaptureInput{
		Kind:  KindObservation,
		Title: "login timeout",
		Text:  "fixed the login timeout",
		Topic: ObservationBugfix,
		Files: []string{win, posix},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Capture(CaptureInput{
		Kind:  KindObservation,
		Title: "jwt decision",
		Text:  "jwt expiry is 15m",
		Topic: ObservationDecision,
	}); err != nil {
		t.Fatal(err)
	}
	bugs, err := store.Search(SearchFilter{Type: ObservationBugfix, Limit: 10})
	if err != nil || len(bugs) != 1 || bugs[0].Topic != ObservationBugfix {
		t.Fatalf("type filter: %+v %v", bugs, err)
	}
	for _, q := range []string{win, posix, `C:/Users/me/auth/login.go`} {
		hits, err := store.Search(SearchFilter{File: q, Limit: 5})
		if err != nil || len(hits) == 0 {
			t.Fatalf("file filter %q: %+v %v", q, hits, err)
		}
	}
}

func TestContentHashIngestIdempotent(t *testing.T) {
	root := t.TempDir()
	id := "hash-sess"
	writeSession(t, root, id, []trace.Span{
		llmSpan("s1", "please fix the login timeout"),
		llmSpan("s2", "please fix the login timeout"),
	})
	first, err := IngestSession(root, id)
	if err != nil {
		t.Fatal(err)
	}
	second, err := IngestSession(root, id)
	if err != nil {
		t.Fatal(err)
	}
	if second.Inserted != 0 {
		t.Fatalf("content-hash ingest must be idempotent, inserted=%d first=%+v", second.Inserted, first)
	}
}

func TestObserverHeuristicDoesNotRewritePrompts(t *testing.T) {
	root := t.TempDir()
	id := "obs-sess"
	prompt := "please fix the login timeout"
	writeSession(t, root, id, []trace.Span{llmSpan("s1", prompt)})
	if _, err := IngestSession(root, id); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	res, err := ObserveSession(root, id)
	if err != nil {
		t.Fatal(err)
	}
	if res.Inserted < 1 {
		t.Fatalf("expected heuristic observation, got %+v", res)
	}
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	prompts, err := store.Search(SearchFilter{Kind: KindPrompt, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	foundPrompt := false
	for _, h := range prompts {
		if h.Kind == KindPrompt && strings.Contains(h.Text, prompt) {
			foundPrompt = true
			if isCompressible(h.Episode) {
				t.Fatal("prompt marked compressible")
			}
		}
	}
	if !foundPrompt {
		t.Fatal("verbatim prompt missing after observer")
	}
	obs, err := store.Search(SearchFilter{Type: ObservationBugfix, Limit: 10})
	if err != nil || len(obs) == 0 {
		t.Fatalf("expected bugfix observation: %+v %v", obs, err)
	}
}

func TestEmbedderRefuseMixedGenerations(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.setMeta(metaEmbedder, "other-embedder-id"); err != nil {
		t.Fatal(err)
	}
	db := paths.Resolve(root).Database
	store.Close()
	_, err = Open(db)
	if err == nil || !strings.Contains(err.Error(), "refuse mixed embedder") {
		t.Fatalf("expected refuse mixed embedder, got %v", err)
	}
}

func TestSessionStartIndexEmptyAndCap(t *testing.T) {
	root := testRoot(t)
	if text := SessionStartIndex(root); text != "" {
		t.Fatalf("empty store must be silent, got %q", text)
	}
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	body := "UNIQUE_INDEX_BODY_xyzzy"
	if _, err := store.Capture(CaptureInput{Kind: KindSession, Title: "JWT expiry is 15m", Text: body, Topic: ObservationDecision}); err != nil {
		t.Fatal(err)
	}
	store.Close()
	text := SessionStartIndex(root)
	if !strings.HasPrefix(text, "Superopen: codebase questions") {
		t.Fatalf("graph-first prefix missing: %q", text)
	}
	if strings.Contains(text, body) {
		t.Fatalf("index leaked body: %q", text)
	}
	if EstimateTokens(text) > 350 {
		t.Fatalf("index over budget: %d %q", EstimateTokens(text), text)
	}
}

func TestProjectSpanReadsInputMessages(t *testing.T) {
	input, err := json.Marshal([]map[string]any{
		{"role": "user", "parts": []map[string]any{{"type": "text", "content": "splitting dashboard panels into a dedicated layout"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := json.Marshal([]map[string]any{
		{"role": "assistant", "parts": []map[string]any{{"type": "text", "content": "I will extract the layout."}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	root := testRoot(t)
	id := "fd5cfe3c-fixture"
	writeSession(t, root, id, []trace.Span{{
		SpanID: "s-prompt",
		Name:   "coding_agent.llm.turn",
		Attributes: map[string]string{
			"gen_ai.input.messages":  string(input),
			"gen_ai.output.messages": string(output),
		},
		StartTimeUnixN: time.Now().UnixNano(),
	}})
	if _, err := IngestSession(root, id); err != nil {
		t.Fatal(err)
	}
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hits, err := store.Search(SearchFilter{Query: "dashboard layout", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range hits {
		if h.Kind == KindPrompt && strings.Contains(h.Text, "splitting dashboard panels") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected KindPrompt from gen_ai.input.messages, hits=%v", titlesOf(hits))
	}
}

func TestIngestToolObservationSearchByFile(t *testing.T) {
	root := testRoot(t)
	id := "sess-file-obs"
	writeSession(t, root, id, []trace.Span{
		llmSpan("s1", "please inspect the app entrypoint"),
		{
			SpanID: "t-read",
			Name:   "coding_agent.tool.call",
			Attributes: map[string]string{
				"gen_ai.tool.name":           "Read",
				"gen_ai.tool.call.arguments": `{"file_path":"src/app.ts"}`,
			},
			StartTimeUnixN: time.Now().UnixNano(),
		},
	})
	if _, err := IngestSession(root, id); err != nil {
		t.Fatal(err)
	}
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hits, err := store.Search(SearchFilter{File: "src/app.ts", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected file-linked episode for src/app.ts")
	}
	prompts, err := store.Search(SearchFilter{Kind: KindPrompt, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(prompts) == 0 {
		t.Fatal("expected verbatim prompt")
	}
	working, err := store.Search(SearchFilter{Kind: KindWorking, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(working) == 0 {
		t.Fatal("expected working episode")
	}
}

func TestLocalRollupOmitsInventedLearned(t *testing.T) {
	root := testRoot(t)
	id := "sess-note"
	writeSession(t, root, id, []trace.Span{llmSpan("s1", "please jot a note about the login timeout")})
	if _, err := IngestSession(root, id); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	got := MaybeDistill(root, id, false)
	if got.Provider != "local" && got.Skipped != "already_rolled_up" {
		t.Fatalf("expected local rollup, got %+v", got)
	}
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hits, err := store.Search(SearchFilter{Kind: KindSession, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected session rollup")
	}
	if strings.Contains(hits[0].Text, "learned:") {
		t.Fatalf("invented learned: %q", hits[0].Text)
	}
	if !strings.Contains(hits[0].Text, "request:") {
		t.Fatalf("missing request: %q", hits[0].Text)
	}
}

func TestMaybeDistillStaysLocalWhenHeadlessBinaryOnPath(t *testing.T) {
	root := testRoot(t)
	id := "sess-note-headless"
	writeSession(t, root, id, []trace.Span{llmSpan("s1", "please jot a note about the login timeout")})
	if _, err := IngestSession(root, id); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	script := filepath.Join(bin, "claude")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho '{\"learned\":\"invented design fact\"}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("HOME", t.TempDir())
	got := MaybeDistill(root, id, false)
	if got.Provider != "local" && got.Skipped != "already_rolled_up" {
		t.Fatalf("auto distill must stay local, got %+v", got)
	}
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hits, err := store.Search(SearchFilter{Kind: KindSession, SessionID: id, Limit: 5})
	if err != nil || len(hits) == 0 {
		t.Fatalf("expected local rollup: %+v %v", hits, err)
	}
	if strings.Contains(hits[0].Text, "invented") || strings.Contains(hits[0].Text, "learned:") {
		t.Fatalf("headless invented learned: %q", hits[0].Text)
	}
}

func TestIngestUnwrapsSystemReminderAndSkipsPack(t *testing.T) {
	root := testRoot(t)
	id := "sess-wrap"
	wrapped := "<system-reminder>stay on the graph</system-reminder>\n\nplease inspect the app entrypoint"
	writeSession(t, root, id, []trace.Span{
		llmSpan("s1", wrapped),
		llmSpan("s2", "Fetch: memory_get / so memory get. Hints, not authority."),
		{
			SpanID: "submit",
			Name:   "coding_agent.user_prompt.submit",
			Attributes: map[string]string{
				"gen_ai.prompt": "please inspect the app entrypoint from submit",
			},
			StartTimeUnixN: time.Now().UnixNano(),
		},
	})
	if _, err := IngestSession(root, id); err != nil {
		t.Fatal(err)
	}
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	prompts, err := store.Search(SearchFilter{Kind: KindPrompt, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	foundUser := false
	foundSubmit := false
	for _, h := range prompts {
		if strings.Contains(h.Text, "system-reminder") {
			t.Fatalf("wrapper survived: %q", h.Text)
		}
		if strings.Contains(h.Text, "Fetch: memory_get") {
			t.Fatalf("pack ingested as prompt: %q", h.Text)
		}
		if strings.Contains(h.Text, "please inspect the app entrypoint") && !strings.Contains(h.Text, "from submit") {
			foundUser = true
		}
		if strings.Contains(h.Text, "from submit") {
			foundSubmit = true
		}
	}
	if !foundUser {
		t.Fatalf("unwrapped user prompt missing: %v", titlesOf(prompts))
	}
	if !foundSubmit {
		t.Fatalf("codex submit event missing: %v", titlesOf(prompts))
	}
}

func TestIngestShellCommandIsNotFileEpisode(t *testing.T) {
	root := testRoot(t)
	id := "sess-shell-mem"
	writeSession(t, root, id, []trace.Span{
		llmSpan("s1", "please inspect the app entrypoint"),
		{
			SpanID: "t-shell",
			Name:   "coding_agent.tool.call",
			Attributes: map[string]string{
				"gen_ai.tool.name":           "shell",
				"coding_agent.file_path":     `so graph query "who wraps app"`,
				"gen_ai.tool.call.arguments": `so graph query "who wraps app"`,
			},
			StartTimeUnixN: time.Now().UnixNano(),
		},
		{
			SpanID: "t-make",
			Name:   "coding_agent.tool.call",
			Attributes: map[string]string{
				"gen_ai.tool.name":           "Read",
				"gen_ai.tool.call.arguments": `{"file_path":"Makefile"}`,
			},
			StartTimeUnixN: time.Now().UnixNano(),
		},
	})
	if _, err := IngestSession(root, id); err != nil {
		t.Fatal(err)
	}
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hits, err := store.Search(SearchFilter{File: "src/app.ts", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if strings.Contains(h.Text, "so graph") {
			t.Fatalf("shell command became file episode: %q", h.Text)
		}
	}
	makeHits, err := store.Search(SearchFilter{File: "Makefile", Limit: 10})
	if err != nil || len(makeHits) == 0 {
		t.Fatalf("expected Makefile --file hit: %+v %v", makeHits, err)
	}
}

func TestIngestBackfillSkipsCurrentAndCapsAtEight(t *testing.T) {
	root := testRoot(t)
	for i := 0; i < 3; i++ {
		id := "old-" + string(rune('a'+i))
		writeSession(t, root, id, []trace.Span{llmSpan("s1", "please inspect the app entrypoint "+id)})
	}
	writeSession(t, root, "current", []trace.Span{llmSpan("s1", "please inspect the app entrypoint current")})
	got := IngestBackfill(root, "current", 8)
	if len(got) != 3 {
		t.Fatalf("backfill count=%d want 3: %+v", len(got), got)
	}
	for _, res := range got {
		if res.SessionID == "current" {
			t.Fatal("backfill ingested current session")
		}
	}
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if store.HasKindPrompt("current") {
		t.Fatal("current session should not be ingested by backfill")
	}
	if !store.HasKindPrompt("old-a") {
		t.Fatal("expected old-a prompt")
	}
}

func TestFormatIndexLineTokenSuffix(t *testing.T) {
	line := FormatIndexLine(Episode{ID: 12, Kind: KindPrompt, Title: "fix login", Tokens: 192})
	if strings.Contains(line, "~192") && !strings.Contains(line, "~192t") {
		t.Fatalf("bare ~192 looks like a source line: %q", line)
	}
	if !strings.Contains(line, "~192t") {
		t.Fatalf("want ~192t, got %q", line)
	}
}

func TestSearchFallsBackToSessionTitles(t *testing.T) {
	root := testRoot(t)
	layout := paths.Resolve(root)
	sess := session.NewStore(layout)
	if err := sess.Start(session.Meta{
		ID: "sess-title", Vendor: "cursor", Title: "dashboard layout from last chat",
		PromptPreview: "splitting dashboard panels", StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hits, err := store.Search(SearchFilter{Query: "dashboard layout", Limit: 8})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected session.json title fallback")
	}
	text := SessionStartIndex(root)
	if !strings.Contains(text, "dashboard layout") {
		t.Fatalf("SessionStart index missing session title: %q", text)
	}
}

func TestExtractJSONArrayTypedObservations(t *testing.T) {
	body := extractJSONArray("Here you go:\n[{\"type\":\"decision\",\"title\":\"keep layout in one file\",\"facts\":[\"split panels\"],\"narrative\":\"\",\"concepts\":[\"layout\"]}]\n")
	var parsed []struct {
		Type  string   `json:"type"`
		Title string   `json:"title"`
		Facts []string `json:"facts"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 1 || parsed[0].Type != "decision" || parsed[0].Title == "" {
		t.Fatalf("parsed=%#v", parsed)
	}
}

func TestLiveDistillInstructionOnPending(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkPending("sess-pending"); err != nil {
		t.Fatal(err)
	}
	store.Close()
	text := SessionStartIndex(root)
	if !strings.Contains(text, "sess-pending") || !strings.Contains(text, "memory_capture") {
		t.Fatalf("expected LiveDistill on pending empty index, got %q", text)
	}
}

func TestSchemaVersionIsOne(t *testing.T) {
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
	if st.SchemaVersion != "1" {
		t.Fatalf("schema_version=%s want 1", st.SchemaVersion)
	}
}

func TestIngestSkipsFencedAndGraphDumps(t *testing.T) {
	cases := []struct {
		name   string
		prompt string
	}{
		{
			name: "fenced",
			prompt: "```\nChat dump\nuser: what wraps the page\nassistant: I read src/app.ts\n```",
		},
		{
			name: "zwsp-fenced",
			prompt: "\u200b```\ntranscript paste\nmore lines of copied chat\n```",
		},
		{
			name: "graph-nodes",
			prompt: "NODE File [src=src/app.ts loc=L1-80 community=src]\nNODE Function [qn=src.app.main src=src/app.ts loc=L10 community=src]\nNODE Variable [qn=src.app.FOO src=src/app.ts loc=L3 community=src]\nEDGE CONTAINS File Function\nEDGE CONTAINS File Variable",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !dumpCapture(tc.prompt) {
				t.Fatal("expected dumpCapture")
			}
			root := t.TempDir()
			id := "dump-" + tc.name
			writeSession(t, root, id, []trace.Span{llmSpan("s1", tc.prompt)})
			if _, err := IngestSession(root, id); err != nil {
				t.Fatal(err)
			}
			store, err := OpenRoot(root)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			hits, err := store.Search(SearchFilter{Kind: KindPrompt, Limit: 20, IncludeFaded: true})
			if err != nil {
				t.Fatal(err)
			}
			for _, h := range hits {
				t.Fatalf("dump stored as KindPrompt: %q", h.Text)
			}
		})
	}
	if dumpCapture("please fix the login timeout in src/app.ts") {
		t.Fatal("normal prompt must not look like a dump")
	}
}

func TestObserverSkipsQuestionPrompts(t *testing.T) {
	root := t.TempDir()
	id := "q-sess"
	prompt := "What did we decide last time about the login timeout?"
	writeSession(t, root, id, []trace.Span{llmSpan("s1", prompt)})
	if _, err := IngestSession(root, id); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	res, err := ObserveSession(root, id)
	if err != nil {
		t.Fatal(err)
	}
	if res.Inserted != 0 {
		t.Fatalf("question must not spawn a typed observation, got %+v", res)
	}
	if heuristicTopic(prompt) == ObservationDecision {
		t.Fatal("question typed as decision")
	}
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	prompts, err := store.Search(SearchFilter{Kind: KindPrompt, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range prompts {
		if h.Kind == KindPrompt && strings.Contains(h.Text, "login timeout") {
			found = true
		}
	}
	if !found {
		t.Fatal("KindPrompt missing for question")
	}
	decisions, err := store.Search(SearchFilter{Type: ObservationDecision, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 0 {
		t.Fatalf("unexpected decision rows: %+v", titlesOf(decisions))
	}
}

func TestSearchPrefersUserNoteOverLearnedFiction(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.Capture(CaptureInput{
		Kind:   KindSession,
		Source: SourceHeadless,
		Title:  "session leftover",
		Text:   "learned: login timeout lives in redis\nnext: rewrite auth",
	}); err != nil {
		t.Fatal(err)
	}
	note, err := store.Capture(CaptureInput{
		Kind:  KindWorking,
		Title: "login timeout",
		Text:  "keep the login timeout at 30s in sqlite",
	})
	if err != nil {
		t.Fatal(err)
	}
	hits, err := store.Search(SearchFilter{Query: "login timeout", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected hits")
	}
	if hits[0].ID != note.ID {
		t.Fatalf("user note should rank above invented learned:, got %q %q", hits[0].Title, hits[0].Text)
	}
	if strings.HasPrefix(strings.TrimSpace(strings.ToLower(hits[0].Text)), "learned:") {
		t.Fatalf("top hit is learned: fiction: %q", hits[0].Text)
	}
}

func titlesOf(hits []Hit) []string {
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.Title)
	}
	return out
}
