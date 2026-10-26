package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// queryMaxBodies bounds attached source. The ranked rows already name the
// rest, and a body costs more than a row; snippet fetches any other one.
const queryMaxBodies = 3

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
	return renderQueryBodies(s.collectQueryBodies(ctx, project, nodes, leftover))
}

// queryBodyLineBudget is the source lines a page attaches across all bodies.
// Bodies follow the ranking and a long one is clipped to what is left, never
// skipped: skipping the second-ranked, 109-line method for a short one
// further down sent the agent to Read the file the page had just located.
const queryBodyLineBudget = 120

// queryMinBodyLines is the least worth attaching; below it the page names the
// symbol and snippet reads it.
const queryMinBodyLines = 10

// collectQueryBodies is appendQueryBodies as data: bodies of the listed
// callables in listed order, clipped to the line budget left.
func (s *Store) collectQueryBodies(ctx context.Context, project string, nodes []api.Node, leftover int) []api.QueryBody {
	if len(nodes) == 0 || leftover <= 0 {
		return nil
	}
	var parts []api.QueryBody
	remaining := queryBodyLineBudget
	for _, node := range nodes {
		if len(parts) >= queryMaxBodies || remaining < queryMinBodyLines {
			break
		}
		qn := strings.TrimSpace(node.QualifiedName)
		if qn == "" {
			qn = strings.TrimSpace(node.Name)
		}
		if qn == "" {
			continue
		}
		span := remaining
		if span > queryBodyMaxLines {
			span = queryBodyMaxLines
		}
		got, err := s.Snippet(ctx, api.SnippetRequest{Project: project, QualifiedName: qn, MaxLines: span})
		if err != nil || got.Status == "ambiguous" || strings.TrimSpace(got.Code) == "" {
			continue
		}
		display := node
		if strings.TrimSpace(got.Name) != "" {
			display = api.Node{Label: got.Label, Name: got.Name, QualifiedName: got.QualifiedName, Location: got.Location}
		}
		total := got.Location.EndLine - got.Location.StartLine + 1
		if got.OmittedEnd > got.Location.EndLine {
			total = got.OmittedEnd - got.Location.StartLine + 1
		}
		bodyQN := got.QualifiedName
		if bodyQN == "" {
			bodyQN = qn
		}
		parts = append(parts, api.QueryBody{
			QualifiedName: bodyQN, Name: display.Name, Label: display.Label, File: got.Location.File,
			StartLine: got.Location.StartLine, EndLine: got.Location.EndLine,
			TotalLines: total, Code: got.Code, Clipped: got.Clipped,
		})
		remaining -= got.Location.EndLine - got.Location.StartLine + 1
	}
	return parts
}

func queryBodyTitle(display api.Node, file string) string {
	return fmt.Sprintf("%s [%s src=%s loc=%s]", queryNodeDisplayName(display), display.Label, file, queryNodeLoc(display))
}

func renderQueryBodies(parts []api.QueryBody) string {
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\nBODIES: %d matching callables (80-line cap). If this answers, stop. Else snippet another listed qn.\n", len(parts))
	for _, p := range parts {
		display := api.Node{Label: p.Label, Name: p.Name, QualifiedName: p.QualifiedName,
			Location: api.Location{File: p.File, StartLine: p.StartLine, EndLine: p.EndLine}}
		fmt.Fprintf(&b, "\n--- %s ---\n%s\n", queryBodyTitle(display, p.File), p.Code)
	}
	return b.String()
}
