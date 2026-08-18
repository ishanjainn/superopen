package engine

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func (s *Store) searchPattern(ctx context.Context, req api.SearchRequest, limit int) (api.SearchResult, error) {
	if !hasStructuralSearchFilter(req) {
		return api.SearchResult{}, errors.New("search requires query or a symbol/file pattern")
	}
	if req.Relationship != "" && !validSearchRelationship(req.Relationship) {
		return api.SearchResult{}, errors.New("relationship must be uppercase letters and underscores")
	}
	namePattern, err := optionalPattern(req.NamePattern)
	if err != nil {
		return api.SearchResult{}, err
	}
	qualifiedPattern, err := optionalPattern(req.QualifiedNamePattern)
	if err != nil {
		return api.SearchResult{}, err
	}
	filePattern, err := optionalFilePattern(req.FilePattern)
	if err != nil {
		return api.SearchResult{}, err
	}
	where := []string{"1=1"}
	args := []any{}
	if req.Project != "" {
		where = append(where, "n.project=?")
		args = append(args, req.Project)
	}
	if req.PathPrefix != "" {
		where = append(where, `n.file_path LIKE ? ESCAPE '\'`)
		args = append(args, escapeLike(filepath.ToSlash(req.PathPrefix))+"%")
	}
	if len(req.Labels) > 0 {
		placeholders := make([]string, len(req.Labels))
		for i, label := range req.Labels {
			placeholders[i] = "?"
			args = append(args, label)
		}
		where = append(where, "n.label IN ("+strings.Join(placeholders, ",")+")")
	}
	if len(req.Languages) > 0 {
		placeholders := make([]string, len(req.Languages))
		for i, language := range req.Languages {
			placeholders[i] = "?"
			args = append(args, language)
		}
		where = append(where, `EXISTS (SELECT 1 FROM file_hashes f WHERE f.project=n.project AND f.rel_path=n.file_path AND f.language IN (`+strings.Join(placeholders, ",")+`))`)
	}
	if req.Relationship != "" {
		where = append(where, `EXISTS (SELECT 1 FROM edges relationship_edge
			WHERE relationship_edge.project=n.project
			AND (relationship_edge.source_id=n.id OR relationship_edge.target_id=n.id)
			AND relationship_edge.type=?)`)
		args = append(args, req.Relationship)
	}
	if req.ExcludeEntryPoints {
		// Superopen defines a graph entry point as a node with outbound CALLS and
		// no inbound CALLS. Isolated nodes are retained so max_degree=0 can be
		// used for dead-code discovery.
		where = append(where, `NOT (
			NOT EXISTS (SELECT 1 FROM edges incoming_call WHERE incoming_call.project=n.project AND incoming_call.target_id=n.id AND incoming_call.type='CALLS')
			AND EXISTS (SELECT 1 FROM edges outgoing_call WHERE outgoing_call.project=n.project AND outgoing_call.source_id=n.id AND outgoing_call.type='CALLS')
		)`)
	}
	query := `SELECT n.` + strings.ReplaceAll(nodeColumns, ",", ",n.") + `,
		(SELECT count(*) FROM edges incoming_edge WHERE incoming_edge.project=n.project
			AND incoming_edge.target_id=n.id AND incoming_edge.type IN ('CALLS','USAGE','CALL_REFERENCE','INHERITS','IMPLEMENTS')) in_degree,
		(SELECT count(*) FROM edges outgoing_edge WHERE outgoing_edge.project=n.project
			AND outgoing_edge.source_id=n.id AND outgoing_edge.type IN ('CALLS','USAGE','CALL_REFERENCE','INHERITS','IMPLEMENTS')) out_degree
		FROM nodes n WHERE ` + strings.Join(where, " AND ") + ` ORDER BY n.qualified_name`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return api.SearchResult{}, err
	}
	defer rows.Close()
	var matches []api.RankedNode
	for rows.Next() {
		var match api.RankedNode
		var properties string
		var inDegree, outDegree int
		if err := rows.Scan(&match.ID, &match.Project, &match.Label, &match.Name, &match.QualifiedName,
			&match.Location.File, &match.Location.StartLine, &match.Location.StartColumn,
			&match.Location.EndLine, &match.Location.EndColumn, &properties, &inDegree, &outDegree); err != nil {
			return api.SearchResult{}, err
		}
		degree := inDegree + outDegree
		if !patternMatches(namePattern, match.Name) || !patternMatches(qualifiedPattern, match.QualifiedName) || !patternMatches(filePattern, match.Location.File) {
			continue
		}
		if (req.MinDegree != nil && degree < *req.MinDegree) || (req.MaxDegree != nil && degree > *req.MaxDegree) {
			continue
		}
		_ = json.Unmarshal([]byte(properties), &match.Properties)
		match.Properties = requestedSearchProperties(match.Properties, req.Fields)
		match.Score = float64(labelBoost(match.Label)*1000 + degree)
		match.Signals = api.Properties{"degree": degree, "in_degree": inDegree, "out_degree": outDegree, "label_boost": labelBoost(match.Label)}
		matches = append(matches, match)
	}
	if err := rows.Err(); err != nil {
		return api.SearchResult{}, err
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].QualifiedName < matches[j].QualifiedName
	})
	generation := ""
	if req.Project != "" {
		_ = s.db.QueryRowContext(ctx, `SELECT generation FROM projects WHERE name=?`, req.Project).Scan(&generation)
	}
	fingerprint := searchFingerprint(req)
	offset := 0
	if req.Cursor != "" {
		cursor, err := decodeSearchCursor(req.Cursor)
		if err != nil || cursor.Fingerprint != fingerprint {
			return api.SearchResult{}, errors.New("invalid search cursor")
		}
		if cursor.Generation != generation {
			return api.SearchResult{}, errors.New("stale search cursor")
		}
		offset = cursor.Offset
	}
	result := api.SearchResult{Page: api.Page{Limit: limit, Cursor: req.Cursor, Total: len(matches)}}
	if offset > len(matches) {
		return api.SearchResult{}, errors.New("search cursor offset exceeds result set")
	}
	end := offset + limit
	if end > len(matches) {
		end = len(matches)
	}
	result.Matches = append(result.Matches, matches[offset:end]...)
	if req.Budget > 0 {
		used := 0
		kept := result.Matches[:0]
		for _, match := range result.Matches {
			cost := 12 + (len(match.QualifiedName)+len(match.Location.File)+3)/4
			if len(kept) > 0 && used+cost > req.Budget {
				result.Budget.Truncated = true
				break
			}
			used += cost
			kept = append(kept, match)
		}
		result.Matches = kept
		result.Budget.RequestedTokens = req.Budget
		result.Budget.ReturnedTokens = used
	}
	if req.IncludeConnected {
		for index := range result.Matches {
			connected, err := s.connectedNodeNames(ctx, result.Matches[index].ID, req.Relationship)
			if err != nil {
				return api.SearchResult{}, err
			}
			result.Matches[index].Connected = connected
		}
	}
	nextOffset := offset + len(result.Matches)
	if nextOffset < len(matches) {
		result.Page.Truncated = true
		result.Page.NextCursor = encodeSearchCursor(searchCursor{Version: 1, Offset: nextOffset, Fingerprint: fingerprint, Generation: generation})
	}
	return result, nil
}

func requestedSearchProperties(properties api.Properties, fields []string) api.Properties {
	if len(fields) == 0 {
		return nil
	}
	if len(fields) > 16 {
		fields = fields[:16]
	}
	result := api.Properties{}
	for _, field := range fields {
		if value, ok := properties[field]; ok {
			result[field] = value
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

var searchRelationshipPattern = regexp.MustCompile(`^[A-Z_]{1,64}$`)

func validSearchRelationship(value string) bool {
	return searchRelationshipPattern.MatchString(value)
}

func (s *Store) connectedNodeNames(ctx context.Context, nodeID int64, relationship string) ([]string, error) {
	if relationship == "" {
		relationship = "CALLS"
	}
	var result []string
	for _, query := range []string{
		`SELECT n.name FROM edges e JOIN nodes n ON n.id=e.source_id WHERE e.target_id=? AND e.type=? ORDER BY e.id LIMIT 50`,
		`SELECT n.name FROM edges e JOIN nodes n ON n.id=e.target_id WHERE e.source_id=? AND e.type=? ORDER BY e.id LIMIT 50`,
	} {
		rows, err := s.db.QueryContext(ctx, query, nodeID, relationship)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				rows.Close()
				return nil, err
			}
			result = append(result, name)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func hasStructuralSearchFilter(req api.SearchRequest) bool {
	return req.NamePattern != "" || req.QualifiedNamePattern != "" || req.FilePattern != "" ||
		len(req.Labels) > 0 || len(req.Languages) > 0 || req.PathPrefix != "" ||
		req.Relationship != "" || req.MinDegree != nil || req.MaxDegree != nil || req.ExcludeEntryPoints
}

func optionalPattern(value string) (*regexp.Regexp, error) {
	if value == "" {
		return nil, nil
	}
	pattern, err := regexp.Compile("(?i)" + value)
	if err != nil {
		return nil, errors.New("invalid search pattern: " + err.Error())
	}
	return pattern, nil
}

func optionalFilePattern(value string) (*regexp.Regexp, error) {
	if value == "" {
		return nil, nil
	}
	like := globToLike(value)
	if !strings.ContainsAny(value, "*?") {
		like = "%" + like + "%"
	}
	var expression strings.Builder
	expression.WriteString("(?i)^")
	for _, character := range like {
		switch character {
		case '%':
			expression.WriteString(".*")
		case '_':
			expression.WriteByte('.')
		default:
			expression.WriteString(regexp.QuoteMeta(string(character)))
		}
	}
	expression.WriteByte('$')
	return regexp.Compile(expression.String())
}

func patternMatches(pattern *regexp.Regexp, value string) bool {
	return pattern == nil || pattern.MatchString(value)
}

func labelBoost(label string) int {
	switch label {
	case "Function", "Method":
		return 10
	case "Route":
		return 8
	case "Class", "Interface":
		return 5
	default:
		return 0
	}
}
