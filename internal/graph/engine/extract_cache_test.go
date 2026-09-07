package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

type countingSyntaxParser struct {
	inner fixtureSyntaxParser
	calls int
}

func (p *countingSyntaxParser) Parse(ctx context.Context, language string, source []byte) (SyntaxNode, error) {
	p.calls++
	return p.inner.Parse(ctx, language, source)
}

func TestParseFromProbeUsesExtractCache(t *testing.T) {
	root := initGitRepo(t, map[string]string{
		"a.py": "def run():\n pass\n",
		"b.py": "def other():\n pass\n",
	})
	if err := os.MkdirAll(filepath.Join(root, ".so", "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := []string{"a.py", "b.py"}
	first := &countingSyntaxParser{}
	repo, err := ParseSyntaxRepository(context.Background(), first, root, "fixture", files, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	saveParsedExtracts(root, repo)
	if first.calls < 2 {
		t.Fatalf("cold parse calls=%d, want at least 2", first.calls)
	}
	if err := WriteFingerprint(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.py"), []byte("def run():\n return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second := &countingSyntaxParser{}
	changes := api.ChangeSet{Modified: []api.FileChange{{Kind: "modified", Path: "a.py"}}}
	got, parsed, err := parseSyntaxRepositoryFromProbe(context.Background(), second, root, "fixture", files, changes, 1)
	if err != nil {
		t.Fatal(err)
	}
	if parsed != 1 {
		t.Fatalf("parsed=%d, want 1 dirty file", parsed)
	}
	if second.calls != 1 {
		t.Fatalf("cached probe parse calls=%d, want 1", second.calls)
	}
	if len(got.Files) != 2 {
		t.Fatalf("files=%d, want both cached and dirty", len(got.Files))
	}
}

func TestParseFromProbeDropsDeleted(t *testing.T) {
	root := initGitRepo(t, map[string]string{
		"a.py": "def run():\n pass\n",
		"b.py": "def other():\n pass\n",
	})
	if err := os.MkdirAll(filepath.Join(root, ".so", "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := []string{"a.py", "b.py"}
	repo, err := ParseSyntaxRepository(context.Background(), fixtureSyntaxParser{}, root, "fixture", files, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	saveParsedExtracts(root, repo)
	if err := WriteFingerprint(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "b.py")); err != nil {
		t.Fatal(err)
	}
	changes := api.ChangeSet{Deleted: []api.FileChange{{Kind: "deleted", Path: "b.py"}}}
	got, parsed, err := parseSyntaxRepositoryFromProbe(context.Background(), fixtureSyntaxParser{}, root, "fixture", files, changes, 1)
	if err != nil {
		t.Fatal(err)
	}
	if parsed != 0 {
		t.Fatalf("parsed=%d, want 0 (deleted must not be parsed)", parsed)
	}
	if len(got.Files) != 1 || got.Files[0].File.Path != "a.py" {
		t.Fatalf("assemble set=%v, want only a.py", pathsOf(got.Files))
	}
}

func pathsOf(files []ParsedSyntaxFile) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.File.Path
	}
	return out
}
