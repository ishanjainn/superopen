// Package format renders compact agent-facing text views of graph API results.
// Full JSON remains available via CLI --json; these helpers never mutate the graph.
package format

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func SearchCompact(result api.SearchResult) string {
	matches := result.Matches
	if len(matches) == 0 && len(result.Semantic) > 0 {
		matches = result.Semantic
	}
	total := result.Page.Total
	if total == 0 {
		total = len(matches)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "total: %d\n", total)
	mode := "bm25"
	if len(result.Matches) == 0 && len(result.Semantic) > 0 {
		mode = "semantic"
	}
	fmt.Fprintf(&b, "search_mode: %s\n", mode)
	fmt.Fprintf(&b, "results: %d  (cols: qn label file lines rank)\n", len(matches))
	for _, match := range matches {
		lines := lineRange(match.Location.StartLine, match.Location.EndLine)
		fmt.Fprintf(&b, "  %s %s %s %s %.4g\n",
			match.QualifiedName, match.Label, match.Location.File, lines, match.Score)
	}
	if result.Page.Truncated || result.Budget.Truncated {
		b.WriteString("truncated: true\n")
	}
	if result.Page.NextCursor != "" {
		fmt.Fprintf(&b, "has_more: true\nnext_cursor: %s\n", result.Page.NextCursor)
	}
	return b.String()
}

func TraceCompact(result api.TraceResult) string {
	if result.Status == "ambiguous" {
		var b strings.Builder
		b.WriteString("status: ambiguous\n")
		if result.Message != "" {
			fmt.Fprintf(&b, "message: %s\n", result.Message)
		}
		fmt.Fprintf(&b, "suggestions: %d  (cols: qn label file lines)\n", len(result.Suggestions))
		for _, node := range result.Suggestions {
			fmt.Fprintf(&b, "  %s %s %s %s\n",
				node.QualifiedName, node.Label, node.Location.File,
				lineRange(node.Location.StartLine, node.Location.EndLine))
		}
		return b.String()
	}

	start := ""
	direction := normalizeTraceDirection(result.Direction)
	type hopHit struct {
		name string
		qn   string
		hop  int
		rel  string
	}
	neighbors := map[string]hopHit{}
	for _, path := range result.Paths {
		if len(path) == 0 {
			continue
		}
		if start == "" {
			start = path[0].Node.QualifiedName
		}
		for _, step := range path {
			if step.Hop == 0 {
				continue
			}
			via := ""
			if step.Via != nil {
				via = step.Via.Type
			}
			if via != "" && via != "CALLS" && via != "USAGE" && via != "CONFIGURES" {
				continue
			}
			if skipCompactVariable(step.Node) {
				continue
			}
			qn := step.Node.QualifiedName
			prev, ok := neighbors[qn]
			if !ok || step.Hop < prev.hop {
				rel := via
				if rel == "" {
					rel = "CALLS"
				}
				neighbors[qn] = hopHit{name: step.Node.Name, qn: qn, hop: step.Hop, rel: rel}
			}
		}
	}

	groups := map[string][]hopHit{}
	var groupOrder []string
	for _, hit := range neighbors {
		prefix := PackagePrefix(hit.qn)
		if _, ok := groups[prefix]; !ok {
			groupOrder = append(groupOrder, prefix)
		}
		groups[prefix] = append(groups[prefix], hit)
	}
	sort.Strings(groupOrder)
	for _, prefix := range groupOrder {
		sort.Slice(groups[prefix], func(i, j int) bool {
			if groups[prefix][i].hop != groups[prefix][j].hop {
				return groups[prefix][i].hop < groups[prefix][j].hop
			}
			return groups[prefix][i].name < groups[prefix][j].name
		})
	}

	group, total := traceNeighborLabels(direction)
	var b strings.Builder
	fmt.Fprintf(&b, "function: %s\n", start)
	fmt.Fprintf(&b, "direction: %s\n", direction)
	fmt.Fprintf(&b, "%s: %d\n", total, len(neighbors))
	fmt.Fprintf(&b, "unresolved_calls: %d\n", result.UnresolvedCalls)
	if result.Truncated {
		b.WriteString("truncated: true\n")
	}
	fmt.Fprintf(&b, "%s: %d  (rows: name rel hop; qn = group prefix + \".\" + name)\n", group, len(neighbors))
	for _, prefix := range groupOrder {
		fmt.Fprintf(&b, "%s:\n", prefix)
		for _, hit := range groups[prefix] {
			fmt.Fprintf(&b, "  %s %s %d\n", hit.name, strings.ToLower(hit.rel), hit.hop)
		}
	}
	return b.String()
}

func normalizeTraceDirection(direction string) string {
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "incoming", "inbound":
		return "incoming"
	case "both":
		return "both"
	default:
		return "outgoing"
	}
}

func traceNeighborLabels(direction string) (group, total string) {
	switch direction {
	case "incoming":
		return "callers", "callers_total"
	case "both":
		return "neighbors", "neighbors_total"
	default:
		return "callees", "callees_total"
	}
}

func skipCompactVariable(node api.Node) bool {
	if node.Label != "Variable" {
		return false
	}
	switch strings.ToLower(filepathExt(node.Location.File)) {
	case ".json", ".json5", ".yaml", ".yml", ".toml", ".ini", ".hcl", ".properties":
		return true
	default:
		return false
	}
}

func filepathExt(path string) string {
	path = strings.TrimSpace(path)
	i := strings.LastIndex(path, ".")
	if i < 0 {
		return ""
	}
	return path[i:]
}

func SnippetCompact(result api.SnippetResult) string {
	if result.Status == "ambiguous" {
		var b strings.Builder
		b.WriteString("status: ambiguous\n")
		if result.Message != "" {
			fmt.Fprintf(&b, "message: %s\n", result.Message)
		}
		fmt.Fprintf(&b, "suggestions: %d  (cols: qn label file lines)\n", len(result.Suggestions))
		for _, node := range result.Suggestions {
			fmt.Fprintf(&b, "  %s %s %s %s\n",
				node.QualifiedName, node.Label, node.Location.File,
				lineRange(node.Location.StartLine, node.Location.EndLine))
		}
		return b.String()
	}
	var b strings.Builder
	if result.QualifiedName != "" {
		fmt.Fprintf(&b, "qualified_name: %s\n", result.QualifiedName)
	}
	if result.Name != "" {
		fmt.Fprintf(&b, "name: %s\n", result.Name)
	}
	if result.Label != "" {
		fmt.Fprintf(&b, "label: %s\n", result.Label)
	}
	fmt.Fprintf(&b, "file: %s\n", result.Location.File)
	fmt.Fprintf(&b, "lines: %s\n", lineRange(result.Location.StartLine, result.Location.EndLine))
	fmt.Fprintf(&b, "callers: %d\n", result.Callers)
	fmt.Fprintf(&b, "callees: %d\n", result.Callees)
	if result.Clipped {
		b.WriteString("clipped: true\n")
	}
	if len(result.Suggestions) > 0 && result.Status != "ambiguous" {
		fmt.Fprintf(&b, "also: %d  (cols: qn label file)\n", len(result.Suggestions))
		for _, node := range result.Suggestions {
			fmt.Fprintf(&b, "  %s %s %s\n", node.QualifiedName, node.Label, node.Location.File)
		}
	}
	b.WriteString("source:\n")
	b.WriteString(result.Code)
	if !strings.HasSuffix(result.Code, "\n") {
		b.WriteByte('\n')
	}
	return b.String()
}

func ArchitectureCompact(result api.ArchitectureResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "total_nodes: %d\n", result.TotalNodes)
	fmt.Fprintf(&b, "total_edges: %d\n", result.TotalEdges)
	if result.Summary != "" {
		fmt.Fprintf(&b, "summary: %s\n", result.Summary)
	}
	if len(result.Languages) > 0 {
		b.WriteString("languages:\n")
		for _, lang := range result.Languages {
			fmt.Fprintf(&b, "  %s %d\n", lang.Language, lang.FileCount)
		}
	}
	if len(result.Packages) > 0 {
		b.WriteString("packages:\n")
		limit := len(result.Packages)
		if limit > 20 {
			limit = 20
		}
		for _, pkg := range result.Packages[:limit] {
			fmt.Fprintf(&b, "  %s nodes=%d fan_in=%d fan_out=%d\n", pkg.Name, pkg.NodeCount, pkg.FanIn, pkg.FanOut)
		}
	}
	if len(result.Communities) > 0 {
		b.WriteString("clusters:\n")
		for _, community := range result.Communities {
			fmt.Fprintf(&b, "  id=%d label=%s members=%d cohesion=%.3f\n",
				community.ID, community.Name, community.Members, community.Cohesion)
		}
	} else {
		b.WriteString("aspects_hint: pass aspects=[\"clusters\"] for Leiden clusters\n")
	}
	return b.String()
}

// QueryAgentJSON is the default --json graph query payload: compact text plus
// slim seeds. Full Node/Edge structs are omitted (default query output is text-only;
// use --full for the complete QueryResult).
func QueryAgentJSON(result api.QueryResult) map[string]any {
	seeds := make([]map[string]any, 0, len(result.Seeds))
	for _, seed := range result.Seeds {
		seeds = append(seeds, map[string]any{
			"qualified_name": seed.QualifiedName,
			"name":           seed.Name,
			"file":           seed.Location.File,
			"lines":          lineRange(seed.Location.StartLine, seed.Location.EndLine),
			"score":          seed.Score,
		})
	}
	return map[string]any{
		"text":      result.Text,
		"budget":    result.Budget,
		"seeds":     seeds,
		"truncated": result.Budget.Truncated,
	}
}

func lineRange(start, end int) string {
	if start <= 0 {
		return "-"
	}
	if end <= 0 || end == start {
		return fmt.Sprintf("%d", start)
	}
	return fmt.Sprintf("%d-%d", start, end)
}

func PackagePrefix(qn string) string {
	if i := strings.LastIndex(qn, "."); i > 0 {
		return qn[:i]
	}
	return qn
}
