package engine

import (
	"context"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/graph/engine/cpreproc"
)

const sourceOriginPreprocessed = "preprocessed"

func isCFamilyLanguage(language string) bool {
	switch language {
	case "c", "cpp", "cuda":
		return true
	default:
		return false
	}
}

func cFamilyCPPMode(language string) bool {
	return language != "c"
}

// enrichCFamilyExtract runs C-family extras after the raw Tree-sitter extract:
// a simplecpp second parse for macro-hidden CALLS, then keeps raw defs except
// ERROR-region recovery. Fail closed: missing preprocessor or parse errors
// leave the raw FileResult unchanged.
func enrichCFamilyExtract(ctx context.Context, parser SyntaxParser, language, grammar, filename string, raw []byte, result FileResult) FileResult {
	if !isCFamilyLanguage(language) || len(raw) == 0 {
		return result
	}
	if err := ctx.Err(); err != nil {
		return result
	}
	expanded := cpreproc.WithMap(raw, filename, cFamilyCPPMode(language))
	if expanded == nil || expanded.Source == "" {
		return result
	}
	second, err := extractSyntaxFile(ctx, parser, language, grammar, []byte(expanded.Source))
	if err != nil {
		return result
	}
	return mergePreprocessedFileResult(result, second, expanded, raw)
}

func mergePreprocessedFileResult(raw, expanded FileResult, pp *cpreproc.Result, original []byte) FileResult {
	for _, call := range expanded.Calls {
		if fact, ok := remapPreprocessedFact(call, pp); ok {
			raw.Calls = append(raw.Calls, fact)
		}
	}
	for _, usage := range expanded.Usages {
		if fact, ok := remapPreprocessedOccurrence(usage, pp); ok {
			raw.Usages = append(raw.Usages, fact)
		}
	}
	raw.Definitions = append(raw.Definitions, adoptPreprocessedDefs(raw, expanded, pp, original)...)
	return raw
}

func remapPreprocessedFact(fact SyntaxFact, pp *cpreproc.Result) (SyntaxFact, bool) {
	line, main := remapExpandedLine(pp, fact.StartLine)
	if !main || line <= 0 {
		return SyntaxFact{}, false
	}
	end, endMain := remapExpandedLine(pp, fact.EndLine)
	if !endMain || end <= 0 {
		end = line
	}
	fact.StartLine = line
	fact.EndLine = end
	fact.SourceOrigin = sourceOriginPreprocessed
	return fact, true
}

func remapPreprocessedOccurrence(fact OccurrenceFact, pp *cpreproc.Result) (OccurrenceFact, bool) {
	line, main := remapExpandedLine(pp, int(fact.StartLine))
	if !main || line <= 0 {
		return OccurrenceFact{}, false
	}
	end, endMain := remapExpandedLine(pp, int(fact.EndLine))
	if !endMain || end <= 0 {
		end = line
	}
	fact.StartLine = int32(line)
	fact.EndLine = int32(end)
	fact.SourceOrigin = sourceOriginPreprocessed
	return fact, true
}

func remapExpandedLine(pp *cpreproc.Result, expandedLine int) (int, bool) {
	if pp == nil || expandedLine <= 0 || expandedLine >= len(pp.OriginalLine) {
		return expandedLine, true
	}
	orig := int(pp.OriginalLine[expandedLine])
	return orig, pp.BelongsToMain[expandedLine]
}

func adoptPreprocessedDefs(raw, expanded FileResult, pp *cpreproc.Result, original []byte) []SyntaxFact {
	if len(raw.ParseStatus.ErrorRanges) == 0 {
		return nil
	}
	seen := map[string]bool{}
	for _, def := range raw.Definitions {
		seen[def.Kind+"\x00"+def.Scope+"\x00"+def.Name] = true
	}
	var adopted []SyntaxFact
	for _, def := range expanded.Definitions {
		if def.Kind != "function" || def.Name == "" {
			continue
		}
		fact, ok := remapPreprocessedFact(def, pp)
		if !ok {
			continue
		}
		if !defIntersectsError(fact, raw.ParseStatus.ErrorRanges) {
			continue
		}
		if !sourceLineContains(original, fact.StartLine, fact.Name) {
			continue
		}
		key := fact.Kind + "\x00" + fact.Scope + "\x00" + fact.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		adopted = append(adopted, fact)
	}
	return adopted
}

func defIntersectsError(def SyntaxFact, ranges []api.Location) bool {
	for _, region := range ranges {
		if def.StartLine <= region.EndLine && def.EndLine >= region.StartLine {
			return true
		}
	}
	return false
}

func sourceLineContains(source []byte, line int, name string) bool {
	if line <= 0 || name == "" {
		return false
	}
	current := 1
	start := 0
	for i := 0; i <= len(source); i++ {
		if i == len(source) || source[i] == '\n' {
			if current == line {
				return strings.Contains(string(source[start:i]), name)
			}
			current++
			start = i + 1
		}
	}
	return false
}

func syntaxCFamilyParameterList(node syntaxView) (syntaxView, bool) {
	if viewMissing(node) {
		return nil, false
	}
	if node.Kind() == "parameter_list" {
		return node, true
	}
	if params, ok := findField(node, "parameters"); ok && !viewMissing(params) {
		if params.Kind() == "parameter_list" {
			return params, true
		}
		if nested, ok := syntaxCFamilyParameterList(params); ok {
			return nested, true
		}
	}
	if decl, ok := findField(node, "declarator"); ok {
		if nested, ok := syntaxCFamilyParameterList(decl); ok {
			return nested, true
		}
	}
	for i := 0; i < node.ChildCount(); i++ {
		child := node.ChildAt(i)
		switch child.Kind() {
		case "function_declarator", "parameter_list", "parenthesized_declarator", "pointer_declarator":
			if nested, ok := syntaxCFamilyParameterList(child); ok {
				return nested, true
			}
		}
	}
	return nil, false
}

func syntaxCFamilyParams(list syntaxView, source []byte) (names, types []string) {
	if viewMissing(list) {
		return nil, nil
	}
	for i := 0; i < list.ChildCount(); i++ {
		parameter := list.ChildAt(i)
		if !parameter.IsNamed() || parameter.Kind() != "parameter_declaration" {
			continue
		}
		name := syntaxCDeclaratorIdent(parameter, source)
		if name == "" {
			continue
		}
		typ := ""
		if typeNode, ok := findField(parameter, "type"); ok {
			typ = cleanCTypeName(nodeText(typeNode, source))
		}
		names = append(names, name)
		types = append(types, typ)
	}
	return names, types
}

func syntaxCDeclaratorIdent(node syntaxView, source []byte) string {
	if viewMissing(node) {
		return ""
	}
	if node.Kind() == "identifier" || node.Kind() == "field_identifier" {
		return strings.TrimSpace(nodeText(node, source))
	}
	if decl, ok := findField(node, "declarator"); ok {
		return syntaxCDeclaratorIdent(decl, source)
	}
	if node.Kind() == "parameter_declaration" {
		return ""
	}
	for i := 0; i < node.ChildCount(); i++ {
		child := node.ChildAt(i)
		switch child.Kind() {
		case "identifier", "field_identifier", "pointer_declarator", "array_declarator",
			"function_declarator", "parenthesized_declarator", "reference_declarator":
			if name := syntaxCDeclaratorIdent(child, source); name != "" {
				return name
			}
		}
	}
	return ""
}

func cleanCTypeName(raw string) string {
	name := cleanSyntaxType(raw)
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "struct ")
	name = strings.TrimPrefix(name, "enum ")
	name = strings.TrimPrefix(name, "union ")
	name = strings.TrimPrefix(name, "class ")
	name = strings.Trim(name, "*& \t")
	if index := strings.LastIndexAny(name, " \t"); index >= 0 {
		name = strings.TrimSpace(name[index+1:])
	}
	return strings.Trim(name, "*&")
}

func fillCFamilyFunctionParams(language string, node syntaxView, source []byte, fact *SyntaxFact) {
	if fact == nil || !isCFamilyLanguage(language) || len(fact.ParamNames) > 0 {
		return
	}
	list, ok := syntaxCFamilyParameterList(node)
	if !ok {
		return
	}
	names, types := syntaxCFamilyParams(list, source)
	fact.ParamNames = names
	if len(fact.ParamTypes) == 0 {
		fact.ParamTypes = types
	}
	if fact.Signature == "" {
		fact.Signature = strings.TrimSpace(nodeText(list, source))
	}
}
