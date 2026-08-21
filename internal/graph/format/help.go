package format

import (
	"fmt"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// HelpForQuery returns AXI next-step commands after a graph query.
func HelpForQuery(result api.QueryResult) []string {
	qn := ""
	if len(result.Seeds) > 0 {
		qn = result.Seeds[0].QualifiedName
	}
	if qn == "" {
		return []string{
			"so graph search <name>",
			"so graph snippet <qualified_name>",
		}
	}
	return []string{fmt.Sprintf("so graph snippet %s", qn)}
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
	qn := result.QualifiedName
	if qn == "" {
		return []string{"so graph trace <qualified_name> --direction both"}
	}
	return []string{fmt.Sprintf("so graph trace %s --direction both", qn)}
}
