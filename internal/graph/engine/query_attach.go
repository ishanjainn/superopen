package engine

import (
	"context"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// queryMaxBodies bounds attached source. The ranked rows already name the
// rest, and a body costs more than a row; snippet fetches any other one.
const queryMaxBodies = 3

func queryAttachableLabel(label string) bool {
	switch label {
	case "Method", "Function", "Constructor", "Variable":
		return true
	default:
		return false
	}
}

func queryAttachableNode(n api.Node) bool {
	return queryAttachableLabel(n.Label) && !skipDataLanguageVariable(n)
}

func queryNodeLooksLikeAuxiliary(n api.Node) bool {
	return codeSearchFileRank(n.Location.File) != 0
}

// queryBodyCandidates reserves the first body for the best implementation
// hit, then shows implementation symbols joined by a behavioral edge. Tests
// and isolated symbols use any remaining slots.
func queryBodyCandidates(rows []api.Node, edges []api.Edge, terms []string) []api.Node {
	if len(rows) > 0 && rows[0].Label == "Route" {
		out := []api.Node{rows[0]}
		for _, n := range rows[1:] {
			if !queryAttachableNode(n) {
				continue
			}
			for _, edge := range edges {
				if edge.Type == "HANDLES" && edge.SourceID == n.ID && edge.TargetID == rows[0].ID {
					return append(out, n)
				}
			}
		}
		return out
	}
	byID := make(map[int64]bool, len(rows))
	for _, n := range rows {
		byID[n.ID] = true
	}
	eligible := func(n api.Node) bool {
		if !queryAttachableNode(n) {
			return false
		}
		span := n.Location.EndLine - n.Location.StartLine + 1
		if n.Location.StartLine > 0 && span > 0 && span <= 2 {
			if n.Label == "Variable" {
				return false // the row itself already shows a one-line value
			}
			for _, e := range edges {
				if e.SourceID == n.ID && e.Type == "CALLS" && byID[e.TargetID] {
					return false // show the wrapper's callee body instead
				}
			}
		}
		return true
	}
	sharedUse := func(left, right int64) bool {
		for _, a := range edges {
			if a.SourceID != left || a.Type != "USAGE" {
				continue
			}
			for _, b := range edges {
				if b.SourceID == right && b.TargetID == a.TargetID && b.Type == "USAGE" {
					return true
				}
			}
		}
		return false
	}
	seen := map[int64]bool{}
	var out []api.Node
	add := func(n api.Node) {
		if eligible(n) && !seen[n.ID] {
			seen[n.ID] = true
			out = append(out, n)
		}
	}
	for _, n := range rows {
		if !queryNodeLooksLikeAuxiliary(n) && eligible(n) {
			add(n)
			break
		}
	}
	if len(out) > 0 {
		primary := out[0]
		bestScore := -1
		var best api.Node
		for i, n := range rows {
			if seen[n.ID] || queryNodeLooksLikeAuxiliary(n) || !eligible(n) {
				continue
			}
			score := -1
			for _, e := range edges {
				if e.SourceID == primary.ID && e.TargetID == n.ID || e.TargetID == primary.ID && e.SourceID == n.ID {
					score = 40
					if e.Type == "CALLS" {
						score = 100
					}
				}
			}
			if score < 0 && sharedUse(primary.ID, n.ID) {
				score = 60
			}
			if score < 0 {
				continue
			}
			if n.Location.File != primary.Location.File {
				score += 20
			}
			score += len(rows) - i
			if score > bestScore {
				best, bestScore = n, score
			}
		}
		if best.ID != 0 {
			add(best)
		} else if len(terms) > 0 {
			// When no direct edge survives the page, a method named for the
			// question's first concept is a better second body than another
			// broad parser or entry point.
			bestScore = 0
			for _, n := range rows {
				if seen[n.ID] || queryNodeLooksLikeAuxiliary(n) || !eligible(n) {
					continue
				}
				if score := queryNameOverlap(n, terms[:1]); score > bestScore {
					best, bestScore = n, score
				}
			}
			if best.ID != 0 {
				add(best)
			}
		}
	}
	if len(out) > 0 {
		bestScore := -1
		var best api.Node
		for i, n := range rows {
			if seen[n.ID] || queryNodeLooksLikeAuxiliary(n) || !eligible(n) {
				continue
			}
			// The answer row order already combines lexical and structural
			// evidence. Keep a broad name match late in the page from taking
			// a body slot from an earlier implementation.
			score := (len(rows) - i) * 25
			newFile := true
			for _, chosen := range out {
				if chosen.Location.File == n.Location.File {
					newFile = false
				}
				if chosen.Name != "" && chosen.Name == n.Name {
					score += 80
				}
			}
			if newFile {
				score += 50
			}
			score += queryNameOverlap(n, terms) * 2
			for _, e := range edges {
				if (e.SourceID == n.ID || e.TargetID == n.ID) && byID[e.SourceID] && byID[e.TargetID] {
					score += 30
					break
				}
			}
			for _, e := range edges {
				for _, chosen := range out {
					if e.SourceID == chosen.ID && e.TargetID == n.ID || e.TargetID == chosen.ID && e.SourceID == n.ID {
						score += 80
					}
				}
			}
			for _, chosen := range out {
				if sharedUse(chosen.ID, n.ID) {
					score += 80
				}
			}
			if score > bestScore {
				best, bestScore = n, score
			}
		}
		if best.ID != 0 {
			add(best)
		}
	}
	for _, n := range rows {
		if !queryNodeLooksLikeAuxiliary(n) {
			add(n)
		}
	}
	for _, n := range rows {
		add(n)
	}
	// A single short wrapper or one-line variable may still be the answer.
	// Keep it available after the larger implementations have used the budget.
	for _, n := range rows {
		if queryAttachableNode(n) && !seen[n.ID] {
			seen[n.ID] = true
			out = append(out, n)
		}
	}
	return out
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
func (s *Store) collectQueryBodies(ctx context.Context, project string, nodes []api.Node) []api.QueryBody {
	if len(nodes) == 0 {
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
