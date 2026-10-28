package engine

import (
	"bufio"
	"context"
	"database/sql"
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

func CodeSearch(ctx context.Context, root string, req api.CodeSearchRequest) (result api.CodeSearchResult, err error) {
	root, err = CanonicalRoot(root)
	if err != nil {
		return api.CodeSearchResult{}, err
	}
	pattern := req.Pattern
	if strings.TrimSpace(pattern) == "" {
		return api.CodeSearchResult{}, nil
	}
	defer func() {
		if err == nil {
			attachCodeSearchNameMatches(ctx, root, req, &result)
		}
	}()
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	targets := []string{root}
	if len(req.Paths) > 0 {
		targets = targets[:0]
		for _, filter := range req.Paths {
			prefix, ok := codeSearchPathPrefix(filter)
			if !ok {
				return api.CodeSearchResult{}, errors.New("code-search paths must be repo-relative files or directories")
			}
			target, targetErr := codeSearchTarget(root, prefix)
			if targetErr != nil {
				return api.CodeSearchResult{}, targetErr
			}
			targets = append(targets, target)
		}
	} else if prefix, ok := codeSearchPathPrefix(req.PathFilter); ok {
		target, targetErr := codeSearchTarget(root, prefix)
		if targetErr != nil {
			return api.CodeSearchResult{}, targetErr
		}
		targets = []string{target}
	}
	if _, err := exec.LookPath("rg"); err != nil {
		return codeSearchFallback(ctx, root, req, pattern, limit)
	}
	args := []string{"--line-number", "--with-filename", "--no-heading", "--color=never", "--max-count", "1000"}
	if req.Regex {
		args = append(args, "--regexp", pattern)
	} else {
		args = append(args, "--fixed-strings", "--regexp", pattern)
	}
	if req.FileGlob != "" {
		args = append(args, "--glob", req.FileGlob)
	}
	if len(req.Paths) == 0 {
		if _, ok := codeSearchPathPrefix(req.PathFilter); !ok && req.PathFilter != "" {
			args = append(args, "--glob", req.PathFilter)
		}
	}
	args = append(args, "--")
	args = append(args, targets...)
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
	return parseCodeSearchOutput(root, string(output), pattern, limit)
}

var simpleLongFlag = regexp.MustCompile(`^--[A-Za-z][A-Za-z0-9-]*$`)
var namespacedCodeKey = regexp.MustCompile(`^(?:[A-Za-z][A-Za-z0-9_-]*:|:)[A-Za-z][A-Za-z0-9_-]*:?$`)

// Flags and namespaced directives often have bare-name registrations. Keep
// those candidates separate so the literal count still means exact matches.
func attachCodeSearchNameMatches(ctx context.Context, root string, req api.CodeSearchRequest, result *api.CodeSearchResult) {
	if req.Regex {
		return
	}
	bare, quoted := "", ""
	if simpleLongFlag.MatchString(req.Pattern) {
		bare = strings.TrimPrefix(req.Pattern, "--")
		quoted = "[\"'`]" + regexp.QuoteMeta(bare) + "[\"'`]"
	} else if strings.Contains(req.Pattern, ":") && namespacedCodeKey.MatchString(req.Pattern) {
		bare = strings.Trim(req.Pattern[strings.LastIndex(req.Pattern, ":")+1:], ":")
		if bare == "" {
			bare = strings.Trim(req.Pattern, ":")
		}
		quoted = "[\"'`]" + regexp.QuoteMeta(bare) + "[\"'`]\\s*:\\s*[A-Z][A-Za-z0-9_]*\\s*(?:,|}|$)"
	} else {
		return
	}
	aliasReq := api.CodeSearchRequest{Pattern: quoted, Regex: true, FileGlob: req.FileGlob, Limit: 30}
	if simpleLongFlag.MatchString(req.Pattern) {
		aliasReq.PathFilter, aliasReq.Paths = req.PathFilter, req.Paths
	}
	related, err := CodeSearch(ctx, root, aliasReq)
	if err != nil {
		return
	}
	seen := map[string]bool{}
	for _, match := range result.Matches {
		seen[match.Location.File] = true
	}
	for _, match := range related.Matches {
		if len(result.NameMatches) >= 2 {
			break
		}
		file := match.Location.File
		if seen[file] || codeSearchFileRank(file) != 0 {
			continue
		}
		seen[file] = true
		result.NameMatches = append(result.NameMatches, match)
	}
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

func parseCodeSearchOutput(root, output, pattern string, limit int) (api.CodeSearchResult, error) {
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
	return codeSearchResult(matches, files, pattern, limit), nil
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
		if !matchesSearchGlob(rel, req.FileGlob) || !matchesCodeSearchPath(rel, req) {
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
	return codeSearchResult(matches, files, pattern, limit), nil
}

func matchesCodeSearchPath(rel string, req api.CodeSearchRequest) bool {
	if len(req.Paths) == 0 {
		return matchesSearchPath(rel, req.PathFilter)
	}
	for _, filter := range req.Paths {
		if matchesSearchPath(rel, filter) {
			return true
		}
	}
	return false
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

// codeSearchResult puts literal definitions ahead of incidental uses and
// shows distinct files before repeated hits from one file. The total still
// counts every collected matching line.
func codeSearchResult(matches []api.CodeSearchMatch, files map[string]bool, pattern string, limit int) api.CodeSearchResult {
	total := len(matches)
	sort.SliceStable(matches, func(i, j int) bool {
		return codeSearchMatchScore(matches[i], pattern) > codeSearchMatchScore(matches[j], pattern)
	})
	var ranked []api.CodeSearchMatch
	first := map[string]int{}
	for _, match := range matches {
		if _, exists := first[match.Location.File]; !exists {
			ranked = append(ranked, match)
			first[match.Location.File] = match.Location.StartLine
		}
	}
	for _, match := range matches {
		// The first hit for each file was already placed above.
		if match.Location.StartLine != first[match.Location.File] {
			ranked = append(ranked, match)
		}
	}
	matches = ranked
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

func codeSearchMatchScore(match api.CodeSearchMatch, pattern string) int {
	score := -100 * codeSearchFileRank(match.Location.File)
	if pattern == "" {
		return score
	}
	line, needle := strings.ToLower(match.Source), strings.ToLower(pattern)
	trimmed := strings.TrimSpace(line)
	if trimmed == "@"+needle {
		score -= 50 // bare decorators are less useful than their consumers
	}
	if strings.HasPrefix(trimmed, "from ") || strings.HasPrefix(trimmed, "import ") {
		score -= 70
	}
	if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
		score -= 30
	}
	if filepath.Base(match.Location.File) == "__init__.py" && !strings.Contains(line, "=") &&
		(strings.Contains(line, "'"+needle+"'") || strings.Contains(line, "\""+needle+"\"")) {
		score -= 60 // exports are less useful than the implementation they expose
	}
	for _, quote := range []string{"\"", "'", "`"} {
		if strings.Contains(line, quote+needle+quote) {
			score += 40
			break
		}
	}
	if strings.Contains(line, "--"+needle) {
		score += 20
	}
	if !strings.HasPrefix(needle, "-") {
		if strings.Contains(line, "="+needle) || strings.Contains(line, "= "+needle) || strings.Contains(line, ": "+needle) {
			score += 55 // a value flowing into a call or assignment
		}
		if strings.HasPrefix(trimmed, needle+" =") || strings.HasPrefix(trimmed, needle+":") {
			score += 90 // put the shared value's definition before its consumers
		} else if strings.HasPrefix(trimmed, "def "+needle+"(") || strings.HasPrefix(trimmed, "func "+needle+"(") {
			score += 35
		}
	}
	return score
}

func codeSearchNoiseMatch(match api.CodeSearchMatch) bool {
	line := strings.TrimSpace(match.Source)
	if strings.HasPrefix(line, "from ") || strings.HasPrefix(line, "import ") {
		return true
	}
	if filepath.Base(match.Location.File) != "__init__.py" {
		return false
	}
	// Package re-exports can be multiline and contain neither import nor
	// assignment on the matching line. They are useful as rows, not contexts.
	return match.Node == nil || match.Node.Name == "__all__"
}

// attachCodeSearchSymbols links literal hits back to the narrowest indexed
// source symbol containing the line, so the next step can be graph snippet on
// an exact qualified name instead of an open-ended file read.
func (s *Store) attachCodeSearchSymbols(ctx context.Context, project string, result *api.CodeSearchResult, pattern ...string) error {
	if project == "" {
		project, _ = s.defaultProject(ctx)
	}
	var hits []*api.CodeSearchMatch
	for i := range result.Matches {
		hits = append(hits, &result.Matches[i])
	}
	for i := range result.NameMatches {
		hits = append(hits, &result.NameMatches[i])
	}
	for _, match := range hits {
		if match.Location.File == "" || match.Location.StartLine <= 0 {
			continue
		}
		row := s.db.QueryRowContext(ctx, `SELECT `+nodeColumns+` FROM nodes
			WHERE project=? AND file_path=? AND start_line<=? AND end_line>=?
			AND label IN ('Function','Method','Constructor','Class','Variable','Struct','Interface')
			ORDER BY (end_line-start_line) ASC, id ASC LIMIT 1`,
			project, match.Location.File, match.Location.StartLine, match.Location.StartLine)
		n, err := scanNode(row)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return err
		}
		n.Properties = nil
		match.Node = &n
	}
	// Show source windows and immediate graph users for the exact definition,
	// a bare-name registration, and a matching test when available. Distinct
	// classes in one file can each need a window; overlapping hits do not.
	type sourceWindow struct{ start, end int }
	windows := map[string][]sourceWindow{}
	seenQN := map[string]bool{}
	sourceRemaining := 90 // reserve 30 lines for a relevant test body
	if err := s.attachCodeSearchRegistration(ctx, project, result, &sourceRemaining); err != nil {
		return err
	}
	for _, body := range result.Bodies {
		windows[body.File] = append(windows[body.File], sourceWindow{start: body.StartLine, end: body.EndLine})
		seenQN[body.QualifiedName] = true
	}
	seenLink := map[string]bool{}
	addLink := func(from, rel string, n api.Node) {
		if len(result.Related) >= 6 || n.QualifiedName == "" {
			return
		}
		key := from + "\x00" + rel + "\x00" + n.QualifiedName
		if seenLink[key] {
			return
		}
		seenLink[key] = true
		result.Related = append(result.Related, api.QueryLink{
			From: from, Rel: rel, To: n.QualifiedName,
			File: n.Location.File, Start: n.Location.StartLine, End: n.Location.EndLine,
		})
	}
	var candidates []*api.CodeSearchMatch
	for i := range result.Matches {
		if codeSearchFileRank(result.Matches[i].Location.File) == 0 && !codeSearchNoiseMatch(result.Matches[i]) {
			candidates = append(candidates, &result.Matches[i])
		}
	}
	for i := range result.NameMatches {
		candidates = append(candidates, &result.NameMatches[i])
	}
	if len(candidates) == 0 {
		for i := range result.Matches {
			if !isTestPath(result.Matches[i].Location.File) {
				candidates = append(candidates, &result.Matches[i])
			}
		}
	}
	for _, match := range candidates {
		file := match.Location.File
		if file == "" || len(result.Bodies) >= 3 || sourceRemaining <= 0 {
			continue
		}
		qn := ""
		if match.Node != nil {
			qn = match.Node.QualifiedName
		}
		if qn != "" && seenQN[qn] {
			continue
		}
		adjacentEnd := 0
		start := max(1, match.Location.StartLine-8)
		lines := 30
		if match.Node != nil && match.Node.Label == "Variable" {
			start = max(1, match.Location.StartLine-18)
		}
		if match.Node != nil && match.Node.Label == "Class" &&
			match.Location.StartLine == match.Node.Location.StartLine {
			span := match.Node.Location.EndLine - match.Node.Location.StartLine + 1
			if span > 0 && span <= 80 {
				start, lines = match.Node.Location.StartLine, span
			}
		}
		if match.Node != nil && match.Node.Label != "Class" && match.Node.Label != "Variable" {
			span := match.Node.Location.EndLine - match.Node.Location.StartLine + 1
			if span > 0 && span <= 80 {
				start, lines = match.Node.Location.StartLine, span
			}
		}
		if match.Node != nil && match.Node.ID != 0 {
			outgoing, err := s.neighbors(ctx, *match.Node, "outgoing", []string{"CALLS", "USAGE", "READS", "WRITES"}, 24)
			if err != nil {
				return err
			}
			// Exact source lines often name a shared value. Show its other users
			// before generic callers of the enclosing function.
			for _, item := range outgoing {
				n := item.node
				if edgeConfidence(item.edge) < traceMinConfidence || n.Label != "Variable" || n.Name == "" || !strings.Contains(match.Source, n.Name) {
					continue
				}
				addLink(match.Node.QualifiedName, "uses", n)
				users, err := s.neighbors(ctx, n, "incoming", []string{"USAGE", "READS", "WRITES"}, 12)
				if err != nil {
					return err
				}
				for _, user := range users {
					if edgeConfidence(user.edge) < traceMinConfidence || user.node.ID == match.Node.ID || user.node.Label != "Method" && user.node.Label != "Function" {
						continue
					}
					addLink(n.QualifiedName, "used_by", user.node)
				}
			}
			neighbors, err := s.neighbors(ctx, *match.Node, "incoming", []string{"CALLS", "USAGE", "READS", "WRITES"}, 12)
			if err != nil {
				return err
			}
			for _, neighbor := range neighbors {
				if len(result.Related) >= 6 {
					break
				}
				n := neighbor.node
				if edgeConfidence(neighbor.edge) < traceMinConfidence || n.Label != "Function" && n.Label != "Method" && n.Label != "Constructor" {
					continue
				}
				if match.Node.Label == "Variable" && n.Location.File == file &&
					n.Location.StartLine >= match.Node.Location.EndLine &&
					n.Location.StartLine-match.Node.Location.EndLine <= 5 &&
					n.Location.EndLine-start+1 <= 80 && n.Location.EndLine > adjacentEnd {
					adjacentEnd = n.Location.EndLine
				}
				rel := map[string]string{"CALLS": "called_by", "USAGE": "used_by", "READS": "read_by", "WRITES": "written_by"}[neighbor.edge.Type]
				addLink(match.Node.QualifiedName, rel, n)
			}
		}
		if adjacentEnd > start+lines-1 {
			lines = adjacentEnd - start + 1
		}
		lines = min(lines, sourceRemaining)
		end := start + lines - 1
		overlaps := false
		for _, prior := range windows[file] {
			if start <= prior.end && prior.start <= end {
				overlaps = true
				break
			}
		}
		if overlaps {
			continue
		}
		got, err := s.Snippet(ctx, api.SnippetRequest{Project: project, File: file, StartLine: start, MaxLines: lines})
		if err != nil || strings.TrimSpace(got.Code) == "" {
			continue
		}
		windows[file] = append(windows[file], sourceWindow{start: start, end: end})
		if qn != "" {
			seenQN[qn] = true
		}
		result.Bodies = append(result.Bodies, api.QueryBody{
			QualifiedName: qn, File: file, StartLine: got.Location.StartLine,
			EndLine: got.Location.EndLine, TotalLines: codeSearchBodyTotal(match.Node, got),
			Code: got.Code,
		})
		sourceRemaining -= got.Location.EndLine - got.Location.StartLine + 1
	}
	substantiveFiles := map[string]bool{}
	for _, match := range result.Matches {
		if codeSearchFileRank(match.Location.File) == 0 && !codeSearchNoiseMatch(match) {
			substantiveFiles[match.Location.File] = true
		}
	}
	if len(substantiveFiles) == 1 && len(result.Bodies) < 3 {
		seenBody := map[string]bool{}
		for _, body := range result.Bodies {
			seenBody[body.QualifiedName] = true
		}
		for _, link := range result.Related {
			if len(result.Bodies) >= 3 || sourceRemaining <= 0 {
				break
			}
			if seenBody[link.To] || link.File == "" || codeSearchFileRank(link.File) != 0 {
				continue
			}
			start := max(1, link.Start-2)
			if link.Rel == "uses" {
				start = max(1, link.Start-18)
			}
			got, err := s.Snippet(ctx, api.SnippetRequest{Project: project, File: link.File, StartLine: start, MaxLines: min(30, sourceRemaining)})
			if err != nil || strings.TrimSpace(got.Code) == "" {
				continue
			}
			seenBody[link.To] = true
			result.Bodies = append(result.Bodies, api.QueryBody{
				QualifiedName: link.To, File: link.File, StartLine: got.Location.StartLine,
				EndLine: got.Location.EndLine, TotalLines: got.Location.EndLine - got.Location.StartLine + 1,
				Code: got.Code,
			})
			sourceRemaining -= got.Location.EndLine - got.Location.StartLine + 1
		}
	}
	result.Shared = codeSearchSharedValues(result.Related, result.Matches)
	codeSearchAttachUseSites(result)
	queryPattern := ""
	if len(pattern) > 0 {
		queryPattern = pattern[0]
	}
	if err := s.attachCodeSearchTests(ctx, project, result, queryPattern); err != nil {
		return err
	}
	return nil
}

func codeSearchSharedValues(links []api.QueryLink, matches []api.CodeSearchMatch) []api.CodeSearchSharedValue {
	ordered := []string{}
	consumers := map[string][]string{}
	matched := map[string]bool{}
	for _, match := range matches {
		if match.Node != nil && match.Node.Label == "Variable" && match.Node.QualifiedName != "" {
			qn := match.Node.QualifiedName
			if !matched[qn] {
				ordered = append(ordered, qn)
				consumers[qn] = nil
			}
			matched[qn] = true
		}
	}
	add := func(value, consumer string) {
		if value == "" || consumer == "" {
			return
		}
		if _, exists := consumers[value]; !exists {
			ordered = append(ordered, value)
		}
		for _, prior := range consumers[value] {
			if prior == consumer {
				return
			}
		}
		consumers[value] = append(consumers[value], consumer)
	}
	for _, link := range links {
		if link.Rel == "uses" {
			matched[link.To] = true
			add(link.To, link.From)
		}
	}
	for _, link := range links {
		if link.Rel == "used_by" && matched[link.From] {
			add(link.From, link.To)
		}
	}
	var shared []api.CodeSearchSharedValue
	for _, value := range ordered {
		if len(consumers[value]) < 2 {
			continue
		}
		shared = append(shared, api.CodeSearchSharedValue{Value: value, Consumers: consumers[value]})
		if len(shared) == 2 {
			break
		}
	}
	return shared
}

func codeSearchFileRank(path string) int {
	lower := strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(lower)
	switch base {
	case "changelog", "changes", "changes.old", "history", "news", "license", "licence", "copying", "notice", "readme", "authors", "contributors":
		return 2
	}
	switch strings.ToLower(filepath.Ext(lower)) {
	case ".po", ".pot", ".mo", ".svg", ".map", ".lock", ".min.js", ".csv":
		return 3
	case ".yml", ".yaml", ".toml", ".json", ".xml", ".ini", ".cfg":
		return 2
	case ".md", ".rst", ".txt", ".html", ".adoc":
		return 2
	}
	if strings.HasSuffix(lower, ".min.js") || strings.Contains(lower, "/locale/") {
		return 3
	}
	for _, prefix := range []string{"docs/", "doc/", "script/", "scripts/", "tools/", "examples/"} {
		if strings.HasPrefix(lower, prefix) {
			return 1
		}
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
