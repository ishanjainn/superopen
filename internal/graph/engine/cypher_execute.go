package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/api"
	cy "github.com/ishanjainn/superopen/internal/graph/engine/cypher"
)

const cypherRuntimeBinding = "\x00runtime"

type cypherRuntime struct {
	ctx   context.Context
	graph cypherGraph
}

// executeCypher remains protocol-private until the Superopen asset differential
// suite passes. This lets tests exercise the real SQLite-backed executor without
// exposing incomplete semantics through so-graph.
func (s *Store) executeCypher(ctx context.Context, req api.CypherRequest) (api.CypherResult, error) {
	started := time.Now()
	deadlineCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ctx = deadlineCtx
	query, err := cy.Parse(req.Query)
	if err != nil {
		return api.CypherResult{}, err
	}
	switch req.Graph {
	case "", "code":
		if req.Project == "" {
			req.Project, _ = s.defaultProject(ctx)
		}
	case "missed":
		if req.Project == "" {
			req.Project, _ = s.defaultProject(ctx)
		}
		req.Project += "::missed"
	default:
		return api.CypherResult{}, fmt.Errorf("cypher: graph must be code or missed")
	}
	graph, err := s.loadCypherGraph(ctx, req.Project)
	if err != nil {
		return api.CypherResult{}, err
	}
	maxRows := req.MaxRows
	if maxRows <= 0 {
		maxRows = cypherDefaultRows
	}
	if maxRows > cypherMaxRows {
		maxRows = cypherMaxRows
	}
	// Superopen treats an explicit RETURN LIMIT as the caller's chosen
	// result bound, even when max_rows is smaller (test_cypher.c,
	// cypher_apply_limit). Keep the hard engine ceiling regardless.
	if query.Return != nil && query.Return.Limit > maxRows {
		maxRows = minInt(query.Return.Limit, cypherMaxRows)
	}
	branchRows := maxRows
	if query.Union != nil {
		branchRows = cypherMaxRows
	}
	result, err := executeParsedCypher(ctx, graph, query, req.Params, branchRows)
	if err != nil {
		return api.CypherResult{}, err
	}
	for current := query; current.Union != nil; current = current.Union {
		next, err := executeParsedCypher(ctx, graph, current.Union, req.Params, branchRows)
		if err != nil {
			return api.CypherResult{}, err
		}
		if !sameStrings(result.Columns, next.Columns) {
			return api.CypherResult{}, fmt.Errorf("cypher: UNION branches must return the same columns")
		}
		result.Rows = append(result.Rows, next.Rows...)
		if !current.UnionAll {
			result.Rows = distinctMaps(result.Rows, result.Columns)
		}
	}
	if query.Union != nil {
		result.Page.Limit = maxRows
		result.Page.Total = len(result.Rows)
		if len(result.Rows) > maxRows {
			result.Rows = result.Rows[:maxRows]
			result.Page.Truncated = true
		}
	}
	result.ElapsedNS = time.Since(started).Nanoseconds()
	return result, nil
}

func executeParsedCypher(ctx context.Context, graph cypherGraph, query *cy.Query, params map[string]any, maxRows int) (api.CypherResult, error) {
	bindings := []cypherBinding{{cypherRuntimeBinding: {scalar: &cypherRuntime{ctx: ctx, graph: graph}}}}
	if query.Unwind != nil {
		value, err := evalCypherExpr(query.Unwind.Expression, bindings[0], params)
		if err != nil {
			return api.CypherResult{}, err
		}
		items, ok := value.([]any)
		if !ok {
			return api.CypherResult{}, fmt.Errorf("cypher: UNWIND requires a list")
		}
		bindings = make([]cypherBinding, 0, len(items))
		for _, item := range items {
			bindings = append(bindings, cypherBinding{
				cypherRuntimeBinding: {scalar: &cypherRuntime{ctx: ctx, graph: graph}},
				query.Unwind.Alias:   {scalar: item},
			})
		}
	}
	var err error
	for _, clause := range query.Matches {
		for _, pattern := range clause.Patterns {
			bindings, err = matchCypherPattern(ctx, graph, bindings, pattern, clause.Optional, params)
			if err != nil {
				return api.CypherResult{}, err
			}
		}
	}
	if query.Where != nil {
		bindings, err = filterCypherBindings(ctx, bindings, query.Where, params)
		if err != nil {
			return api.CypherResult{}, err
		}
	}
	if query.With != nil {
		bindings, _, _, err = projectCypherBindings(ctx, bindings, *query.With, params, cypherBindingCap)
		if err != nil {
			return api.CypherResult{}, err
		}
		if query.PostWhere != nil {
			bindings, err = filterCypherBindings(ctx, bindings, query.PostWhere, params)
			if err != nil {
				return api.CypherResult{}, err
			}
		}
	}
	columns := bindingColumns(bindings)
	total := len(bindings)
	if query.Return != nil {
		if query.Return.Star {
			bindings, columns, total = projectCypherStar(bindings, query, *query.Return, maxRows)
		} else {
			bindings, columns, total, err = projectCypherBindings(ctx, bindings, *query.Return, params, maxRows)
		}
		if err != nil {
			return api.CypherResult{}, err
		}
	} else {
		bindings, columns, total = projectCypherDefault(bindings, query, maxRows)
	}
	rows := make([]map[string]any, 0, minInt(len(bindings), maxRows))
	for _, binding := range bindings {
		if err := ctx.Err(); err != nil {
			return api.CypherResult{}, err
		}
		if len(rows) == maxRows {
			break
		}
		row := make(map[string]any, len(columns))
		for _, column := range columns {
			row[column] = exportCypherValue(binding[column])
		}
		rows = append(rows, row)
	}
	return api.CypherResult{
		Columns: columns,
		Rows:    rows,
		Page:    api.Page{Limit: maxRows, Total: total, Truncated: total > len(rows)},
	}, nil
}

func projectCypherDefault(input []cypherBinding, query *cy.Query, ceiling int) ([]cypherBinding, []string, int) {
	if len(query.Matches) == 0 || len(query.Matches[0].Patterns) == 0 {
		return input, bindingColumns(input), len(input)
	}
	variables := make([]string, 0, len(query.Matches[0].Patterns[0].Nodes))
	for _, node := range query.Matches[0].Patterns[0].Nodes {
		if node.Variable != "" {
			variables = append(variables, node.Variable)
		}
	}
	columns := make([]string, 0, len(variables)*3)
	for _, variable := range variables {
		columns = append(columns, variable+".name", variable+".qualified_name", variable+".label")
	}
	limit := minInt(len(input), ceiling)
	rows := make([]cypherBinding, 0, limit)
	for _, source := range input[:limit] {
		row := cypherBinding{}
		preserveCypherRuntime(row, source)
		for _, variable := range variables {
			values := [3]any{"", "", ""}
			if node := source[variable].node; node != nil {
				values = [3]any{node.Name, node.QualifiedName, node.Label}
			}
			row[variable+".name"] = cypherValue{scalar: values[0]}
			row[variable+".qualified_name"] = cypherValue{scalar: values[1]}
			row[variable+".label"] = cypherValue{scalar: values[2]}
		}
		rows = append(rows, row)
	}
	return rows, columns, len(input)
}

func projectCypherStar(input []cypherBinding, query *cy.Query, projection cy.Projection, ceiling int) ([]cypherBinding, []string, int) {
	variables := make([]string, 0, 8)
	for _, clause := range query.Matches {
		for _, pattern := range clause.Patterns {
			for index, node := range pattern.Nodes {
				if node.Variable != "" {
					variables = append(variables, node.Variable)
				}
				if index < len(pattern.Relationships) && pattern.Relationships[index].Variable != "" {
					variables = append(variables, pattern.Relationships[index].Variable)
				}
			}
		}
	}
	columns := make([]string, 0, len(variables)*4)
	for _, variable := range variables {
		columns = append(columns, variable+".name", variable+".qualified_name", variable+".label", variable+".file_path")
	}
	rows := make([]cypherBinding, 0, minInt(len(input), ceiling))
	for _, source := range input {
		row := cypherBinding{}
		preserveCypherRuntime(row, source)
		for _, variable := range variables {
			values := [4]any{"", "", "", ""}
			bound := source[variable]
			if bound.node != nil {
				values = [4]any{bound.node.Name, bound.node.QualifiedName, bound.node.Label, bound.node.Location.File}
			} else if bound.edge != nil {
				values[0] = bound.edge.Type
			}
			row[variable+".name"] = cypherValue{scalar: values[0]}
			row[variable+".qualified_name"] = cypherValue{scalar: values[1]}
			row[variable+".label"] = cypherValue{scalar: values[2]}
			row[variable+".file_path"] = cypherValue{scalar: values[3]}
		}
		rows = append(rows, row)
	}
	if projection.Distinct {
		projected := make([]projectedCypherRow, len(rows))
		for index := range rows {
			projected[index] = projectedCypherRow{binding: rows[index], source: rows[index]}
		}
		projected = distinctProjected(projected, columns)
		rows = rows[:0]
		for _, item := range projected {
			rows = append(rows, item.binding)
		}
	}
	total := len(rows)
	rows = applyCypherSlice(rows, projection.Skip, projection.Limit, ceiling)
	return rows, columns, total
}

func filterCypherBindings(ctx context.Context, bindings []cypherBinding, expr cy.Expr, params map[string]any) ([]cypherBinding, error) {
	result := make([]cypherBinding, 0, len(bindings))
	for _, binding := range bindings {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		value, err := evalCypherExpr(expr, binding, params)
		if err != nil {
			return nil, err
		}
		if truthy(value) {
			result = append(result, binding)
		}
	}
	return result, nil
}

type projectedCypherRow struct {
	binding cypherBinding
	source  cypherBinding
}

func projectCypherBindings(ctx context.Context, input []cypherBinding, projection cy.Projection, params map[string]any, ceiling int) ([]cypherBinding, []string, int, error) {
	if projection.Star {
		rows := append([]cypherBinding(nil), input...)
		columns := bindingColumns(rows)
		rows = applyCypherSlice(rows, projection.Skip, projection.Limit, ceiling)
		return rows, columns, len(input), nil
	}
	columns := make([]string, len(projection.Items))
	for index, item := range projection.Items {
		columns[index] = projectionColumn(item, index)
	}
	aggregate := false
	for _, item := range projection.Items {
		aggregate = aggregate || isAggregateExpr(item.Expression)
	}
	var rows []projectedCypherRow
	if aggregate {
		groups, err := groupCypherBindings(ctx, input, projection.Items, params)
		if err != nil {
			return nil, nil, 0, err
		}
		for _, group := range groups {
			if err := ctx.Err(); err != nil {
				return nil, nil, 0, err
			}
			projected := cypherBinding{}
			var source cypherBinding
			if len(group) > 0 {
				source = group[0]
				preserveCypherRuntime(projected, source)
			}
			for index, item := range projection.Items {
				value, err := evalProjectedExpr(item.Expression, group, params)
				if err != nil {
					return nil, nil, 0, err
				}
				projected[columns[index]] = valueToBinding(value, item.Expression, source)
			}
			rows = append(rows, projectedCypherRow{binding: projected, source: source})
		}
	} else {
		for _, source := range input {
			if err := ctx.Err(); err != nil {
				return nil, nil, 0, err
			}
			projected := cypherBinding{}
			preserveCypherRuntime(projected, source)
			for index, item := range projection.Items {
				value, err := evalCypherExpr(item.Expression, source, params)
				if err != nil {
					return nil, nil, 0, err
				}
				projected[columns[index]] = valueToBinding(value, item.Expression, source)
			}
			rows = append(rows, projectedCypherRow{binding: projected, source: source})
		}
	}
	if projection.Distinct {
		rows = distinctProjected(rows, columns)
	}
	if len(projection.OrderBy) > 0 {
		var sortErr error
		sort.SliceStable(rows, func(left, right int) bool {
			if err := ctx.Err(); err != nil {
				sortErr = err
				return false
			}
			for _, order := range projection.OrderBy {
				leftValue, err := evalOrderExpr(order.Expression, rows[left], params)
				if err != nil {
					sortErr = err
					return false
				}
				rightValue, err := evalOrderExpr(order.Expression, rows[right], params)
				if err != nil {
					sortErr = err
					return false
				}
				comparison := compareCypher(leftValue, rightValue)
				if comparison != 0 {
					if order.Descending {
						return comparison > 0
					}
					return comparison < 0
				}
			}
			return false
		})
		if sortErr != nil {
			return nil, nil, 0, sortErr
		}
	}
	total := len(rows)
	start := minInt(projection.Skip, len(rows))
	limit := projection.Limit
	if limit <= 0 || limit > ceiling {
		limit = ceiling
	}
	limit = minInt(limit, cypherBindingCap)
	// Cap by remaining rows without computing start+limit for capacity, so the
	// allocation size cannot overflow.
	count := len(rows) - start
	if limit < count {
		count = limit
	}
	result := make([]cypherBinding, 0, count)
	for _, row := range rows[start : start+count] {
		result = append(result, row.binding)
	}
	return result, columns, total, nil
}

func groupCypherBindings(ctx context.Context, input []cypherBinding, items []cy.ProjectionItem, params map[string]any) ([][]cypherBinding, error) {
	if len(input) == 0 {
		return [][]cypherBinding{{}}, nil
	}
	groups := map[string][]cypherBinding{}
	order := []string{}
	for _, binding := range input {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		keyValues := []any{}
		for _, item := range items {
			if isAggregateExpr(item.Expression) {
				continue
			}
			value, err := evalCypherExpr(item.Expression, binding, params)
			if err != nil {
				return nil, err
			}
			keyValues = append(keyValues, value)
		}
		encoded, _ := json.Marshal(keyValues)
		key := string(encoded)
		if _, exists := groups[key]; !exists {
			order = append(order, key)
		}
		groups[key] = append(groups[key], binding)
	}
	result := make([][]cypherBinding, 0, len(order))
	for _, key := range order {
		result = append(result, groups[key])
	}
	return result, nil
}

func evalProjectedExpr(expr cy.Expr, group []cypherBinding, params map[string]any) (any, error) {
	call, aggregate := expr.(cy.CallExpr)
	if !aggregate || !isAggregateName(call.Name) {
		if len(group) == 0 {
			return nil, nil
		}
		return evalCypherExpr(expr, group[0], params)
	}
	name := strings.ToLower(call.Name)
	values := []any{}
	for _, binding := range group {
		if len(call.Args) == 0 {
			values = append(values, nil)
			continue
		}
		if variable, ok := call.Args[0].(cy.Variable); ok && variable.Name == "*" {
			values = append(values, int64(1))
			continue
		}
		value, err := evalCypherExpr(call.Args[0], binding, params)
		if err != nil {
			return nil, err
		}
		if value != nil {
			values = append(values, value)
		}
	}
	if call.Distinct {
		values = distinctValues(values)
	}
	switch name {
	case "count":
		return int64(len(values)), nil
	case "collect":
		return values, nil
	case "sum", "avg":
		var total float64
		count := 0
		for _, value := range values {
			number, ok := numericValue(value)
			if ok {
				total += number
				count++
			}
		}
		if name == "avg" {
			if count == 0 {
				return nil, nil
			}
			return total / float64(count), nil
		}
		return total, nil
	case "min", "max":
		if len(values) == 0 {
			return nil, nil
		}
		best := values[0]
		for _, value := range values[1:] {
			comparison := compareCypher(value, best)
			if name == "min" && comparison < 0 || name == "max" && comparison > 0 {
				best = value
			}
		}
		return best, nil
	default:
		return nil, fmt.Errorf("cypher: unsupported aggregate %s", call.Name)
	}
}

func valueToBinding(value any, expr cy.Expr, source cypherBinding) cypherValue {
	if variable, ok := expr.(cy.Variable); ok {
		if original, exists := source[variable.Name]; exists {
			return original
		}
	}
	return cypherValue{scalar: value}
}

func projectionColumn(item cy.ProjectionItem, index int) string {
	if item.Alias != "" {
		return item.Alias
	}
	switch expr := item.Expression.(type) {
	case cy.Variable:
		return expr.Name
	case cy.Property:
		return expr.Variable + "." + expr.Name
	case cy.CallExpr:
		return strings.ToLower(expr.Name)
	default:
		return fmt.Sprintf("column_%d", index+1)
	}
}

func evalOrderExpr(expr cy.Expr, row projectedCypherRow, params map[string]any) (any, error) {
	if variable, ok := expr.(cy.Variable); ok {
		if value, exists := row.binding[variable.Name]; exists {
			return exportCypherValue(value), nil
		}
	}
	return evalCypherExpr(expr, row.source, params)
}

func isAggregateExpr(expr cy.Expr) bool {
	call, ok := expr.(cy.CallExpr)
	return ok && isAggregateName(call.Name)
}

func isAggregateName(name string) bool {
	switch strings.ToLower(name) {
	case "count", "sum", "avg", "min", "max", "collect":
		return true
	default:
		return false
	}
}

func distinctProjected(rows []projectedCypherRow, columns []string) []projectedCypherRow {
	seen := map[string]bool{}
	result := rows[:0]
	for _, row := range rows {
		values := make([]any, len(columns))
		for index, column := range columns {
			values[index] = exportCypherValue(row.binding[column])
		}
		encoded, _ := json.Marshal(values)
		if !seen[string(encoded)] {
			seen[string(encoded)] = true
			result = append(result, row)
		}
	}
	return result
}

func distinctValues(values []any) []any {
	seen := map[string]bool{}
	result := values[:0]
	for _, value := range values {
		encoded, _ := json.Marshal(value)
		if !seen[string(encoded)] {
			seen[string(encoded)] = true
			result = append(result, value)
		}
	}
	return result
}

func applyCypherSlice(rows []cypherBinding, skip, limit, ceiling int) []cypherBinding {
	start := minInt(skip, len(rows))
	if limit == 0 {
		limit = ceiling
	}
	limit = minInt(limit, cypherBindingCap)
	end := len(rows)
	if limit < len(rows)-start {
		end = start + limit
	}
	return rows[start:end]
}

func bindingColumns(bindings []cypherBinding) []string {
	seen := map[string]bool{}
	for _, binding := range bindings {
		for key := range binding {
			if key != cypherRuntimeBinding {
				seen[key] = true
			}
		}
	}
	columns := make([]string, 0, len(seen))
	for key := range seen {
		columns = append(columns, key)
	}
	sort.Strings(columns)
	return columns
}

func preserveCypherRuntime(target, source cypherBinding) {
	if runtime, ok := source[cypherRuntimeBinding]; ok {
		target[cypherRuntimeBinding] = runtime
	}
}

func distinctMaps(rows []map[string]any, columns []string) []map[string]any {
	seen := map[string]bool{}
	result := rows[:0]
	for _, row := range rows {
		values := make([]any, len(columns))
		for index, column := range columns {
			values[index] = row[column]
		}
		encoded, _ := json.Marshal(values)
		if !seen[string(encoded)] {
			seen[string(encoded)] = true
			result = append(result, row)
		}
	}
	return result
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
