package engine

import (
	"bufio"
	"context"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func CodeSearch(ctx context.Context, root string, req api.CodeSearchRequest) (api.CodeSearchResult, error) {
	root, err := CanonicalRoot(root)
	if err != nil {
		return api.CodeSearchResult{}, err
	}
	pattern := strings.TrimSpace(req.Pattern)
	if pattern == "" {
		return api.CodeSearchResult{}, nil
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	args := []string{"--line-number", "--no-heading", "--color=never", "--max-count", "1000"}
	if req.Regex {
		args = append(args, "--regexp", pattern)
	} else {
		args = append(args, "--fixed-strings", pattern)
	}
	if req.FileGlob != "" {
		args = append(args, "--glob", req.FileGlob)
	}
	if req.PathFilter != "" {
		args = append(args, "--glob", req.PathFilter)
	}
	args = append(args, root)
	command := exec.CommandContext(ctx, "rg", args...)
	output, err := command.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
			return api.CodeSearchResult{Page: api.Page{Limit: limit}}, nil
		}
		return api.CodeSearchResult{}, err
	}
	matches := make([]api.CodeSearchMatch, 0, limit)
	files := map[string]bool{}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}
		rel, err := filepath.Rel(root, parts[0])
		if err != nil {
			rel = parts[0]
		}
		rel = filepath.ToSlash(rel)
		files[rel] = true
		match := api.CodeSearchMatch{
			Location: api.Location{File: rel},
			Source:   parts[2],
		}
		matches = append(matches, match)
		if len(matches) >= limit {
			break
		}
	}
	fileList := make([]string, 0, len(files))
	for file := range files {
		fileList = append(fileList, file)
	}
	sort.Strings(fileList)
	return api.CodeSearchResult{
		Matches: matches, Files: fileList, TotalGrepMatches: len(matches),
		TotalResults: len(matches), Page: api.Page{Limit: limit, Total: len(matches)},
	}, nil
}

func TraceIngest(ctx context.Context, root string, req api.TraceIngestRequest) (api.TraceIngestResult, error) {
	result := api.TraceIngestResult{}
	for _, trace := range req.Traces {
		if trace.Source == "" || trace.Target == "" {
			result.Rejected++
			result.Errors = append(result.Errors, api.Diagnostic{Code: "invalid_trace", Message: "source and target are required"})
			continue
		}
		edgeType := trace.Type
		if edgeType == "" {
			edgeType = "CALLS"
		}
		result.Edges = append(result.Edges, api.Edge{
			Type: edgeType, Properties: trace.Properties,
		})
		result.Accepted++
	}
	return result, nil
}
