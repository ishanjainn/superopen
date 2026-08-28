package engine

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

const (
	queryAttachBodyMax         = 2
	queryAttachContainsMinTerm = 8
)

func queryAttachableLabel(label string) bool {
	switch label {
	case "Method", "Function", "Constructor":
		return true
	default:
		return false
	}
}

func queryShouldAttachBody(n api.Node, terms []string) bool {
	if !queryAttachableLabel(n.Label) {
		return false
	}
	if queryNameOverlap(n, terms) >= 10 {
		return true
	}
	name, last := querySymbolNames(n)
	for _, t := range terms {
		tl := strings.ToLower(strings.TrimSpace(t))
		if len(tl) < queryAttachContainsMinTerm {
			continue
		}
		if strings.Contains(name, tl) || strings.Contains(last, tl) {
			return true
		}
	}
	return false
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

func pickQueryAttachNodes(ordered []queryNodeHit, terms []string, limit int) []api.Node {
	if limit <= 0 || len(ordered) == 0 {
		return nil
	}
	type scored struct {
		node  api.Node
		score int
		idx   int
	}
	var cand []scored
	for i, hit := range ordered {
		if !queryShouldAttachBody(hit.node, terms) {
			continue
		}
		cand = append(cand, scored{node: hit.node, score: queryNameOverlap(hit.node, terms), idx: i})
	}
	sort.SliceStable(cand, func(i, j int) bool {
		if cand[i].score != cand[j].score {
			return cand[i].score > cand[j].score
		}
		return cand[i].idx < cand[j].idx
	})
	if len(cand) > limit {
		cand = cand[:limit]
	}
	out := make([]api.Node, 0, len(cand))
	for _, c := range cand {
		out = append(out, c.node)
	}
	return out
}

// appendQueryBodies reads clipped source for name-matching listed callables.
// Fail-open: a missing file or snippet error skips that symbol so query
// locators still return. Bodies are not Class/Module/File dumps.
func (s *Store) appendQueryBodies(ctx context.Context, project string, nodes []api.Node) string {
	if len(nodes) == 0 {
		return ""
	}
	type body struct {
		title string
		code  string
	}
	var parts []body
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
		parts = append(parts, body{
			title: fmt.Sprintf("%s [%s src=%s loc=%s]", queryNodeDisplayName(display), display.Label, got.Location.File, queryNodeLoc(display)),
			code:  got.Code,
		})
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
