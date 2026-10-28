// Package format renders compact agent-facing text views of graph API results.
// Every view is TOON (see toon.go). Full JSON remains available via CLI --json;
// these helpers never mutate the graph.
package format

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// QueryCompact renders one page of a graph query answer: ranked nodes, the
// edges among them, and attached bodies. An empty answer says so.
func QueryCompact(result api.QueryResult) string {
	var b strings.Builder
	if len(result.CallersOf) > 0 {
		List(&b, "", "callers_of", result.CallersOf)
	}
	if len(result.Rows) == 0 {
		if result.Offset > 0 && result.RowsTotal > 0 {
			fmt.Fprintf(&b, "count: 0 of %d\n", result.RowsTotal)
			fmt.Fprintf(&b, "nodes: none past offset %d\n", result.Offset)
			return b.String()
		}
		b.WriteString("count: 0\n")
		if len(result.CallersOf) > 0 {
			b.WriteString("callers: nothing in the graph calls these symbols\n")
		} else {
			b.WriteString("nodes: no indexed symbol matches this question\n")
		}
		return b.String()
	}
	Count(&b, len(result.Rows), result.RowsTotal)
	if result.Offset > 0 {
		Field(&b, "", "offset", result.Offset)
	}
	name := "nodes"
	if len(result.CallersOf) > 0 {
		name = "callers"
	}
	Table(&b, "", name, []string{"qn", "kind", "file", "lines"}, nodeRows(result.Rows))
	if edges := queryEdgeRows(result.Rows, result.Edges); len(edges) > 0 {
		Table(&b, "", "edges", []string{"from", "rel", "to"}, edges)
	}
	if len(result.Linked) > 0 {
		rows := make([][]string, 0, len(result.Linked))
		for _, link := range result.Linked {
			rows = append(rows, []string{shortQN(link.From), link.Rel, link.To, link.File + ":" + lineRange(link.Start, link.End)})
		}
		Table(&b, "", "linked", []string{"from", "rel", "qn", "location"}, rows)
	}
	if len(result.Bodies) > 0 {
		fmt.Fprintf(&b, "bodies[%d]:\n", len(result.Bodies))
		for _, body := range result.Bodies {
			Field(&b, "  - ", "qn", body.QualifiedName)
			Field(&b, "    ", "file", body.File)
			Code(&b, "    ", body.StartLine, body.TotalLines, body.Code)
		}
	}
	return b.String()
}

func nodeRows(nodes []api.Node) [][]string {
	rows := make([][]string, 0, len(nodes))
	for _, n := range nodes {
		qn := n.QualifiedName
		if qn == "" {
			qn = n.Name
		}
		rows = append(rows, []string{qn, n.Label, n.Location.File, lineRange(n.Location.StartLine, n.Location.EndLine)})
	}
	return rows
}

// queryEdgeRows keeps behavioral edges whose ends are both listed. Structural
// DEFINES/CONTAINS edges restate the qn prefix and are dropped.
func queryEdgeRows(nodes []api.Node, edges []api.Edge) [][]string {
	byID := map[int64]api.Node{}
	for _, n := range nodes {
		if n.ID != 0 {
			byID[n.ID] = n
		}
	}
	seen := map[string]bool{}
	var rows [][]string
	for _, e := range edges {
		switch e.Type {
		case "CALLS", "USAGE", "INHERITS", "IMPLEMENTS", "WRITES", "READS", "CONFIGURES", "OVERRIDE", "OVERRIDES":
		default:
			continue
		}
		from, ok1 := byID[e.SourceID]
		to, ok2 := byID[e.TargetID]
		if !ok1 || !ok2 || e.SourceID == e.TargetID {
			continue
		}
		row := []string{shortQN(from.QualifiedName), e.Type, shortQN(to.QualifiedName)}
		key := strings.Join(row, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		rows = append(rows, row)
	}
	return rows
}

// shortQN keeps the last two segments (Class.method) for edge rows; the full
// qn is in the nodes table above.
func shortQN(qn string) string {
	parts := strings.Split(qn, ".")
	if len(parts) <= 2 {
		return qn
	}
	return strings.Join(parts[len(parts)-2:], ".")
}

func SearchCompact(result api.SearchResult, query string) string {
	matches := result.Matches
	if len(matches) == 0 && len(result.Semantic) > 0 {
		matches = result.Semantic
	}
	total := result.Page.Total
	var b strings.Builder
	if len(matches) == 0 {
		b.WriteString("count: 0\n")
		fmt.Fprintf(&b, "results: no indexed symbol name matches %s\n", Quote(query))
		return b.String()
	}
	Count(&b, len(matches), total)
	nodes := make([]api.Node, 0, len(matches))
	for _, m := range matches {
		nodes = append(nodes, m.Node)
	}
	Table(&b, "", "results", []string{"qn", "kind", "file", "lines"}, nodeRows(nodes))
	return b.String()
}

func TraceCompact(result api.TraceResult) string {
	if result.Status == "ambiguous" {
		return ambiguousCompact(result.Message, result.Suggestions)
	}

	start := result.Start
	direction := normalizeTraceDirection(result.Direction)
	type hopHit struct {
		qn  string
		hop int
		rel string
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
			if via != "" && via != "CALLS" && via != "USAGE" && via != "CONFIGURES" && via != "HANDLES" && via != "HTTP_CALLS" && via != "REDIRECTS_TO" {
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
				neighbors[qn] = hopHit{qn: qn, hop: step.Hop, rel: rel}
			}
		}
	}
	hits := make([]hopHit, 0, len(neighbors))
	for _, hit := range neighbors {
		hits = append(hits, hit)
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].hop != hits[j].hop {
			return hits[i].hop < hits[j].hop
		}
		return hits[i].qn < hits[j].qn
	})

	group := traceNeighborLabel(direction)
	var b strings.Builder
	Field(&b, "", "function", start)
	Field(&b, "", "direction", direction)
	if len(hits) == 0 {
		b.WriteString("count: 0\n")
		fmt.Fprintf(&b, "%s: none in the graph\n", group)
	} else {
		Count(&b, len(hits), len(hits))
		rows := make([][]string, 0, len(hits))
		for _, hit := range hits {
			rows = append(rows, []string{hit.qn, strings.ToLower(hit.rel), fmt.Sprint(hit.hop)})
		}
		Table(&b, "", group, []string{"qn", "rel", "hop"}, rows)
	}
	if len(result.External) > 0 {
		List(&b, "", "external", result.External)
	}
	if result.Tests > 0 {
		Field(&b, "", "tests", fmt.Sprintf("%d test symbols reached, not listed", result.Tests))
	}
	if result.Uncertain > 0 {
		Field(&b, "", "uncertain", fmt.Sprintf("%d symbols reached only by guessed edges, not walked", result.Uncertain))
	}
	if result.UnresolvedCalls > 0 {
		Field(&b, "", "unresolved_calls", result.UnresolvedCalls)
	}
	if result.Truncated {
		Field(&b, "", "truncated", true)
	}
	return b.String()
}

// ambiguousCompact lists the candidates for a short name. The engine's
// prose message is dropped: the table and the help line say the same.
func ambiguousCompact(_ string, suggestions []api.Node) string {
	var b strings.Builder
	Field(&b, "", "status", "ambiguous; several symbols share this name")
	Table(&b, "", "candidates", []string{"qn", "kind", "file", "lines"}, nodeRows(suggestions))
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

func traceNeighborLabel(direction string) string {
	switch direction {
	case "incoming":
		return "callers"
	case "both":
		return "neighbors"
	default:
		return "callees"
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
		return ambiguousCompact(result.Message, result.Suggestions)
	}
	var b strings.Builder
	b.WriteString("symbol:\n")
	if result.QualifiedName != "" {
		Field(&b, "  ", "qn", result.QualifiedName)
	}
	if result.Label != "" {
		Field(&b, "  ", "kind", result.Label)
	}
	Field(&b, "  ", "file", result.Location.File)
	Field(&b, "  ", "lines", lineRange(result.Location.StartLine, result.Location.EndLine))
	Field(&b, "  ", "callers", result.Callers)
	Field(&b, "  ", "callees", result.Callees)
	total := 0
	if result.OmittedStart > 0 && result.OmittedEnd >= result.OmittedStart {
		total = result.OmittedEnd - result.Location.StartLine + 1
	}
	Code(&b, "", result.Location.StartLine, total, result.Code)
	if result.Clipped && len(result.DirectCallees) > 0 {
		rows := make([][]string, 0, len(result.DirectCallees))
		for _, node := range result.DirectCallees {
			rows = append(rows, []string{node.QualifiedName, node.Location.File})
		}
		Table(&b, "", "callees", []string{"qn", "file"}, rows)
	}
	if len(result.Suggestions) > 0 {
		rows := make([][]string, 0, len(result.Suggestions))
		for _, node := range result.Suggestions {
			rows = append(rows, []string{node.QualifiedName, node.Label, node.Location.File})
		}
		Table(&b, "", "also", []string{"qn", "kind", "file"}, rows)
	}
	return b.String()
}

func ArchitectureCompact(result api.ArchitectureResult, full ...bool) string {
	var b strings.Builder
	Field(&b, "", "total_nodes", result.TotalNodes)
	Field(&b, "", "total_edges", result.TotalEdges)
	if result.Summary != "" {
		Field(&b, "", "summary", result.Summary)
	}
	if len(result.Languages) > 0 {
		rows := make([][]string, 0, len(result.Languages))
		for _, lang := range result.Languages {
			rows = append(rows, []string{lang.Language, fmt.Sprint(lang.FileCount)})
		}
		Table(&b, "", "languages", []string{"language", "files"}, rows)
	}
	if len(result.Packages) > 0 {
		limit := len(result.Packages)
		if limit > 20 && (len(full) == 0 || !full[0]) {
			limit = 20
		}
		Count(&b, limit, len(result.Packages))
		rows := make([][]string, 0, limit)
		hasFlow := false
		for _, pkg := range result.Packages[:limit] {
			if pkg.FanIn != 0 || pkg.FanOut != 0 {
				hasFlow = true
				break
			}
		}
		for _, pkg := range result.Packages[:limit] {
			row := []string{pkg.Name, fmt.Sprint(pkg.NodeCount)}
			if hasFlow {
				row = append(row, fmt.Sprint(pkg.FanIn), fmt.Sprint(pkg.FanOut))
			}
			rows = append(rows, row)
		}
		columns := []string{"name", "nodes"}
		if hasFlow {
			columns = append(columns, "fan_in", "fan_out")
		}
		Table(&b, "", "packages", columns, rows)
	}
	if len(result.Communities) > 0 {
		rows := make([][]string, 0, len(result.Communities))
		for _, c := range result.Communities {
			rows = append(rows, []string{fmt.Sprint(c.ID), c.Name, fmt.Sprint(c.Members), fmt.Sprintf("%.3f", c.Cohesion)})
		}
		Table(&b, "", "clusters", []string{"id", "label", "members", "cohesion"}, rows)
	}
	return b.String()
}

func ImpactCompact(result api.ImpactResult) string {
	var b strings.Builder
	if result.Base != "" {
		Field(&b, "", "base", result.Base)
	}
	if result.MergeBase != "" {
		Field(&b, "", "merge_base", result.MergeBase)
	}
	if len(result.ChangedFiles) > 0 {
		List(&b, "", "changed_files", result.ChangedFiles)
	}
	if len(result.ImpactedFiles) == 0 {
		b.WriteString("impacted_files: none; no indexed code depends on these changes\n")
	} else {
		rows := make([][]string, 0, len(result.ImpactedFiles))
		for _, f := range result.ImpactedFiles {
			rows = append(rows, []string{f.Path, fmt.Sprint(f.Symbols), strings.Join(f.Reasons, ";")})
		}
		Table(&b, "", "impacted_files", []string{"path", "symbols", "reasons"}, rows)
	}
	Field(&b, "", "impacted_symbols", result.Total)
	if result.Truncated {
		Field(&b, "", "truncated", true)
	}
	return b.String()
}

// CodeSearchCompact lists matching lines. Text is clipped to one screen line;
// file:line is the handle for snippet or Read.
func CodeSearchCompact(result api.CodeSearchResult, pattern, path string, full ...bool) string {
	showFull := len(full) > 0 && full[0]
	var b strings.Builder
	if len(result.Matches) == 0 && len(result.NameMatches) == 0 {
		b.WriteString("count: 0\n")
		scope := "indexed files"
		if path != "" {
			scope = path
		}
		fmt.Fprintf(&b, "matches: no line in %s contains %s\n", scope, Quote(pattern))
		return b.String()
	}
	total := result.TotalGrepMatches
	Count(&b, len(result.Matches), total)
	if len(result.Shared) > 0 {
		shared := make([][]string, 0, len(result.Shared))
		uses := [][]string{}
		for _, value := range result.Shared {
			shared = append(shared, []string{value.Value, strings.Join(value.Consumers, " | ")})
			for _, use := range value.Uses {
				uses = append(uses, []string{value.Value, use.Consumer, fmt.Sprintf("%s:%d", use.File, use.Line), use.Expr})
			}
		}
		Table(&b, "", "shared", []string{"value", "consumers"}, shared)
		if len(uses) > 0 {
			Table(&b, "", "uses", []string{"value", "consumer", "location", "expr"}, uses)
		}
		Field(&b, "", "test_scope", "inject the issue input at the shared producer and check every consumer; mocking one consumer alias hides the others")
	}
	rows := make([][]string, 0, len(result.Matches))
	hasSymbol := false
	for _, m := range result.Matches {
		if m.Node != nil && m.Node.QualifiedName != "" {
			hasSymbol = true
			break
		}
	}
	for _, m := range result.Matches {
		text := strings.TrimSpace(m.Source)
		text = codeSearchLine(text, showFull)
		row := []string{m.Location.File, fmt.Sprint(m.Location.StartLine)}
		if hasSymbol {
			qn := ""
			if m.Node != nil {
				qn = m.Node.QualifiedName
			}
			row = append(row, qn)
		}
		rows = append(rows, append(row, text))
	}
	columns := []string{"file", "line", "text"}
	if hasSymbol {
		columns = []string{"file", "line", "qn", "text"}
	}
	Table(&b, "", "matches", columns, rows)
	if len(result.NameMatches) > 0 {
		nameRows := make([][]string, 0, len(result.NameMatches))
		for _, m := range result.NameMatches {
			qn := ""
			if m.Node != nil {
				qn = m.Node.QualifiedName
			}
			line := strings.TrimSpace(m.Source)
			line = codeSearchLine(line, showFull)
			nameRows = append(nameRows, []string{m.Location.File, fmt.Sprint(m.Location.StartLine), qn, line})
		}
		Table(&b, "", "name_matches", []string{"file", "line", "qn", "text"}, nameRows)
	}
	if len(result.Related) > 0 {
		links := make([][]string, 0, len(result.Related))
		for _, link := range result.Related {
			links = append(links, []string{link.From, link.Rel, link.To, link.File + ":" + lineRange(link.Start, link.End)})
		}
		Table(&b, "", "related", []string{"from", "rel", "qn", "location"}, links)
	}
	if len(result.Tests) > 0 {
		rows := make([][]string, 0, len(result.Tests))
		for _, test := range result.Tests {
			rows = append(rows, []string{test.QualifiedName, test.Target, test.File, fmt.Sprint(test.Line)})
		}
		Table(&b, "", "tests", []string{"qn", "target", "file", "line"}, rows)
	}
	if len(result.TestRuns) > 0 {
		runs := make([][]string, 0, len(result.TestRuns))
		for _, command := range result.TestRuns {
			runs = append(runs, []string{command})
		}
		Table(&b, "", "test_run", []string{"command"}, runs)
	}
	if len(result.Bodies) > 0 {
		fmt.Fprintf(&b, "contexts[%d]:\n", len(result.Bodies))
		for _, body := range result.Bodies {
			Field(&b, "  - ", "qn", body.QualifiedName)
			Field(&b, "    ", "file", body.File)
			Code(&b, "    ", body.StartLine, body.TotalLines, body.Code)
		}
	}
	return b.String()
}

func codeSearchLine(line string, full bool) string {
	runes := []rune(line)
	if full || len(runes) <= 160 {
		return line
	}
	return fmt.Sprintf("%s… (truncated, %d chars; use --full)", string(runes[:160]), len(runes))
}

// QueryAgentJSON is the default --json graph query payload: the same page the
// text view shows. Full Node/Edge structs need --full.
func QueryAgentJSON(result api.QueryResult) map[string]any {
	nodes := make([]map[string]any, 0, len(result.Rows))
	for _, n := range result.Rows {
		nodes = append(nodes, map[string]any{
			"qn":    n.QualifiedName,
			"kind":  n.Label,
			"file":  n.Location.File,
			"lines": lineRange(n.Location.StartLine, n.Location.EndLine),
		})
	}
	edges := []map[string]any{}
	for _, row := range queryEdgeRows(result.Rows, result.Edges) {
		edges = append(edges, map[string]any{"from": row[0], "rel": row[1], "to": row[2]})
	}
	payload := map[string]any{
		"count":  len(result.Rows),
		"total":  result.RowsTotal,
		"offset": result.Offset,
		"nodes":  nodes,
		"edges":  edges,
		"linked": result.Linked,
		"bodies": result.Bodies,
	}
	if len(result.CallersOf) > 0 {
		payload["callers_of"] = result.CallersOf
	}
	return payload
}

func lineRange(start, end int) string {
	if start <= 0 {
		return ""
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
