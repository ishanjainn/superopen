package engine

import (
	"context"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

const (
	impactFileCap     = 16
	impactCallSeedCap = 12
	reasonCaller      = "caller"
	reasonImpl        = "impl"
	reasonOverride    = "override"
	reasonSibling     = "sibling"
	reasonCoChange    = "co-change"
)

var (
	familyEdgeTypes   = []string{"INHERITS", "IMPLEMENTS", "OVERRIDE", "DEFINES_METHOD"}
	coChangeEdgeTypes = []string{"FILE_CHANGES_WITH"}
	// caller last so same-dir siblings and impls outrank alphabetically
	// first incoming CALLS (django/apps/config.py).
	reasonRank = map[string]int{
		reasonImpl:     0,
		reasonOverride: 1,
		reasonSibling:  2,
		reasonCoChange: 3,
		reasonCaller:   4,
	}
)

func (s *Store) Impact(ctx context.Context, req api.ImpactRequest) (api.ImpactResult, error) {
	if req.Project == "" {
		req.Project, _ = s.defaultProject(ctx)
	}
	result := api.ImpactResult{
		Base:            req.Base,
		ImpactedModules: map[string]int{},
	}
	files := uniqueSlash(req.Files)
	wholeFile := map[string]bool{}
	for _, f := range files {
		wholeFile[filepath.ToSlash(f)] = true
	}
	var hunks map[string][]lineSpan
	if req.Base != "" {
		root := strings.TrimSpace(req.RepoRoot)
		if root == "" {
			root, _ = s.projectRootPath(ctx, req.Project)
		}
		mergeBase, changed := gitChangedFiles(ctx, root, req.Base)
		result.MergeBase = mergeBase
		result.ChangedFiles = changed
		files = uniqueSlash(append(files, changed...))
		hunks = gitChangedLineSpans(ctx, root, mergeBase)
	}
	normalized := make([]string, 0, len(files))
	seedFiles := map[string]bool{}
	explicit := map[string]bool{}
	for _, f := range files {
		n := normalizeImpactFile(req.RepoRoot, f)
		if n == "" {
			n = filepath.ToSlash(f)
		}
		seedFiles[n] = true
		if wholeFile[filepath.ToSlash(f)] || wholeFile[n] {
			explicit[n] = true
		}
		normalized = append(normalized, n)
	}
	files = uniqueSlash(normalized)

	seen := map[int64]bool{}
	// Family then co-change before incoming CALLS so siblings and
	// historical couples outrank alphabetically-first importers.
	seedNodes := s.impactSeedNodes(ctx, req.Project, req.Symbols, files, hunks, explicit)
	for _, node := range seedNodes {
		if node.Location.File != "" {
			seedFiles[filepath.ToSlash(node.Location.File)] = true
		}
		neighbors, err := s.neighbors(ctx, node, "both", familyEdgeTypes, 80)
		if err != nil {
			continue
		}
		for _, item := range neighbors {
			if seen[item.node.ID] {
				continue
			}
			if filepath.ToSlash(item.node.Location.File) == filepath.ToSlash(node.Location.File) && item.edge.Type == "DEFINES_METHOD" {
				continue
			}
			seen[item.node.ID] = true
			result.Impacted = append(result.Impacted, api.ImpactedNode{
				Node: item.node, Hop: 1, Reason: reasonForEdge(item.edge.Type),
			})
		}
	}
	s.appendCoChange(ctx, req.Project, files, seen, &result)

	callSeeds := append([]string(nil), req.Symbols...)
	for _, file := range files {
		spans, filter := hunkFilter(hunks, file, explicit[file])
		qns, err := s.fileCallSeeds(ctx, req.Project, file, spans, filter)
		if err != nil {
			return api.ImpactResult{}, err
		}
		callSeeds = append(callSeeds, qns...)
	}
	for _, seed := range uniqueSlash(callSeeds) {
		trace, err := s.Trace(ctx, api.TraceRequest{
			Project: req.Project, Start: seed, Direction: "incoming", EdgeTypes: req.EdgeTypes,
			Depth: req.Depth, Limit: req.Limit,
		})
		if err != nil {
			continue
		}
		for _, path := range trace.Paths {
			if len(path) < 2 {
				continue
			}
			step := path[len(path)-1]
			if seen[step.Node.ID] {
				continue
			}
			seen[step.Node.ID] = true
			reason := reasonCaller
			if step.Via != nil {
				reason = reasonForEdge(step.Via.Type)
			}
			result.Impacted = append(result.Impacted, api.ImpactedNode{Node: step.Node, Hop: step.Hop, Reason: reason})
			result.Truncated = result.Truncated || trace.Truncated
		}
	}

	sort.Slice(result.Impacted, func(i, j int) bool {
		if result.Impacted[i].Hop != result.Impacted[j].Hop {
			return result.Impacted[i].Hop < result.Impacted[j].Hop
		}
		ri, rj := reasonRank[result.Impacted[i].Reason], reasonRank[result.Impacted[j].Reason]
		if ri != rj {
			return ri < rj
		}
		return result.Impacted[i].QualifiedName < result.Impacted[j].QualifiedName
	})
	result.ImpactedFiles = rollupImpactedFiles(result.Impacted, seedFiles)
	if len(result.ImpactedFiles) > impactFileCap {
		result.ImpactedFiles = result.ImpactedFiles[:impactFileCap]
		result.Truncated = true
	}
	for _, f := range result.ImpactedFiles {
		result.ImpactedModules[filepath.Dir(f.Path)] += f.Symbols
	}
	result.Total = len(result.Impacted)
	result.Coverage, _ = s.Coverage(ctx, api.CoverageRequest{Project: req.Project})
	return result, nil
}

func (s *Store) fileCallSeeds(ctx context.Context, project, file string, spans []lineSpan, filter bool) ([]string, error) {
	file = filepath.ToSlash(file)
	rows, err := s.db.QueryContext(ctx, `SELECT `+nodeColumns+` FROM nodes WHERE project=? AND (file_path=? OR file_path LIKE ?) ORDER BY qualified_name LIMIT 80`, project, file, "%/"+file)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type hit struct {
		qn    string
		label string
	}
	var hits []hit
	for rows.Next() {
		node, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(node.QualifiedName) == "" {
			continue
		}
		if !nodeHitsSpans(node, spans, filter) {
			continue
		}
		hits = append(hits, hit{qn: node.QualifiedName, label: node.Label})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(hits, func(i, j int) bool {
		pi, pj := callSeedPrefer(hits[i].label), callSeedPrefer(hits[j].label)
		if pi != pj {
			return pi < pj
		}
		return hits[i].qn < hits[j].qn
	})
	if len(hits) > impactCallSeedCap {
		hits = hits[:impactCallSeedCap]
	}
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.qn)
	}
	return out, nil
}

func callSeedPrefer(label string) int {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "file":
		return 0
	case "class", "interface", "struct", "trait", "enum", "type":
		return 1
	default:
		return 2
	}
}

func (s *Store) impactSeedNodes(ctx context.Context, project string, seeds, files []string, hunks map[string][]lineSpan, wholeFile map[string]bool) []api.Node {
	seen := map[int64]bool{}
	var nodes []api.Node
	for _, seed := range seeds {
		found, err := s.findNodes(ctx, project, seed, 8)
		if err != nil {
			continue
		}
		for _, n := range found {
			if seen[n.ID] {
				continue
			}
			seen[n.ID] = true
			nodes = append(nodes, n)
		}
	}
	for _, file := range files {
		found, err := s.nodesInFile(ctx, project, file)
		if err != nil {
			continue
		}
		spans, filter := hunkFilter(hunks, file, wholeFile[filepath.ToSlash(file)])
		for _, n := range found {
			if seen[n.ID] {
				continue
			}
			if !nodeHitsSpans(n, spans, filter) {
				continue
			}
			seen[n.ID] = true
			nodes = append(nodes, n)
		}
	}
	return nodes
}

func (s *Store) nodesInFile(ctx context.Context, project, file string) ([]api.Node, error) {
	file = filepath.ToSlash(file)
	rows, err := s.db.QueryContext(ctx, `SELECT `+nodeColumns+` FROM nodes WHERE project=? AND (file_path=? OR file_path LIKE ?) ORDER BY qualified_name LIMIT 80`, project, file, "%/"+file)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []api.Node
	for rows.Next() {
		node, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, node)
	}
	return out, rows.Err()
}

func (s *Store) appendCoChange(ctx context.Context, project string, files []string, seen map[int64]bool, result *api.ImpactResult) {
	if result == nil || seen == nil {
		return
	}
	for _, node := range s.fileNodes(ctx, project, files) {
		if ctx.Err() != nil {
			return
		}
		neighbors, err := s.neighbors(ctx, node, "both", coChangeEdgeTypes, 40)
		if err != nil {
			continue
		}
		for _, item := range neighbors {
			if seen[item.node.ID] {
				continue
			}
			seen[item.node.ID] = true
			result.Impacted = append(result.Impacted, api.ImpactedNode{
				Node: item.node, Hop: 1, Reason: reasonCoChange,
			})
		}
	}
}

func (s *Store) fileNodes(ctx context.Context, project string, files []string) []api.Node {
	var out []api.Node
	for _, file := range files {
		file = filepath.ToSlash(file)
		rows, err := s.db.QueryContext(ctx, `SELECT `+nodeColumns+` FROM nodes WHERE project=? AND label='File' AND (file_path=? OR file_path LIKE ?) LIMIT 4`, project, file, "%/"+file)
		if err != nil {
			continue
		}
		for rows.Next() {
			node, err := scanNode(rows)
			if err != nil {
				break
			}
			out = append(out, node)
		}
		_ = rows.Close()
	}
	return out
}

func (s *Store) projectRootPath(ctx context.Context, project string) (string, error) {
	var root string
	err := s.db.QueryRowContext(ctx, `SELECT root_path FROM projects WHERE name=?`, project).Scan(&root)
	return root, err
}

func reasonForEdge(edgeType string) string {
	switch edgeType {
	case "INHERITS", "IMPLEMENTS":
		return reasonImpl
	case "OVERRIDE":
		return reasonOverride
	case "DEFINES_METHOD", "DEFINES":
		return reasonSibling
	case "FILE_CHANGES_WITH":
		return reasonCoChange
	default:
		return reasonCaller
	}
}

func rollupImpactedFiles(nodes []api.ImpactedNode, seedFiles map[string]bool) []api.ImpactedFile {
	type acc struct {
		symbols int
		reasons map[string]bool
		hop     int
	}
	byFile := map[string]*acc{}
	var order []string
	for _, n := range nodes {
		path := filepath.ToSlash(n.Location.File)
		if path == "" || seedFiles[path] {
			continue
		}
		item, ok := byFile[path]
		if !ok {
			item = &acc{reasons: map[string]bool{}, hop: n.Hop}
			byFile[path] = item
			order = append(order, path)
		}
		item.symbols++
		if n.Reason != "" {
			item.reasons[n.Reason] = true
		}
		if n.Hop < item.hop {
			item.hop = n.Hop
		}
	}
	seeds := make([]string, 0, len(seedFiles))
	for p := range seedFiles {
		seeds = append(seeds, p)
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := byFile[order[i]], byFile[order[j]]
		na := NearbyImpactPath(order[i], seeds, reasonList(a.reasons))
		nb := NearbyImpactPath(order[j], seeds, reasonList(b.reasons))
		if na != nb {
			return na
		}
		ra, rb := bestReasonRank(a.reasons), bestReasonRank(b.reasons)
		if ra != rb {
			return ra < rb
		}
		if a.hop != b.hop {
			return a.hop < b.hop
		}
		return order[i] < order[j]
	})
	out := make([]api.ImpactedFile, 0, len(order))
	for _, path := range order {
		item := byFile[path]
		reasons := make([]string, 0, len(item.reasons))
		for r := range item.reasons {
			reasons = append(reasons, r)
		}
		sort.Slice(reasons, func(i, j int) bool {
			return reasonRank[reasons[i]] < reasonRank[reasons[j]]
		})
		out = append(out, api.ImpactedFile{Path: path, Symbols: item.symbols, Reasons: reasons})
	}
	return out
}

func reasonList(reasons map[string]bool) []string {
	out := make([]string, 0, len(reasons))
	for r := range reasons {
		out = append(out, r)
	}
	return out
}

// NearbyImpactPath is true when path is local to a seed file: same
// directory, a file in the parent directory, a file in a sibling
// directory under that parent (sql/query.py → models/expressions.py),
// or a non-caller reason sharing at least two directory components.
func NearbyImpactPath(path string, seeds []string, reasons []string) bool {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "" {
		return false
	}
	family := false
	for _, r := range reasons {
		if r != "" && r != reasonCaller {
			family = true
			break
		}
	}
	for _, seed := range seeds {
		seed = filepath.ToSlash(strings.TrimSpace(seed))
		if seed == "" {
			continue
		}
		if slashDir(path) == slashDir(seed) {
			return true
		}
		if slashDir(path) == parentDir(seed) {
			return true
		}
		if pd := parentDir(seed); pd != "" && parentDir(path) == pd {
			return true
		}
		if family && sharedDirPrefix(path, seed) >= 2 {
			return true
		}
	}
	return false
}

func slashDir(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "" {
		return ""
	}
	d := filepath.ToSlash(filepath.Dir(path))
	if d == "." || d == "/" {
		return ""
	}
	return strings.TrimSuffix(d, "/")
}

func parentDir(path string) string {
	return slashDir(slashDir(path))
}

func sharedDirPrefix(a, b string) int {
	ap := dirParts(a)
	bp := dirParts(b)
	n := 0
	for n < len(ap) && n < len(bp) && ap[n] == bp[n] {
		n++
	}
	return n
}

func dirParts(path string) []string {
	d := slashDir(path)
	if d == "" {
		return nil
	}
	return strings.Split(d, "/")
}

func bestReasonRank(reasons map[string]bool) int {
	best := 99
	for r := range reasons {
		if n, ok := reasonRank[r]; ok && n < best {
			best = n
		}
	}
	return best
}

func normalizeImpactFile(repoRoot, file string) string {
	file = strings.TrimSpace(file)
	if file == "" {
		return ""
	}
	file = filepath.ToSlash(file)
	if repoRoot != "" && filepath.IsAbs(file) {
		if rel, err := filepath.Rel(repoRoot, file); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return file
}

func uniqueSlash(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range in {
		v = filepath.ToSlash(strings.TrimSpace(v))
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func gitChangedFiles(ctx context.Context, repoRoot, base string) (mergeBase string, files []string) {
	repoRoot = strings.TrimSpace(repoRoot)
	base = strings.TrimSpace(base)
	if repoRoot == "" || base == "" {
		return "", nil
	}
	mb, err := gitOutput(ctx, repoRoot, "merge-base", "HEAD", base)
	if err != nil || mb == "" {
		mb, err = gitOutput(ctx, repoRoot, "rev-parse", "--verify", base)
		if err != nil {
			return "", nil
		}
	}
	mergeBase = mb
	names := func(args ...string) {
		out, err := gitOutput(ctx, repoRoot, args...)
		if err != nil || out == "" {
			return
		}
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				files = append(files, filepath.ToSlash(line))
			}
		}
	}
	names("diff", "--name-only", mergeBase+"...HEAD")
	names("diff", "--name-only")
	names("diff", "--name-only", "--cached")
	names("ls-files", "--others", "--exclude-standard")
	return mergeBase, uniqueSlash(files)
}

type lineSpan struct{ start, end int }

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

func gitChangedLineSpans(ctx context.Context, repoRoot, mergeBase string) map[string][]lineSpan {
	repoRoot = strings.TrimSpace(repoRoot)
	mergeBase = strings.TrimSpace(mergeBase)
	if repoRoot == "" || mergeBase == "" {
		return nil
	}
	var parts []string
	if out, err := gitOutput(ctx, repoRoot, "diff", "-U0", mergeBase+"...HEAD"); err == nil && out != "" {
		parts = append(parts, out)
	}
	if out, err := gitOutput(ctx, repoRoot, "diff", "-U0"); err == nil && out != "" {
		parts = append(parts, out)
	}
	if out, err := gitOutput(ctx, repoRoot, "diff", "-U0", "--cached"); err == nil && out != "" {
		parts = append(parts, out)
	}
	hunks := parseUnifiedDiffHunks(strings.Join(parts, "\n"))
	if len(hunks) == 0 {
		return nil
	}
	return hunks
}

func parseUnifiedDiffHunks(text string) map[string][]lineSpan {
	out := map[string][]lineSpan{}
	file := ""
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			file = ""
			continue
		}
		if strings.HasPrefix(line, "+++ ") {
			rest := strings.TrimSpace(strings.TrimPrefix(line, "+++ "))
			if i := strings.IndexAny(rest, "\t"); i >= 0 {
				rest = rest[:i]
			}
			rest = strings.TrimPrefix(rest, "b/")
			if rest == "/dev/null" || rest == "" {
				file = ""
				continue
			}
			file = filepath.ToSlash(rest)
			continue
		}
		m := hunkHeader.FindStringSubmatch(line)
		if m == nil || file == "" {
			continue
		}
		newStart, _ := strconv.Atoi(m[3])
		newCount := 1
		if m[4] != "" {
			newCount, _ = strconv.Atoi(m[4])
		}
		var sp lineSpan
		if newCount == 0 {
			if newStart < 1 {
				newStart = 1
			}
			sp = lineSpan{start: newStart, end: newStart}
		} else {
			sp = lineSpan{start: newStart, end: newStart + newCount - 1}
		}
		out[file] = append(out[file], sp)
	}
	return out
}

func hunkFilter(hunks map[string][]lineSpan, file string, wholeFile bool) ([]lineSpan, bool) {
	if wholeFile || hunks == nil {
		return nil, false
	}
	file = filepath.ToSlash(file)
	if sp, ok := hunks[file]; ok {
		return sp, true
	}
	for p, sp := range hunks {
		if strings.HasSuffix(file, "/"+p) || strings.HasSuffix(p, "/"+file) {
			return sp, true
		}
	}
	return nil, false
}

func nodeHitsSpans(n api.Node, spans []lineSpan, filter bool) bool {
	if !filter {
		return true
	}
	start, end := n.Location.StartLine, n.Location.EndLine
	if start <= 0 {
		return false
	}
	if end < start {
		end = start
	}
	if len(spans) == 0 {
		return false
	}
	for _, sp := range spans {
		if start <= sp.end && end >= sp.start {
			return true
		}
	}
	return false
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
