package engine

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	if _, err := exec.LookPath("rg"); err != nil {
		return codeSearchFallback(ctx, root, req, pattern, limit)
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
		// Prefer the portable walker if ripgrep became unavailable between LookPath and exec.
		if errors.Is(err, exec.ErrNotFound) || isExecutableNotFound(err) {
			return codeSearchFallback(ctx, root, req, pattern, limit)
		}
		return api.CodeSearchResult{}, err
	}
	return parseCodeSearchOutput(root, string(output), limit)
}

func isExecutableNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "executable file not found")
}

func parseCodeSearchOutput(root, output string, limit int) (api.CodeSearchResult, error) {
	matches := make([]api.CodeSearchMatch, 0, limit)
	files := map[string]bool{}
	scanner := bufio.NewScanner(strings.NewReader(output))
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
		matches = append(matches, api.CodeSearchMatch{
			Location: api.Location{File: rel},
			Source:   parts[2],
		})
		if len(matches) >= limit {
			break
		}
	}
	return codeSearchResult(matches, files, limit), nil
}

func codeSearchFallback(ctx context.Context, root string, req api.CodeSearchRequest, pattern string, limit int) (api.CodeSearchResult, error) {
	var matcher func(string) bool
	if req.Regex {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return api.CodeSearchResult{}, err
		}
		matcher = compiled.MatchString
	} else {
		matcher = func(line string) bool { return strings.Contains(line, pattern) }
	}

	matches := make([]api.CodeSearchMatch, 0, limit)
	files := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return nil
		}
		name := entry.Name()
		if entry.IsDir() {
			if shouldSkipSearchDir(name) {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !matchesSearchGlob(rel, req.FileGlob) || !matchesSearchGlob(rel, req.PathFilter) {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		scanner := bufio.NewScanner(strings.NewReader(string(content)))
		for scanner.Scan() {
			line := scanner.Text()
			if !matcher(line) {
				continue
			}
			files[rel] = true
			matches = append(matches, api.CodeSearchMatch{
				Location: api.Location{File: rel},
				Source:   line,
			})
			if len(matches) >= limit {
				return errSearchLimitReached
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, errSearchLimitReached) {
		return api.CodeSearchResult{}, err
	}
	return codeSearchResult(matches, files, limit), nil
}

var errSearchLimitReached = errors.New("search limit reached")

func shouldSkipSearchDir(name string) bool {
	switch name {
	case ".git", ".hg", ".svn", "node_modules", "vendor", "dist", "bin", ".so":
		return true
	default:
		return false
	}
}

func matchesSearchGlob(rel, pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return true
	}
	matched, err := filepath.Match(pattern, filepath.Base(rel))
	if err == nil && matched {
		return true
	}
	matched, err = filepath.Match(pattern, rel)
	return err == nil && matched
}

func codeSearchResult(matches []api.CodeSearchMatch, files map[string]bool, limit int) api.CodeSearchResult {
	fileList := make([]string, 0, len(files))
	for file := range files {
		fileList = append(fileList, file)
	}
	sort.Strings(fileList)
	return api.CodeSearchResult{
		Matches: matches, Files: fileList, TotalGrepMatches: len(matches),
		TotalResults: len(matches), Page: api.Page{Limit: limit, Total: len(matches)},
	}
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
