package engine

import (
	"context"
	"sort"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// callCycles returns strongly connected components of the static CALLS graph.
// Components and their members have stable ordering so repeated builds produce
// byte-for-byte equivalent architecture results.
func (s *Store) callCycles(ctx context.Context, project, pathPrefix string) ([][]api.Node, error) {
	query := `SELECT ` + nodeColumns + ` FROM nodes WHERE project=? AND label IN ('Function','Method')`
	args := []any{project}
	if pathPrefix != "" {
		query += ` AND file_path LIKE ? ESCAPE '\\'`
		args = append(args, escapeLike(strings.TrimSuffix(pathPrefix, "/"))+"%")
	}
	query += ` ORDER BY qualified_name`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	nodes := map[int64]api.Node{}
	for rows.Next() {
		node, scanErr := scanNode(rows)
		if scanErr != nil {
			rows.Close()
			return nil, scanErr
		}
		nodes[node.ID] = node
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	adjacency := map[int64][]int64{}
	edgeRows, err := s.db.QueryContext(ctx, `SELECT source_id,target_id FROM edges WHERE project=? AND type='CALLS' ORDER BY source_id,target_id`, project)
	if err != nil {
		return nil, err
	}
	for edgeRows.Next() {
		var source, target int64
		if err := edgeRows.Scan(&source, &target); err != nil {
			edgeRows.Close()
			return nil, err
		}
		if _, sourceOK := nodes[source]; sourceOK {
			if _, targetOK := nodes[target]; targetOK {
				adjacency[source] = append(adjacency[source], target)
			}
		}
	}
	if err := edgeRows.Close(); err != nil {
		return nil, err
	}

	index := 0
	indices := map[int64]int{}
	lowlink := map[int64]int{}
	onStack := map[int64]bool{}
	var stack []int64
	var components [][]api.Node
	var connect func(int64)
	connect = func(nodeID int64) {
		index++
		indices[nodeID] = index
		lowlink[nodeID] = index
		stack = append(stack, nodeID)
		onStack[nodeID] = true
		for _, target := range adjacency[nodeID] {
			if indices[target] == 0 {
				connect(target)
				if lowlink[target] < lowlink[nodeID] {
					lowlink[nodeID] = lowlink[target]
				}
			} else if onStack[target] && indices[target] < lowlink[nodeID] {
				lowlink[nodeID] = indices[target]
			}
		}
		if lowlink[nodeID] != indices[nodeID] {
			return
		}
		var component []api.Node
		for len(stack) > 0 {
			last := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[last] = false
			component = append(component, nodes[last])
			if last == nodeID {
				break
			}
		}
		selfLoop := len(component) == 1 && containsID(adjacency[component[0].ID], component[0].ID)
		if len(component) > 1 || selfLoop {
			sort.Slice(component, func(i, j int) bool { return component[i].QualifiedName < component[j].QualifiedName })
			components = append(components, component)
		}
	}
	ordered := make([]api.Node, 0, len(nodes))
	for _, node := range nodes {
		ordered = append(ordered, node)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].QualifiedName < ordered[j].QualifiedName })
	for _, node := range ordered {
		if indices[node.ID] == 0 {
			connect(node.ID)
		}
	}
	sort.Slice(components, func(i, j int) bool {
		return components[i][0].QualifiedName < components[j][0].QualifiedName
	})
	return components, nil
}

func containsID(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
