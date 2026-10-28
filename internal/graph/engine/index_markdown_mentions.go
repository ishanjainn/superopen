package engine

import (
	"regexp"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

var markdownCodeSpan = regexp.MustCompile("`([^`\\n]+)`")

func indexMarkdownMentions(files []ParsedSyntaxFile, graph *goGraph) {
	byPath := make(map[string]api.Node)
	sections := documentSections(graph)
	for _, node := range graph.nodes {
		switch node.Label {
		case "File", "Folder":
			byPath[node.Location.File] = node
		}
	}
	for _, parsed := range files {
		if parsed.File.Language != "markdown" || len(parsed.Body) == 0 {
			continue
		}
		lines := strings.Split(string(parsed.Body), "\n")
		activeSections := append([]api.Node(nil), sections[parsed.File.Path]...)
		for i := range activeSections {
			activeSections[i].Location.EndLine = len(lines)
			if i+1 < len(activeSections) {
				activeSections[i].Location.EndLine = activeSections[i+1].Location.StartLine - 1
			}
		}
		fence := ""
		comment := false
		for index, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.Contains(trimmed, "<!--") {
				comment = true
			}
			if comment {
				if strings.Contains(trimmed, "-->") {
					comment = false
				}
				continue
			}
			if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
				marker := trimmed[:3]
				if fence == "" {
					fence = marker
				} else if fence == marker {
					fence = ""
				}
				continue
			}
			if fence != "" {
				continue
			}
			for _, match := range markdownCodeSpan.FindAllStringSubmatch(line, -1) {
				if target, ok := rstPathTarget(parsed.File.Path, strings.TrimSpace(match[1]), byPath); ok {
					documentMention(graph, activeSections, parsed.File.Path, index+1, target, "markdown", "code_path")
				}
			}
		}
	}
}
