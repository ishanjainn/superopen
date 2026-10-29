package guards

import (
	"testing"

	"github.com/ishanjainn/superopen/internal/session/trace"
)

func TestEventsFromSpansProjectsBashAndRead(t *testing.T) {
	spans := []trace.Span{
		{
			Name:      "coding_agent.tool.call",
			SessionID: "s1",
			Attributes: map[string]string{
				"gen_ai.tool.name":           "Bash",
				"gen_ai.tool.call.arguments": `{"command":"echo aGk= | base64 --decode | bash"}`,
			},
		},
		{
			Name: "coding_agent.tool.call",
			Attributes: map[string]string{
				"gen_ai.tool.name":           "Read",
				"gen_ai.tool.call.arguments": `{"file_path":"main.go"}`,
			},
		},
		{
			Name: "coding_agent.llm.turn",
			Attributes: map[string]string{
				"gen_ai.tool.name": "Bash",
			},
		},
	}
	events := EventsFromSpans(spans)
	if len(events) != 2 {
		t.Fatalf("events: %d", len(events))
	}
	if events[0].Event.Action != "command.executed" || events[0].Command.Command == "" {
		t.Fatalf("bash: %+v", events[0])
	}
	if events[1].Event.Action != "file.read" || events[1].File.Path != "main.go" {
		t.Fatalf("read: %+v", events[1])
	}
}

func TestActionForToolAcrossHostNames(t *testing.T) {
	for _, name := range []string{"Bash", "bash", "Shell", "shell", "exec_command"} {
		if action, ok := actionForTool(name); !ok || action != "command.executed" {
			t.Fatalf("%q mapped to %q, %v", name, action, ok)
		}
	}
	if action, ok := actionForTool("read"); !ok || action != "file.read" {
		t.Fatalf("read mapped to %q, %v", action, ok)
	}
}
