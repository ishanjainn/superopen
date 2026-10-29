package session

import "testing"

func TestInputMessagePrompt(t *testing.T) {
	raw := `[{"role":"user","parts":[{"type":"text","content":"Current value is READY. Future edits must run unittest."}]}]`
	if got := inputMessagePrompt(raw); got != "Current value is READY. Future edits must run unittest." {
		t.Fatalf("prompt = %q", got)
	}
	if got := inputMessagePrompt(`[{"role":"assistant","parts":[{"type":"text","content":"not a user prompt"}]}]`); got != "" {
		t.Fatalf("assistant text became a prompt: %q", got)
	}
}
