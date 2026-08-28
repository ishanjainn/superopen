package memory

import (
	"encoding/json"
	"fmt"
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
	if len(layout.Nodes) != 0 {
		t.Fatalf("diary working memory should stay off the galaxy: %+v", layout.Nodes)
	}
}

func TestLayoutPlotsKnowledgeAndSkillsNotPrompts(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, err := store.Capture(CaptureInput{
		Kind: KindPrompt, Title: "how does auth work", Text: "user asked about cookies",
	}); err != nil {
		t.Fatal(err)
	}
	knowledge, err := store.Capture(CaptureInput{
		Kind: KindSession, Title: "auth uses cookies", Text: "session auth is cookie based", Horizon: HorizonMedium,
	})
	if err != nil {
		t.Fatal(err)
	}
	skill, err := store.Capture(CaptureInput{
		Kind: KindTeaching, Title: "always pin sqlite facts", Text: "keep sqlite facts in long memory",
	})
	if err != nil {
		t.Fatal(err)
	}

	layout, err := store.Layout(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(layout.Nodes) != 2 {
		t.Fatalf("expected knowledge+skill stars, got %+v", layout.Nodes)
	}
	var knowledgeSize, skillSize float64
	var knowledgeColor, skillColor string
	for _, n := range layout.Nodes {
		switch n.ID {
		case knowledge.ID:
			knowledgeSize = n.Size
			knowledgeColor = n.Color
		case skill.ID:
			skillSize = n.Size
			skillColor = n.Color
		default:
			t.Fatalf("unexpected galaxy node %d (%s)", n.ID, n.Name)
		}
	}
	if knowledgeColor == "" {
		t.Fatalf("knowledge missing from layout: %+v", layout.Nodes)
	}
	if skillColor == "" {
		t.Fatalf("skill missing from layout: %+v", layout.Nodes)
	}
	if knowledgeColor == "#eab308" {
		t.Fatalf("medium knowledge should not use long-horizon gold: color=%s", knowledgeColor)
	}
	if skillColor != "#eab308" {
		t.Fatalf("long skill should use long-horizon gold, got %s", skillColor)
	}
	if skillSize <= knowledgeSize {
		t.Fatalf("long should be larger than medium: skill=%v knowledge=%v", skillSize, knowledgeSize)
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

func TestMaybeDistillNoopsUnderTest(t *testing.T) {
	root := t.TempDir()
	id := "sess-pending"
	writeSession(t, root, id, []trace.Span{llmSpan("s1", "investigate the layout bloom")})
	if _, err := IngestSession(root, id); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	res := MaybeDistill(root, id, false)
	if res.Skipped != "test" {
		t.Fatalf("expected test skip, got %+v", res)
	}
	got := Distill(root, id)
	if !got.Pending || got.Skipped != "no-auth" {
		t.Fatalf("expected pending no-auth, got %+v", got)
	}
	store, _ := OpenRoot(root)
	defer store.Close()
	hits, err := store.Search(SearchFilter{Kind: KindSession, SessionID: id, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("distill must not invent knowledge: %+v", hits)
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
	line := FormatIndexLine(hits[0].Episode)
	if strings.Contains(line, body) {
		t.Fatalf("index line leaked body: %s", line)
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
	if res.Inserted != 0 {
		t.Fatalf("observe is disabled, got %+v", res)
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
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 0 {
		t.Fatalf("observe must not invent typed rows: %+v", obs)
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
	if !strings.Contains(text, "memories in this workspace") {
		t.Fatalf("must say memories exist: %q", text)
	}
	if !strings.Contains(text, "memory recall") {
		t.Fatalf("must give recall command: %q", text)
	}
	if strings.Contains(text, "JWT expiry") || strings.Contains(text, "#") && strings.Contains(text, "15m") {
		t.Fatalf("SessionStart must not dump uncued ids or titles: %q", text)
	}
	if strings.Contains(text, body) {
		t.Fatalf("index leaked body: %q", text)
	}
	if EstimateTokens(text) > 350 {
		t.Fatalf("index over budget: %d %q", EstimateTokens(text), text)
	}
}

func TestPromptRecallPackInjectsMatchingBodies(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	body := "UNIQUE_PROMPT_PACK_BODY decided retry is three"
	if _, err := store.Capture(CaptureInput{Kind: KindSession, Title: "auth retry decision", Text: body, Topic: ObservationDecision}); err != nil {
		t.Fatal(err)
	}
	store.Close()
	text := PromptRecallPack(root, "what did we decide last time about retry")
	if !strings.Contains(text, body) {
		t.Fatalf("prompt pack must include matching body, got %q", text)
	}
	if !strings.Contains(text, "past sessions") {
		t.Fatalf("prompt pack must frame ownership, got %q", text)
	}
	if !strings.Contains(text, "--full") {
		t.Fatalf("prompt pack must point at get --full, got %q", text)
	}
	empty := testRoot(t)
	if PromptRecallPack(empty, "what did we decide last time") != "" {
		t.Fatal("empty store must not invent a pack")
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

func TestDistillDoesNotInventLocalRollup(t *testing.T) {
	root := testRoot(t)
	id := "sess-note"
	writeSession(t, root, id, []trace.Span{llmSpan("s1", "please jot a note about the login timeout")})
	if _, err := IngestSession(root, id); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	got := MaybeDistill(root, id, false)
	if got.Skipped != "test" {
		t.Fatalf("MaybeDistill under test must skip, got %+v", got)
	}
	res := Distill(root, id)
	if res.Provider == "local" {
		t.Fatalf("must not write a local rollup: %+v", res)
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
	if len(hits) != 0 {
		t.Fatalf("no-auth distill must not invent knowledge: %+v", hits)
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
	got := Distill(root, id)
	if got.Provider == "local" {
		t.Fatalf("must not fall back to local rollup, got %+v", got)
	}
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hits, err := store.Search(SearchFilter{Kind: KindSession, SessionID: id, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("unauthenticated distill must not invent knowledge: %+v", hits)
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
	if strings.Contains(line, "~192t") || strings.Contains(line, "~192") {
		t.Fatalf("index line must not carry token suffix: %q", line)
	}
	if !strings.Contains(line, "#12") || !strings.Contains(line, "fix login") {
		t.Fatalf("want #id and title, got %q", line)
	}
}

func TestSearchDoesNotInventSessionTitles(t *testing.T) {
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
	if len(hits) != 0 {
		t.Fatalf("zero-hit search must not invent session.json titles, got %#v", hits)
	}
	text := SessionStartIndex(root)
	if strings.Contains(text, "dashboard layout") {
		t.Fatalf("SessionStart must not inject session titles as knowledge: %q", text)
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
	if !strings.Contains(text, "sess-pending") || !strings.Contains(text, "so memory distill") {
		t.Fatalf("expected pending distill line, got %q", text)
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
	if st.SchemaVersion != "2" {
		t.Fatalf("schema_version=%s want 2", st.SchemaVersion)
	}
}

func TestIngestSkipsFencedAndGraphDumps(t *testing.T) {
	cases := []struct {
		name   string
		prompt string
	}{
		{
			name:   "fenced",
			prompt: "```\nChat dump\nuser: what wraps the page\nassistant: I read src/app.ts\n```",
		},
		{
			name:   "zwsp-fenced",
			prompt: "\u200b```\ntranscript paste\nmore lines of copied chat\n```",
		},
		{
			name:   "graph-nodes",
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

func TestApplyDistillJSONWritesHorizon(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	items := parseDistillItems(`[
	  {"kind":"knowledge","horizon":"medium","title":"login timeout is 30s","text":"keep login timeout in sqlite"},
	  {"kind":"skill","horizon":"long","title":"run graph query first","text":"code questions use so graph query"}
	]`)
	n, _, err := store.applyDistillItems("s1", items)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("written=%d want 2", n)
	}
	hits, err := store.Search(SearchFilter{Horizon: HorizonMedium, Limit: 10})
	if err != nil || len(hits) == 0 {
		t.Fatalf("medium knowledge missing: %+v %v", hits, err)
	}
	skills, err := store.Search(SearchFilter{Horizon: HorizonLong, Limit: 10})
	if err != nil || len(skills) == 0 {
		t.Fatalf("long skill missing: %+v %v", skills, err)
	}
	text := SessionStartIndex(root)
	if !strings.Contains(text, "memories in this workspace") || !strings.Contains(text, "memory recall") {
		t.Fatalf("index must point at recall, got %q", text)
	}
	if strings.Contains(text, "keep login timeout in sqlite") {
		t.Fatalf("index leaked body: %q", text)
	}
	if strings.Contains(text, "login timeout is 30s") {
		t.Fatalf("SessionStart must not dump uncued titles: %q", text)
	}
}

func TestHorizonExpiryHidesShort(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ep, err := store.Capture(CaptureInput{
		Kind:             KindSession,
		Title:            "wip auth rewrite",
		Text:             "still moving cookies to sqlite",
		Horizon:          HorizonShort,
		KeepUntilSession: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = store.setMeta(metaSessionSeq, "2")
	if _, err := store.ExpireHorizons(); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Faded {
		t.Fatal("short row should fade after keep_until_session")
	}
	text := SessionStartIndex(root)
	if strings.Contains(text, "wip auth rewrite") {
		t.Fatalf("expired short still in index: %q", text)
	}
}

func TestSessionStartIndexOmitsPromptBodies(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Capture(CaptureInput{
		Kind: KindPrompt, Title: "secret prompt body xyz", Text: "secret prompt body xyz please dump the diary",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Capture(CaptureInput{
		Kind: KindSession, Title: "auth cookies stay in sqlite", Text: "learned: cookies live in sqlite", Horizon: HorizonMedium,
	}); err != nil {
		t.Fatal(err)
	}
	store.Close()
	text := SessionStartIndex(root)
	if strings.Contains(text, "secret prompt body xyz") {
		t.Fatalf("prompt leaked into index: %q", text)
	}
	if strings.Contains(text, "auth cookies stay in sqlite") {
		t.Fatalf("SessionStart must not dump uncued titles: %q", text)
	}
	if !strings.Contains(text, "memories in this workspace") || !strings.Contains(text, "memory recall") {
		t.Fatalf("index must point at recall, got %q", text)
	}
}

func TestPromoteLongSurvivesExpiry(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ep, err := store.Capture(CaptureInput{
		Kind:             KindSession,
		Title:            "sqlite cookies for auth",
		Text:             "auth cookies live in sqlite",
		Horizon:          HorizonShort,
		KeepUntilSession: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PromoteHorizon(ep.ID, HorizonLong); err != nil {
		t.Fatal(err)
	}
	_ = store.setMeta(metaSessionSeq, "9")
	if _, err := store.ExpireHorizons(); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Faded {
		t.Fatal("promoted long row must not expire")
	}
	if got.Horizon != HorizonLong {
		t.Fatalf("horizon=%s want long", got.Horizon)
	}
	text := SessionStartIndex(root)
	if !strings.Contains(text, "memories in this workspace") {
		t.Fatalf("long knowledge store must still announce memories: %q", text)
	}
	if strings.Contains(text, "sqlite cookies for auth") {
		t.Fatalf("SessionStart must not dump uncued titles: %q", text)
	}
}

func TestForgetHidesFromIndex(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ep, err := store.Capture(CaptureInput{
		Kind:    KindSession,
		Title:   "timeout is 30s",
		Text:    "login timeout is thirty seconds",
		Horizon: HorizonMedium,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ForgetEpisode(ep.ID); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Faded {
		t.Fatal("forgotten row should be faded")
	}
	text := SessionStartIndex(root)
	if strings.Contains(text, "timeout is 30s") {
		t.Fatalf("forgotten knowledge still in index: %q", text)
	}
}

func TestApplyDistillJSONForgetAndPromote(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	old, err := store.Capture(CaptureInput{
		Kind: KindSession, Title: "timeout is 5s", Text: "login timeout is five seconds", Horizon: HorizonMedium,
	})
	if err != nil {
		t.Fatal(err)
	}
	keep, err := store.Capture(CaptureInput{
		Kind: KindSession, Title: "use sqlite sessions", Text: "sessions live in sqlite", Horizon: HorizonShort, KeepUntilSession: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	items := parseDistillItems(fmt.Sprintf(`[
	  {"kind":"forget","id":%d,"reason":"superseded"},
	  {"kind":"promote","id":%d,"horizon":"long"}
	]`, old.ID, keep.ID))
	if _, _, err := store.applyDistillItems("s1", items); err != nil {
		t.Fatal(err)
	}
	gone, err := store.Get(old.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !gone.Faded {
		t.Fatal("forget should fade the old id")
	}
	live, err := store.Get(keep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if live.Horizon != HorizonLong {
		t.Fatalf("promote horizon=%s want long", live.Horizon)
	}
	_ = store.setMeta(metaSessionSeq, "9")
	if _, err := store.ExpireHorizons(); err != nil {
		t.Fatal(err)
	}
	text := SessionStartIndex(root)
	if strings.Contains(text, "timeout is 5s") {
		t.Fatalf("forgotten id still in index: %q", text)
	}
	if !strings.Contains(text, "memories in this workspace") {
		t.Fatalf("store still has live knowledge: %q", text)
	}
	if strings.Contains(text, "use sqlite sessions") {
		t.Fatalf("SessionStart must not dump uncued titles: %q", text)
	}
}

func TestApplyDistillJSONCollapsesSameTitle(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first, err := store.Capture(CaptureInput{
		Kind: KindSession, Title: "Session auth stays in SQLite", Text: "sqlite is the source of truth for sessions", Horizon: HorizonMedium,
	})
	if err != nil {
		t.Fatal(err)
	}
	items := parseDistillItems(`[
	  {"kind":"knowledge","horizon":"long","title":"Session auth stays in SQLite","text":"cookies carry the session id only; sqlite stays authoritative"}
	]`)
	n, last, err := store.applyDistillItems("s2", items)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("duplicate title should not insert, written=%d", n)
	}
	if last != first.ID {
		t.Fatalf("collapsed onto %d want %d", last, first.ID)
	}
	got, err := store.Get(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Horizon != HorizonLong {
		t.Fatalf("collapse should promote medium→long, got %s", got.Horizon)
	}
	hits, err := store.Search(SearchFilter{Kind: KindSession, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	live := 0
	for _, h := range hits {
		if h.Kind == KindSession && !h.Faded && normalizeMemoryTitle(h.Title) == normalizeMemoryTitle(first.Title) {
			live++
		}
	}
	if live != 1 {
		t.Fatalf("expected one live knowledge row with that title, got %d (%v)", live, titlesOf(hits))
	}
}

func TestApplyDistillJSONCollapsesNearDuplicateKnowledge(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first, err := store.Capture(CaptureInput{
		Kind: KindSession, Title: "keep login timeout in sqlite", Text: "the login timeout stays at thirty seconds in sqlite", Horizon: HorizonMedium,
	})
	if err != nil {
		t.Fatal(err)
	}
	items := parseDistillItems(`[
	  {"kind":"knowledge","horizon":"medium","title":"login timeout stays in sqlite","text":"the login timeout stays at thirty seconds in sqlite"}
	]`)
	n, last, err := store.applyDistillItems("s3", items)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 || last != first.ID {
		t.Fatalf("near-duplicate knowledge should collapse: written=%d last=%d want 0/%d", n, last, first.ID)
	}
	hits, err := store.Search(SearchFilter{Kind: KindSession, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	live := 0
	for _, h := range hits {
		if h.Kind == KindSession && !h.Faded {
			live++
		}
	}
	if live != 1 {
		t.Fatalf("expected a single live knowledge row, got %d (%v)", live, titlesOf(hits))
	}
}

func TestCaptureDistinctSessionTitlesDoNotCollapse(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first, err := store.Capture(CaptureInput{
		Kind: KindSession, Title: "Monday diary: login timeout",
		Text: "we discussed login timeout and sqlite sessions for the web app",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Capture(CaptureInput{
		Kind: KindSession, Title: "Tuesday diary: login timeout",
		Text: "we discussed login timeout and sqlite sessions for the web app",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == 0 || second.ID == 0 || first.ID == second.ID {
		t.Fatalf("distinct diary days must store two episodes, got %d and %d", first.ID, second.ID)
	}
}

func TestTeachingNearDuplicateStillCollapses(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first, err := store.Capture(CaptureInput{
		Kind: KindTeaching, Title: "keep timeout in sqlite",
		Text: "the login timeout stays at thirty seconds in sqlite",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Capture(CaptureInput{
		Kind: KindTeaching, Title: "keep timeout in sqlite",
		Text: "the login timeout stays at thirty seconds in sqlite",
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("teaching near-dup should collapse onto %d, got %d", first.ID, second.ID)
	}
}

func TestUpgradeHashStoreToBGE(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	ep, err := store.Capture(CaptureInput{Kind: KindSession, Title: "hash row", Text: "a stored hash embedding that must be wiped on upgrade"})
	if err != nil {
		t.Fatal(err)
	}
	var vectors int
	if err := store.db.QueryRow(`SELECT count(*) FROM memory_vectors`).Scan(&vectors); err != nil {
		t.Fatal(err)
	}
	if vectors == 0 {
		t.Fatal("expected a hash vector before upgrade")
	}
	db := paths.Resolve(root).Database
	store.Close()

	old := activeEmbedderID
	activeEmbedderID = bgeEmbedderID
	t.Cleanup(func() { activeEmbedderID = old })

	upgraded, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	got, err := upgraded.meta(metaEmbedder)
	if err != nil || got != bgeEmbedderID {
		t.Fatalf("embedder meta=%s err=%v want %s", got, err, bgeEmbedderID)
	}
	if err := upgraded.db.QueryRow(`SELECT count(*) FROM memory_vectors`).Scan(&vectors); err != nil {
		t.Fatal(err)
	}
	if vectors != 0 {
		t.Fatalf("upgrade must wipe hash vectors, still have %d", vectors)
	}
	var pending int
	if err := upgraded.db.QueryRow(`SELECT embedding_pending FROM memory_episodes WHERE id=?`, ep.ID).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 1 {
		t.Fatalf("upgrade must mark embedding_pending, got %d", pending)
	}
}

func TestFTSQueryUsesContentOR(t *testing.T) {
	got := ftsQuery("What did Caroline say about the race yesterday?")
	if !strings.Contains(got, " OR ") {
		t.Fatalf("expected OR of content terms, got %q", got)
	}
	if strings.Contains(got, `"what"`) || strings.Contains(got, `"did"`) || strings.Contains(got, `"the"`) {
		t.Fatalf("stopwords must not be required, got %q", got)
	}
	if !strings.Contains(got, "caroline") || !strings.Contains(got, "race") {
		t.Fatalf("content words missing: %q", got)
	}
	if !strings.Contains(got, " AND ") {
		t.Fatalf("proper name must be required, got %q", got)
	}
}

func TestFTSQuerySplitsPossessives(t *testing.T) {
	got := ftsQuery("What is Caroline's identity?")
	if strings.Contains(got, `"carolines"`) || strings.Contains(got, "carolines*") {
		t.Fatalf("possessive must not concatenate 's: %q", got)
	}
	if !strings.Contains(got, "caroline") || !strings.Contains(got, "identity") {
		t.Fatalf("want caroline and identity, got %q", got)
	}
}

func TestFTSQueryPrefixesVerbsAndRequiresName(t *testing.T) {
	got := ftsQuery("Where did Caroline move from 4 years ago?")
	if !strings.Contains(got, "move*") {
		t.Fatalf("verb should prefix-match moved/moving, got %q", got)
	}
	if !strings.Contains(got, "caroline") || !strings.Contains(got, " AND ") {
		t.Fatalf("Caroline must be required, got %q", got)
	}
}

func TestSearchFindsWordOnlyInSealedBody(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	body := "the diary mentions ZXBODYONLYWORD once and never in the title"
	ep, err := store.Capture(CaptureInput{Kind: KindSession, Title: "monday notes", Text: body})
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := store.db.QueryRow(`SELECT text FROM memory_episodes WHERE id=?`, ep.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored, encPrefix) {
		t.Fatalf("episodes.text must stay sealed, got %q", stored[:min(40, len(stored))])
	}
	hits, err := store.Search(SearchFilter{Query: "ZXBODYONLYWORD", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range hits {
		if h.ID == ep.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected body-only FTS hit, got %+v", titlesOf(hits))
	}
}

func TestRecallFTSMatchesQueryNotExcludedKindName(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ep, err := store.Capture(CaptureInput{
		Kind:  KindSession,
		Title: "graduation notes",
		Text:  "the diary mentions ZXRECALLWORD in the body and never the word prompt",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := store.Recall("where is ZXRECALLWORD recorded", 1500)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range res.Hits {
		if h.ID == ep.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("default recall must FTS-match the question, not excluded kind names; hits=%v", titlesOf(res.Hits))
	}
}

func TestSearchLongQuestionMatchesBodyWord(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ep, err := store.Capture(CaptureInput{
		Kind: KindSession, Title: "monday notes",
		Text: "we booked the community fundraiser ZXCHARITYRACEWORD at the park",
	})
	if err != nil {
		t.Fatal(err)
	}
	hits, err := store.Search(SearchFilter{
		Query: "What did we note about ZXCHARITYRACEWORD in the diary yesterday?",
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range hits {
		if h.ID == ep.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("content-word FTS must match a long question, got %+v", titlesOf(hits))
	}
}

func TestSearchFTSUnionOutsideRecencyWindow(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	old, err := store.Capture(CaptureInput{
		Kind: KindSession, Title: "oldest diary", Text: "needle ZXOLDSESSIONWORD lives only here",
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 800; i++ {
		created := time.Now().UTC().Add(time.Duration(i+1) * time.Second).Format(time.RFC3339Nano)
		uid := "fill-" + itoa(i)
		if _, err := store.db.Exec(`
INSERT INTO memory_episodes(uid,session_id,span_id,kind,source,title,text,files,tool_name,tokens,pinned,faded,embedding_pending,created_at,updated_at,valid_from,valid_to,faded_at,last_accessed_at,community_id,centrality,tier,horizon,keep_until_session,never_decay,tags,fading,topic,facts,narrative,concepts,content_hash)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			uid, "", "", KindSession, SourceAgent, "later "+itoa(i), "unrelated later diary", "", "", 4,
			0, 0, 1, created, created, created, "", "", "", "", 0, HorizonMedium, HorizonMedium, 0, 0, "", 0, "", "[]", "", "[]", ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.Exec(`UPDATE memory_episodes SET created_at=? WHERE id=?`, "2020-01-01T00:00:00Z", old.ID); err != nil {
		t.Fatal(err)
	}
	hits, err := store.Search(SearchFilter{Query: "ZXOLDSESSIONWORD", Limit: 10})
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
		t.Fatalf("FTS must pull the old row outside the newest-800 window, got %+v", titlesOf(hits))
	}
}

func TestRecallKeepsTwoLargeHits(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	big := strings.Repeat("login timeout stays in sqlite sessions. ", 200)
	a, err := store.Capture(CaptureInput{Kind: KindSession, Title: "first long diary", Text: big + " ALPHAUNIQUE"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.Capture(CaptureInput{Kind: KindSession, Title: "second long diary", Text: big + " BETAUNIQUE"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := store.Recall("login timeout sqlite", 1500)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[int64]bool{}
	for _, h := range res.Hits {
		ids[h.ID] = true
		if h.Tokens > 1500 {
			t.Fatalf("clipped hit still over budget: id=%d tokens=%d", h.ID, h.Tokens)
		}
	}
	if !ids[a.ID] || !ids[b.ID] {
		t.Fatalf("recall must keep both large sessions, hits=%d ids=%v want %d and %d", len(res.Hits), ids, a.ID, b.ID)
	}
}

func TestClipAroundQueryKeepsTailFact(t *testing.T) {
	head := strings.Repeat("padding words about weather and lunch. ", 80)
	tail := "Caroline moved from Sweden four years ago."
	got := clipAroundQuery(head+tail, "where did Caroline move from", 40)
	if !strings.Contains(got, "Sweden") {
		t.Fatalf("query-window clip dropped the tail fact: %q", got)
	}
	if strings.HasPrefix(got, "padding") && !strings.Contains(got, "Sweden") {
		t.Fatalf("fell back to head clip: %q", got)
	}
}

func TestClipAroundQueryAnchorsOnRareTerm(t *testing.T) {
	head := strings.Repeat("Alex said hello to the team. ", 80)
	tail := "We will use exponential backoff for the retry budget."
	got := clipAroundQuery(head+tail, "Alex retry budget", 40)
	if !strings.Contains(got, "retry budget") {
		t.Fatalf("rarest term must keep the tail fact, got %q", got)
	}
	if strings.Contains(got, "said hello") && !strings.Contains(got, "retry") {
		t.Fatalf("common name stole the window: %q", got)
	}
}

func TestClipAroundQueryTwoWindows(t *testing.T) {
	head := "Kickoff notes from Alex about staffing."
	mid := strings.Repeat("padding filler words for the diary body. ", 80)
	tail := "The retry budget is three with jitter."
	got := clipAroundQuery(head+mid+tail, "Alex retry budget", 50)
	if !strings.Contains(got, "Alex") {
		t.Fatalf("first window should keep Alex, got %q", got)
	}
	if !strings.Contains(got, "retry budget") {
		t.Fatalf("second window should keep the rare tail fact, got %q", got)
	}
}

func TestClipAroundQueryKeepsHeadDate(t *testing.T) {
	body := "DATE: 1:56 pm on 8 May, 2023\n" + strings.Repeat("padding about lunch and weather. ", 80) +
		"Melanie: Yeah, I painted that lake sunrise last year! It's special to me.\n"
	got := clipAroundQuery(body, "when did Melanie paint the lake sunrise", 40)
	if !strings.Contains(got, "8 May, 2023") && !strings.Contains(got, "DATE:") {
		t.Fatalf("clip must keep the head date line, got %q", got)
	}
	if !strings.Contains(got, "last year") && !strings.Contains(got, "sunrise") {
		t.Fatalf("clip must still keep the query span, got %q", got)
	}
}

func TestFormatIndexLineIncludesDate(t *testing.T) {
	line := FormatIndexLine(Episode{ID: 3, Kind: KindSession, Title: "lake sunrise", CreatedAt: "2023-05-08T13:56:00Z"})
	if !strings.Contains(line, "2023-05-08") {
		t.Fatalf("index line should carry the episode date, got %q", line)
	}
}

func TestTrimToBudgetFrontLoadsDeepHits(t *testing.T) {
	var hits []Hit
	body := strings.Repeat("session note about the auth retry decision. ", 80)
	for i := 1; i <= 10; i++ {
		hits = append(hits, Hit{Episode: Episode{ID: int64(i), Title: fmt.Sprintf("hit-%d", i), Text: body, Tokens: EstimateTokens(body)}})
	}
	got := trimToBudget(hits, 1500, "auth retry")
	if len(got) != 10 {
		t.Fatalf("recall must keep 10 ranked ids, got %d", len(got))
	}
	for i, h := range got {
		if i < recallDeepHits {
			if strings.TrimSpace(h.Text) == "" {
				t.Fatalf("top hit %d must have a body", h.ID)
			}
			if h.Tokens > recallDeepTok+20 {
				t.Fatalf("deep hit %d tokens=%d want around %d", h.ID, h.Tokens, recallDeepTok)
			}
		} else if strings.TrimSpace(h.Text) != "" {
			t.Fatalf("index-only hit %d should drop the body, got %q", h.ID, h.Text)
		}
	}
}

func TestHelpForSearchPointsAtFullGet(t *testing.T) {
	hits := []Hit{{Episode: Episode{ID: 7, Title: "x"}}}
	help := HelpForSearch(hits)
	joined := strings.Join(help, "\n")
	if !strings.Contains(joined, "so memory get 7 --full") {
		t.Fatalf("help must point at --full, got %q", joined)
	}
	if hint := ClippedBodyHint([]Hit{{Episode: Episode{ID: 7, Text: "…clipped…"}}}); !strings.Contains(hint, "--full") {
		t.Fatalf("clip hint must name --full, got %q", hint)
	}
}

func TestSelfEchoDropsQueryPromptKeepsKnowledge(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	q := "Where did Caroline move from 4 years ago?"
	if _, err := store.Capture(CaptureInput{Kind: KindPrompt, Title: q, Text: q}); err != nil {
		t.Fatal(err)
	}
	session, err := store.Capture(CaptureInput{
		Kind: KindSession, Title: "conv-26:session_3",
		Text: "Caroline told Melanie she moved from Sweden about four years ago.",
	})
	if err != nil {
		t.Fatal(err)
	}
	hits, err := store.Search(SearchFilter{Query: q, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if h.Kind == KindPrompt {
			t.Fatalf("self-echo prompt ranked: %+v", h)
		}
	}
	found := false
	for _, h := range hits {
		if h.ID == session.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("knowledge row missing, hits=%v", titlesOf(hits))
	}
}

func TestDiaryOnlyStoreStillRecalls(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ep, err := store.Capture(CaptureInput{
		Kind: KindPrompt, Title: "use JWT with 15m expiry",
		Text: "Decision: cookies stay in sqlite, JWT expiry is 15 minutes.",
	})
	if err != nil {
		t.Fatal(err)
	}
	hits, err := store.Search(SearchFilter{Query: "JWT expiry", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range hits {
		if h.ID == ep.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("pre-distill diary must still rank, hits=%v", titlesOf(hits))
	}
}

func TestEmptyHitHintDistinguishesSearchFromRecall(t *testing.T) {
	search := EmptyHitHint(12, 12, false)
	if !strings.Contains(search, "title") || !strings.Contains(search, "recall") {
		t.Fatalf("search hint should send agents to recall: %q", search)
	}
	if strings.Contains(search, "search is titles only") && !strings.Contains(search, "recall returns bodies") {
		t.Fatalf("search hint must not imply recall is titles-only: %q", search)
	}
	recall := EmptyRecallHint(12, 12, false)
	if strings.Contains(recall, "titles only") {
		t.Fatalf("recall hint must not say titles only: %q", recall)
	}
}

func TestPassagesRankTailFact(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	head := strings.Repeat("padding about lunch weather and traffic. ", 200)
	tail := "The studio is called SerenityYogaUniqueTail."
	ep, err := store.Capture(CaptureInput{Kind: KindSession, Title: "long diary", Text: head + tail})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := store.db.QueryRow(`SELECT count(*) FROM memory_passages WHERE episode_id=?`, ep.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("long episode must write passage vectors")
	}
	hits, err := store.Search(SearchFilter{Query: "SerenityYogaUniqueTail", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range hits {
		if h.ID == ep.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("tail fact should rank via passages, hits=%v", titlesOf(hits))
	}
}

func TestRecallPrefersKnowledgeOverLivePrompt(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	q := "Where did Caroline move from?"
	if _, err := store.Capture(CaptureInput{Kind: KindPrompt, Title: q, Text: q, SessionID: "live"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Capture(CaptureInput{
		Kind: KindWorking, Title: q, Text: "Working snapshot: " + q, SessionID: "live",
	}); err != nil {
		t.Fatal(err)
	}
	note, err := store.Capture(CaptureInput{
		Kind:  KindSession,
		Title: "caroline relocation",
		Text:  "Caroline moved from Sweden four years ago.",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := store.Recall(q, 1500)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) == 0 {
		t.Fatal("expected knowledge hit")
	}
	for _, h := range res.Hits {
		if h.Kind == KindPrompt || h.Kind == KindWorking {
			t.Fatalf("live prompt/working ranked in recall: %+v", h)
		}
	}
	if res.Hits[0].ID != note.ID {
		t.Fatalf("session note should rank first, got %q %s", res.Hits[0].Title, res.Hits[0].Kind)
	}
	pack := PromptRecallPack(root, q)
	if !strings.Contains(pack, "Sweden") {
		t.Fatalf("prompt pack should quote the note, got %q", pack)
	}
	if strings.Contains(pack, "Working snapshot") {
		t.Fatalf("prompt pack leaked working copy: %q", pack)
	}
}

func TestRecallDiaryWhenNoKnowledge(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ep, err := store.Capture(CaptureInput{
		Kind: KindPrompt, Title: "use JWT with 15m expiry",
		Text: "Decision: cookies stay in sqlite, JWT expiry is 15 minutes.",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := store.Recall("JWT expiry", 1500)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range res.Hits {
		if h.ID == ep.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("pre-distill diary must still recall, hits=%v", titlesOf(res.Hits))
	}
}

func TestRecallDemotesHitsMissingProperName(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.Capture(CaptureInput{
		Kind:  KindSession,
		Title: "family move",
		Text:  "Melanie said the family might move next year.",
	}); err != nil {
		t.Fatal(err)
	}
	named, err := store.Capture(CaptureInput{
		Kind:  KindSession,
		Title: "caroline relocation",
		Text:  "Caroline moved from Sweden four years ago.",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := store.Recall("Where did Caroline move from?", 1500)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) == 0 {
		t.Fatal("expected hits")
	}
	foundNamed := false
	for i, h := range res.Hits {
		if h.ID == named.ID {
			foundNamed = true
			if i > 2 {
				t.Fatalf("named note should stay in the top hits, rank=%d", i)
			}
		}
	}
	if !foundNamed {
		t.Fatalf("named note missing, hits=%v", titlesOf(res.Hits))
	}
	if res.Hits[0].ID != named.ID {
		t.Fatalf("named origin should rank first, got %q", res.Hits[0].Title)
	}
}

func TestRecallPrefixRanksMovedOriginOverUnrelatedMove(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.Capture(CaptureInput{
		Kind:  KindSession,
		Title: "peace after the move",
		Text:  "Alex asked how family had been supportive during the move to a new apartment.",
	}); err != nil {
		t.Fatal(err)
	}
	origin, err := store.Capture(CaptureInput{
		Kind:  KindSession,
		Title: "alex relocation",
		Text:  "Alex moved from Portugal four years ago and still mentions it.",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := store.Recall("Where did Alex move from 4 years ago?", 1500)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) == 0 || res.Hits[0].ID != origin.ID {
		t.Fatalf("origin note should rank first, hits=%v", titlesOf(res.Hits))
	}
}
