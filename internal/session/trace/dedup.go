package trace

import "strings"

// DedupSpans drops duplicate thoughts, extra read_file when a Read of the
// same path exists, and requested+completed pairs that share gen_ai.tool.call.id.
// Storage-only; adapters still emit. Spans without a tool.call.id are kept.
func DedupSpans(spans []Span) []Span {
	if len(spans) < 2 {
		return spans
	}
	readPaths := map[string]bool{}
	for _, sp := range spans {
		tool := strings.ToLower(strings.TrimSpace(attr(sp, "gen_ai.tool.name")))
		path := filePathAttr(sp)
		if tool == "read" || strings.EqualFold(sp.Name, "coding_agent.read") {
			if path != "" {
				readPaths[path] = true
			}
		}
	}
	seenThought := map[string]bool{}
	out := make([]Span, 0, len(spans))
	for _, sp := range spans {
		name := strings.ToLower(sp.Name)
		if strings.Contains(name, "thought") {
			text := attr(sp, "coding_agent.llm.thought.text")
			if text != "" {
				if seenThought[text] {
					continue
				}
				seenThought[text] = true
			}
		}
		tool := strings.ToLower(strings.TrimSpace(attr(sp, "gen_ai.tool.name")))
		path := filePathAttr(sp)
		if (tool == "read_file" || strings.Contains(name, "read_file")) && path != "" && readPaths[path] {
			continue
		}
		out = append(out, sp)
	}
	return dropRequestedWhenCompleted(out)
}

func dropRequestedWhenCompleted(spans []Span) []Span {
	hasCompleted := map[string]bool{}
	for _, sp := range spans {
		id := attr(sp, "gen_ai.tool.call.id")
		if id == "" || isRequestedName(sp.Name) {
			continue
		}
		hasCompleted[id] = true
	}
	if len(hasCompleted) == 0 {
		return spans
	}
	out := make([]Span, 0, len(spans))
	for _, sp := range spans {
		id := attr(sp, "gen_ai.tool.call.id")
		if id != "" && isRequestedName(sp.Name) && hasCompleted[id] {
			continue
		}
		out = append(out, sp)
	}
	return out
}

func isRequestedName(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "requested")
}

func attr(sp Span, key string) string {
	if sp.Attributes == nil {
		return ""
	}
	return strings.TrimSpace(sp.Attributes[key])
}

func filePathAttr(sp Span) string {
	if p := attr(sp, "coding_agent.file_path"); p != "" {
		return p
	}
	return attr(sp, "code.file.path")
}
