package inject

import "testing"

func TestReplaceOrAppendIdempotent(t *testing.T) {
	block := startMarker + "\n## Superopen\nbody\n" + endMarker + "\n"
	base := "# Title\n\ncontent\n\n" + block
	once := replaceOrAppend(base, block)
	twice := replaceOrAppend(once, block)
	if once != twice {
		t.Fatalf("re-inject grew content:\n once=%q\n twice=%q", once, twice)
	}
	if once != base {
		t.Fatalf("first re-inject should be no-op when block unchanged:\n got=%q\n want=%q", once, base)
	}
}
