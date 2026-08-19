package engine

import (
	"strings"
	"testing"
)

func TestSemanticTokensPinnedBehavior(t *testing.T) {
	got := semanticTokens("HTTPServer.ctx/db_repo-Parse2JSON(req)", 32)
	want := []string{"httpserver", "ctx", "db", "repo", "parse2json", "req", "context", "database", "repository", "request"}
	if !sameStrings(got, want) {
		t.Fatalf("tokens = %#v, want %#v", got, want)
	}
}

func TestSemanticTokensBoundsAndExpansionOrder(t *testing.T) {
	got := semanticTokens("ctx_req_err", 4)
	want := []string{"ctx", "req", "err", "context"}
	if !sameStrings(got, want) {
		t.Fatalf("tokens = %#v, want %#v", got, want)
	}
	long := semanticTokens(strings.Repeat("a", 200), 10)
	if len(long) != 1 || len(long[0]) != 127 {
		t.Fatalf("long token = %#v", long)
	}
	if got := semanticTokens("ctx", 0); got != nil {
		t.Fatalf("zero limit = %#v", got)
	}
}
