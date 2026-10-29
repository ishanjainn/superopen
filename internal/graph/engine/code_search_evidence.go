package engine

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

var codeSearchRegistryEntry = regexp.MustCompile("[\"'`]([A-Za-z][A-Za-z0-9_-]*)[\"'`]\\s*:\\s*([A-Z][A-Za-z0-9_]*)\\s*(?:,|}|$)")
var codeSearchTermSplit = regexp.MustCompile(`[^A-Za-z0-9]+`)

// A quoted registry key pointing at a class is a source-level relationship,
// even when a directive or option name never appears in that class body.
func (s *Store) attachCodeSearchRegistration(ctx context.Context, project string, result *api.CodeSearchResult, remaining *int) error {
	for _, match := range result.NameMatches {
		parts := codeSearchRegistryEntry.FindStringSubmatch(match.Source)
		if len(parts) != 3 || *remaining <= 0 {
			continue
		}
		row := s.db.QueryRowContext(ctx, `SELECT `+nodeColumns+` FROM nodes
			WHERE project=? AND file_path=? AND name=? AND label IN ('Class','Struct')
			ORDER BY start_line LIMIT 1`, project, match.Location.File, parts[2])
		node, err := scanNode(row)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return err
		}
		from := ""
		if match.Node != nil {
			from = match.Node.QualifiedName
		}
		result.Related = append(result.Related, api.QueryLink{
			From: from, Rel: "registers", To: node.QualifiedName,
			File: node.Location.File, Start: node.Location.StartLine, End: node.Location.EndLine,
		})
		span := node.Location.EndLine - node.Location.StartLine + 1
		if span <= 0 {
			continue
		}
		got, err := s.Snippet(ctx, api.SnippetRequest{
			Project: project, File: node.Location.File, StartLine: node.Location.StartLine,
			MaxLines: min(span, 80, *remaining),
		})
		if err != nil || got.Code == "" {
			continue
		}
		result.Bodies = append(result.Bodies, api.QueryBody{
			QualifiedName: node.QualifiedName, File: node.Location.File,
			StartLine: got.Location.StartLine, EndLine: got.Location.EndLine,
			TotalLines: span, Code: got.Code,
		})
		*remaining -= got.Location.EndLine - got.Location.StartLine + 1
		break
	}
	return nil
}

func codeSearchBodyTotal(node *api.Node, got api.SnippetResult) int {
	shown := got.Location.EndLine - got.Location.StartLine + 1
	if node != nil && got.Location.StartLine == node.Location.StartLine {
		span := node.Location.EndLine - node.Location.StartLine + 1
		if span > shown {
			return span
		}
	}
	return shown
}

func codeSearchAttachUseSites(result *api.CodeSearchResult) {
	for i := range result.Shared {
		value := &result.Shared[i]
		name := value.Value[strings.LastIndex(value.Value, ".")+1:]
		for _, consumer := range value.Consumers {
			for _, body := range result.Bodies {
				if body.QualifiedName != consumer {
					continue
				}
				for offset, line := range strings.Split(body.Code, "\n") {
					if !strings.Contains(line, name) || strings.HasPrefix(strings.TrimSpace(line), "#") {
						continue
					}
					expr := strings.TrimSpace(line)
					if len([]rune(expr)) > 160 {
						expr = string([]rune(expr)[:160]) + "…"
					}
					value.Uses = append(value.Uses, api.CodeSearchUseSite{
						Consumer: consumer, File: body.File, Line: body.StartLine + offset, Expr: expr,
					})
					break
				}
				break
			}
		}
	}
}

type codeSearchTestCandidate struct {
	node   api.Node
	target string
	score  int
}

func (s *Store) attachCodeSearchTests(ctx context.Context, project string, result *api.CodeSearchResult, pattern string) error {
	var sourceNodes []api.Node
	seenSource := map[string]bool{}
	addSource := func(node api.Node) {
		if node.ID != 0 && !isTestPath(node.Location.File) && !seenSource[node.QualifiedName] {
			seenSource[node.QualifiedName] = true
			sourceNodes = append(sourceNodes, node)
		}
	}
	for _, match := range result.Matches {
		if match.Node != nil {
			addSource(*match.Node)
		}
	}
	for _, link := range result.Related {
		if link.Rel != "used_by" && link.Rel != "registers" || len(sourceNodes) >= 4 {
			continue
		}
		row := s.db.QueryRowContext(ctx, `SELECT `+nodeColumns+` FROM nodes WHERE project=? AND qualified_name=? LIMIT 1`, project, link.To)
		node, err := scanNode(row)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return err
		}
		addSource(node)
	}
	var candidates []codeSearchTestCandidate
	seenTest := map[string]bool{}
	terms := codeSearchPatternTerms(pattern)
	for _, match := range result.Matches {
		if match.Node != nil && isTestPath(match.Location.File) && isTestFunctionName(match.Node.Name) {
			qn := match.Node.QualifiedName
			if !seenTest[qn] {
				seenTest[qn] = true
				candidates = append(candidates, codeSearchTestCandidate{node: *match.Node, score: 100})
			}
		}
	}
	for _, source := range sourceNodes {
		neighbors, err := s.neighbors(ctx, source, "incoming", []string{"TESTS"}, 200)
		if err != nil {
			return err
		}
		for _, item := range neighbors {
			test := item.node
			if seenTest[test.QualifiedName] || !isTestPath(test.Location.File) || !isTestFunctionName(test.Name) {
				continue
			}
			seenTest[test.QualifiedName] = true
			score := 20
			for _, part := range strings.Split(filepath.ToSlash(source.Location.File), "/") {
				if len(part) > 3 && strings.Contains(filepath.ToSlash(test.Location.File), "/"+part+"/") {
					score += 20
					break
				}
			}
			if strings.Contains(strings.ToLower(test.Name), strings.ToLower(source.Name)) {
				score += 10
			}
			for _, term := range terms {
				if strings.Contains(strings.ToLower(test.Name), term) {
					score += 20
					break
				}
			}
			candidates = append(candidates, codeSearchTestCandidate{node: test, target: source.QualifiedName, score: score})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].node.QualifiedName < candidates[j].node.QualifiedName
	})
	ordered := make([]codeSearchTestCandidate, 0, min(4, len(candidates)))
	selected := map[string]bool{}
	for _, shared := range result.Shared {
		for _, consumer := range shared.Consumers {
			for _, candidate := range candidates {
				if candidate.target == consumer && !selected[candidate.node.QualifiedName] {
					ordered = append(ordered, candidate)
					selected[candidate.node.QualifiedName] = true
					break
				}
			}
		}
	}
	if len(ordered) > 4 {
		ordered = ordered[:4]
	}
	for _, candidate := range candidates {
		if len(ordered) >= 4 {
			break
		}
		if !selected[candidate.node.QualifiedName] {
			ordered = append(ordered, candidate)
			selected[candidate.node.QualifiedName] = true
		}
	}
	var root string
	if err := s.db.QueryRowContext(ctx, `SELECT root_path FROM projects WHERE name=?`, project).Scan(&root); err != nil {
		return err
	}
	seenRun := map[string]bool{}
	for _, candidate := range ordered {
		node := candidate.node
		result.Tests = append(result.Tests, api.CodeSearchTest{
			QualifiedName: node.QualifiedName, Target: candidate.target,
			File: node.Location.File, Line: node.Location.StartLine,
		})
		if run := codeSearchTestRun(root, node.Location.File); run != "" && !seenRun[run] && len(result.TestRuns) < 2 {
			seenRun[run] = true
			result.TestRuns = append(result.TestRuns, run)
		}
	}
	if len(result.Tests) == 0 || len(result.Bodies) >= 5 {
		return nil
	}
	remaining := 30
	shownTargets := map[string]bool{}
	for _, candidate := range ordered {
		if len(result.Bodies) >= 5 || remaining <= 0 {
			break
		}
		if candidate.target != "" && shownTargets[candidate.target] {
			continue
		}
		span := candidate.node.Location.EndLine - candidate.node.Location.StartLine + 1
		if span <= 0 {
			continue
		}
		maxLines := min(span, remaining)
		if len(result.Shared) > 0 {
			maxLines = min(maxLines, 15)
		}
		got, err := s.Snippet(ctx, api.SnippetRequest{
			Project: project, File: candidate.node.Location.File, StartLine: candidate.node.Location.StartLine,
			MaxLines: maxLines,
		})
		if err != nil || got.Code == "" {
			continue
		}
		result.Bodies = append(result.Bodies, api.QueryBody{
			QualifiedName: candidate.node.QualifiedName, File: candidate.node.Location.File,
			StartLine: got.Location.StartLine, EndLine: got.Location.EndLine,
			TotalLines: span, Code: got.Code,
		})
		remaining -= got.Location.EndLine - got.Location.StartLine + 1
		if candidate.target != "" {
			shownTargets[candidate.target] = true
		}
		if len(result.Shared) == 0 || len(shownTargets) >= 2 {
			break
		}
	}
	return nil
}

func codeSearchPatternTerms(pattern string) []string {
	var terms []string
	for _, part := range codeSearchTermSplit.Split(strings.ToLower(pattern), -1) {
		if len(part) >= 3 && part != "name" && part != "test" {
			terms = append(terms, part)
		}
	}
	return terms
}

func codeSearchTestRun(root, file string) string {
	file = filepath.ToSlash(file)
	if strings.HasSuffix(file, "_test.go") {
		dir := filepath.ToSlash(filepath.Dir(file))
		if dir == "." {
			return "go test ."
		}
		return "go test ./" + dir
	}
	if !strings.HasSuffix(file, ".py") {
		return ""
	}
	if strings.HasPrefix(file, "tests/") {
		if _, err := os.Stat(filepath.Join(root, "tests", "runtests.py")); err == nil {
			module := strings.TrimSuffix(strings.TrimPrefix(file, "tests/"), ".py")
			module = strings.ReplaceAll(module, "/", ".")
			if regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`).MatchString(module) {
				return "PYTHONPATH=. python3 tests/runtests.py " + module
			}
		}
	}
	return "python3 -m pytest '" + strings.ReplaceAll(file, "'", "'\"'\"'") + "'"
}
