package export

import "testing"

func TestDetectTerminalTypeUsesCursorEnv(t *testing.T) {
	t.Setenv("CURSOR_AGENT", "1")
	if got := DetectTerminalType(); got != "cursor" {
		t.Fatalf("got %q want cursor", got)
	}
}
