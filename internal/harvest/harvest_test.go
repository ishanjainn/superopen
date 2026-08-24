package harvest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/paths"
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

func TestSessionStartLine(t *testing.T) {
	root := testRepo(t)
	if line := SessionStartLine(root); line != "" {
		t.Fatalf("expected empty, got %q", line)
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
	line := SessionStartLine(root)
	if !strings.Contains(line, "HARVEST") || !strings.Contains(line, "OPEN") {
		t.Fatalf("line %q", line)
	}
	attached := AttachSessionStart("", root)
	if !strings.HasPrefix(attached, "Superopen: codebase questions") {
		t.Fatalf("must stay graph-first: %q", attached)
	}
	if !strings.Contains(attached, "HARVEST") {
		t.Fatalf("missing harvest: %q", attached)
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
