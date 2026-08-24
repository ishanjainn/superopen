package trace

import "testing"

func TestDedupSpansDropsRequestedWhenCallIDMatches(t *testing.T) {
	spans := []Span{
		{Name: "coding_agent.tool.requested", Attributes: map[string]string{"gen_ai.tool.call.id": "c1", "gen_ai.tool.name": "Grep"}},
		{Name: "coding_agent.tool.call", Attributes: map[string]string{"gen_ai.tool.call.id": "c1", "gen_ai.tool.name": "Grep"}},
		{Name: "coding_agent.session.loop.stop"},
		{Name: "coding_agent.llm.turn"},
	}
	got := DedupSpans(spans)
	if len(got) != 3 {
		t.Fatalf("len=%d want 3 (requested dropped, loop+llm kept): %#v", len(got), names(got))
	}
	if got[0].Name != "coding_agent.tool.call" {
		t.Fatalf("first=%q want tool.call", got[0].Name)
	}
}

func TestDedupSpansKeepsRequestedWithoutCallID(t *testing.T) {
	spans := []Span{
		{Name: "coding_agent.shell.requested", Attributes: map[string]string{"coding_agent.tool.command": "so graph query"}},
		{Name: "coding_agent.tool.call", Attributes: map[string]string{"gen_ai.tool.name": "shell"}},
	}
	got := DedupSpans(spans)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2 (no shared tool.call.id)", len(got))
	}
}

func names(spans []Span) []string {
	out := make([]string, len(spans))
	for i, sp := range spans {
		out[i] = sp.Name
	}
	return out
}
