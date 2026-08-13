package adapters

import "testing"

func TestEncodeClaudeProjectDirPortable(t *testing.T) {
	windows := map[string]string{
		`C:\Users\me\repo`:        "C--Users-me-repo",
		`\\server\share\repo`:     "--server-share-repo",
		`D:/work/mixed\separator`: "D--work-mixed-separator",
	}
	for input, want := range windows {
		if got := encodeClaudeProjectDirForOS(input, "windows"); got != want {
			t.Errorf("Windows encoding of %q = %q, want %q", input, got, want)
		}
	}
	if got := encodeClaudeProjectDirForOS(`/Users/me/repo:archive`, "darwin"); got != `-Users-me-repo:archive` {
		t.Errorf("Unix encoding changed legal filename characters: %q", got)
	}
}
