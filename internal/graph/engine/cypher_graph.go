package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
	cy "github.com/ishanjainn/superopen/internal/graph/engine/cypher"
)

const (
	cypherDefaultRows = 100_000
	cypherMaxRows     = 100_000
	cypherMaxDepth    = 10
	cypherBindingCap  = 1_000_000
)

type cypherGraph struct {
	nodes    []api.Node
	nodeByID map[int64]api.Node
	out      map[int64][]api.Edge
	in       map[int64][]api.Edge
}

type cypherValue struct {
	node   *api.Node
	edge   *api.Edge
	scalar any
}

type cypherBinding map[string]cypherValue

func (s *Store) loadCypherGraph(ctx context.Context, project string) (cypherGraph, error) {
	graph := cypherGraph{nodeByID: map[int64]api.Node{}, out: map[int64][]api.Edge{}, in: map[int64][]api.Edge{}}
	args := []any{}
	where := ""
	if project != "" {
		where, args = " WHERE project=?", append(args, project)
	}
	rows, err := s.db.QueryContext(ctx, "SELECT id,project,label,name,qualified_name,file_path,start_line,start_column,end_line,end_column,properties FROM nodes"+where+" ORDER BY id", args...)
	if err != nil {
		return graph, err
	}
	for rows.Next() {
		var node api.Node
		var properties string
		if err := rows.Scan(&node.ID, &node.Project, &node.Label, &node.Name, &node.QualifiedName,
			&node.Location.File, &node.Location.StartLine, &node.Location.StartColumn,
			&node.Location.EndLine, &node.Location.EndColumn, &properties); err != nil {
			rows.Close()
			return graph, err
		}
		if err := json.Unmarshal([]byte(properties), &node.Properties); err != nil {
			rows.Close()
			return graph, fmt.Errorf("decode node %d properties: %w", node.ID, err)
		}
		graph.nodes = append(graph.nodes, node)
		graph.nodeByID[node.ID] = node
	}
	if err := rows.Close(); err != nil {
		return graph, err
	}
	edgeRows, err := s.db.QueryContext(ctx, "SELECT id,project,source_id,target_id,type,properties,evidence FROM edges"+where+" ORDER BY id", args...)
	if err != nil {
		return graph, err
	}
	defer edgeRows.Close()
	for edgeRows.Next() {
		var edge api.Edge
		var properties, evidence string
		if err := edgeRows.Scan(&edge.ID, &edge.Project, &edge.SourceID, &edge.TargetID, &edge.Type, &properties, &evidence); err != nil {
			return graph, err
		}
		if err := json.Unmarshal([]byte(properties), &edge.Properties); err != nil {
			return graph, fmt.Errorf("decode edge %d properties: %w", edge.ID, err)
		}
		if evidence != "" && evidence != "null" && evidence != "{}" {
			if err := json.Unmarshal([]byte(evidence), &edge.Evidence); err != nil {
				return graph, fmt.Errorf("decode edge %d evidence: %w", edge.ID, err)
			}
		}
		graph.out[edge.SourceID] = append(graph.out[edge.SourceID], edge)
		graph.in[edge.TargetID] = append(graph.in[edge.TargetID], edge)
	}
	return graph, edgeRows.Err()
}

func matchCypherPattern(ctx context.Context, graph cypherGraph, input []cypherBinding, pattern cy.Pattern, optional bool, params map[string]any) ([]cypherBinding, error) {
	result := make([]cypherBinding, 0)
	for _, original := range input {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		starts := graph.nodes
		if variable := pattern.Nodes[0].Variable; variable != "" {
			if bound, ok := original[variable]; ok && bound.node != nil {
				starts = []api.Node{*bound.node}
			}
		}
		matched := false
		for i := range starts {
			if !nodeMatches(starts[i], pattern.Nodes[0], original, params) {
				continue
			}
			binding := cloneCypherBinding(original)
			if !bindNode(binding, pattern.Nodes[0].Variable, starts[i]) {
				continue
			}
			paths, err := extendCypherPattern(ctx, graph, pattern, 0, starts[i], binding, map[int64]bool{}, params)
			if err != nil {
				return nil, err
			}
			matched = matched || len(paths) > 0
			result = append(result, paths...)
			if len(result) > cypherBindingCap {
				return nil, fmt.Errorf("cypher: intermediate binding ceiling exceeded")
			}
		}
		if optional && !matched {
			fallback := cloneCypherBinding(original)
			for _, node := range pattern.Nodes {
				if node.Variable != "" {
					if _, exists := fallback[node.Variable]; !exists {
						fallback[node.Variable] = cypherValue{scalar: nil}
					}
				}
			}
			for _, rel := range pattern.Relationships {
				if rel.Variable != "" {
					if _, exists := fallback[rel.Variable]; !exists {
						fallback[rel.Variable] = cypherValue{scalar: nil}
					}
				}
			}
			result = append(result, fallback)
		}
	}
	return result, nil
}

func extendCypherPattern(ctx context.Context, graph cypherGraph, pattern cy.Pattern, relIndex int, current api.Node, binding cypherBinding, usedEdges map[int64]bool, params map[string]any) ([]cypherBinding, error) {
	if relIndex == len(pattern.Relationships) {
		return []cypherBinding{binding}, nil
	}
	rel := pattern.Relationships[relIndex]
	maxDepth := rel.MaxHops
	if maxDepth == 0 || maxDepth > cypherMaxDepth {
		maxDepth = cypherMaxDepth
	}
	type traversal struct {
		node  api.Node
		depth int
		used  map[int64]bool
		last  api.Edge
	}
	queue := []traversal{{node: current, used: cloneEdgeSet(usedEdges)}}
	var result []cypherBinding
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		state := queue[0]
		queue = queue[1:]
		if state.depth >= rel.MinHops && (state.depth > 0 || rel.Variable == "") {
			nextPattern := pattern.Nodes[relIndex+1]
			if nodeMatches(state.node, nextPattern, binding, params) {
				nextBinding := cloneCypherBinding(binding)
				edgeBound := state.depth == 0 || bindEdge(nextBinding, rel.Variable, state.last)
				if bindNode(nextBinding, nextPattern.Variable, state.node) && edgeBound {
					tails, err := extendCypherPattern(ctx, graph, pattern, relIndex+1, state.node, nextBinding, state.used, params)
					if err != nil {
						return nil, err
					}
					result = append(result, tails...)
				}
			}
		}
		if state.depth == maxDepth {
			continue
		}
		for _, step := range relationshipSteps(graph, state.node.ID, rel.Direction) {
			if state.used[step.edge.ID] || !edgeMatches(step.edge, rel) {
				continue
			}
			nextUsed := cloneEdgeSet(state.used)
			nextUsed[step.edge.ID] = true
			queue = append(queue, traversal{node: step.node, depth: state.depth + 1, used: nextUsed, last: step.edge})
		}
	}
	return result, nil
}

type cypherStep struct {
	edge api.Edge
	node api.Node
}

func relationshipSteps(graph cypherGraph, nodeID int64, direction cy.Direction) []cypherStep {
	var result []cypherStep
	seen := map[int64]bool{}
	if direction == cy.Outbound || direction == cy.Any {
		for _, edge := range graph.out[nodeID] {
			if node, ok := graph.nodeByID[edge.TargetID]; ok {
				result = append(result, cypherStep{edge: edge, node: node})
				seen[edge.ID] = true
			}
		}
	}
	if direction == cy.Inbound || direction == cy.Any {
		for _, edge := range graph.in[nodeID] {
			if seen[edge.ID] {
				continue
			}
			if node, ok := graph.nodeByID[edge.SourceID]; ok {
				result = append(result, cypherStep{edge: edge, node: node})
			}
		}
	}
	return result
}

func nodeMatches(node api.Node, pattern cy.NodePattern, binding cypherBinding, params map[string]any) bool {
	if pattern.Variable != "" {
		if bound, ok := binding[pattern.Variable]; ok && (bound.node == nil || bound.node.ID != node.ID) {
			return false
		}
	}
	if len(pattern.Labels) > 0 {
		matched := false
		for _, label := range pattern.Labels {
			matched = matched || strings.EqualFold(node.Label, label)
		}
		if !matched {
			return false
		}
	}
	probe := cloneCypherBinding(binding)
	probe[pattern.Variable] = cypherValue{node: &node}
	for name, expectedExpr := range pattern.Properties {
		expected, err := evalCypherExpr(expectedExpr, probe, params)
		if err != nil || !cypherEqual(nodeProperty(node, name), expected) {
			return false
		}
	}
	return true
}

func edgeMatches(edge api.Edge, pattern cy.RelationshipPattern) bool {
	if len(pattern.Types) > 0 {
		matched := false
		for _, edgeType := range pattern.Types {
			matched = matched || strings.EqualFold(edge.Type, edgeType)
		}
		if !matched {
			return false
		}
	}
	for name, expectedExpr := range pattern.Properties {
		expected, err := evalCypherExpr(expectedExpr, nil, nil)
		if err != nil || !cypherEqual(edgeProperty(edge, name), expected) {
			return false
		}
	}
	return true
}

func bindNode(binding cypherBinding, variable string, node api.Node) bool {
	if variable == "" {
		return true
	}
	if prior, ok := binding[variable]; ok {
		return prior.node != nil && prior.node.ID == node.ID
	}
	copy := node
	binding[variable] = cypherValue{node: &copy}
	return true
}

func bindEdge(binding cypherBinding, variable string, edge api.Edge) bool {
	if variable == "" {
		return true
	}
	if prior, ok := binding[variable]; ok {
		return prior.edge != nil && prior.edge.ID == edge.ID
	}
	copy := edge
	binding[variable] = cypherValue{edge: &copy}
	return true
}

func cloneCypherBinding(source cypherBinding) cypherBinding {
	result := make(cypherBinding, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneEdgeSet(source map[int64]bool) map[int64]bool {
	result := make(map[int64]bool, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
