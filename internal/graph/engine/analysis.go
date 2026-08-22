package engine

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

type rowScanner interface {
	Scan(...any) error
}

func scanNode(row rowScanner) (api.Node, error) {
	var node api.Node
	var properties string
	err := row.Scan(&node.ID, &node.Project, &node.Label, &node.Name, &node.QualifiedName,
		&node.Location.File, &node.Location.StartLine, &node.Location.StartColumn,
		&node.Location.EndLine, &node.Location.EndColumn, &properties)
	if err == nil {
		_ = json.Unmarshal([]byte(properties), &node.Properties)
	}
	return node, err
}

const nodeColumns = `id,project,label,name,qualified_name,file_path,start_line,start_column,end_line,end_column,properties`

func (s *Store) findNodes(ctx context.Context, project, identity string, limit int) ([]api.Node, error) {
	if limit <= 0 {
		limit = 20
	}
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return nil, nil
	}
	queryExact := `SELECT ` + nodeColumns + ` FROM nodes WHERE qualified_name=?`
	argsExact := []any{identity}
	if project != "" {
		queryExact += ` AND project=?`
		argsExact = append(argsExact, project)
	}
	queryExact += ` ORDER BY qualified_name LIMIT ?`
	argsExact = append(argsExact, limit)
	nodes, err := s.scanFindNodes(ctx, queryExact, argsExact...)
	if err != nil || len(nodes) > 0 {
		return nodes, err
	}
	if !snippetLooksLikePath(identity) {
		querySuffix := `SELECT ` + nodeColumns + ` FROM nodes WHERE qualified_name LIKE ? ESCAPE '\'`
		argsSuffix := []any{"%." + escapeLike(identity)}
		if project != "" {
			querySuffix += ` AND project=?`
			argsSuffix = append(argsSuffix, project)
		}
		querySuffix += ` ORDER BY qualified_name LIMIT ?`
		argsSuffix = append(argsSuffix, limit)
		nodes, err = s.scanFindNodes(ctx, querySuffix, argsSuffix...)
		if err != nil || len(nodes) > 0 {
			return nodes, err
		}
	}
	queryName := `SELECT ` + nodeColumns + ` FROM nodes WHERE name=?`
	argsName := []any{identity}
	if project != "" {
		queryName += ` AND project=?`
		argsName = append(argsName, project)
	}
	queryName += ` ORDER BY qualified_name LIMIT ?`
	argsName = append(argsName, limit)
	return s.scanFindNodes(ctx, queryName, argsName...)
}

func (s *Store) scanFindNodes(ctx context.Context, query string, args ...any) ([]api.Node, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []api.Node
	for rows.Next() {
		node, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, node)
	}
	return result, rows.Err()
}

func snippetLooksLikePath(identity string) bool {
	if strings.ContainsAny(identity, `/\`) {
		return true
	}
	switch strings.ToLower(filepath.Ext(identity)) {
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".go", ".py", ".rs", ".java",
		".json", ".yaml", ".yml", ".toml", ".md", ".css", ".html":
		return true
	default:
		return false
	}
}

type neighbor struct {
	node api.Node
	edge api.Edge
}

func (s *Store) neighbors(ctx context.Context, node api.Node, direction string, edgeTypes []string, limit int) ([]neighbor, error) {
	if limit <= 0 {
		limit = 100
	}
	directions := []string{direction}
	if direction == "both" || direction == "" {
		directions = []string{"outgoing", "incoming"}
	}
	var result []neighbor
	for _, current := range directions {
		joinID, matchID := "e.target_id", "e.source_id"
		if current == "incoming" || current == "inbound" {
			joinID, matchID = "e.source_id", "e.target_id"
		}
		query := `SELECT n.` + strings.ReplaceAll(nodeColumns, ",", ",n.") +
			`,e.id,e.project,e.source_id,e.target_id,e.type,e.properties,e.evidence
			 FROM edges e JOIN nodes n ON n.id=` + joinID + ` WHERE ` + matchID + `=? AND e.project=?`
		args := []any{node.ID, node.Project}
		if len(edgeTypes) > 0 {
			placeholders := make([]string, len(edgeTypes))
			for i, edgeType := range edgeTypes {
				placeholders[i] = "?"
				args = append(args, edgeType)
			}
			query += ` AND e.type IN (` + strings.Join(placeholders, ",") + `)`
		}
		query += ` ORDER BY e.type,n.qualified_name LIMIT ?`
		args = append(args, limit)
		rows, err := s.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var item neighbor
			var nodeProperties, edgeProperties, evidence string
			if err := rows.Scan(&item.node.ID, &item.node.Project, &item.node.Label, &item.node.Name,
				&item.node.QualifiedName, &item.node.Location.File, &item.node.Location.StartLine,
				&item.node.Location.StartColumn, &item.node.Location.EndLine, &item.node.Location.EndColumn,
				&nodeProperties, &item.edge.ID, &item.edge.Project, &item.edge.SourceID, &item.edge.TargetID,
				&item.edge.Type, &edgeProperties, &evidence); err != nil {
				rows.Close()
				return nil, err
			}
			_ = json.Unmarshal([]byte(nodeProperties), &item.node.Properties)
			_ = json.Unmarshal([]byte(edgeProperties), &item.edge.Properties)
			if evidence != "" && evidence != "null" && evidence != "{}" {
				_ = json.Unmarshal([]byte(evidence), &item.edge.Evidence)
			}
			result = append(result, item)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (s *Store) Trace(ctx context.Context, req api.TraceRequest) (api.TraceResult, error) {
	if req.Project == "" {
		req.Project, _ = s.defaultProject(ctx)
	}
	starts, err := s.findNodes(ctx, req.Project, req.Start, 40)
	if err != nil {
		return api.TraceResult{}, err
	}
	if len(starts) == 0 {
		return api.TraceResult{}, fmt.Errorf("start node not found: %s", req.Start)
	}
	picked, leftovers, amb := resolveGraphIdentity(req.Start, starts)
	if amb || picked == nil {
		suggestions := make([]api.Node, 0, len(starts))
		for _, node := range starts {
			slim := node
			slim.Properties = nil
			suggestions = append(suggestions, slim)
		}
		return api.TraceResult{
			Status:      "ambiguous",
			Message:     fmt.Sprintf("%q matches %d nodes; retry with a qualified_name", req.Start, len(starts)),
			Suggestions: suggestions,
		}, nil
	}
	starts = []api.Node{*picked}
	depth := req.Depth
	if depth <= 0 {
		depth = 3
	}
	if depth > 12 {
		depth = 12
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	targets := map[int64]bool{}
	if req.Target != "" {
		nodes, err := s.findNodes(ctx, req.Project, req.Target, 50)
		if err != nil {
			return api.TraceResult{}, err
		}
		for _, node := range nodes {
			targets[node.ID] = true
		}
	}
	type state struct {
		node api.Node
		path []api.TraceStep
	}
	queue := make([]state, 0, len(starts))
	visited := map[int64]bool{}
	for _, start := range starts {
		queue = append(queue, state{node: start, path: []api.TraceStep{{Node: start, Hop: 0}}})
		visited[start.ID] = true
	}
	direction := strings.ToLower(strings.TrimSpace(req.Direction))
	switch direction {
	case "incoming", "inbound":
		direction = "incoming"
	case "both":
		direction = "both"
	default:
		direction = "outgoing"
	}
	if len(req.EdgeTypes) == 0 && (direction == "both" || direction == "incoming") {
		req.EdgeTypes = []string{"CALLS", "USAGE", "CONFIGURES"}
	}
	req.Direction = direction
	result := api.TraceResult{Direction: direction}
	startIDs := map[int64]bool{}
	for _, start := range starts {
		startIDs[start.ID] = true
	}
	for len(queue) > 0 && result.Visited < limit {
		current := queue[0]
		queue = queue[1:]
		result.Visited++
		hop := len(current.path) - 1
		if hop >= depth {
			continue
		}
		neighbors, err := s.neighbors(ctx, current.node, req.Direction, req.EdgeTypes, limit-result.Visited+1)
		if err != nil {
			return api.TraceResult{}, err
		}
		for _, next := range neighbors {
			if visited[next.node.ID] {
				continue
			}
			if skipDataLanguageVariable(next.node) && !startIDs[next.node.ID] {
				continue
			}
			visited[next.node.ID] = true
			path := append(append([]api.TraceStep(nil), current.path...), api.TraceStep{
				Node: next.node, Via: &next.edge, Evidence: next.edge.Evidence, Hop: hop + 1,
			})
			if len(targets) == 0 || targets[next.node.ID] {
				result.Paths = append(result.Paths, path)
			}
			queue = append(queue, state{node: next.node, path: path})
		}
	}
	result.Truncated = len(queue) > 0
	includeUnresolved := req.Direction == "" || req.Direction == "outgoing" || req.Direction == "both"
	for qualifiedName := range visitedQualifiedNames(result.Paths, starts) {
		if !includeUnresolved {
			break
		}
		rows, err := s.db.QueryContext(ctx, `SELECT target_text,type,properties,evidence
			FROM unresolved_relationships WHERE project=? AND source_qn=? ORDER BY type,target_text`, req.Project, qualifiedName)
		if err != nil {
			return api.TraceResult{}, err
		}
		for rows.Next() {
			item := api.UnresolvedRelationship{Project: req.Project, Source: qualifiedName}
			var properties, evidence string
			if err := rows.Scan(&item.TargetText, &item.Type, &properties, &evidence); err != nil {
				rows.Close()
				return api.TraceResult{}, err
			}
			if len(req.EdgeTypes) > 0 && !containsString(req.EdgeTypes, item.Type) {
				continue
			}
			_ = json.Unmarshal([]byte(properties), &item.Properties)
			if evidence != "" && evidence != "null" && evidence != "{}" {
				_ = json.Unmarshal([]byte(evidence), &item.Evidence)
			}
			result.Unresolved = append(result.Unresolved, item)
		}
		if err := rows.Close(); err != nil {
			return api.TraceResult{}, err
		}
	}
	coverage, _ := s.Coverage(ctx, api.CoverageRequest{Project: req.Project})
	result.Coverage = coverage
	if result.UnresolvedCalls == 0 {
		result.UnresolvedCalls = len(result.Unresolved)
	}
	if (req.Direction == "incoming" || req.Direction == "inbound") && len(result.Paths) == 0 {
		qn := starts[0].QualifiedName
		name := starts[0].Name
		var n int
		_ = s.db.QueryRowContext(ctx, `SELECT count(*) FROM unresolved_relationships WHERE project=? AND (source_qn=? OR target_text=? OR target_text LIKE ? ESCAPE '\')`,
			req.Project, qn, name, "%."+escapeLike(name)).Scan(&n)
		if n > result.UnresolvedCalls {
			result.UnresolvedCalls = n
		}
	}
	if len(leftovers) > 0 {
		result.Suggestions = slimIdentityNodes(leftovers)
	}
	return result, nil
}

func callableLabel(label string) bool {
	switch label {
	case "Function", "Method", "Class", "Constructor":
		return true
	default:
		return false
	}
}

func firstNodeWithLabel(nodes []api.Node, labels ...string) *api.Node {
	want := map[string]bool{}
	for _, label := range labels {
		want[label] = true
	}
	for i := range nodes {
		if want[nodes[i].Label] {
			return &nodes[i]
		}
	}
	return nil
}

func slimIdentityNodes(nodes []api.Node) []api.Node {
	out := make([]api.Node, 0, len(nodes))
	for _, node := range nodes {
		slim := node
		slim.Properties = nil
		out = append(out, slim)
	}
	return out
}

func othersExcept(nodes []api.Node, picked *api.Node) []api.Node {
	if picked == nil {
		return slimIdentityNodes(nodes)
	}
	out := make([]api.Node, 0, len(nodes))
	for _, node := range nodes {
		if node.ID != 0 && picked.ID != 0 && node.ID == picked.ID {
			continue
		}
		if node.QualifiedName == picked.QualifiedName && node.Label == picked.Label {
			continue
		}
		slim := node
		slim.Properties = nil
		out = append(out, slim)
	}
	return out
}

// resolveGraphIdentity picks one node from findNodes hits.
// Path-shaped identities prefer File then Module. Symbol-shaped identities
// prefer a single Function/Method/Class. Two callables of equal rank stay ambiguous.
func resolveGraphIdentity(identity string, nodes []api.Node) (picked *api.Node, leftovers []api.Node, ambiguous bool) {
	if len(nodes) == 0 {
		return nil, nil, false
	}
	if exact := exactQualifiedMatch(identity, nodes); exact != nil {
		return exact, othersExcept(nodes, exact), false
	}
	if len(nodes) == 1 {
		return &nodes[0], nil, false
	}
	if snippetLooksLikePath(identity) {
		if n := firstNodeWithLabel(nodes, "File"); n != nil {
			return n, othersExcept(nodes, n), false
		}
		if n := firstNodeWithLabel(nodes, "Module"); n != nil {
			return n, othersExcept(nodes, n), false
		}
	}
	var callables []api.Node
	for _, node := range nodes {
		if callableLabel(node.Label) {
			callables = append(callables, node)
		}
	}
	if len(callables) == 1 {
		return &callables[0], othersExcept(nodes, &callables[0]), false
	}
	if len(callables) > 1 {
		return nil, slimIdentityNodes(callables), true
	}
	if n := firstNodeWithLabel(nodes, "File"); n != nil {
		return n, othersExcept(nodes, n), false
	}
	if n := firstNodeWithLabel(nodes, "Module"); n != nil {
		return n, othersExcept(nodes, n), false
	}
	return nil, slimIdentityNodes(nodes), true
}

func ambiguousTraceStart(identity string, nodes []api.Node) bool {
	_, _, amb := resolveGraphIdentity(identity, nodes)
	return amb
}

func exactQualifiedMatch(identity string, nodes []api.Node) *api.Node {
	for i := range nodes {
		if nodes[i].QualifiedName == identity {
			return &nodes[i]
		}
	}
	return nil
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func visitedQualifiedNames(paths [][]api.TraceStep, starts []api.Node) map[string]bool {
	result := make(map[string]bool)
	for _, start := range starts {
		result[start.QualifiedName] = true
	}
	for _, path := range paths {
		for _, step := range path {
			result[step.Node.QualifiedName] = true
		}
	}
	return result
}

func (s *Store) Query(ctx context.Context, req api.QueryRequest) (api.QueryResult, error) {
	if req.Project == "" {
		req.Project, _ = s.defaultProject(ctx)
	}
	depth := req.Depth
	if depth <= 0 {
		depth = 2
	}
	budget := req.Budget
	if budget <= 0 {
		budget = queryDefaultBudget
	}
	maxChars := budget * queryCharsPerToken

	terms := queryTerms(req.Question, req.Terms)
	candidates, err := s.querySeedCandidates(ctx, req.Project, req.Question, terms)
	if err != nil {
		return api.QueryResult{}, err
	}
	seeded := scoreQuerySeeds(candidates, terms)
	if len(seeded.seeds) == 0 {
		return api.QueryResult{
			Text:   "No matching nodes found.",
			Budget: api.Budget{RequestedTokens: budget, ReturnedTokens: 1, Truncated: false},
		}, nil
	}

	communityByID, _ := s.communityLabelByNodeID(ctx, req.Project)
	degrees, err := s.nodeDegrees(ctx, req.Project)
	if err != nil {
		return api.QueryResult{}, err
	}

	result := api.QueryResult{Seeds: seeded.seeds}
	seenEdges := map[int64]bool{}
	nodesByID := map[int64]queryNodeHit{}
	seedOrder := make([]int64, 0, len(seeded.seeds))
	seedLabels := make([]string, 0, len(seeded.seeds))
	seedIDs := map[int64]bool{}
	for _, seed := range seeded.seeds {
		seedOrder = append(seedOrder, seed.ID)
		seedLabels = append(seedLabels, queryNodeDisplayName(seed.Node))
		seedIDs[seed.ID] = true
		nodesByID[seed.ID] = queryNodeHit{node: seed.Node, hop: 0, seed: true, deg: degrees[seed.ID]}
		result.Nodes = append(result.Nodes, seed.Node)
	}

	expand := seeded.primary
	if len(expand) == 0 {
		n := seedMaxK
		if n > len(seeded.seeds) {
			n = len(seeded.seeds)
		}
		expand = seeded.seeds[:n]
	} else if len(expand) > seedMaxK {
		expand = expand[:seedMaxK]
	}

	edgeLines, err := s.queryExpandBFS(ctx, expand, seedIDs, depth, degrees, nodesByID, seenEdges, &result.Edges)
	if err != nil {
		return api.QueryResult{}, err
	}

	orderedNodes := orderQueryNodes(seedOrder, nodesByID)
	result.Nodes = result.Nodes[:0]
	for _, hit := range orderedNodes {
		result.Nodes = append(result.Nodes, hit.node)
	}

	var body strings.Builder
	for _, hit := range orderedNodes {
		body.WriteString(formatQueryNodeLine(hit, communityByID))
	}
	for _, line := range edgeLines {
		body.WriteString(line)
		body.WriteByte('\n')
	}

	header := fmt.Sprintf("Traversal: BFS depth=%d | Start: %v | %d nodes found\n\n", depth, seedLabels, len(orderedNodes))
	output, truncated := applyQueryBudget(header, body.String(), len(seedOrder), orderedNodes, communityByID, budget, maxChars)

	result.Text = output
	result.Budget.RequestedTokens = budget
	result.Budget.ReturnedTokens = (len(output) + queryCharsPerToken - 1) / queryCharsPerToken
	result.Budget.Truncated = truncated
	result.Page.Total = len(result.Nodes)
	result.Page.Truncated = truncated
	return result, nil
}

func (s *Store) communityLabelByNodeID(ctx context.Context, project string) (map[int64]string, error) {
	communities, err := s.architectureCommunities(ctx, project, "")
	if err != nil || len(communities) == 0 {
		return map[int64]string{}, err
	}
	// Rebuild membership cheaply from the same CALLS Leiden path used by architecture.
	where := []string{"project=?", "label IN ('Function','Method','Class','Struct','Interface','Enum','Type','Trait')"}
	args := []any{project}
	rows, err := s.db.QueryContext(ctx, `SELECT id,qualified_name FROM nodes WHERE `+strings.Join(where, " AND ")+` ORDER BY id LIMIT 8000`, args...)
	if err != nil {
		return map[int64]string{}, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		var qn string
		if err := rows.Scan(&id, &qn); err != nil {
			rows.Close()
			return map[int64]string{}, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return map[int64]string{}, err
	}
	edgeRows, err := s.db.QueryContext(ctx, `SELECT source_id,target_id FROM edges WHERE project=? AND type='CALLS' ORDER BY id`, project)
	if err != nil {
		return map[int64]string{}, err
	}
	var edges []LeidenEdge
	for edgeRows.Next() {
		var sourceID, targetID int64
		if err := edgeRows.Scan(&sourceID, &targetID); err != nil {
			edgeRows.Close()
			return map[int64]string{}, err
		}
		edges = append(edges, LeidenEdge{Source: sourceID, Target: targetID})
	}
	if err := edgeRows.Close(); err != nil {
		return map[int64]string{}, err
	}
	membership := Leiden(ids, edges, 1)
	nameByCommunity := map[int]string{}
	for _, community := range communities {
		nameByCommunity[int(community.ID)] = community.Name
	}
	out := map[int64]string{}
	for _, item := range membership {
		if name := nameByCommunity[item.Community]; name != "" {
			out[item.NodeID] = name
		}
	}
	return out, nil
}

func queryEdgePreferred(edgeType string) bool {
	switch edgeType {
	case "CALLS", "CONFIGURES", "DEFINES", "DEFINES_METHOD", "USAGE":
		return true
	default:
		return false
	}
}

func querySnippetSeeds(matches []api.RankedNode, limit int) []api.RankedNode {
	if limit <= 0 || len(matches) == 0 {
		return nil
	}
	var callables, others []api.RankedNode
	for _, match := range matches {
		switch match.Label {
		case "Method", "Function", "Constructor":
			callables = append(callables, match)
		default:
			others = append(others, match)
		}
	}
	out := append(callables, others...)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (s *Store) Coverage(ctx context.Context, req api.CoverageRequest) (api.Coverage, error) {
	if req.Project == "" {
		req.Project, _ = s.defaultProject(ctx)
	}
	var coverage api.Coverage
	var recorded string
	err := s.db.QueryRowContext(ctx, `SELECT generation,index_mode,recorded_at,recording_status,hash_records_complete
		FROM index_coverage_meta WHERE project=?`, req.Project).Scan(&coverage.Generation, &coverage.IndexMode,
		&recorded, &coverage.RecordingStatus, &coverage.HashRecordsComplete)
	if errors.Is(err, sql.ErrNoRows) {
		coverage.Status = "coverage_unavailable"
		return coverage, nil
	}
	if err != nil {
		return api.Coverage{}, err
	}
	if parsed, err := time.Parse(time.RFC3339Nano, recorded); err == nil {
		coverage.RecordedAt = &parsed
	}
	query := `SELECT rel_path,kind,detail FROM index_coverage WHERE project=?`
	args := []any{req.Project}
	if req.Scope != "" {
		query += ` AND rel_path LIKE ? ESCAPE '\'`
		args = append(args, escapeLike(filepath.ToSlash(req.Scope))+"%")
	}
	if len(req.Paths) > 0 {
		placeholders := make([]string, len(req.Paths))
		for i, path := range req.Paths {
			placeholders[i] = "?"
			args = append(args, filepath.ToSlash(path))
		}
		query += ` AND rel_path IN (` + strings.Join(placeholders, ",") + `)`
	}
	query += ` ORDER BY rel_path,kind`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return api.Coverage{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var row api.CoverageRow
		if err := rows.Scan(&row.Path, &row.Kind, &row.Detail); err != nil {
			return api.Coverage{}, err
		}
		coverage.Rows = append(coverage.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return api.Coverage{}, err
	}
	if len(req.Paths) > 0 {
		var root string
		_ = s.db.QueryRowContext(ctx, `SELECT root_path FROM projects WHERE name=?`, req.Project).Scan(&root)
		rowIndexes := map[string][]int{}
		for index := range coverage.Rows {
			rowIndexes[coverage.Rows[index].Path] = append(rowIndexes[coverage.Rows[index].Path], index)
		}
		for _, requested := range req.Paths {
			path := filepath.ToSlash(requested)
			var storedHash string
			hashErr := s.db.QueryRowContext(ctx, `SELECT sha256 FROM file_hashes WHERE project=? AND rel_path=?`, req.Project, path).Scan(&storedHash)
			freshness := coverageFreshness(root, path, storedHash, hashErr == nil)
			indexes := rowIndexes[path]
			if len(indexes) == 0 {
				kind := "indexed_no_recorded_gap"
				if hashErr != nil {
					kind = "not_indexed"
				}
				coverage.Rows = append(coverage.Rows, api.CoverageRow{Path: path, Kind: kind, Freshness: freshness})
				continue
			}
			for _, index := range indexes {
				coverage.Rows[index].Freshness = freshness
			}
		}
		sort.SliceStable(coverage.Rows, func(i, j int) bool {
			if coverage.Rows[i].Path != coverage.Rows[j].Path {
				return coverage.Rows[i].Path < coverage.Rows[j].Path
			}
			return coverage.Rows[i].Kind < coverage.Rows[j].Kind
		})
	}
	hasGap := false
	for _, row := range coverage.Rows {
		if row.Kind != "indexed_no_recorded_gap" {
			hasGap = true
			break
		}
	}
	if coverage.RecordingStatus == "complete" && !hasGap {
		coverage.Status = "complete"
	} else {
		coverage.Status = "partial"
	}
	coverage.Total = len(coverage.Rows)
	return coverage, nil
}

func coverageFreshness(root, rel, storedHash string, indexed bool) string {
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if os.IsNotExist(err) {
		return "deleted"
	}
	if err != nil {
		return "unreadable"
	}
	if !indexed {
		return "not_indexed"
	}
	hash := sha256.Sum256(body)
	if hex.EncodeToString(hash[:]) != storedHash {
		return "modified"
	}
	return "fresh"
}

func (s *Store) Snippet(ctx context.Context, req api.SnippetRequest) (api.SnippetResult, error) {
	if req.Project == "" {
		req.Project, _ = s.defaultProject(ctx)
	}
	file := filepath.ToSlash(req.File)
	start, end := req.StartLine, req.EndLine
	var matched *api.Node
	var snippetLeftovers []api.Node
	identity := strings.TrimSpace(req.QualifiedName)
	if snippetLooksLikePath(identity) && file == "" {
		file = filepath.ToSlash(identity)
		identity = ""
	}
	if identity != "" {
		nodes, err := s.findNodes(ctx, req.Project, identity, 20)
		if err != nil {
			return api.SnippetResult{}, err
		}
		if len(nodes) == 0 {
			return api.SnippetResult{}, fmt.Errorf("symbol not found: %s", identity)
		}
		picked, leftovers, amb := resolveGraphIdentity(identity, nodes)
		if amb || picked == nil {
			return api.SnippetResult{
				Status:      "ambiguous",
				Message:     fmt.Sprintf("%q matches %d nodes; retry with a qualified_name", identity, len(nodes)),
				Suggestions: leftovers,
			}, nil
		}
		matched = picked
		file = matched.Location.File
		start, end = matched.Location.StartLine, matched.Location.EndLine
		snippetLeftovers = leftovers
	} else if file != "" {
		node, err := s.findFileOrCallable(ctx, req.Project, file)
		if err != nil {
			return api.SnippetResult{}, err
		}
		if node != nil {
			matched = node
			file = node.Location.File
			if start <= 0 {
				start = node.Location.StartLine
			}
			if end <= 0 {
				end = node.Location.EndLine
			}
		}
	}
	if strings.TrimSpace(file) == "" {
		return api.SnippetResult{}, fmt.Errorf("symbol not found: %s", strings.TrimSpace(req.QualifiedName+" "+req.File))
	}
	var root string
	if err := s.db.QueryRowContext(ctx, `SELECT root_path FROM projects WHERE name=?`, req.Project).Scan(&root); err != nil {
		return api.SnippetResult{}, err
	}
	canonical, err := CanonicalRoot(root)
	if err != nil {
		return api.SnippetResult{}, err
	}
	abs := filepath.Join(canonical, filepath.FromSlash(file))
	rel, err := filepath.Rel(canonical, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return api.SnippetResult{}, errors.New("snippet path escapes repository")
	}
	fileOrModule := matched != nil && (matched.Label == "File" || matched.Label == "Module")
	missingFileSpan := fileOrModule && matched.Location.EndLine <= matched.Location.StartLine
	if start <= 0 {
		start = 1
	}
	if missingFileSpan {
		end = start + 500
	} else if end < start {
		end = start
	}
	contextLines := req.ContextLines
	if contextLines < 0 {
		contextLines = 0
	}
	start -= contextLines
	if start < 1 {
		start = 1
	}
	end += contextLines
	clipped := false
	if !missingFileSpan && end-start > 500 {
		end = start + 500
		clipped = true
	}
	handle, err := os.Open(abs)
	if err != nil {
		return api.SnippetResult{}, err
	}
	defer handle.Close()
	var lines []string
	scanner := bufio.NewScanner(handle)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	line := 0
	for scanner.Scan() {
		line++
		if line >= start && line <= end {
			lines = append(lines, scanner.Text())
		}
		if line > end {
			if missingFileSpan {
				clipped = true
			}
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return api.SnippetResult{}, err
	}
	coverage, _ := s.Coverage(ctx, api.CoverageRequest{Project: req.Project, Paths: []string{file}})
	result := api.SnippetResult{
		Location: api.Location{File: file, StartLine: start, EndLine: start + len(lines) - 1},
		Language: "go", Code: strings.Join(lines, "\n"), Coverage: coverage, Clipped: clipped,
	}
	if matched != nil {
		result.QualifiedName = matched.QualifiedName
		result.Name = matched.Name
		result.Label = matched.Label
		result.Suggestions = snippetLeftovers
		_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM edges WHERE project=? AND target_id=? AND type='CALLS'`, req.Project, matched.ID).Scan(&result.Callers)
		_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM edges WHERE project=? AND source_id=? AND type='CALLS'`, req.Project, matched.ID).Scan(&result.Callees)
	}
	return result, nil
}

func (s *Store) findFileOrCallable(ctx context.Context, project, file string) (*api.Node, error) {
	file = filepath.ToSlash(strings.TrimSpace(file))
	if file == "" {
		return nil, nil
	}
	query := `SELECT ` + nodeColumns + ` FROM nodes WHERE project=? AND (file_path=? OR file_path LIKE ? ESCAPE '\') AND label='File' ORDER BY length(file_path) LIMIT 1`
	nodes, err := s.scanFindNodes(ctx, query, project, file, "%/"+escapeLike(file))
	if err != nil {
		return nil, err
	}
	if len(nodes) > 0 {
		return &nodes[0], nil
	}
	query = `SELECT ` + nodeColumns + ` FROM nodes WHERE project=? AND (file_path=? OR file_path LIKE ? ESCAPE '\') AND label IN ('Function','Method','Class','Constructor') ORDER BY start_line LIMIT 1`
	nodes, err = s.scanFindNodes(ctx, query, project, file, "%/"+escapeLike(file))
	if err != nil || len(nodes) == 0 {
		return nil, err
	}
	return &nodes[0], nil
}

func (s *Store) Architecture(ctx context.Context, req api.ArchitectureRequest) (api.ArchitectureResult, error) {
	if req.Project == "" {
		req.Project, _ = s.defaultProject(ctx)
	}
	result := api.ArchitectureResult{Aspects: map[string]any{}}
	if invalid := invalidArchitectureAspect(req.Aspects); invalid != "" {
		return result, fmt.Errorf("unknown architecture aspect %q", invalid)
	}
	scope := normalizedArchitecturePath(req.Path)
	labelQuery := `SELECT label,count(*) FROM nodes WHERE project=?`
	labelArgs := []any{req.Project}
	if scope != "" {
		labelQuery += ` AND (file_path=? OR file_path LIKE ?)`
		labelArgs = append(labelArgs, scope, escapeLike(scope)+"/%")
	}
	labelQuery += ` GROUP BY label ORDER BY label`
	labelCounts, err := countBy(ctx, s.db, labelQuery, labelArgs...)
	if err != nil {
		return result, err
	}
	edgeQuery := `SELECT e.type,count(*) FROM edges e WHERE e.project=?`
	edgeArgs := []any{req.Project}
	if scope != "" {
		edgeQuery += ` AND EXISTS (SELECT 1 FROM nodes source WHERE source.id=e.source_id AND source.project=e.project AND (source.file_path=? OR source.file_path LIKE ?))
			AND EXISTS (SELECT 1 FROM nodes target WHERE target.id=e.target_id AND target.project=e.project AND (target.file_path=? OR target.file_path LIKE ?))`
		edgeArgs = append(edgeArgs, scope, escapeLike(scope)+"/%", scope, escapeLike(scope)+"/%")
	}
	edgeQuery += ` GROUP BY e.type ORDER BY e.type`
	edgeCounts, err := countBy(ctx, s.db, edgeQuery, edgeArgs...)
	if err != nil {
		return result, err
	}
	result.Aspects["node_labels"] = labelCounts
	result.Aspects["edge_types"] = edgeCounts
	for _, count := range labelCounts {
		result.TotalNodes += count
	}
	for _, count := range edgeCounts {
		result.TotalEdges += count
	}
	if scope != "" {
		result.Path = scope
		_ = s.db.QueryRowContext(ctx, `SELECT count(*) FROM nodes WHERE project=?`, req.Project).Scan(&result.RootTotalNodes)
		_ = s.db.QueryRowContext(ctx, `SELECT count(*) FROM edges WHERE project=?`, req.Project).Scan(&result.RootTotalEdges)
	}
	result.Summary = fmt.Sprintf("%d nodes and %d relationships across %d node labels and %d relationship types", result.TotalNodes, result.TotalEdges, len(labelCounts), len(edgeCounts))
	if wantsArchitectureAspect(req.Aspects, "languages") {
		result.Languages, err = s.architectureLanguages(ctx, req.Project, req.Path)
		if err != nil {
			return result, err
		}
	}
	if wantsArchitectureAspect(req.Aspects, "packages") {
		result.Packages, err = s.architecturePackages(ctx, req.Project, req.Path)
		if err != nil {
			return result, err
		}
	}
	if wantsArchitectureAspect(req.Aspects, "entry_points") {
		result.EntryPoints, err = s.architectureEntryPoints(ctx, req.Project, req.Path)
		if err != nil {
			return result, err
		}
	}
	if wantsArchitectureAspect(req.Aspects, "routes") {
		result.Routes, err = s.architectureRoutes(ctx, req.Project, req.Path)
		if err != nil {
			return result, err
		}
	}
	if wantsArchitectureAspect(req.Aspects, "hotspots") {
		result.Hotspots, err = s.architectureHotspots(ctx, req.Project, req.Path)
		if err != nil {
			return result, err
		}
	}
	if wantsArchitectureAspect(req.Aspects, "boundaries") || wantsArchitectureAspect(req.Aspects, "layers") {
		boundaries, boundaryErr := s.architectureBoundaries(ctx, req.Project, req.Path)
		if boundaryErr != nil {
			return result, boundaryErr
		}
		if wantsArchitectureAspect(req.Aspects, "boundaries") {
			result.Boundaries = boundaries
		}
		if wantsArchitectureAspect(req.Aspects, "layers") {
			result.Layers, err = s.architectureLayers(ctx, req.Project, req.Path, boundaries)
			if err != nil {
				return result, err
			}
		}
	}
	if wantsArchitectureAspect(req.Aspects, "file_tree") {
		result.FileTree, err = s.architectureFileTree(ctx, req.Project, req.Path)
		if err != nil {
			return result, err
		}
	}
	if wantsArchitectureAspect(req.Aspects, "clusters") {
		result.Communities, err = s.architectureCommunities(ctx, req.Project, req.Path)
		if err != nil {
			return result, err
		}
	}
	if hasAspect(req.Aspects, "cycles") {
		result.Cycles, err = s.callCycles(ctx, req.Project, req.Path)
		if err != nil {
			return result, err
		}
	}
	result.Coverage, _ = s.Coverage(ctx, api.CoverageRequest{Project: req.Project, Scope: req.Path})
	return result, nil
}

func invalidArchitectureAspect(aspects []string) string {
	valid := map[string]bool{
		"all": true, "overview": true, "structure": true, "dependencies": true,
		"routes": true, "languages": true, "packages": true, "entry_points": true,
		"hotspots": true, "boundaries": true, "layers": true, "file_tree": true,
		"clusters": true, "cycles": true,
	}
	for _, aspect := range aspects {
		if !valid[aspect] {
			return aspect
		}
	}
	return ""
}

var pinnedArchitectureLanguages = map[string]string{
	".py": "Python", ".go": "Go", ".js": "JavaScript", ".jsx": "JavaScript",
	".ts": "TypeScript", ".tsx": "TypeScript", ".rs": "Rust", ".java": "Java",
	".cpp": "C++", ".cc": "C++", ".cxx": "C++", ".c": "C", ".h": "C",
	".cs": "C#", ".php": "PHP", ".lua": "Lua", ".scala": "Scala", ".kt": "Kotlin",
	".rb": "Ruby", ".sh": "Bash", ".bash": "Bash", ".zig": "Zig", ".ex": "Elixir",
	".exs": "Elixir", ".hs": "Haskell", ".ml": "OCaml", ".mli": "OCaml", ".html": "HTML",
	".css": "CSS", ".yaml": "YAML", ".yml": "YAML", ".toml": "TOML", ".hcl": "HCL",
	".tf": "HCL", ".sql": "SQL", ".erl": "Erlang", ".swift": "Swift", ".dart": "Dart",
	".groovy": "Groovy", ".pl": "Perl", ".r": "R", ".scss": "SCSS", ".vue": "Vue",
	".svelte": "Svelte",
}

func (s *Store) architectureLanguages(ctx context.Context, project, path string) ([]api.LanguageCount, error) {
	where := []string{"project=?", "label='File'"}
	args := []any{project}
	if scope := normalizedArchitecturePath(path); scope != "" {
		where = append(where, "(file_path=? OR file_path LIKE ?)")
		args = append(args, scope, escapeLike(scope)+"/%")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT file_path FROM nodes WHERE `+strings.Join(where, " AND "), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		if language := pinnedArchitectureLanguages[strings.ToLower(filepath.Ext(path))]; language != "" {
			counts[language]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]api.LanguageCount, 0, len(counts))
	for language, count := range counts {
		result = append(result, api.LanguageCount{Language: language, FileCount: count})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].FileCount != result[j].FileCount {
			return result[i].FileCount > result[j].FileCount
		}
		return result[i].Language < result[j].Language
	})
	if len(result) > 10 {
		result = result[:10]
	}
	return result, nil
}

func (s *Store) architecturePackages(ctx context.Context, project, path string) ([]api.PackageSummary, error) {
	where := []string{"project=?", "label='Package'"}
	args := []any{project}
	if scope := normalizedArchitecturePath(path); scope != "" {
		where = append(where, "(file_path=? OR file_path LIKE ?)")
		args = append(args, scope, escapeLike(scope)+"/%")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT name,count(*) FROM nodes WHERE `+strings.Join(where, " AND ")+`
		GROUP BY name ORDER BY count(*) DESC,name LIMIT 15`, args...)
	if err != nil {
		return nil, err
	}
	var result []api.PackageSummary
	for rows.Next() {
		var item api.PackageSummary
		if err := rows.Scan(&item.Name, &item.NodeCount); err != nil {
			rows.Close()
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(result) > 0 {
		return result, nil
	}

	where = []string{"project=?", "label IN ('Function','Method','Class','Struct','Interface','Enum','Type','Trait')"}
	args = []any{project}
	if scope := normalizedArchitecturePath(path); scope != "" {
		where = append(where, "(file_path=? OR file_path LIKE ?)")
		args = append(args, scope, escapeLike(scope)+"/%")
	}
	rows, err = s.db.QueryContext(ctx, `SELECT qualified_name FROM nodes WHERE `+strings.Join(where, " AND "), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var qualifiedName string
		if err := rows.Scan(&qualifiedName); err != nil {
			return nil, err
		}
		if name := pinnedPackageFromQualifiedName(qualifiedName); name != "" {
			counts[name]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for name, count := range counts {
		result = append(result, api.PackageSummary{Name: name, NodeCount: count})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].NodeCount != result[j].NodeCount {
			return result[i].NodeCount > result[j].NodeCount
		}
		return result[i].Name < result[j].Name
	})
	if len(result) > 15 {
		result = result[:15]
	}
	return result, nil
}

func pinnedPackageFromQualifiedName(qualifiedName string) string {
	segments := strings.Split(qualifiedName, ".")
	if len(segments) >= 4 {
		return segments[2]
	}
	if len(segments) >= 2 {
		return segments[1]
	}
	return ""
}

func (s *Store) architectureBoundaries(ctx context.Context, project, path string) ([]api.Boundary, error) {
	where := []string{"project=?", "label IN ('Function','Method','Class','Struct','Interface','Enum','Type','Trait')"}
	args := []any{project}
	if scope := normalizedArchitecturePath(path); scope != "" {
		where = append(where, "(file_path=? OR file_path LIKE ?)")
		args = append(args, scope, escapeLike(scope)+"/%")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,qualified_name FROM nodes WHERE `+strings.Join(where, " AND ")+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	packages := map[int64]string{}
	for rows.Next() {
		var id int64
		var qualifiedName string
		if err := rows.Scan(&id, &qualifiedName); err != nil {
			rows.Close()
			return nil, err
		}
		packages[id] = pinnedPackageFromQualifiedName(qualifiedName)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	edges, err := s.db.QueryContext(ctx, `SELECT source_id,target_id FROM edges WHERE project=? AND type='CALLS'`, project)
	if err != nil {
		return nil, err
	}
	defer edges.Close()
	counts := map[[2]string]int{}
	for edges.Next() {
		var sourceID, targetID int64
		if err := edges.Scan(&sourceID, &targetID); err != nil {
			return nil, err
		}
		source, sourceOK := packages[sourceID]
		target, targetOK := packages[targetID]
		if !sourceOK || !targetOK || source == "" || target == "" || source == target {
			continue
		}
		counts[[2]string{source, target}]++
	}
	if err := edges.Err(); err != nil {
		return nil, err
	}
	result := make([]api.Boundary, 0, len(counts))
	for pair, count := range counts {
		result = append(result, api.Boundary{From: pair[0], To: pair[1], CallCount: count})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CallCount != result[j].CallCount {
			return result[i].CallCount > result[j].CallCount
		}
		if result[i].From != result[j].From {
			return result[i].From < result[j].From
		}
		return result[i].To < result[j].To
	})
	if len(result) > 20 {
		result = result[:20]
	}
	return result, nil
}

func (s *Store) architectureLayers(ctx context.Context, project, path string, boundaries []api.Boundary) ([]api.PackageLayer, error) {
	fanIn := map[string]int{}
	fanOut := map[string]int{}
	packages := map[string]bool{}
	for _, boundary := range boundaries {
		packages[boundary.From] = true
		packages[boundary.To] = true
		fanOut[boundary.From] += boundary.CallCount
		fanIn[boundary.To] += boundary.CallCount
	}
	marked, err := s.architectureMarkedPackages(ctx, project, path)
	if err != nil {
		return nil, err
	}
	for packageName := range marked.routes {
		packages[packageName] = true
	}
	for packageName := range marked.entries {
		packages[packageName] = true
	}
	result := make([]api.PackageLayer, 0, len(packages))
	for packageName := range packages {
		in, out := fanIn[packageName], fanOut[packageName]
		item := api.PackageLayer{Name: packageName}
		switch {
		case marked.entries[packageName] && out > 0 && in == 0:
			item.Layer, item.Reason = "entry", "has entry points, only outbound calls"
		case marked.routes[packageName]:
			item.Layer, item.Reason = "api", "has HTTP route definitions"
		case in > out && in > 3:
			item.Layer, item.Reason = "core", fmt.Sprintf("high fan-in (%d in, %d out)", in, out)
		case out == 0 && in > 0:
			item.Layer, item.Reason = "leaf", "only inbound calls, no outbound"
		case in == 0 && out > 0:
			item.Layer, item.Reason = "entry", "only outbound calls"
		default:
			item.Layer, item.Reason = "internal", fmt.Sprintf("fan-in=%d, fan-out=%d", in, out)
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

type architecturePackageMarks struct {
	routes  map[string]bool
	entries map[string]bool
}

func (s *Store) architectureMarkedPackages(ctx context.Context, project, path string) (architecturePackageMarks, error) {
	result := architecturePackageMarks{routes: map[string]bool{}, entries: map[string]bool{}}
	where := []string{"project=?", "(label='Route' OR json_extract(properties, '$.is_entry_point')=1)"}
	args := []any{project}
	if scope := normalizedArchitecturePath(path); scope != "" {
		where = append(where, "(file_path=? OR file_path LIKE ?)")
		args = append(args, scope, escapeLike(scope)+"/%")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT label,qualified_name,json_extract(properties, '$.is_entry_point') FROM nodes WHERE `+strings.Join(where, " AND "), args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var label, qualifiedName string
		var entry sql.NullInt64
		if err := rows.Scan(&label, &qualifiedName, &entry); err != nil {
			return result, err
		}
		packageName := pinnedPackageFromQualifiedName(qualifiedName)
		if label == "Route" {
			result.routes[packageName] = true
		}
		if entry.Valid && entry.Int64 == 1 {
			result.entries[packageName] = true
		}
	}
	return result, rows.Err()
}

func (s *Store) architectureFileTree(ctx context.Context, project, path string) ([]api.FileTreeEntry, error) {
	where := []string{"project=?", "label='File'"}
	args := []any{project}
	if scope := normalizedArchitecturePath(path); scope != "" {
		where = append(where, "(file_path=? OR file_path LIKE ?)")
		args = append(args, scope, escapeLike(scope)+"/%")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT file_path FROM nodes WHERE `+strings.Join(where, " AND "), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := map[string]bool{}
	children := map[string]map[string]bool{"": {}}
	for rows.Next() {
		var file string
		if err := rows.Scan(&file); err != nil {
			return nil, err
		}
		file = strings.Trim(filepath.ToSlash(file), "/")
		if file == "" {
			continue
		}
		files[file] = true
		parts := strings.Split(file, "/")
		children[""][parts[0]] = true
		for depth := 0; depth < len(parts)-1 && depth < 3; depth++ {
			directory := strings.Join(parts[:depth+1], "/")
			if children[directory] == nil {
				children[directory] = map[string]bool{}
			}
			children[directory][parts[depth+1]] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	entries := map[string]api.FileTreeEntry{}
	for parent, names := range children {
		for name := range names {
			entryPath := name
			if parent != "" {
				entryPath = parent + "/" + name
			}
			entry := api.FileTreeEntry{Path: entryPath, Type: "dir", Children: len(children[entryPath])}
			if files[entryPath] {
				entry.Type = "file"
			}
			entries[entryPath] = entry
		}
	}
	result := make([]api.FileTreeEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}

type architectureCommunityNode struct {
	node api.Node
}

func (s *Store) architectureCommunities(ctx context.Context, project, path string) ([]api.Community, error) {
	where := []string{"project=?", "label IN ('Function','Method','Class','Struct','Interface','Enum','Type','Trait')"}
	args := []any{project}
	if scope := normalizedArchitecturePath(path); scope != "" {
		where = append(where, "(file_path=? OR file_path LIKE ?)")
		args = append(args, scope, escapeLike(scope)+"/%")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+nodeColumns+` FROM nodes WHERE `+strings.Join(where, " AND ")+` ORDER BY id LIMIT 8000`, args...)
	if err != nil {
		return nil, err
	}
	var nodes []architectureCommunityNode
	ids := make([]int64, 0, 64)
	for rows.Next() {
		var item architectureCommunityNode
		var properties string
		if err := rows.Scan(&item.node.ID, &item.node.Project, &item.node.Label, &item.node.Name, &item.node.QualifiedName,
			&item.node.Location.File, &item.node.Location.StartLine, &item.node.Location.StartColumn,
			&item.node.Location.EndLine, &item.node.Location.EndColumn, &properties); err != nil {
			rows.Close()
			return nil, err
		}
		_ = json.Unmarshal([]byte(properties), &item.node.Properties)
		nodes = append(nodes, item)
		ids = append(ids, item.node.ID)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(nodes) < 2 {
		return nil, nil
	}
	indices := make(map[int64]int, len(ids))
	for index, id := range ids {
		indices[id] = index
	}
	edgeRows, err := s.db.QueryContext(ctx, `SELECT source_id,target_id FROM edges WHERE project=? AND type='CALLS' ORDER BY id`, project)
	if err != nil {
		return nil, err
	}
	var edges []LeidenEdge
	var endpoints [][2]int
	degree := make([]int, len(nodes))
	for edgeRows.Next() {
		var sourceID, targetID int64
		if err := edgeRows.Scan(&sourceID, &targetID); err != nil {
			edgeRows.Close()
			return nil, err
		}
		source, sourceOK := indices[sourceID]
		target, targetOK := indices[targetID]
		if !sourceOK || !targetOK || source == target {
			continue
		}
		edges = append(edges, LeidenEdge{Source: sourceID, Target: targetID})
		endpoints = append(endpoints, [2]int{source, target})
		degree[source]++
		degree[target]++
	}
	if err := edgeRows.Close(); err != nil {
		return nil, err
	}
	membership := Leiden(ids, edges, 1)
	communityFor := make([]int, len(membership))
	members := map[int]int{}
	internal := map[int]int{}
	boundary := map[int]int{}
	for index, item := range membership {
		communityFor[index] = item.Community
		members[item.Community]++
	}
	for _, endpoints := range endpoints {
		sourceCommunity, targetCommunity := communityFor[endpoints[0]], communityFor[endpoints[1]]
		if sourceCommunity == targetCommunity {
			internal[sourceCommunity]++
		} else {
			boundary[sourceCommunity]++
			boundary[targetCommunity]++
		}
	}
	communityIDs := make([]int, 0, len(members))
	for communityID, count := range members {
		if count >= 2 {
			communityIDs = append(communityIDs, communityID)
		}
	}
	sort.Slice(communityIDs, func(i, j int) bool {
		if members[communityIDs[i]] != members[communityIDs[j]] {
			return members[communityIDs[i]] > members[communityIDs[j]]
		}
		return communityIDs[i] < communityIDs[j]
	})
	if len(communityIDs) > 12 {
		communityIDs = communityIDs[:12]
	}
	result := make([]api.Community, 0, len(communityIDs))
	for _, communityID := range communityIDs {
		denominator := internal[communityID] + boundary[communityID]
		item := api.Community{ID: int64(communityID), Members: members[communityID], EdgeTypes: []string{"CALLS"}}
		if denominator > 0 {
			item.Cohesion = float64(internal[communityID]) / float64(denominator)
		}
		top := make([]int, 0, 5)
		packageCounts := map[string]int{}
		var packageOrder []string
		for nodeIndex := range nodes {
			if communityFor[nodeIndex] != communityID {
				continue
			}
			position := len(top)
			for position > 0 && degree[top[position-1]] < degree[nodeIndex] {
				position--
			}
			if position < 5 {
				top = append(top, 0)
				copy(top[position+1:], top[position:])
				top[position] = nodeIndex
				if len(top) > 5 {
					top = top[:5]
				}
			}
			packageName := pinnedTopPackageFromQualifiedName(nodes[nodeIndex].node.QualifiedName)
			if packageName != "" {
				if _, known := packageCounts[packageName]; known {
					packageCounts[packageName]++
				} else if len(packageOrder) < 5 {
					packageOrder = append(packageOrder, packageName)
					packageCounts[packageName] = 1
				}
			}
		}
		for _, nodeIndex := range top {
			item.TopNodes = append(item.TopNodes, nodes[nodeIndex].node.Name)
		}
		item.Packages = append(item.Packages, packageOrder...)
		bestPackage := ""
		for _, packageName := range packageOrder {
			if bestPackage == "" || packageCounts[packageName] > packageCounts[bestPackage] {
				bestPackage = packageName
			}
		}
		item.Name = bestPackage
		if item.Name == "" && len(item.TopNodes) > 0 {
			item.Name = item.TopNodes[0]
		}
		if item.Name == "" {
			item.Name = "cluster"
		}
		if len(top) > 0 {
			hub := nodes[top[0]].node
			item.Hub = &hub
		}
		result = append(result, item)
	}
	return result, nil
}

func pinnedTopPackageFromQualifiedName(qualifiedName string) string {
	segments := strings.Split(qualifiedName, ".")
	if len(segments) < 2 {
		return ""
	}
	return segments[1]
}

func (s *Store) architectureEntryPoints(ctx context.Context, project, path string) ([]api.Node, error) {
	where := []string{
		"n.project=?", "json_extract(n.properties, '$.is_entry_point')=1",
		"(json_extract(n.properties, '$.is_test') IS NULL OR json_extract(n.properties, '$.is_test')!=1)",
		"n.file_path NOT LIKE '%test%'",
	}
	args := []any{project}
	if scope := normalizedArchitecturePath(path); scope != "" {
		where = append(where, "(n.file_path=? OR n.file_path LIKE ?)")
		args = append(args, scope, escapeLike(scope)+"/%")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT n.`+strings.ReplaceAll(nodeColumns, ",", ",n.")+`
		FROM nodes n WHERE `+strings.Join(where, " AND ")+` ORDER BY n.id LIMIT 20`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []api.Node
	for rows.Next() {
		var node api.Node
		var properties string
		if err := rows.Scan(&node.ID, &node.Project, &node.Label, &node.Name, &node.QualifiedName,
			&node.Location.File, &node.Location.StartLine, &node.Location.StartColumn,
			&node.Location.EndLine, &node.Location.EndColumn, &properties); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(properties), &node.Properties)
		result = append(result, node)
	}
	return result, rows.Err()
}

func (s *Store) architectureRoutes(ctx context.Context, project, path string) ([]api.Route, error) {
	where := []string{
		"project=?", "label='Route'",
		"(json_extract(properties, '$.is_test') IS NULL OR json_extract(properties, '$.is_test')!=1)",
	}
	args := []any{project}
	if scope := normalizedArchitecturePath(path); scope != "" {
		where = append(where, "(file_path=? OR file_path LIKE ?)")
		args = append(args, scope, escapeLike(scope)+"/%")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT name,properties,file_path FROM nodes WHERE `+strings.Join(where, " AND ")+` ORDER BY id LIMIT 20`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []api.Route
	for rows.Next() {
		var name, properties, file string
		if err := rows.Scan(&name, &properties, &file); err != nil {
			return nil, err
		}
		if strings.Contains(strings.ToLower(file), "test") {
			continue
		}
		values := map[string]any{}
		_ = json.Unmarshal([]byte(properties), &values)
		route := api.Route{Path: name}
		if value, ok := values["method"].(string); ok {
			route.Method = value
		}
		if value, ok := values["path"].(string); ok {
			route.Path = value
		}
		if value, ok := values["handler"].(string); ok {
			route.Handler = value
		}
		result = append(result, route)
	}
	return result, rows.Err()
}

func wantsArchitectureAspect(aspects []string, wanted string) bool {
	if len(aspects) == 0 {
		return wanted == "languages" || wanted == "packages" || wanted == "entry_points"
	}
	for _, aspect := range aspects {
		if aspect == "all" || aspect == wanted || (aspect == "overview" && wanted != "file_tree") {
			return true
		}
	}
	return false
}

func (s *Store) architectureHotspots(ctx context.Context, project, path string) ([]api.RankedNode, error) {
	where := []string{
		"n.project=?",
		"n.label IN ('Function','Method')",
		"(json_extract(n.properties, '$.is_test') IS NULL OR json_extract(n.properties, '$.is_test') != 1)",
		"n.file_path NOT LIKE '%test%'",
	}
	args := []any{project}
	if scope := normalizedArchitecturePath(path); scope != "" {
		where = append(where, "(n.file_path=? OR n.file_path LIKE ?)")
		args = append(args, scope, escapeLike(scope)+"/%")
	}
	query := `SELECT n.` + strings.ReplaceAll(nodeColumns, ",", ",n.") + `,COUNT(*) fan_in
		FROM nodes n JOIN edges e ON e.target_id=n.id AND e.project=n.project AND e.type='CALLS'
		WHERE ` + strings.Join(where, " AND ") + `
		GROUP BY n.id ORDER BY fan_in DESC,n.qualified_name LIMIT 10`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hotspots []api.RankedNode
	for rows.Next() {
		var node api.RankedNode
		var properties string
		if err := rows.Scan(&node.ID, &node.Project, &node.Label, &node.Name, &node.QualifiedName,
			&node.Location.File, &node.Location.StartLine, &node.Location.StartColumn,
			&node.Location.EndLine, &node.Location.EndColumn, &properties, &node.Score); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(properties), &node.Properties)
		node.Signals = api.Properties{"fan_in": int(node.Score)}
		hotspots = append(hotspots, node)
	}
	return hotspots, rows.Err()
}

func normalizedArchitecturePath(path string) string {
	path = strings.TrimSpace(filepath.ToSlash(path))
	path = strings.TrimPrefix(path, "./")
	path = strings.TrimLeft(path, "/")
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	return strings.TrimRight(path, "/ \t")
}

func hasAspect(aspects []string, wanted string) bool {
	for _, aspect := range aspects {
		if aspect == wanted {
			return true
		}
	}
	return false
}

func countBy(ctx context.Context, db *sql.DB, query string, args ...any) (map[string]int, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]int{}
	for rows.Next() {
		var name string
		var count int
		if err := rows.Scan(&name, &count); err != nil {
			return nil, err
		}
		result[name] = count
	}
	return result, rows.Err()
}

func (s *Store) Impact(ctx context.Context, req api.ImpactRequest) (api.ImpactResult, error) {
	if req.Project == "" {
		req.Project, _ = s.defaultProject(ctx)
	}
	seeds := append([]string(nil), req.Symbols...)
	for _, file := range req.Files {
		rows, err := s.db.QueryContext(ctx, `SELECT qualified_name FROM nodes WHERE project=? AND file_path=? ORDER BY qualified_name`, req.Project, filepath.ToSlash(file))
		if err != nil {
			return api.ImpactResult{}, err
		}
		for rows.Next() {
			var qn string
			if err := rows.Scan(&qn); err != nil {
				rows.Close()
				return api.ImpactResult{}, err
			}
			seeds = append(seeds, qn)
		}
		_ = rows.Close()
	}
	result := api.ImpactResult{ImpactedModules: map[string]int{}}
	seen := map[int64]bool{}
	for _, seed := range seeds {
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
			result.Impacted = append(result.Impacted, api.ImpactedNode{Node: step.Node, Hop: step.Hop})
			module := filepath.Dir(step.Node.Location.File)
			result.ImpactedModules[module]++
		}
		result.Truncated = result.Truncated || trace.Truncated
	}
	sort.Slice(result.Impacted, func(i, j int) bool {
		if result.Impacted[i].Hop != result.Impacted[j].Hop {
			return result.Impacted[i].Hop < result.Impacted[j].Hop
		}
		return result.Impacted[i].QualifiedName < result.Impacted[j].QualifiedName
	})
	result.Total = len(result.Impacted)
	result.Coverage, _ = s.Coverage(ctx, api.CoverageRequest{Project: req.Project})
	return result, nil
}

func (s *Store) defaultProject(ctx context.Context) (string, error) {
	var project string
	err := s.db.QueryRowContext(ctx, `SELECT name FROM projects WHERE substr(name, -8) != '::missed' ORDER BY indexed_at DESC,name LIMIT 1`).Scan(&project)
	return project, err
}
