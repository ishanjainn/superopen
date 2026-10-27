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
	"strconv"
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
	target := root
	if prefix, ok := codeSearchPathPrefix(req.PathFilter); ok {
		if target, err = codeSearchTarget(root, prefix); err != nil {
			return api.CodeSearchResult{}, err
		}
	}
	if _, err := exec.LookPath("rg"); err != nil {
		return codeSearchFallback(ctx, root, req, pattern, limit)
	}
	args := []string{"--line-number", "--with-filename", "--no-heading", "--color=never", "--max-count", "1000"}
	if req.Regex {
		args = append(args, "--regexp", pattern)
	} else {
		args = append(args, "--fixed-strings", pattern)
	}
	if req.FileGlob != "" {
		args = append(args, "--glob", req.FileGlob)
	}
	if _, ok := codeSearchPathPrefix(req.PathFilter); !ok && req.PathFilter != "" {
		args = append(args, "--glob", req.PathFilter)
	}
	args = append(args, target)
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

// codeSearchCollectCap bounds how many matching lines are kept for ranking
// and counted for the total. Past it the total is a floor.
const codeSearchCollectCap = 5000

func parseCodeSearchOutput(root, output string, limit int) (api.CodeSearchResult, error) {
	var matches []api.CodeSearchMatch
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
		lineNo, _ := strconv.Atoi(parts[1])
		matches = append(matches, api.CodeSearchMatch{
			Location: api.Location{File: rel, StartLine: lineNo, EndLine: lineNo},
			Source:   parts[2],
		})
		if len(matches) >= codeSearchCollectCap {
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

	var matches []api.CodeSearchMatch
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
		if !matchesSearchGlob(rel, req.FileGlob) || !matchesSearchPath(rel, req.PathFilter) {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil || codeSearchBinary(content) {
			return nil
		}
		scanner := bufio.NewScanner(strings.NewReader(string(content)))
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			if !matcher(line) {
				continue
			}
			files[rel] = true
			matches = append(matches, api.CodeSearchMatch{
				Location: api.Location{File: rel, StartLine: lineNo, EndLine: lineNo},
				Source:   line,
			})
			if len(matches) >= codeSearchCollectCap {
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

// codeSearchPathPrefix reads a path filter without glob characters as a
// repo-relative file or directory (django/core/mail), not a basename glob.
func codeSearchPathPrefix(filter string) (string, bool) {
	filter = strings.Trim(strings.TrimSpace(filepath.ToSlash(filter)), "/")
	filter = strings.TrimPrefix(filter, "./")
	if filter == "" || filter == "." || strings.ContainsAny(filter, "*?[{") {
		return "", false
	}
	return filter, true
}

func codeSearchTarget(root, prefix string) (string, error) {
	target := filepath.Join(root, filepath.FromSlash(prefix))
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("code-search path escapes repository: " + prefix)
	}
	if _, err := os.Stat(target); err != nil {
		return "", errors.New("code-search path not found: " + prefix)
	}
	return target, nil
}

func matchesSearchPath(rel, filter string) bool {
	prefix, ok := codeSearchPathPrefix(filter)
	if !ok {
		return matchesSearchGlob(rel, filter)
	}
	return rel == prefix || strings.HasPrefix(rel, prefix+"/")
}

// codeSearchResult ranks source lines ahead of tests, docs and locale
// catalogs (rg and the walker both list django/conf/locale first), then
// keeps limit. The total counts every collected line.
func codeSearchResult(matches []api.CodeSearchMatch, files map[string]bool, limit int) api.CodeSearchResult {
	total := len(matches)
	sort.SliceStable(matches, func(i, j int) bool {
		return codeSearchFileRank(matches[i].Location.File) < codeSearchFileRank(matches[j].Location.File)
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	shown := map[string]bool{}
	for _, m := range matches {
		shown[m.Location.File] = true
	}
	fileList := make([]string, 0, len(shown))
	for file := range shown {
		fileList = append(fileList, file)
	}
	sort.Strings(fileList)
	return api.CodeSearchResult{
		Matches: matches, Files: fileList, TotalGrepMatches: total,
		TotalResults: total, Page: api.Page{Limit: limit, Total: total, Truncated: total > len(matches)},
	}
}

func codeSearchFileRank(path string) int {
	lower := strings.ToLower(path)
	switch strings.ToLower(filepath.Ext(lower)) {
	case ".po", ".pot", ".mo", ".svg", ".map", ".lock", ".min.js", ".csv":
		return 3
	case ".md", ".rst", ".txt", ".html", ".adoc":
		return 2
	}
	if strings.HasSuffix(lower, ".min.js") || strings.Contains(lower, "/locale/") {
		return 3
	}
	if isTestPath(path) {
		return 1
	}
	return 0
}

// codeSearchBinary is ripgrep's rule: a NUL byte in the first 8 KiB.
func codeSearchBinary(content []byte) bool {
	head := content
	if len(head) > 8192 {
		head = head[:8192]
	}
	for _, c := range head {
		if c == 0 {
			return true
		}
	}
	return false
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
