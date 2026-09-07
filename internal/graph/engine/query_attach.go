package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func queryAttachableLabel(label string) bool {
	switch label {
	case "Method", "Function", "Constructor":
		return true
	default:
		return false
	}
}

func querySymbolNames(n api.Node) (name, last string) {
	name = strings.ToLower(strings.TrimSpace(n.Name))
	last = name
	if qn := strings.TrimSpace(n.QualifiedName); qn != "" {
		if i := strings.LastIndex(qn, "."); i >= 0 {
			last = strings.ToLower(qn[i+1:])
		} else {
			last = strings.ToLower(qn)
		}
	}
	return name, last
}

func pickQueryAttachNodes(ordered []queryNodeHit) []api.Node {
	out := make([]api.Node, 0, len(ordered))
	for _, hit := range ordered {
		if queryAttachableLabel(hit.node.Label) {
			out = append(out, hit.node)
		}
	}
	return out
}

func fileRoundRobinNodes(ordered []queryNodeHit) []queryNodeHit {
	if len(ordered) < 2 {
		return ordered
	}
	lockFile := ordered[0].node.Location.File
	type group struct {
		file  string
		nodes []queryNodeHit
	}
	var groups []group
	index := map[string]int{}
	for _, hit := range ordered {
		file := hit.node.Location.File
		if i, ok := index[file]; ok {
			groups[i].nodes = append(groups[i].nodes, hit)
			continue
		}
		index[file] = len(groups)
		groups = append(groups, group{file: file, nodes: []queryNodeHit{hit}})
	}
	if lockFile != "" {
		if i, ok := index[lockFile]; ok && i != 0 {
			groups[0], groups[i] = groups[i], groups[0]
		}
	}
	out := make([]queryNodeHit, 0, len(ordered))
	if len(groups) > 0 {
		out = append(out, groups[0].nodes...)
		groups = groups[1:]
	}
	max := 0
	for _, g := range groups {
		if len(g.nodes) > max {
			max = len(g.nodes)
		}
	}
	for depth := 0; depth < max; depth++ {
		for _, g := range groups {
			if depth < len(g.nodes) {
				out = append(out, g.nodes[depth])
			}
		}
	}
	return out
}

// appendQueryBodies reads clipped source for listed callables until leftover
// chars are exhausted. Fail-open per symbol. Not Class/Module/File dumps.
func (s *Store) appendQueryBodies(ctx context.Context, project string, nodes []api.Node, leftover int) string {
	if len(nodes) == 0 || leftover <= 0 {
		return ""
	}
	type body struct {
		title string
		code  string
	}
	var parts []body
	used := 0
	headerBudget := 80
	for _, node := range nodes {
		qn := strings.TrimSpace(node.QualifiedName)
		if qn == "" {
			qn = strings.TrimSpace(node.Name)
		}
		if qn == "" {
			continue
		}
		got, err := s.Snippet(ctx, api.SnippetRequest{Project: project, QualifiedName: qn})
		if err != nil || got.Status == "ambiguous" || strings.TrimSpace(got.Code) == "" {
			continue
		}
		display := node
		if strings.TrimSpace(got.Name) != "" {
			display = api.Node{Label: got.Label, Name: got.Name, QualifiedName: got.QualifiedName, Location: got.Location}
		}
		title := fmt.Sprintf("%s [%s src=%s loc=%s]", queryNodeDisplayName(display), display.Label, got.Location.File, queryNodeLoc(display))
		block := "\n--- " + title + " ---\n" + got.Code + "\n"
		if used == 0 {
			if leftover < headerBudget+len(block) {
				break
			}
		} else if used+len(block) > leftover {
			break
		}
		parts = append(parts, body{title: title, code: got.Code})
		if used == 0 {
			used += headerBudget
		}
		used += len(block)
	}
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\nBODIES: %d matching callables (80-line cap). If this answers, stop. Else snippet another listed qn.\n", len(parts))
	for _, p := range parts {
		fmt.Fprintf(&b, "\n--- %s ---\n%s\n", p.title, p.code)
	}
	return b.String()
}
