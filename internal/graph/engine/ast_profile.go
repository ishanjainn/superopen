package engine

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	profileIf = iota
	profileFor
	profileWhile
	profileSwitch
	profileTry
	profileReturn
	profileMaxDepth
	profileAverageDepthX10
	profileComparison
	profileArithmetic
	profileLogical
	profileAssignment
	profileString
	profileNumber
	profileBool
	profileParameters
	profileParamsInReturns
	profileParamsInConditions
	profileVariableReassigns
	profileUniqueOperators
	profileUniqueOperands
	profileTotalOperators
	profileTotalOperands
	profileBodyLines
	profileBodyTokens
)

type ASTProfile [semanticProfileDims]uint16

func ComputeASTProfile(root syntaxView, source []byte, parameterNames []string) (ASTProfile, bool) {
	var profile ASTProfile
	profile[profileParameters] = uint16(len(parameterNames))
	type frame struct {
		node  syntaxView
		depth int
	}
	stack := []frame{{node: root}}
	operatorSet, operandSet := map[uint32]bool{}, map[uint32]bool{}
	totalDepth, nodeCount := 0, 0
	for len(stack) > 0 && nodeCount < 2048 {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		node := current.node
		if node == nil {
			continue
		}
		if node.IsNamed() || node.ChildCount() > 0 {
			nodeCount++
			totalDepth += current.depth
			if current.depth > int(profile[profileMaxDepth]) {
				profile[profileMaxDepth] = uint16(current.depth)
			}
			accumulateProfileKind(&profile, node.Kind())
			if profileOperator(node.Kind()) {
				profile[profileTotalOperators]++
				hash := profileHash(node.Kind())
				if !operatorSet[hash] && len(operatorSet) < 512 {
					operatorSet[hash] = true
					profile[profileUniqueOperators]++
				}
			}
			if node.ChildCount() == 0 && profileOperand(node.Kind()) {
				profile[profileTotalOperands]++
				profile[profileBodyTokens]++
				hash := profileHash(node.Kind())
				if !operandSet[hash] && len(operandSet) < 512 {
					operandSet[hash] = true
					profile[profileUniqueOperands]++
				}
			}
		}
		var kids []syntaxView
		viewEachChild(node, func(child syntaxView) { kids = append(kids, child) })
		for index := len(kids) - 1; index >= 0 && len(stack) < 2048; index-- {
			stack = append(stack, frame{node: kids[index], depth: current.depth + 1})
		}
	}
	if nodeCount == 0 {
		return ASTProfile{}, false
	}
	profile[profileAverageDepthX10] = uint16(totalDepth * 10 / nodeCount)
	lines := sourceLineIndex(source)
	startLine, _ := bytePosition(lines, root.StartByte())
	endLine, _ := bytePosition(lines, root.EndByte())
	if endLine >= startLine {
		profile[profileBodyLines] = uint16(endLine - startLine + 1)
	}
	return profile, true
}

func accumulateProfileKind(profile *ASTProfile, kind string) {
	if profileControlIf(kind) {
		profile[profileIf]++
	}
	if profileControlFor(kind) {
		profile[profileFor]++
	}
	if profileControlWhile(kind) {
		profile[profileWhile]++
	}
	if profileControlSwitch(kind) {
		profile[profileSwitch]++
	}
	if profileControlTry(kind) {
		profile[profileTry]++
	}
	if kind == "return_statement" || kind == "return_expression" {
		profile[profileReturn]++
	}
	if kind == "binary_expression" || kind == "comparison_operator" || kind == "boolean_operator" {
		profile[profileComparison]++
	}
	if kind == "unary_expression" || kind == "update_expression" {
		profile[profileArithmetic]++
	}
	if kind == "not_operator" || kind == "boolean_operator" {
		profile[profileLogical]++
	}
	if kind == "assignment_expression" || kind == "assignment_statement" || kind == "augmented_assignment" || kind == "short_var_declaration" {
		profile[profileAssignment]++
		profile[profileVariableReassigns]++
	}
	if kind == "string" || kind == "string_literal" || kind == "interpreted_string_literal" || kind == "raw_string_literal" {
		profile[profileString]++
	}
	if kind == "number" || kind == "integer" || kind == "float" || kind == "integer_literal" || kind == "float_literal" {
		profile[profileNumber]++
	}
	if kind == "true" || kind == "false" {
		profile[profileBool]++
	}
}

func profileControlIf(kind string) bool {
	return kind == "if_statement" || kind == "if_expression" || kind == "elif_clause"
}
func profileControlFor(kind string) bool {
	return kind == "for_statement" || kind == "for_range_loop" || kind == "for_expression" || kind == "for_in_clause"
}
func profileControlWhile(kind string) bool {
	return kind == "while_statement" || kind == "while_expression" || kind == "do_statement"
}
func profileControlSwitch(kind string) bool {
	return kind == "switch_statement" || kind == "switch_expression" || kind == "match_expression" || kind == "type_switch_statement"
}
func profileControlTry(kind string) bool {
	return kind == "try_statement" || kind == "try_expression" || kind == "catch_clause" || kind == "except_clause"
}

func profileOperator(kind string) bool {
	return profileControlIf(kind) || profileControlFor(kind) || profileControlWhile(kind) || profileControlSwitch(kind) || profileControlTry(kind) ||
		kind == "return_statement" || kind == "return_expression" || kind == "binary_expression" || kind == "comparison_operator" ||
		kind == "boolean_operator" || kind == "unary_expression" || kind == "update_expression" || kind == "assignment_expression" ||
		kind == "assignment_statement" || kind == "augmented_assignment" || kind == "short_var_declaration" || kind == "call_expression" ||
		kind == "member_expression" || kind == "subscript_expression"
}

func profileOperand(kind string) bool {
	return kind == "identifier" || kind == "field_identifier" || kind == "property_identifier" || kind == "type_identifier" ||
		kind == "string" || kind == "string_literal" || kind == "interpreted_string_literal" || kind == "raw_string_literal" ||
		kind == "number" || kind == "integer" || kind == "float" || kind == "integer_literal" || kind == "float_literal" ||
		kind == "true" || kind == "false"
}

func profileHash(value string) uint32 {
	var result uint32
	for _, current := range []byte(value) {
		result = result*31 + uint32(current)
	}
	return result | 1
}

func (profile ASTProfile) Vector() [semanticProfileDims]float32 {
	divisors := [semanticProfileDims]float32{100, 100, 100, 100, 100, 100, 20, 200, 100, 100, 100, 100, 100, 100, 100, 20, 100, 100, 100, 200, 200, 200, 200, 2000, 2000}
	var result [semanticProfileDims]float32
	for index, value := range profile {
		result[index] = float32(value) / divisors[index]
	}
	return result
}

// parseASTProfile reads back the comma-separated form written by String.
func parseASTProfile(encoded string) (ASTProfile, bool) {
	parts := strings.Split(encoded, ",")
	if len(parts) != semanticProfileDims {
		return ASTProfile{}, false
	}
	var profile ASTProfile
	for index, part := range parts {
		value, err := strconv.ParseUint(strings.TrimSpace(part), 10, 16)
		if err != nil {
			return ASTProfile{}, false
		}
		profile[index] = uint16(value)
	}
	return profile, true
}

func (profile ASTProfile) String() string {
	parts := make([]string, len(profile))
	for index, value := range profile {
		parts[index] = fmt.Sprint(value)
	}
	return strings.Join(parts, ",")
}
