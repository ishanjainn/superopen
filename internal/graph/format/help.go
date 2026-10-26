package format

import (
	"fmt"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// Hints follow AXI §9: a few runnable next steps, only where the next step is
// not obvious. A self-contained answer gets none. Dynamic values the CLI
// cannot know (which row the agent wants) stay placeholders like <qn>.

// HelpForQuery returns next steps after a graph query page.
func HelpForQuery(result api.QueryResult, question string, limit int) []string {
	var hints []string
	shownEnd := result.Offset + len(result.Rows)
	if more := result.RowsTotal - shownEnd; more > 0 && len(result.Rows) > 0 {
		cmd := fmt.Sprintf("so graph query %s --offset %d", Quote(question), shownEnd)
		if limit > 0 {
			cmd += fmt.Sprintf(" --limit %d", limit)
		}
		hints = append(hints, fmt.Sprintf("%d more ranked: %s", more, cmd))
	}
	for _, body := range result.Bodies {
		if body.TotalLines > body.EndLine-body.StartLine+1 {
			hints = append(hints, fmt.Sprintf("rest of %s: so graph snippet %s --from %d", lastSegment(body.QualifiedName), body.QualifiedName, body.EndLine+1))
		}
	}
	switch {
	case len(result.Rows) == 0 && result.Offset == 0:
		hints = append(hints, "so graph search <name>", "so graph code-search \"<text>\"")
	case len(result.Bodies) == 0 && len(result.Rows) > 0:
		hints = append(hints, "so graph snippet <qn>")
	}
	if looksLikeImpactQuestion(question) && len(result.CallersOf) == 0 {
		hints = append(hints, "so graph impact --files <path>")
	}
	return hints
}

func looksLikeImpactQuestion(q string) bool {
	lower := strings.ToLower(q)
	for _, needle := range []string{
		"caller", "usage", "usages", "rename", "blast", "sibling",
		"impact", "across codebase", "dependen", "who uses", "who calls",
	} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

// HelpForImpact returns next steps after a graph impact dump.
func HelpForImpact(result api.ImpactResult) []string {
	if len(result.ImpactedFiles) > 0 || len(result.Impacted) > 0 {
		return []string{"so graph trace <qn> --direction incoming"}
	}
	return nil
}

// HelpForSearch returns next steps after a graph search.
func HelpForSearch(result api.SearchResult, query string, limit int) []string {
	matches := result.Matches
	if len(matches) == 0 {
		matches = result.Semantic
	}
	if len(matches) == 0 {
		// Imports, strings and stdlib names are not graph symbols.
		return []string{fmt.Sprintf("text search (imports, strings, unindexed files): so graph code-search %s", Quote(query))}
	}
	var hints []string
	if more := result.Page.Total - len(matches); more > 0 {
		next := len(matches) * 3
		if limit > 0 {
			next = limit * 3
		}
		hints = append(hints, fmt.Sprintf("%d more: so graph search %s --limit %d", more, Quote(query), next))
	}
	return append(hints, "so graph snippet <qn>")
}

// HelpForTrace returns next steps after a graph trace.
func HelpForTrace(result api.TraceResult) []string {
	if result.Status == "ambiguous" {
		return []string{"so graph trace <qn>"}
	}
	start := result.Start
	if start == "" {
		start = "<qn>"
	}
	empty := true
	for _, path := range result.Paths {
		if len(path) > 1 {
			empty = false
		}
	}
	var hints []string
	if result.Uncertain > 0 {
		hints = append(hints, fmt.Sprintf("%d guessed: so graph trace %s --direction %s --uncertain", result.Uncertain, start, normalizeTraceDirection(result.Direction)))
	}
	if result.Tests > 0 {
		hints = append(hints, fmt.Sprintf("%d in tests: so graph trace %s --direction %s --tests", result.Tests, start, normalizeTraceDirection(result.Direction)))
	}
	if empty {
		return append(hints, "dynamic or string references: so graph code-search \"<name>\"")
	}
	if result.Truncated {
		hints = append(hints, fmt.Sprintf("so graph trace %s --direction %s --limit 300", start, normalizeTraceDirection(result.Direction)))
	}
	return append(hints, "so graph snippet <qn>")
}

// HelpForSnippet returns next steps after a graph snippet. A complete body is
// self-contained and gets no hint.
func HelpForSnippet(result api.SnippetResult) []string {
	if result.Status == "ambiguous" {
		return []string{"so graph snippet <qn>"}
	}
	if result.OmittedStart > 0 && strings.TrimSpace(result.QualifiedName) != "" {
		return []string{fmt.Sprintf("rest (L%d-%d): so graph snippet %s --from %d", result.OmittedStart, result.OmittedEnd, result.QualifiedName, result.OmittedStart)}
	}
	return nil
}

// HelpForCodeSearch returns next steps after a text search.
func HelpForCodeSearch(result api.CodeSearchResult, pattern string, limit int) []string {
	if more := result.TotalGrepMatches - len(result.Matches); more > 0 {
		next := limit * 5
		if next <= 0 {
			next = 100
		}
		return []string{
			fmt.Sprintf("%d more: so graph code-search %s --limit %d", more, Quote(pattern), next),
			fmt.Sprintf("narrow: so graph code-search %s <path>", Quote(pattern)),
		}
	}
	return nil
}

func lastSegment(qn string) string {
	if i := strings.LastIndex(qn, "."); i >= 0 && i+1 < len(qn) {
		return qn[i+1:]
	}
	return qn
}
