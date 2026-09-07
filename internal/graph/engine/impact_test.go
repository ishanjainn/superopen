package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestImpactCallersAndFamily(t *testing.T) {
	ctx := context.Background()
	store, err := OpenWritable(t.TempDir() + "/graph.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: "/repo", Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		iface, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Interface", Name: "Handler", QualifiedName: "pkg.Handler",
			Location: api.Location{File: "pkg/handler.py", StartLine: 1, EndLine: 4},
		})
		if err != nil {
			return err
		}
		impl, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Class", Name: "HTTPHandler", QualifiedName: "pkg.HTTPHandler",
			Location: api.Location{File: "pkg/http.py", StartLine: 1, EndLine: 8},
		})
		if err != nil {
			return err
		}
		caller, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "dispatch", QualifiedName: "pkg.dispatch",
			Location: api.Location{File: "pkg/dispatch.py", StartLine: 1, EndLine: 6},
		})
		if err != nil {
			return err
		}
		sibling, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Method", Name: "handle", QualifiedName: "pkg.HTTPHandler.handle",
			Location: api.Location{File: "pkg/http_extra.py", StartLine: 1, EndLine: 5},
		})
		if err != nil {
			return err
		}
		if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: caller, TargetID: iface, Type: "CALLS"}); err != nil {
			return err
		}
		if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: impl, TargetID: iface, Type: "IMPLEMENTS"}); err != nil {
			return err
		}
		if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: impl, TargetID: sibling, Type: "DEFINES_METHOD"}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := store.Impact(ctx, api.ImpactRequest{
		Project: "fixture",
		Files:   []string{"pkg/handler.py"},
		Depth:   1,
		Limit:   40,
	})
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string][]string{}
	for _, f := range result.ImpactedFiles {
		paths[f.Path] = f.Reasons
	}
	if _, ok := paths["pkg/dispatch.py"]; !ok {
		t.Fatalf("caller file missing: %+v", result.ImpactedFiles)
	}
	if !containsReason(paths["pkg/dispatch.py"], "caller") {
		t.Fatalf("dispatch should be caller: %+v", paths["pkg/dispatch.py"])
	}
	if !containsReason(paths["pkg/http.py"], "impl") {
		t.Fatalf("http.py should be impl: %+v", result.ImpactedFiles)
	}
	if _, ok := paths["pkg/handler.py"]; ok {
		t.Fatal("seed file must not appear in ImpactedFiles")
	}

	fromImpl, err := store.Impact(ctx, api.ImpactRequest{
		Project: "fixture",
		Files:   []string{"pkg/http.py"},
		Depth:   1,
		Limit:   40,
	})
	if err != nil {
		t.Fatal(err)
	}
	implPaths := map[string][]string{}
	for _, f := range fromImpl.ImpactedFiles {
		implPaths[f.Path] = f.Reasons
	}
	if !containsReason(implPaths["pkg/http_extra.py"], "sibling") {
		t.Fatalf("http_extra.py should be sibling: %+v", fromImpl.ImpactedFiles)
	}
}

func TestImpactCoChangeFallback(t *testing.T) {
	ctx := context.Background()
	store, err := OpenWritable(t.TempDir() + "/graph.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: "/repo", Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		left, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "File", Name: "a.py", QualifiedName: "a.py",
			Location: api.Location{File: "a.py"},
		})
		if err != nil {
			return err
		}
		right, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "File", Name: "b.py", QualifiedName: "b.py",
			Location: api.Location{File: "b.py"},
		})
		if err != nil {
			return err
		}
		fn, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "work", QualifiedName: "a.work",
			Location: api.Location{File: "a.py", StartLine: 1, EndLine: 2},
		})
		if err != nil {
			return err
		}
		_ = fn
		if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: left, TargetID: right, Type: "FILE_CHANGES_WITH"}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Impact(ctx, api.ImpactRequest{
		Project: "fixture",
		Files:   []string{"a.py"},
		Depth:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range result.ImpactedFiles {
		if f.Path == "b.py" && containsReason(f.Reasons, "co-change") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected co-change b.py, got %+v", result.ImpactedFiles)
	}
}

func TestImpactBaseSeedsChangedFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	root := t.TempDir()
	runImpactGit(t, root, "init")
	runImpactGit(t, root, "config", "user.email", "so@example.com")
	runImpactGit(t, root, "config", "user.name", "so")
	runImpactGit(t, root, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(root, "a.py"), []byte("def a():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runImpactGit(t, root, "add", ".")
	runImpactGit(t, root, "commit", "-m", "base")
	runImpactGit(t, root, "branch", "-M", "main")
	if err := os.WriteFile(filepath.Join(root, "a.py"), []byte("def a():\n    return 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.py"), []byte("def b():\n    return 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runImpactGit(t, root, "add", ".")
	runImpactGit(t, root, "commit", "-m", "change")

	ctx := context.Background()
	store, err := OpenWritable(t.TempDir() + "/graph.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: root, Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		_, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "a", QualifiedName: "a.a",
			Location: api.Location{File: "a.py", StartLine: 1, EndLine: 2},
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Impact(ctx, api.ImpactRequest{
		Project:  "fixture",
		RepoRoot: root,
		Base:     "main",
		Depth:    1,
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(result.ChangedFiles, ",")
	if !strings.Contains(joined, "a.py") && !strings.Contains(joined, "b.py") {
		// HEAD is main after branch -M, so diff against main is empty. Use HEAD~1.
		result, err = store.Impact(ctx, api.ImpactRequest{
			Project:  "fixture",
			RepoRoot: root,
			Base:     "HEAD~1",
			Depth:    1,
		})
		if err != nil {
			t.Fatal(err)
		}
		joined = strings.Join(result.ChangedFiles, ",")
	}
	if !strings.Contains(joined, "a.py") {
		t.Fatalf("changed files should include a.py: %v merge=%s", result.ChangedFiles, result.MergeBase)
	}
}

func TestParseUnifiedDiffHunks(t *testing.T) {
	hunks := parseUnifiedDiffHunks("diff --git a/pkg/hub.py b/pkg/hub.py\n--- a/pkg/hub.py\n+++ b/pkg/hub.py\n@@ -2 +2 @@\n-    return 1\n+    return 2\n@@ -10,2 +10,0 @@\n-def gone():\n-    pass\n")
	sp := hunks["pkg/hub.py"]
	if len(sp) != 2 {
		t.Fatalf("hunks=%+v", hunks)
	}
	if sp[0].start != 2 || sp[0].end != 2 {
		t.Fatalf("edit span: %+v", sp[0])
	}
	if sp[1].start != 10 || sp[1].end != 10 {
		t.Fatalf("deletion span: %+v", sp[1])
	}
}

func TestImpactBaseSeedsChangedLinesOnly(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	root := t.TempDir()
	runImpactGit(t, root, "init")
	runImpactGit(t, root, "config", "user.email", "so@example.com")
	runImpactGit(t, root, "config", "user.name", "so")
	runImpactGit(t, root, "config", "commit.gpgsign", "false")
	hub := "def changed():\n    return 1\n\ndef leftover():\n    return 9\n\ndef untouched():\n    return 2\n"
	if err := os.WriteFile(filepath.Join(root, "hub.py"), []byte(hub), 0o644); err != nil {
		t.Fatal(err)
	}
	runImpactGit(t, root, "add", ".")
	runImpactGit(t, root, "commit", "-m", "base")
	changed := "def changed():\n    return 3\n\ndef leftover():\n    return 9\n\ndef untouched():\n    return 2\n"
	if err := os.WriteFile(filepath.Join(root, "hub.py"), []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	runImpactGit(t, root, "add", ".")
	runImpactGit(t, root, "commit", "-m", "edit changed()")

	ctx := context.Background()
	store, err := OpenWritable(t.TempDir() + "/graph.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: root, Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		changedID, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "changed", QualifiedName: "hub.changed",
			Location: api.Location{File: "hub.py", StartLine: 1, EndLine: 2},
		})
		if err != nil {
			return err
		}
		untouchedID, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "untouched", QualifiedName: "hub.untouched",
			Location: api.Location{File: "hub.py", StartLine: 7, EndLine: 8},
		})
		if err != nil {
			return err
		}
		useChanged, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "use_changed", QualifiedName: "use.changed",
			Location: api.Location{File: "use_changed.py", StartLine: 1, EndLine: 2},
		})
		if err != nil {
			return err
		}
		useUntouched, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "use_untouched", QualifiedName: "use.untouched",
			Location: api.Location{File: "use_untouched.py", StartLine: 1, EndLine: 2},
		})
		if err != nil {
			return err
		}
		if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: useChanged, TargetID: changedID, Type: "CALLS"}); err != nil {
			return err
		}
		_, err = builder.PutEdge(api.Edge{Project: "fixture", SourceID: useUntouched, TargetID: untouchedID, Type: "CALLS"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	whole, err := store.Impact(ctx, api.ImpactRequest{
		Project: "fixture", Files: []string{"hub.py"}, Depth: 1, Limit: 40,
	})
	if err != nil {
		t.Fatal(err)
	}
	wholePaths := map[string]bool{}
	for _, f := range whole.ImpactedFiles {
		wholePaths[f.Path] = true
	}
	if !wholePaths["use_changed.py"] || !wholePaths["use_untouched.py"] {
		t.Fatalf("--files should seed the whole file: %+v", whole.ImpactedFiles)
	}

	hunked, err := store.Impact(ctx, api.ImpactRequest{
		Project: "fixture", RepoRoot: root, Base: "HEAD~1", Depth: 1, Limit: 40,
	})
	if err != nil {
		t.Fatal(err)
	}
	hunkPaths := map[string]bool{}
	for _, f := range hunked.ImpactedFiles {
		hunkPaths[f.Path] = true
	}
	if !hunkPaths["use_changed.py"] {
		t.Fatalf("--base should seed callers of changed lines: %+v files=%v", hunked.ImpactedFiles, hunked.ChangedFiles)
	}
	if hunkPaths["use_untouched.py"] {
		t.Fatalf("--base must not seed callers of untouched symbols: %+v", hunked.ImpactedFiles)
	}
}

func TestImpactRanksLocalBeforeDistantCallers(t *testing.T) {
	ctx := context.Background()
	store, err := OpenWritable(t.TempDir() + "/graph.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: "/repo", Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		query, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Class", Name: "Query", QualifiedName: "pkg.sql.Query",
			Location: api.Location{File: "pkg/sql/query.py", StartLine: 1, EndLine: 20},
		})
		if err != nil {
			return err
		}
		where, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "where_sql", QualifiedName: "pkg.sql.where_sql",
			Location: api.Location{File: "pkg/sql/where.py", StartLine: 1, EndLine: 8},
		})
		if err != nil {
			return err
		}
		expr, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Method", Name: "as_sql", QualifiedName: "pkg.models.as_sql",
			Location: api.Location{File: "pkg/models/expressions.py", StartLine: 1, EndLine: 8},
		})
		if err != nil {
			return err
		}
		hub, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "ready", QualifiedName: "aaa.apps.config.ready",
			Location: api.Location{File: "aaa/apps/config.py", StartLine: 1, EndLine: 8},
		})
		if err != nil {
			return err
		}
		if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: where, TargetID: query, Type: "CALLS"}); err != nil {
			return err
		}
		if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: query, TargetID: expr, Type: "DEFINES_METHOD"}); err != nil {
			return err
		}
		if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: hub, TargetID: query, Type: "CALLS"}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Impact(ctx, api.ImpactRequest{
		Project: "fixture",
		Files:   []string{"pkg/sql/query.py"},
		Depth:   1,
		Limit:   40,
	})
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, f := range result.ImpactedFiles {
		order = append(order, f.Path)
	}
	idx := func(p string) int {
		for i, x := range order {
			if x == p {
				return i
			}
		}
		return -1
	}
	if idx("pkg/sql/where.py") < 0 || idx("pkg/models/expressions.py") < 0 {
		t.Fatalf("local files missing: %v", order)
	}
	if idx("aaa/apps/config.py") < 0 {
		t.Fatalf("distant caller must remain in CLI list: %v", order)
	}
	if idx("aaa/apps/config.py") < idx("pkg/sql/where.py") || idx("aaa/apps/config.py") < idx("pkg/models/expressions.py") {
		t.Fatalf("distant caller ranked before locals: %v", order)
	}
}

func TestImpactMergesCoChangeWithCallers(t *testing.T) {
	ctx := context.Background()
	store, err := OpenWritable(t.TempDir() + "/graph.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: "/repo", Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		queryFile, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "File", Name: "query.py", QualifiedName: "pkg/sql/query.py",
			Location: api.Location{File: "pkg/sql/query.py"},
		})
		if err != nil {
			return err
		}
		utilsFile, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "File", Name: "utils.py", QualifiedName: "pkg/sql/utils.py",
			Location: api.Location{File: "pkg/sql/utils.py"},
		})
		if err != nil {
			return err
		}
		legacyFile, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "File", Name: "legacy.py", QualifiedName: "aaa/legacy.py",
			Location: api.Location{File: "aaa/legacy.py"},
		})
		if err != nil {
			return err
		}
		query, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Class", Name: "Query", QualifiedName: "pkg.sql.Query",
			Location: api.Location{File: "pkg/sql/query.py", StartLine: 1, EndLine: 20},
		})
		if err != nil {
			return err
		}
		where, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "where_sql", QualifiedName: "pkg.sql.where_sql",
			Location: api.Location{File: "pkg/sql/where.py", StartLine: 1, EndLine: 8},
		})
		if err != nil {
			return err
		}
		hub, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "ready", QualifiedName: "aaa.apps.config.ready",
			Location: api.Location{File: "aaa/apps/config.py", StartLine: 1, EndLine: 8},
		})
		if err != nil {
			return err
		}
		if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: queryFile, TargetID: utilsFile, Type: "FILE_CHANGES_WITH"}); err != nil {
			return err
		}
		if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: queryFile, TargetID: legacyFile, Type: "FILE_CHANGES_WITH"}); err != nil {
			return err
		}
		if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: where, TargetID: query, Type: "CALLS"}); err != nil {
			return err
		}
		_, err = builder.PutEdge(api.Edge{Project: "fixture", SourceID: hub, TargetID: query, Type: "CALLS"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Impact(ctx, api.ImpactRequest{
		Project: "fixture",
		Files:   []string{"pkg/sql/query.py"},
		Depth:   1,
		Limit:   40,
	})
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	reasons := map[string][]string{}
	for _, f := range result.ImpactedFiles {
		order = append(order, f.Path)
		reasons[f.Path] = f.Reasons
	}
	idx := func(p string) int {
		for i, x := range order {
			if x == p {
				return i
			}
		}
		return -1
	}
	if !containsReason(reasons["pkg/sql/utils.py"], "co-change") {
		t.Fatalf("same-dir co-change missing: %v", result.ImpactedFiles)
	}
	if !containsReason(reasons["pkg/sql/where.py"], "caller") {
		t.Fatalf("same-dir caller missing: %v", result.ImpactedFiles)
	}
	if idx("aaa/apps/config.py") < 0 || idx("aaa/legacy.py") < 0 {
		t.Fatalf("distant files must remain in CLI list: %v", order)
	}
	if idx("pkg/sql/utils.py") > idx("pkg/sql/where.py") {
		t.Fatalf("co-change should rank before caller among locals: %v", order)
	}
	if idx("aaa/apps/config.py") < idx("pkg/sql/utils.py") || idx("aaa/legacy.py") < idx("pkg/sql/utils.py") {
		t.Fatalf("distant files ranked before local co-change: %v", order)
	}
}

func TestNearbyImpactPath(t *testing.T) {
	seeds := []string{"pkg/sql/query.py"}
	if !NearbyImpactPath("pkg/sql/where.py", seeds, []string{"caller"}) {
		t.Fatal("same dir")
	}
	if !NearbyImpactPath("pkg/models/expressions.py", seeds, []string{"sibling"}) {
		t.Fatal("parent dir")
	}
	if NearbyImpactPath("aaa/apps/config.py", seeds, []string{"caller"}) {
		t.Fatal("distant caller must not be nearby")
	}
}

func containsReason(reasons []string, want string) bool {
	for _, r := range reasons {
		if r == want {
			return true
		}
	}
	return false
}

func runImpactGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v %s", strings.Join(args, " "), err, out)
	}
}
