package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestApplyStalePrependsFields(t *testing.T) {
	got := applyStale("[!] graph stale: edit not indexed yet", "count: 1 of 1\n")
	want := "index: stale; answer is from the previous index and may miss recent edits\ncount: 1 of 1\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if applyStale("", "count: 1 of 1\n") != "count: 1 of 1\n" {
		t.Fatal("empty stale must leave text")
	}
	withFiles := applyStale("[!] graph stale: edit not indexed yet\nThese files are absent from this answer. Read them:\n  a/b.py\n  c.py", "x\n")
	if !strings.Contains(withFiles, "stale_files[2]: a/b.py,c.py\n") {
		t.Fatalf("stale files must be listed: %q", withFiles)
	}
}

func TestRenderGraphResultImpact(t *testing.T) {
	raw, err := json.Marshal(api.ImpactResult{
		ImpactedFiles: []api.ImpactedFile{{Path: "pkg/dispatch.py", Symbols: 1, Reasons: []string{"caller"}}},
		Total:         1,
	})
	if err != nil {
		t.Fatal(err)
	}
	text, hints, _, ok := renderGraphResult(api.OpImpact, api.ImpactRequest{}, raw)
	if !ok || !strings.Contains(text, "impacted_files[1]{path,symbols,reasons}:") || !strings.Contains(text, "pkg/dispatch.py") {
		t.Fatalf("expected TOON impact text: %q", text)
	}
	if strings.Contains(text, `"path"`) {
		t.Fatalf("impact must not fall back to JSON: %q", text)
	}
	if len(hints) != 1 {
		t.Fatalf("impact help: %v", hints)
	}
}

func TestRenderGraphResultCodeSearchIsTOON(t *testing.T) {
	raw, err := json.Marshal(api.CodeSearchResult{
		Matches:          []api.CodeSearchMatch{{Location: api.Location{File: "a.py", StartLine: 3}, Source: "idna"}},
		TotalGrepMatches: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	text, _, _, ok := renderGraphResult(api.OpCodeSearch, api.CodeSearchRequest{Pattern: "idna"}, raw)
	if !ok || !strings.Contains(text, "matches[1]{file,line,text}:\n  a.py,3,idna\n") {
		t.Fatalf("code-search must render TOON, got %q", text)
	}
}

func TestGraphCallErrorNotFoundSuggestsSearch(t *testing.T) {
	err := graphCallError(errString("graph store_error: snippet: symbol not found: get_fqdn_nope"))
	if err == nil || !strings.HasPrefix(err.Error(), "symbol not found: get_fqdn_nope") {
		t.Fatalf("err=%v", err)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
