package format

import (
	"fmt"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// HelpForQuery returns AXI next-step commands after a graph query.
func HelpForQuery(result api.QueryResult) []string {
	qn := ""
	if len(result.Seeds) > 0 {
		qn = result.Seeds[0].QualifiedName
	}
	var hints []string
	if qn == "" {
		hints = []string{
			"so graph search <name>",
			"so graph snippet <qualified_name>",
		}
	} else {
		hints = []string{fmt.Sprintf("so graph snippet %s", qn)}
	}
	if looksLikeImpactQuestion(result.Question) {
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

// HelpForImpact returns AXI next-step commands after a graph impact dump.
func HelpForImpact(result api.ImpactResult) []string {
	if len(result.ImpactedFiles) > 0 {
		return []string{
			fmt.Sprintf("so graph snippet <qn>  # then check %s", result.ImpactedFiles[0].Path),
		}
	}
	if len(result.Impacted) > 0 && result.Impacted[0].QualifiedName != "" {
		return []string{fmt.Sprintf("so graph snippet %s", result.Impacted[0].QualifiedName)}
	}
	return []string{"so graph impact --files <path>"}
}

// HelpForSearch returns AXI next-step commands after a graph search.
func HelpForSearch(result api.SearchResult) []string {
	matches := result.Matches
	if len(matches) == 0 {
		matches = result.Semantic
	}
	if len(matches) == 0 {
		return []string{"so graph query \"<question>\""}
	}
	qn := matches[0].QualifiedName
	return []string{
		fmt.Sprintf("so graph snippet %s", qn),
		fmt.Sprintf("so graph trace %s", qn),
	}
}

// HelpForTrace returns AXI next-step commands after a graph trace.
func HelpForTrace(result api.TraceResult) []string {
	qn := ""
	if result.Status == "ambiguous" && len(result.Suggestions) > 0 {
		qn = result.Suggestions[0].QualifiedName
		return []string{fmt.Sprintf("so graph snippet %s", qn)}
	}
	for _, path := range result.Paths {
		if len(path) > 0 && path[0].Node.QualifiedName != "" {
			qn = path[0].Node.QualifiedName
			break
		}
	}
	if qn == "" {
		return []string{"so graph snippet <qualified_name>"}
	}
	return []string{fmt.Sprintf("so graph snippet %s", qn)}
}

// HelpForSnippet returns AXI next-step commands after a graph snippet.
func HelpForSnippet(result api.SnippetResult) []string {
	if result.Status == "ambiguous" && len(result.Suggestions) > 0 {
		qn := result.Suggestions[0].QualifiedName
		if qn != "" {
			return []string{fmt.Sprintf("so graph snippet %s", qn)}
		}
	}
	qn := result.QualifiedName
	if qn == "" {
		return []string{
			"so graph trace <qualified_name> --direction incoming",
			"so graph trace <qualified_name> --direction outgoing",
		}
	}
	return []string{
		fmt.Sprintf("so graph trace %s --direction incoming", qn),
		fmt.Sprintf("so graph trace %s --direction outgoing", qn),
	}
}
