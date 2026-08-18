package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixtureSyntaxParser struct{}

func (fixtureSyntaxParser) Parse(ctx context.Context, language string, source []byte) (SyntaxNode, error) {
	if err := ctx.Err(); err != nil {
		return SyntaxNode{}, err
	}
	if language == "python" {
		return SyntaxNode{Type: "module", Named: true, End: uint32(len(source)), Children: []SyntaxNode{
			{Type: "function_definition", Named: true, Start: 0, End: uint32(len(source)), Children: []SyntaxNode{
				{Type: "identifier", Field: "name", Named: true, Start: 4, End: 7},
			}},
		}}, nil
	}
	return SyntaxNode{}, errors.New("unexpected grammar " + language)
}

type objectScriptSyntaxParser struct{ calls int }

func (parser *objectScriptSyntaxParser) Parse(_ context.Context, language string, source []byte) (SyntaxNode, error) {
	if language != "objectscript_udl" || !strings.Contains(string(source), "Class ") {
		return SyntaxNode{}, errors.New("unexpected ObjectScript input")
	}
	parser.calls++
	return SyntaxNode{Type: "source_file", Named: true, End: uint32(len(source))}, nil
}

func TestParseSyntaxRepositoryIsDeterministicAndGrounded(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "one.py"), []byte("def run():\n pass\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README"), []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := ParseSyntaxRepository(context.Background(), fixtureSyntaxParser{}, root, "fixture", []string{"README", "one.py"}, nil, 4)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ParseSyntaxRepository(context.Background(), fixtureSyntaxParser{}, root, "fixture", []string{"one.py", "README"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Files) != 1 || first.Files[0].File.Language != "python" || len(first.Files[0].Extraction.Definitions) != 1 || first.Files[0].Extraction.Definitions[0].Name != "run" {
		t.Fatalf("repository = %#v", first)
	}
	if first.Generation != second.Generation || first.Generation == "" {
		t.Fatalf("generation is nondeterministic: %q != %q", first.Generation, second.Generation)
	}
}

func TestParseSyntaxRepositoryHonorsCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ParseSyntaxRepository(ctx, fixtureSyntaxParser{}, t.TempDir(), "fixture", []string{"one.py"}, nil, 2); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
}

func TestParseSyntaxRepositoryTranscodesObjectScriptExport(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	body := []byte(`<Export generator="Cache"><Class name="One"></Class><Class name="Two"></Class></Export>`)
	if err := os.WriteFile(filepath.Join(root, "export.xml"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	parser := &objectScriptSyntaxParser{}
	got, err := ParseSyntaxRepository(context.Background(), parser, root, "fixture", []string{"export.xml"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if parser.calls != 2 || len(got.Files) != 1 || got.Files[0].File.Language != "objectscript_export" || len(got.Coverage.Rows) != 0 {
		t.Fatalf("repository = %#v, parser calls = %d", got, parser.calls)
	}
}
