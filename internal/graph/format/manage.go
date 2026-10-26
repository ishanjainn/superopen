package format

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// Compact views for the graph management commands (status, projects, build,
// coverage, schema, diagnostics, cypher). They print what an agent decides
// on, in the same TOON dialect as query, and leave the rest to --json/--full.

func shortGeneration(g string) string {
	if len(g) > 12 {
		return g[:12]
	}
	return g
}

func timeField(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// StatusCompact is the one-glance index state.
func StatusCompact(s api.Status) string {
	var b strings.Builder
	Field(&b, "", "state", s.State)
	if s.Project != "" {
		Field(&b, "", "project", s.Project)
	}
	if s.State == "missing" {
		b.WriteString("index: none for this repository\n")
		return b.String()
	}
	Field(&b, "", "nodes", s.NodeCount)
	Field(&b, "", "edges", s.EdgeCount)
	Field(&b, "", "files", s.FileCount)
	if at := timeField(s.IndexedAt); at != "" {
		Field(&b, "", "indexed_at", at)
	}
	Field(&b, "", "generation", shortGeneration(s.Generation))
	Field(&b, "", "engine", fmt.Sprintf("%s %s schema %d", s.Engine, s.EngineVersion, s.Schema))
	if len(s.Diagnostics) > 0 {
		rows := make([][]string, 0, len(s.Diagnostics))
		for _, d := range s.Diagnostics {
			rows = append(rows, []string{d.Severity, d.Code, d.Message})
		}
		Table(&b, "", "diagnostics", []string{"severity", "code", "message"}, rows)
	}
	return b.String()
}

// HelpForStatus points a missing or stale index at the command that fixes it.
func HelpForStatus(s api.Status) []string {
	if s.State == "missing" {
		return []string{"so graph build"}
	}
	return []string{`so graph query "<question>"`, "so graph architecture"}
}

// BuildCompact reports a build or refresh result.
func BuildCompact(r api.BuildResult) string {
	var b strings.Builder
	Field(&b, "", "build", r.Status)
	Field(&b, "", "project", r.Project)
	Field(&b, "", "nodes", r.NodeCount)
	Field(&b, "", "edges", r.EdgeCount)
	Field(&b, "", "files", r.FileCount)
	if r.Duration > 0 {
		Field(&b, "", "duration", r.Duration.Round(time.Millisecond).String())
	}
	if r.Changes != nil {
		c := r.Changes
		Field(&b, "", "changes", fmt.Sprintf("%d added, %d modified, %d deleted, %d renamed, %d unchanged",
			len(c.Added), len(c.Modified), len(c.Deleted), len(c.Renamed), c.Unchanged))
	}
	if r.Coverage.Total > 0 {
		Field(&b, "", "coverage", fmt.Sprintf("%s; %d files not fully indexed", r.Coverage.Status, r.Coverage.Total))
	}
	return b.String()
}

// HelpForBuild names the coverage view when files were skipped.
func HelpForBuild(r api.BuildResult) []string {
	if r.Coverage.Total > 0 {
		return []string{"so graph coverage"}
	}
	return nil
}

// ProjectsCompact lists indexed projects.
func ProjectsCompact(r api.ProjectsResult) string {
	var b strings.Builder
	if len(r.Projects) == 0 {
		b.WriteString("count: 0\nprojects: none indexed; run so graph build in a repository\n")
		return b.String()
	}
	Count(&b, len(r.Projects), maxInt(r.Page.Total, len(r.Projects)))
	rows := make([][]string, 0, len(r.Projects))
	for _, p := range r.Projects {
		rows = append(rows, []string{p.Name, p.RootPath, p.IndexedAt.UTC().Format(time.RFC3339), shortGeneration(p.Generation)})
	}
	Table(&b, "", "projects", []string{"name", "root", "indexed_at", "generation"}, rows)
	return b.String()
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// CoverageCompact counts what the index skipped by kind and lists the first
// files; the full list stays behind --full.
func CoverageCompact(c api.Coverage) string {
	var b strings.Builder
	Field(&b, "", "status", c.Status)
	if c.IndexMode != "" {
		Field(&b, "", "index_mode", c.IndexMode)
	}
	total := c.Total
	if total < len(c.Rows) {
		total = len(c.Rows)
	}
	if total == 0 {
		b.WriteString("count: 0\nnot_fully_indexed: none; every file was indexed\n")
		return b.String()
	}
	byKind := map[string]int{}
	for _, row := range c.Rows {
		byKind[row.Kind]++
	}
	kinds := make([]string, 0, len(byKind))
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool {
		if byKind[kinds[i]] != byKind[kinds[j]] {
			return byKind[kinds[i]] > byKind[kinds[j]]
		}
		return kinds[i] < kinds[j]
	})
	kindRows := make([][]string, 0, len(kinds))
	for _, k := range kinds {
		kindRows = append(kindRows, []string{k, fmt.Sprint(byKind[k])})
	}
	Table(&b, "", "by_kind", []string{"kind", "files"}, kindRows)
	shown := c.Rows
	if len(shown) > 15 {
		shown = shown[:15]
	}
	rows := make([][]string, 0, len(shown))
	for _, r := range shown {
		rows = append(rows, []string{r.Path, r.Kind, r.Detail})
	}
	Count(&b, len(shown), total)
	Table(&b, "", "files", []string{"path", "kind", "detail"}, rows)
	return b.String()
}

// HelpForCoverage offers the full list when the page was cut.
func HelpForCoverage(c api.Coverage) []string {
	if len(c.Rows) > 15 || c.Total > len(c.Rows) {
		return []string{"so graph coverage --full"}
	}
	return nil
}

// SchemaCompact lists the labels and relationship types with counts.
func SchemaCompact(s api.SchemaResult) string {
	var b strings.Builder
	Field(&b, "", "project", s.Project)
	Field(&b, "", "nodes", s.NodeCount)
	Field(&b, "", "edges", s.EdgeCount)
	counts := func(name string, items []api.SchemaCount) {
		rows := make([][]string, 0, len(items))
		for _, it := range items {
			rows = append(rows, []string{it.Name, fmt.Sprint(it.Count)})
		}
		Table(&b, "", name, []string{"name", "count"}, rows)
	}
	counts("node_labels", s.NodeLabels)
	counts("edge_types", s.EdgeTypes)
	return b.String()
}

// DiagnosticsCompact states the verdict first.
func DiagnosticsCompact(r api.DiagnosticsResult) string {
	var b strings.Builder
	if r.Healthy {
		b.WriteString("healthy: true\ndiagnostics: none; the index passed every integrity check\n")
		return b.String()
	}
	Field(&b, "", "healthy", false)
	if r.Recovered {
		Field(&b, "", "recovered", true)
	}
	rows := make([][]string, 0, len(r.Diagnostics))
	for _, d := range r.Diagnostics {
		rows = append(rows, []string{d.Severity, d.Code, d.Message})
	}
	Count(&b, len(rows), len(rows))
	Table(&b, "", "diagnostics", []string{"severity", "code", "message"}, rows)
	return b.String()
}

// HelpForDiagnostics names the repair when the index is unhealthy.
func HelpForDiagnostics(r api.DiagnosticsResult) []string {
	if !r.Healthy {
		return []string{"so graph build"}
	}
	return nil
}

// CypherCompact prints result rows as one table.
func CypherCompact(r api.CypherResult) string {
	var b strings.Builder
	if len(r.Rows) == 0 {
		b.WriteString("count: 0\nrows: none; the query matched nothing\n")
		return b.String()
	}
	total := maxInt(r.Page.Total, len(r.Rows))
	Count(&b, len(r.Rows), total)
	cols := r.Columns
	if len(cols) == 0 {
		for k := range r.Rows[0] {
			cols = append(cols, k)
		}
		sort.Strings(cols)
	}
	rows := make([][]string, 0, len(r.Rows))
	for _, row := range r.Rows {
		cells := make([]string, len(cols))
		for i, c := range cols {
			cells[i] = fmt.Sprint(row[c])
		}
		rows = append(rows, cells)
	}
	Table(&b, "", "rows", cols, rows)
	return b.String()
}

// HelpForCypher offers a longer page when rows were cut.
func HelpForCypher(r api.CypherResult) []string {
	if total := maxInt(r.Page.Total, len(r.Rows)); total > len(r.Rows) {
		return []string{fmt.Sprintf("%d more: re-run with --max-rows %d", total-len(r.Rows), total)}
	}
	return nil
}
