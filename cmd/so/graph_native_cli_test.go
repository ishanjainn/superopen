package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestApplyStalePrependsHeader(t *testing.T) {
	got := applyStale("[!] graph stale: edit not indexed yet", "NODE Foo\n")
	want := "[!] graph stale: edit not indexed yet\nNODE Foo\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if applyStale("", "NODE Foo\n") != "NODE Foo\n" {
		t.Fatal("empty stale must leave text")
	}
	already := "[!] graph stale: edit not indexed yet\nNODE Foo\n"
	if applyStale("[!] graph stale: edit not indexed yet", already) != already {
		t.Fatal("must not double-prepend")
	}
}

func TestCompactGraphTextImpact(t *testing.T) {
	raw, err := json.Marshal(api.ImpactResult{
		ImpactedFiles: []api.ImpactedFile{{Path: "pkg/dispatch.py", Symbols: 1, Reasons: []string{"caller"}}},
		Total:         1,
	})
	if err != nil {
		t.Fatal(err)
	}
	text := compactGraphText(api.OpImpact, raw)
	if !strings.Contains(text, "impacted_files:") || !strings.Contains(text, "pkg/dispatch.py") {
		t.Fatalf("expected compact impact text: %q", text)
	}
	if strings.Contains(text, `"path"`) {
		t.Fatalf("impact must not fall back to JSON: %q", text)
	}
	hints := graphHelp(api.OpImpact, raw)
	if len(hints) == 0 || !strings.Contains(hints[0], "pkg/dispatch.py") {
		t.Fatalf("impact help: %v", hints)
	}
}
