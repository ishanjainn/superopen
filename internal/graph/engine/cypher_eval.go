package engine

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
	cy "github.com/ishanjainn/superopen/internal/graph/engine/cypher"
)

func evalCypherExpr(expr cy.Expr, binding cypherBinding, params map[string]any) (any, error) {
	switch value := expr.(type) {
	case cy.Literal:
		return value.Value, nil
	case cy.ParameterExpr:
		result, ok := params[value.Name]
		if !ok {
			return nil, fmt.Errorf("cypher: missing parameter $%s", value.Name)
		}
		return result, nil
	case cy.Variable:
		if value.Name == "*" {
			return "*", nil
		}
		return exportCypherValue(binding[value.Name]), nil
	case cy.Property:
		bound, ok := binding[value.Variable]
		if !ok {
			return nil, nil
		}
		if bound.node != nil {
			if value.Name == "in_degree" || value.Name == "out_degree" {
				if runtime, ok := binding[cypherRuntimeBinding].scalar.(*cypherRuntime); ok {
					edges := runtime.graph.in[bound.node.ID]
					if value.Name == "out_degree" {
						edges = runtime.graph.out[bound.node.ID]
					}
					var degree int64
					for _, edge := range edges {
						if edge.Type == "CALLS" {
							degree++
						}
					}
					return degree, nil
				}
			}
			return nodeProperty(*bound.node, value.Name), nil
		}
		if bound.edge != nil {
			return edgeProperty(*bound.edge, value.Name), nil
		}
		if object, ok := bound.scalar.(map[string]any); ok {
			return object[value.Name], nil
		}
		return nil, nil
	case cy.LabelTest:
		bound := binding[value.Variable]
		if bound.node == nil {
			return false, nil
		}
		for _, label := range value.Labels {
			if strings.EqualFold(bound.node.Label, label) {
				return true, nil
			}
		}
		return false, nil
	case cy.ListExpr:
		items := make([]any, 0, len(value.Items))
		for _, item := range value.Items {
			resolved, err := evalCypherExpr(item, binding, params)
			if err != nil {
				return nil, err
			}
			items = append(items, resolved)
		}
		return items, nil
	case cy.UnaryExpr:
		resolved, err := evalCypherExpr(value.Value, binding, params)
		if err != nil {
			return nil, err
		}
		if value.Op == "NOT" {
			return !truthy(resolved), nil
		}
		return nil, fmt.Errorf("cypher: unsupported unary operator %s", value.Op)
	case cy.IsNullExpr:
		resolved, err := evalCypherExpr(value.Value, binding, params)
		if err != nil {
			return nil, err
		}
		// The pinned engine exposes its string-backed store semantics here:
		// absent and empty properties both satisfy IS NULL.
		isNull := resolved == nil || fmt.Sprint(resolved) == ""
		if value.Not {
			isNull = !isNull
		}
		return isNull, nil
	case cy.BinaryExpr:
		return evalCypherBinary(value, binding, params)
	case cy.CallExpr:
		return evalCypherCall(value, binding, params)
	case cy.CaseExpr:
		for _, branch := range value.Branches {
			condition, err := evalCypherExpr(branch.When, binding, params)
			if err != nil {
				return nil, err
			}
			if truthy(condition) {
				return evalCypherExpr(branch.Then, binding, params)
			}
		}
		if value.Else != nil {
			return evalCypherExpr(value.Else, binding, params)
		}
		return nil, nil
	case cy.ExistsPattern:
		runtimeValue := binding[cypherRuntimeBinding].scalar
		runtime, ok := runtimeValue.(*cypherRuntime)
		if !ok {
			return nil, fmt.Errorf("cypher: EXISTS requires graph execution context")
		}
		matches, err := matchCypherPattern(runtime.ctx, runtime.graph, []cypherBinding{binding}, value.Pattern, false, params)
		if err != nil {
			return nil, err
		}
		return len(matches) > 0, nil
	default:
		return nil, fmt.Errorf("cypher: unsupported expression %T", expr)
	}
}

func evalCypherBinary(expr cy.BinaryExpr, binding cypherBinding, params map[string]any) (any, error) {
	left, err := evalCypherExpr(expr.Left, binding, params)
	if err != nil {
		return nil, err
	}
	if expr.Op == "AND" && !truthy(left) {
		return false, nil
	}
	if expr.Op == "OR" && truthy(left) {
		return true, nil
	}
	right, err := evalCypherExpr(expr.Right, binding, params)
	if err != nil {
		return nil, err
	}
	switch expr.Op {
	case "AND":
		return truthy(left) && truthy(right), nil
	case "OR":
		return truthy(left) || truthy(right), nil
	case "XOR":
		return truthy(left) != truthy(right), nil
	case "=":
		return cypherEqual(left, right), nil
	case "<>":
		return !cypherEqual(left, right), nil
	case "CONTAINS":
		return strings.Contains(fmt.Sprint(left), fmt.Sprint(right)), nil
	case "STARTS WITH":
		return strings.HasPrefix(fmt.Sprint(left), fmt.Sprint(right)), nil
	case "ENDS WITH":
		return strings.HasSuffix(fmt.Sprint(left), fmt.Sprint(right)), nil
	case "=~":
		pattern, err := regexp.Compile(fmt.Sprint(right))
		if err != nil {
			return nil, fmt.Errorf("cypher: invalid regular expression: %w", err)
		}
		return pattern.MatchString(fmt.Sprint(left)), nil
	case "IN", "NOT IN":
		items, ok := right.([]any)
		if !ok {
			return nil, fmt.Errorf("cypher: %s requires a list", expr.Op)
		}
		found := false
		for _, item := range items {
			found = found || cypherEqual(left, item)
		}
		if expr.Op == "NOT IN" {
			found = !found
		}
		return found, nil
	case ">", "<", ">=", "<=":
		comparison := compareCypher(left, right)
		switch expr.Op {
		case ">":
			return comparison > 0, nil
		case "<":
			return comparison < 0, nil
		case ">=":
			return comparison >= 0, nil
		default:
			return comparison <= 0, nil
		}
	default:
		return nil, fmt.Errorf("cypher: unsupported binary operator %s", expr.Op)
	}
}

func evalCypherCall(call cy.CallExpr, binding cypherBinding, params map[string]any) (any, error) {
	name := strings.ToLower(call.Name)
	args := make([]any, len(call.Args))
	for i, arg := range call.Args {
		value, err := evalCypherExpr(arg, binding, params)
		if err != nil {
			return nil, err
		}
		args[i] = value
	}
	arg := func(index int) any {
		if index >= len(args) {
			return nil
		}
		return args[index]
	}
	switch name {
	case "tolower":
		return strings.ToLower(fmt.Sprint(arg(0))), nil
	case "toupper":
		return strings.ToUpper(fmt.Sprint(arg(0))), nil
	case "tostring":
		return fmt.Sprint(arg(0)), nil
	case "tointeger":
		value, err := strconv.ParseInt(fmt.Sprint(arg(0)), 10, 64)
		if err != nil {
			return nil, nil
		}
		return value, nil
	case "tofloat":
		value, err := strconv.ParseFloat(fmt.Sprint(arg(0)), 64)
		if err != nil {
			return nil, nil
		}
		return value, nil
	case "coalesce":
		for _, value := range args {
			if value != nil && fmt.Sprint(value) != "" {
				return value, nil
			}
		}
		return nil, nil
	case "substring":
		text := []rune(fmt.Sprint(arg(0)))
		start := cypherIntArg(arg(1))
		if start < 0 || start > len(text) {
			return "", nil
		}
		end := len(text)
		if len(args) > 2 {
			length := cypherIntArg(arg(2))
			if length < 0 {
				length = 0
			}
			remaining := end - start
			if length < remaining {
				end = start + length
			}
		}
		return string(text[start:end]), nil
	case "size", "length":
		switch value := arg(0).(type) {
		case string:
			return int64(len([]rune(value))), nil
		case []any:
			return int64(len(value)), nil
		default:
			return int64(0), nil
		}
	case "reverse":
		runes := []rune(fmt.Sprint(arg(0)))
		for left, right := 0, len(runes)-1; left < right; left, right = left+1, right-1 {
			runes[left], runes[right] = runes[right], runes[left]
		}
		return string(runes), nil
	case "left":
		runes := []rune(fmt.Sprint(arg(0)))
		length := maxInt(0, cypherIntArg(arg(1)))
		end := minInt(len(runes), length)
		return string(runes[:end]), nil
	case "right":
		runes := []rune(fmt.Sprint(arg(0)))
		length := maxInt(0, cypherIntArg(arg(1)))
		start := maxInt(0, len(runes)-length)
		return string(runes[start:]), nil
	case "replace":
		return strings.ReplaceAll(fmt.Sprint(arg(0)), fmt.Sprint(arg(1)), fmt.Sprint(arg(2))), nil
	case "id":
		return entityID(binding, call.Args), nil
	case "type":
		return entityType(binding, call.Args), nil
	case "labels":
		if label := entityType(binding, call.Args); label != nil {
			return []any{label}, nil
		}
		return []any{}, nil
	case "keys":
		object := entityProperties(binding, call.Args)
		keys := make([]any, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i].(string) < keys[j].(string) })
		return keys, nil
	case "properties":
		return entityProperties(binding, call.Args), nil
	case "count", "sum", "avg", "min", "max", "collect":
		return nil, fmt.Errorf("cypher: aggregate %s is only valid in a projection", call.Name)
	default:
		return nil, fmt.Errorf("cypher: unsupported function %s", call.Name)
	}
}

func nodeProperty(node api.Node, name string) any {
	switch strings.ToLower(name) {
	case "id":
		return node.ID
	case "project":
		return node.Project
	case "label":
		return node.Label
	case "name":
		return node.Name
	case "qualified_name":
		return node.QualifiedName
	case "file_path", "file":
		return node.Location.File
	case "start_line":
		return node.Location.StartLine
	case "start_column":
		return node.Location.StartColumn
	case "end_line":
		return node.Location.EndLine
	case "end_column":
		return node.Location.EndColumn
	default:
		return node.Properties[name]
	}
}

func edgeProperty(edge api.Edge, name string) any {
	switch strings.ToLower(name) {
	case "id":
		return edge.ID
	case "project":
		return edge.Project
	case "source_id":
		return edge.SourceID
	case "target_id":
		return edge.TargetID
	case "type", "label":
		return edge.Type
	default:
		return edge.Properties[name]
	}
}

func truthy(value any) bool {
	switch value := value.(type) {
	case nil:
		return false
	case bool:
		return value
	case string:
		return value != ""
	case int:
		return value != 0
	case int64:
		return value != 0
	case float64:
		return value != 0
	default:
		return true
	}
}

func numericValue(value any) (float64, bool) {
	switch value := value.(type) {
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case float64:
		return value, true
	case json.Number:
		parsed, err := value.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(value, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

// cypherIntArg converts Cypher numeric arguments to int with constant bounds
// checks so int64 values from ParseInt/literals cannot silently truncate.
func cypherIntArg(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return clampInt64ToInt(v)
	case float64:
		return clampFloat64ToInt(v)
	case json.Number:
		if parsed, err := v.Int64(); err == nil {
			return clampInt64ToInt(parsed)
		}
		parsed, err := v.Float64()
		if err != nil {
			return 0
		}
		return clampFloat64ToInt(parsed)
	case string:
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			return clampInt64ToInt(parsed)
		}
		parsed, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0
		}
		return clampFloat64ToInt(parsed)
	default:
		parsed, ok := numericValue(value)
		if !ok {
			return 0
		}
		return clampFloat64ToInt(parsed)
	}
}

func clampInt64ToInt(value int64) int {
	if value > math.MaxInt {
		return math.MaxInt
	}
	if value < math.MinInt {
		return math.MinInt
	}
	return int(value)
}

func clampFloat64ToInt(value float64) int {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	if value >= float64(math.MaxInt) {
		return math.MaxInt
	}
	if value <= float64(math.MinInt) {
		return math.MinInt
	}
	return clampInt64ToInt(int64(value))
}

func cypherEqual(left, right any) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	if lnum, ok := numericValue(left); ok {
		if rnum, ok := numericValue(right); ok {
			return lnum == rnum
		}
	}
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return string(leftJSON) == string(rightJSON)
}

func compareCypher(left, right any) int {
	if lnum, ok := numericValue(left); ok {
		if rnum, ok := numericValue(right); ok {
			if lnum < rnum {
				return -1
			}
			if lnum > rnum {
				return 1
			}
			return 0
		}
	}
	return strings.Compare(fmt.Sprint(left), fmt.Sprint(right))
}

func exportCypherValue(value cypherValue) any {
	if value.node != nil {
		return map[string]any{"id": value.node.ID, "project": value.node.Project, "label": value.node.Label,
			"name": value.node.Name, "qualified_name": value.node.QualifiedName, "file_path": value.node.Location.File,
			"start_line": value.node.Location.StartLine, "start_column": value.node.Location.StartColumn,
			"end_line": value.node.Location.EndLine, "end_column": value.node.Location.EndColumn,
			"properties": value.node.Properties}
	}
	if value.edge != nil {
		return map[string]any{"id": value.edge.ID, "project": value.edge.Project, "source_id": value.edge.SourceID,
			"target_id": value.edge.TargetID, "type": value.edge.Type, "properties": value.edge.Properties}
	}
	return value.scalar
}

func entityBinding(binding cypherBinding, args []cy.Expr) cypherValue {
	if len(args) != 1 {
		return cypherValue{}
	}
	variable, ok := args[0].(cy.Variable)
	if !ok {
		return cypherValue{}
	}
	return binding[variable.Name]
}

func entityID(binding cypherBinding, args []cy.Expr) any {
	value := entityBinding(binding, args)
	if value.node != nil {
		return value.node.ID
	}
	if value.edge != nil {
		return value.edge.ID
	}
	return nil
}

func entityType(binding cypherBinding, args []cy.Expr) any {
	value := entityBinding(binding, args)
	if value.node != nil {
		return value.node.Label
	}
	if value.edge != nil {
		return value.edge.Type
	}
	return nil
}

func entityProperties(binding cypherBinding, args []cy.Expr) map[string]any {
	value := entityBinding(binding, args)
	if value.node != nil {
		result := map[string]any{}
		for key, item := range value.node.Properties {
			result[key] = item
		}
		result["name"], result["qualified_name"], result["label"], result["file_path"] =
			value.node.Name, value.node.QualifiedName, value.node.Label, value.node.Location.File
		return result
	}
	if value.edge != nil {
		result := map[string]any{}
		for key, item := range value.edge.Properties {
			result[key] = item
		}
		result["type"] = value.edge.Type
		return result
	}
	if object, ok := value.scalar.(map[string]any); ok {
		return object
	}
	return map[string]any{}
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
