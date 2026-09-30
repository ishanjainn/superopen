package hook

import "testing"

func TestClassifyExtraAgents(t *testing.T) {
	session, tool, compact := classifyExtraEvent("PreToolUse")
	if !tool || session || compact {
		t.Fatalf("PreToolUse = %v %v %v", session, tool, compact)
	}
	session, tool, _ = classifyExtraEvent("on_session_start")
	if !session || tool {
		t.Fatalf("session start = %v %v", session, tool)
	}
	if hookProtocol("qwen") != "claude-code" {
		t.Fatal("qwen should speak the claude hook protocol")
	}
	if hookProtocol("omp") != "pi" || steerVendor("senpi") != "pi" {
		t.Fatal("pi forks should steer like pi")
	}
	if hookProtocol("cline") != "context" {
		t.Fatal("cline should use additionalContext")
	}
}
