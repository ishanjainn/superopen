package engine

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// Schema returns the observable graph schema: counts and discovered JSON
// property names per node label and edge type, plus observed
// source-label/edge/target-label patterns.
func (s *Store) Schema(ctx context.Context, project string) (api.SchemaResult, error) {
	if project == "" {
		project, _ = s.defaultProject(ctx)
	}
	result := api.SchemaResult{Project: project}
	type inventory struct {
		count int
		props map[string]bool
	}
	nodes := map[string]*inventory{}
	rows, err := s.db.QueryContext(ctx, `SELECT label,properties FROM nodes WHERE project=? ORDER BY id`, project)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var label, raw string
		if err := rows.Scan(&label, &raw); err != nil {
			rows.Close()
			return result, err
		}
		item := nodes[label]
		if item == nil {
			item = &inventory{props: map[string]bool{}}
			nodes[label] = item
		}
		item.count++
		var properties map[string]any
		if json.Unmarshal([]byte(raw), &properties) == nil {
			for key := range properties {
				item.props[key] = true
			}
		}
	}
	if err := rows.Close(); err != nil {
		return result, err
	}

	edges := map[string]*inventory{}
	rows, err = s.db.QueryContext(ctx, `SELECT type,properties FROM edges WHERE project=? ORDER BY id`, project)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var edgeType, raw string
		if err := rows.Scan(&edgeType, &raw); err != nil {
			rows.Close()
			return result, err
		}
		item := edges[edgeType]
		if item == nil {
			item = &inventory{props: map[string]bool{}}
			edges[edgeType] = item
		}
		item.count++
		var properties map[string]any
		if json.Unmarshal([]byte(raw), &properties) == nil {
			for key := range properties {
				item.props[key] = true
			}
		}
	}
	if err := rows.Close(); err != nil {
		return result, err
	}

	toCounts := func(source map[string]*inventory) []api.SchemaCount {
		keys := make([]string, 0, len(source))
		for key := range source {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := make([]api.SchemaCount, 0, len(keys))
		for _, key := range keys {
			props := make([]string, 0, len(source[key].props))
			for prop := range source[key].props {
				props = append(props, prop)
			}
			sort.Strings(props)
			out = append(out, api.SchemaCount{Name: key, Count: source[key].count, Properties: props})
		}
		return out
	}
	result.NodeLabels, result.EdgeTypes = toCounts(nodes), toCounts(edges)
	for _, item := range result.NodeLabels {
		result.NodeCount += item.Count
	}
	for _, item := range result.EdgeTypes {
		result.EdgeCount += item.Count
	}
	rows, err = s.db.QueryContext(ctx, `SELECT source.label,e.type,target.label,count(*)
		FROM edges e JOIN nodes source ON source.id=e.source_id JOIN nodes target ON target.id=e.target_id
		WHERE e.project=? GROUP BY source.label,e.type,target.label ORDER BY source.label,e.type,target.label`, project)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var pattern api.SchemaPattern
		if err := rows.Scan(&pattern.Source, &pattern.Edge, &pattern.Target, &pattern.Count); err != nil {
			rows.Close()
			return result, err
		}
		result.Patterns = append(result.Patterns, pattern)
	}
	if err := rows.Close(); err != nil {
		return result, err
	}
	return result, nil
}
