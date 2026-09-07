package harvest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/agent/headless"
	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/session"
)

func testRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := paths.Resolve(root).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# Agents\n\nFollow graph-first search.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func additiveDiff() string {
	return `--- a/AGENTS.md
+++ b/AGENTS.md
@@ -1,3 +1,4 @@
 # Agents
 
 Follow graph-first search.
+Prefer so harvest review for playbook patches.
`
}

func TestInventoryFindsPlaybooks(t *testing.T) {
	root := testRepo(t)
	files, err := Inventory(root)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range files {
		if f.Path == "AGENTS.md" && f.Hash != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("AGENTS.md missing from inventory: %+v", files)
	}
}

func TestProposeRequiresReasonAndEvidence(t *testing.T) {
	root := testRepo(t)
	_, err := Propose(root, ProposeInput{Title: "t", Target: "AGENTS.md", Kind: KindImprove})
	if err == nil || !strings.Contains(err.Error(), "reason") {
		t.Fatalf("expected reason error, got %v", err)
	}
	_, err = Propose(root, ProposeInput{
		SessionID: "s1", Title: "t", Target: "AGENTS.md", Reason: "because", Kind: KindImprove,
	})
	if err == nil || !strings.Contains(err.Error(), "evidence") {
		t.Fatalf("expected evidence error, got %v", err)
	}
}

func TestProposeNoopWhenAlreadyPresent(t *testing.T) {
	root := testRepo(t)
	body := "# Agents\n\nFollow graph-first search.\nPrefer so harvest review for playbook patches.\n"
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Propose(root, ProposeInput{
		Title: "add harvest line", Target: "AGENTS.md", Reason: "session showed missing harvest pointer",
		Kind: KindImprove, Diff: additiveDiff(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != StatusNoop {
		t.Fatalf("status %s", p.Status)
	}
}

func TestProposeRejectsDup(t *testing.T) {
	root := testRepo(t)
	in := ProposeInput{
		Title: "add harvest line", Target: "AGENTS.md", Reason: "missing pointer",
		Kind: KindImprove, Diff: additiveDiff(),
	}
	if _, err := Propose(root, in); err != nil {
		t.Fatal(err)
	}
	if _, err := Propose(root, in); err == nil {
		t.Fatal("expected duplicate")
	}
}

func TestApplyAdditiveAndProtectSentinel(t *testing.T) {
	root := testRepo(t)
	p, err := Propose(root, ProposeInput{
		Title: "add harvest line", Target: "AGENTS.md", Reason: "agents skipped harvest review",
		Kind: KindImprove, Diff: additiveDiff(),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Apply(root, p.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusApplied {
		t.Fatalf("status %s", got.Status)
	}
	body, _ := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if !strings.Contains(string(body), "so harvest review") {
		t.Fatalf("patch not applied: %s", body)
	}

	blocked := filepath.Join(root, "AGENTS.md")
	orig := string(body) + "\n" + sentinelBegin + "\nkeep\n" + sentinelEnd + "\n"
	if err := os.WriteFile(blocked, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	h, _ := HashFile(root, "AGENTS.md")
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	bad, err := store.InsertProposal(Proposal{
		Status: StatusOpen, Kind: KindImprove, Target: "AGENTS.md",
		Title: "touch sentinel", Reason: "no", BaseHash: h,
		Diff: `--- a/AGENTS.md
+++ b/AGENTS.md
@@ -1,4 +1,4 @@
 ` + sentinelBegin + `
-keep
+changed
 ` + sentinelEnd + `
`,
	})
	store.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(root, bad.ID, true); err == nil {
		t.Fatal("expected sentinel protect")
	}
}

func TestGenerateSkipGates(t *testing.T) {
	root := testRepo(t)
	t.Setenv("PATH", t.TempDir())
	res := Generate(root, "sess-empty")
	if res.Skipped != "empty" {
		t.Fatalf("empty session: %+v", res)
	}
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertRun("sess-done", StatusProposed, "live", ""); err != nil {
		t.Fatal(err)
	}
	store.Close()
	res = Generate(root, "sess-done")
	if res.Skipped != "already-harvested" {
		t.Fatalf("already harvested: %+v", res)
	}
}

func TestParseProposalsCapsAndPrefersSimplify(t *testing.T) {
	raw := `[
	  {"kind":"create","title":"c","target":"AGENTS.md","reason":"r"},
	  {"kind":"principle","title":"p","target":"AGENTS.md","reason":"r"},
	  {"kind":"simplify","title":"s","target":"AGENTS.md","reason":"r"},
	  {"kind":"improve","title":"i","target":"AGENTS.md","reason":"r"}
	]`
	got := pickProposals(parseProposals(raw))
	if len(got) != MaxProposals {
		t.Fatalf("len %d want %d", len(got), MaxProposals)
	}
	if got[0].Kind != KindSimplify {
		t.Fatalf("first kind %s want simplify", got[0].Kind)
	}
}

func TestSessionStartLinePendingNotOpenReview(t *testing.T) {
	root := testRepo(t)
	if line := PendingSessionStartLine(root); line != "" {
		t.Fatalf("expected empty, got %q", line)
	}
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertRun("sess-pending", StatusPending, "", "await-live"); err != nil {
		t.Fatal(err)
	}
	store.Close()
	line := PendingSessionStartLine(root)
	if !strings.Contains(line, "HARVEST pending") || !strings.Contains(line, "so harvest propose") {
		t.Fatalf("line %q", line)
	}
	if !strings.Contains(line, "so harvest brief") || !strings.Contains(line, "so harvest skip") {
		t.Fatalf("live line must name brief then skip: %q", line)
	}
	if strings.Contains(line, "harvest scan") {
		t.Fatalf("live line must not name headless scan: %q", line)
	}
	if strings.Contains(line, "OPEN") || strings.Contains(line, "review") {
		t.Fatalf("OPEN review must not be on SessionStart: %q", line)
	}
	p, err := Propose(root, ProposeInput{
		Title: "add harvest line", Target: "AGENTS.md", Reason: "missing",
		Kind: KindImprove, Diff: additiveDiff(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != StatusOpen {
		t.Fatalf("status %s", p.Status)
	}
	// OPEN proposals still must not change the pending pointer into a review nag.
	if strings.Contains(PendingSessionStartLine(root), "review") {
		t.Fatalf("pending line must not become review: %q", PendingSessionStartLine(root))
	}
}

func TestPatchApplyRoundTrip(t *testing.T) {
	src := "# Agents\n\nFollow graph-first search.\n"
	out, err := ApplyUnified(src, additiveDiff())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "so harvest review") {
		t.Fatalf("got %q", out)
	}
}

func TestApplyRejectsStale(t *testing.T) {
	root := testRepo(t)
	p, err := Propose(root, ProposeInput{
		Title: "add harvest line", Target: "AGENTS.md", Reason: "missing pointer",
		Kind: KindImprove, Diff: additiveDiff(),
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetStatus(p.ID, StatusStale); err != nil {
		t.Fatal(err)
	}
	store.Close()
	if _, err := Apply(root, p.ID, true); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("expected stale reject, got %v", err)
	}
}

func TestInventoryFindsMdc(t *testing.T) {
	root := testRepo(t)
	dir := filepath.Join(root, ".cursor", "rules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gpu.mdc"), []byte("use the graph\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := Inventory(root)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range files {
		if f.Path == ".cursor/rules/gpu.mdc" {
			found = true
		}
	}
	if !found {
		t.Fatalf("mdc missing: %+v", files)
	}
}

func startHarvestSession(t *testing.T, root, id, vendor, title, model string) {
	t.Helper()
	store := session.NewStore(paths.Resolve(root))
	if err := store.Start(session.Meta{
		ID: id, Vendor: vendor, Title: title, Model: model,
		PromptPreview: title, StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMaybeGenerateCursorAwaitsLive(t *testing.T) {
	root := testRepo(t)
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	startHarvestSession(t, root, "cursor-1", "cursor", "fix the dashboard", "grok")
	res := MaybeGenerate(root, "cursor-1")
	if !res.Pending || res.Skipped != "await-live" {
		t.Fatalf("cursor must not steal a CLI: %+v", res)
	}
}

func TestMaybeGenerateOncePerSession(t *testing.T) {
	root := testRepo(t)
	startHarvestSession(t, root, "cursor-dup", "cursor", "fix the dashboard", "grok")
	first := MaybeGenerate(root, "cursor-dup")
	if !first.Pending || first.Skipped != "await-live" {
		t.Fatalf("first: %+v", first)
	}
	second := MaybeGenerate(root, "cursor-dup")
	if second.Skipped != "already-queued" {
		t.Fatalf("second SessionEnd must not queue again: %+v", second)
	}
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if !store.HasAttempt("cursor-dup") {
		t.Fatal("expected one harvest attempt")
	}
}

func TestProposeResolvesPending(t *testing.T) {
	root := testRepo(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertRun("sess-pending", StatusPending, "", SkipAwaitLive); err != nil {
		t.Fatal(err)
	}
	store.Close()
	_, err = Propose(root, ProposeInput{
		SessionID: "sess-pending", Title: "add harvest line", Target: "AGENTS.md",
		Reason: "missing", Kind: KindImprove, Diff: additiveDiff(),
		Evidence: []Evidence{{Kind: "session", ID: "sess-pending", Label: "wrap-up"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if PendingSessionStartLine(root) != "" {
		t.Fatalf("propose must clear pending: %q", PendingSessionStartLine(root))
	}
}

func TestSkipResolvesPending(t *testing.T) {
	root := testRepo(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertRun("sess-skip", StatusPending, "", SkipAwaitLive); err != nil {
		t.Fatal(err)
	}
	store.Close()
	if err := Skip(root, "sess-skip", "nothing to change"); err != nil {
		t.Fatal(err)
	}
	if PendingSessionStartLine(root) != "" {
		t.Fatalf("skip must clear pending: %q", PendingSessionStartLine(root))
	}
}

func TestGenerateCursorNoAuth(t *testing.T) {
	root := testRepo(t)
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	startHarvestSession(t, root, "cursor-scan", "cursor", "fix the dashboard", "grok")
	res := Generate(root, "cursor-scan")
	if res.Skipped != "no-auth" || !res.Pending {
		t.Fatalf("cursor scan must not steal a CLI: %+v", res)
	}
	if PendingSessionStartLine(root) == "" {
		t.Fatal("failed own-CLI scan must leave pending for SessionStart")
	}
}

func TestBriefUsesPendingSession(t *testing.T) {
	root := testRepo(t)
	startHarvestSession(t, root, "brief-1", "cursor", "fix the dashboard", "grok")
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertRun("brief-1", StatusPending, "", SkipAwaitLive); err != nil {
		t.Fatal(err)
	}
	store.Close()
	text, err := Brief(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "brief-1") || !strings.Contains(text, "You propose playbook patches") {
		t.Fatalf("brief: %q", text)
	}
}

func TestMaybeGenerateSkipsWorkerSession(t *testing.T) {
	root := testRepo(t)
	startHarvestSession(t, root, "w1", "claude-code", headless.WorkerHarvestPrefix+" for Superopen harvest.", "<synthetic>")
	res := MaybeGenerate(root, "w1")
	if res.Skipped != "worker-session" || res.Pending {
		t.Fatalf("worker session must skip without pending: %+v", res)
	}
}

func TestGenerateSkipsWorkerFingerprint(t *testing.T) {
	root := testRepo(t)
	startHarvestSession(t, root, "w2", "claude-code", headless.WorkerDistillPrefix+" for a coding agent.", "<synthetic>")
	res := Generate(root, "w2")
	if res.Skipped != "worker-session" {
		t.Fatalf("got %+v", res)
	}
}

func TestListHistoryIncludesSkippedRunsAndClosedProposals(t *testing.T) {
	root := testRepo(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.InsertRun("sess-skip", StatusSkipped, "", "nothing to add"); err != nil {
		t.Fatal(err)
	}
	open, err := store.InsertProposal(Proposal{
		SessionID: "sess-open", Status: StatusOpen, Kind: KindImprove, Target: "AGENTS.md",
		Title: "keep me open", Reason: "still waiting",
	})
	if err != nil {
		t.Fatal(err)
	}
	applied, err := store.InsertProposal(Proposal{
		SessionID: "sess-applied", Status: StatusApplied, Kind: KindImprove, Target: "AGENTS.md",
		Title: "already applied", Reason: "done",
	})
	if err != nil {
		t.Fatal(err)
	}
	items, err := store.ListHistory(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	var sawSkip, sawApplied, sawOpen bool
	for _, it := range items {
		if it.Source == "run" && it.Status == StatusSkipped && it.Title == "Nothing to change" {
			sawSkip = true
		}
		if it.Source == "proposal" && it.ID == applied.ID {
			sawApplied = true
		}
		if it.Source == "proposal" && it.ID == open.ID {
			sawOpen = true
		}
	}
	if !sawSkip || !sawApplied {
		t.Fatalf("history missing skip or applied: %+v", items)
	}
	if sawOpen {
		t.Fatal("open proposals must not appear in history")
	}
}

func TestDeleteExpiredKeepsOpenAndPending(t *testing.T) {
	root := testRepo(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	old := time.Now().UTC().Add(-10 * 24 * time.Hour).Format(time.RFC3339)
	skipID, err := store.InsertRun("old-skip", StatusSkipped, "", "stale skip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE harvest_runs SET created_at=?, updated_at=? WHERE id=?`, old, old, skipID); err != nil {
		t.Fatal(err)
	}
	pendingID, err := store.InsertRun("still-pending", StatusPending, "", SkipAwaitLive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE harvest_runs SET created_at=?, updated_at=? WHERE id=?`, old, old, pendingID); err != nil {
		t.Fatal(err)
	}
	open, err := store.InsertProposal(Proposal{
		SessionID: "open-sess", Status: StatusOpen, Kind: KindImprove, Target: "AGENTS.md",
		Title: "open stays", Reason: "actionable", CreatedAt: old,
	})
	if err != nil {
		t.Fatal(err)
	}
	closed, err := store.InsertProposal(Proposal{
		SessionID: "closed-sess", Status: StatusDeclined, Kind: KindImprove, Target: "AGENTS.md",
		Title: "old declined", Reason: "nope", CreatedAt: old,
	})
	if err != nil {
		t.Fatal(err)
	}
	n, err := store.DeleteExpired(time.Now().UTC().Add(-7 * 24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n < 2 {
		t.Fatalf("expected to delete skip+declined, deleted %d", n)
	}
	if _, err := store.GetProposal(open.ID); err != nil {
		t.Fatal("open proposal must remain")
	}
	if _, err := store.GetProposal(closed.ID); err == nil {
		t.Fatal("declined proposal should be gone")
	}
	items, err := store.ListHistory(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Source == "run" && it.SessionID == "old-skip" {
			t.Fatal("expired skip should not be in history")
		}
	}
	if _, ok := store.LatestRun(); !ok {
		t.Fatal("pending run should still exist")
	}
	latest, ok := store.LatestRun()
	if !ok || latest.SessionID != "still-pending" {
		t.Fatalf("pending run must remain, latest=%+v", latest)
	}
}
