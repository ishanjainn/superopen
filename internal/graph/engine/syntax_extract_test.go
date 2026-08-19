package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractSyntaxFactsUsesPinnedFieldsAndScopes(t *testing.T) {
	t.Parallel()
	source := []byte("func Run() {\n helper()\n}\n")
	tree := SyntaxNode{Type: "source_file", Named: true, End: uint32(len(source)), Children: []SyntaxNode{
		{Type: "function_declaration", Named: true, Start: 0, End: 25, Children: []SyntaxNode{
			{Type: "identifier", Field: "name", Named: true, Start: 5, End: 8},
			{Type: "call_expression", Named: true, Start: 14, End: 22, Children: []SyntaxNode{
				{Type: "identifier", Field: "function", Named: true, Start: 14, End: 20},
			}},
		}},
	}}
	got, err := ExtractSyntaxFacts("go", tree, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Definitions) != 1 || got.Definitions[0].Name != "Run" || got.Definitions[0].StartLine != 1 {
		t.Fatalf("definitions = %#v", got.Definitions)
	}
	if got.Definitions[0].StructuralProfile == "" {
		t.Fatal("function structural profile is missing")
	}
	if !got.Definitions[0].IsExported || got.Definitions[0].IsEntryPoint {
		t.Fatalf("definition flags = %#v", got.Definitions[0])
	}
	if len(got.Calls) != 1 || got.Calls[0].Name != "helper" || got.Calls[0].Scope != "Run" || got.Calls[0].StartLine != 2 {
		t.Fatalf("calls = %#v", got.Calls)
	}
}

func TestPinnedExportRules(t *testing.T) {
	for _, test := range []struct {
		name, language string
		want           bool
	}{
		{"Public", "go", true}, {"private", "go", false},
		{"public", "python", true}, {"_private", "python", false},
		{"anything", "rust", true}, {"", "rust", false},
	} {
		if got := pinnedIsExported(test.name, test.language); got != test.want {
			t.Errorf("pinnedIsExported(%q,%q)=%v, want %v", test.name, test.language, got, test.want)
		}
	}
}

func TestExtractSyntaxFactsPreservesPartialTreesAndRejectsUnknownLanguage(t *testing.T) {
	t.Parallel()
	got, err := ExtractSyntaxFacts("python", SyntaxNode{Type: "module", Named: true, HasError: true}, nil)
	if err != nil || !got.Partial {
		t.Fatalf("partial extraction = %#v, %v", got, err)
	}
	if _, err := ExtractSyntaxFacts("unknown", SyntaxNode{}, nil); err == nil {
		t.Fatal("unknown language must fail loudly")
	}
}

func TestExtractSyntaxFactsImportsBranchesAndThrows(t *testing.T) {
	t.Parallel()
	source := []byte("import os\nif ok:\n raise Err()\n")
	errStart := strings.Index(string(source), "Err")
	tree := SyntaxNode{Type: "module", Named: true, End: uint32(len(source)), Children: []SyntaxNode{
		{Type: "import_statement", Named: true, Start: 0, End: 9, Children: []SyntaxNode{{Type: "identifier", Field: "name", Named: true, Start: 7, End: 9}}},
		{Type: "if_statement", Named: true, Start: 10, End: uint32(len(source)), Children: []SyntaxNode{
			{Type: "raise_statement", Named: true, Start: 18, End: uint32(len(source)), Children: []SyntaxNode{
				{Type: "call", Named: true, Start: uint32(errStart), End: uint32(errStart + len("Err()")), Children: []SyntaxNode{
					{Type: "identifier", Field: "function", Named: true, Start: uint32(errStart), End: uint32(errStart + len("Err"))},
				}},
			}},
		}},
	}}
	got, err := ExtractSyntaxFacts("python", tree, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Imports) != 1 || got.Imports[0].Name != "os" || len(got.Branches) != 1 || len(got.Throws) != 1 || got.Throws[0].Name != "Err" {
		t.Fatalf("facts = %#v", got)
	}
}

func TestExtractSyntaxFactsMatchesPythonFromImportSemantics(t *testing.T) {
	t.Parallel()
	source := []byte("from pathlib import Path as P\n")
	moduleStart := strings.Index(string(source), "pathlib")
	nameStart := strings.Index(string(source), "Path")
	aliasStart := strings.LastIndex(string(source), "P")
	aliased := SyntaxNode{Type: "aliased_import", Named: true, Start: uint32(nameStart), End: uint32(aliasStart + 1), Children: []SyntaxNode{
		{Type: "identifier", Field: "name", Named: true, Start: uint32(nameStart), End: uint32(nameStart + len("Path"))},
		{Type: "identifier", Field: "alias", Named: true, Start: uint32(aliasStart), End: uint32(aliasStart + 1)},
	}}
	statement := SyntaxNode{Type: "import_from_statement", Named: true, End: uint32(len(source) - 1), Children: []SyntaxNode{
		{Type: "dotted_name", Field: "module_name", Named: true, Start: uint32(moduleStart), End: uint32(moduleStart + len("pathlib"))},
		aliased,
	}}
	tree := SyntaxNode{Type: "module", Named: true, End: uint32(len(source)), Children: []SyntaxNode{statement}}
	got, err := ExtractSyntaxFacts("python", tree, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Imports) != 1 || got.Imports[0].Name != "pathlib.Path" || got.Imports[0].LocalName != "P" {
		t.Fatalf("imports = %#v", got.Imports)
	}
}

func TestExtractSyntaxFactsAssociatesDecoratorWrapper(t *testing.T) {
	t.Parallel()
	source := []byte("@cache\ndef run():\n pass\n")
	tree := SyntaxNode{Type: "module", Named: true, End: uint32(len(source)), Children: []SyntaxNode{
		{Type: "decorated_definition", Named: true, Start: 0, End: uint32(len(source)), Children: []SyntaxNode{
			{Type: "decorator", Named: true, Start: 0, End: 6, Children: []SyntaxNode{{Type: "identifier", Named: true, Start: 1, End: 6}}},
			{Type: "function_definition", Named: true, Start: 7, End: uint32(len(source)), Children: []SyntaxNode{
				{Type: "identifier", Field: "name", Named: true, Start: 11, End: 14},
			}},
		}},
	}}
	got, err := ExtractSyntaxFacts("python", tree, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Decorators) != 1 || got.Decorators[0].Name != "cache" || got.Decorators[0].Scope != "run" {
		t.Fatalf("decorators = %#v", got.Decorators)
	}
}

func TestExtractSyntaxFactsFindsNestedJSONKeys(t *testing.T) {
	source := []byte(`{"hooks":{"Stop":true}}`)
	key := func(text string) SyntaxNode {
		start := strings.Index(string(source), text)
		return SyntaxNode{Type: "string", Field: "key", Named: true, Start: uint32(start), End: uint32(start + len(text))}
	}
	inner := SyntaxNode{Type: "pair", Named: true, Start: 10, End: 21, Children: []SyntaxNode{key(`"Stop"`)}}
	outer := SyntaxNode{Type: "pair", Named: true, Start: 1, End: 22, Children: []SyntaxNode{
		key(`"hooks"`), {Type: "object", Field: "value", Named: true, Start: 9, End: 22, Children: []SyntaxNode{inner}},
	}}
	tree := SyntaxNode{Type: "document", Named: true, End: uint32(len(source)), Children: []SyntaxNode{
		{Type: "object", Named: true, End: uint32(len(source)), Children: []SyntaxNode{outer}},
	}}
	got, err := ExtractSyntaxFacts("json", tree, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Definitions) != 2 || got.Definitions[0].Name != "hooks" || got.Definitions[1].Name != "Stop" || got.Definitions[1].Scope != "" {
		t.Fatalf("JSON definitions=%#v", got.Definitions)
	}
}

func TestExtractSyntaxFactsFindsMarkdownHeadingsOutsideParserCodeNodes(t *testing.T) {
	source := []byte("# Title\n")
	tree := SyntaxNode{Type: "document", Named: true, End: uint32(len(source)), Children: []SyntaxNode{
		{Type: "atx_heading", Named: true, End: uint32(len(source) - 1)},
	}}
	got, err := ExtractSyntaxFacts("markdown", tree, source)
	if err != nil {
		t.Fatal(err)
	}
	if !got.RootModule || len(got.Sections) != 1 || got.Sections[0].Name != "Title" {
		t.Fatalf("Markdown extraction=%#v", got)
	}
}

func TestExtractSyntaxFactsPromotesArrowFunctionVariable(t *testing.T) {
	source := []byte("const run = async () => {};\n")
	declarator := SyntaxNode{Type: "variable_declarator", Named: true, Start: 6, End: 26, Children: []SyntaxNode{
		{Type: "identifier", Field: "name", Named: true, Start: 6, End: 9},
		{Type: "arrow_function", Field: "value", Named: true, Start: 12, End: 26},
	}}
	tree := SyntaxNode{Type: "program", Named: true, End: uint32(len(source)), Children: []SyntaxNode{
		{Type: "lexical_declaration", Named: true, End: 26, Children: []SyntaxNode{declarator}},
	}}
	got, err := ExtractSyntaxFacts("typescript", tree, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Definitions) != 1 || got.Definitions[0].Kind != "function" || got.Definitions[0].Name != "run" {
		t.Fatalf("arrow extraction=%#v", got.Definitions)
	}
}

func TestExtractSyntaxFactsSkipsNestedClassMethodArrows(t *testing.T) {
	// class C { constructor() { const evaluate = () => {}; } }
	source := []byte("class C { constructor() { const evaluate = () => {}; } }\n")
	evalDecl := SyntaxNode{Type: "lexical_declaration", Named: true, Start: 26, End: 51, Children: []SyntaxNode{
		{Type: "variable_declarator", Named: true, Start: 32, End: 50, Children: []SyntaxNode{
			{Type: "identifier", Field: "name", Named: true, Start: 32, End: 40},
			{Type: "arrow_function", Field: "value", Named: true, Start: 43, End: 50},
		}},
	}}
	ctor := SyntaxNode{Type: "method_definition", Named: true, Start: 10, End: 53, Children: []SyntaxNode{
		{Type: "property_identifier", Field: "name", Named: true, Start: 10, End: 21},
		{Type: "statement_block", Field: "body", Named: true, Start: 24, End: 53, Children: []SyntaxNode{evalDecl}},
	}}
	classBody := SyntaxNode{Type: "class_body", Named: true, Start: 8, End: 55, Children: []SyntaxNode{ctor}}
	tree := SyntaxNode{Type: "program", Named: true, End: uint32(len(source)), Children: []SyntaxNode{
		{Type: "class_declaration", Named: true, End: 55, Children: []SyntaxNode{
			{Type: "type_identifier", Field: "name", Named: true, Start: 6, End: 7},
			classBody,
		}},
	}}
	got, err := ExtractSyntaxFacts("tsx", tree, source)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, def := range got.Definitions {
		names[def.Name] = def.Kind
	}
	if names["C"] != "class" || names["constructor"] != "function" {
		t.Fatalf("expected class+constructor, got %#v", got.Definitions)
	}
	if _, ok := names["evaluate"]; ok {
		t.Fatalf("nested class-method arrow must not publish: %#v", got.Definitions)
	}
}

func TestExtractSyntaxFactsSkipsAmbientModuleVariables(t *testing.T) {
	// declare module '*.svg' { const src: string; }
	source := []byte("declare module '*.svg' { const src: string; }\n")
	lexical := SyntaxNode{Type: "lexical_declaration", Named: true, Start: 25, End: 42, Children: []SyntaxNode{
		{Type: "variable_declarator", Named: true, Start: 31, End: 41, Children: []SyntaxNode{
			{Type: "identifier", Field: "name", Named: true, Start: 31, End: 34},
		}},
	}}
	mod := SyntaxNode{Type: "module", Named: true, Start: 8, End: 44, Children: []SyntaxNode{
		{Type: "string", Named: true, Start: 15, End: 21},
		{Type: "statement_block", Named: true, Start: 23, End: 44, Children: []SyntaxNode{lexical}},
	}}
	tree := SyntaxNode{Type: "program", Named: true, End: uint32(len(source)), Children: []SyntaxNode{
		{Type: "ambient_declaration", Named: true, End: 44, Children: []SyntaxNode{mod}},
	}}
	got, err := ExtractSyntaxFacts("typescript", tree, source)
	if err != nil {
		t.Fatal(err)
	}
	for _, def := range got.Definitions {
		if def.Name == "src" {
			t.Fatalf("ambient module variable must not publish: %#v", got.Definitions)
		}
	}
}

func TestExtractSyntaxFactsExportDeclarationEmitsUsages(t *testing.T) {
	// export const patterns = [...logoPaths.map((x) => x)];
	source := []byte("export const patterns = [...logoPaths.map((x) => x)];\n")
	ident := SyntaxNode{Type: "identifier", Field: "object", Named: true, Start: 28, End: 37}
	member := SyntaxNode{Type: "member_expression", Field: "function", Named: true, Start: 28, End: 41, Children: []SyntaxNode{
		ident,
		{Type: "property_identifier", Field: "property", Named: true, Start: 38, End: 41},
	}}
	call := SyntaxNode{Type: "call_expression", Named: true, Start: 28, End: 50, Children: []SyntaxNode{member}}
	spread := SyntaxNode{Type: "spread_element", Named: true, Start: 25, End: 50, Children: []SyntaxNode{call}}
	array := SyntaxNode{Type: "array", Field: "value", Named: true, Start: 24, End: 51, Children: []SyntaxNode{spread}}
	decl := SyntaxNode{Type: "variable_declarator", Named: true, Start: 13, End: 51, Children: []SyntaxNode{
		{Type: "identifier", Field: "name", Named: true, Start: 13, End: 21},
		array,
	}}
	lexical := SyntaxNode{Type: "lexical_declaration", Field: "declaration", Named: true, Start: 7, End: 52, Children: []SyntaxNode{decl}}
	tree := SyntaxNode{Type: "program", Named: true, End: uint32(len(source)), Children: []SyntaxNode{
		{Type: "export_statement", Named: true, End: 52, Children: []SyntaxNode{lexical}},
	}}
	got, err := ExtractSyntaxFacts("typescript", tree, source)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, usage := range got.Usages {
		if usage.Name == "logoPaths" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("export declaration body must emit USAGE for logoPaths: %#v", got.Usages)
	}
}

func TestESImportLocalNamesDoesNotPublishTypeModifier(t *testing.T) {
	got := esImportLocalNames(`import type { Columns, Row as Item } from "./columns";`)
	if strings.Join(got, ",") != "Columns,Item" {
		t.Fatalf("names=%#v", got)
	}
}

func TestCommandLanguageFunctionNames(t *testing.T) {
	for _, test := range []struct {
		nodeType string
		text     string
		want     string
	}{
		{nodeType: "function_definition", text: `fatal() { printf '%s' "$*"; }`, want: "fatal"},
		{nodeType: "function_definition", text: `function need() { command -v "$1"; }`, want: "need"},
		{nodeType: "function_statement", text: `function Write-So($msg) { Write-Host $msg }`, want: "Write-So"},
		{nodeType: "function_statement", text: `filter Select-So { process {} }`, want: "Select-So"},
		{nodeType: "function_definition", text: `def python(): pass`, want: ""},
	} {
		if got := commandLanguageFunctionName(test.nodeType, test.text); got != test.want {
			t.Errorf("commandLanguageFunctionName(%q, %q)=%q, want %q", test.nodeType, test.text, got, test.want)
		}
	}
}

func TestPythonAttributeAssignmentIsNotDefinition(t *testing.T) {
	source := []byte("sys.dont_write_bytecode = True\n")
	tree := SyntaxNode{Type: "module", Named: true, End: uint32(len(source)), Children: []SyntaxNode{
		{Type: "assignment", Named: true, End: uint32(len(source) - 1), Children: []SyntaxNode{
			{Type: "attribute", Field: "left", Named: true, End: 23},
		}},
	}}
	got, err := ExtractSyntaxFacts("python", tree, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Definitions) != 0 {
		t.Fatalf("attribute assignment definitions=%#v", got.Definitions)
	}
}

func TestUsageWalkSeparatesWritesAndSkipsOnlyExactCallee(t *testing.T) {
	source := []byte("state = next\nrender(state)\n")
	tree := SyntaxNode{Type: "program", Named: true, End: uint32(len(source)), Children: []SyntaxNode{
		{Type: "assignment_expression", Named: true, End: 12, Children: []SyntaxNode{
			{Type: "identifier", Field: "left", Named: true, End: 5},
			{Type: "identifier", Field: "right", Named: true, Start: 8, End: 12},
		}},
		{Type: "call_expression", Named: true, Start: 13, End: 26, Children: []SyntaxNode{
			{Type: "identifier", Field: "function", Named: true, Start: 13, End: 19},
			{Type: "identifier", Named: true, Start: 20, End: 25},
		}},
	}}
	got, err := ExtractSyntaxFacts("javascript", tree, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Writes) != 1 || got.Writes[0].Name != "state" {
		t.Fatalf("writes=%#v", got.Writes)
	}
	if len(got.Usages) != 2 || got.Usages[0].Name != "next" || got.Usages[1].Name != "state" {
		t.Fatalf("usages=%#v", got.Usages)
	}
}

func TestUsageWalkKeepsTypeAnnotationsAndDropsParameterBindings(t *testing.T) {
	source := []byte("func Run(cfg Config) {}\n")
	cfgStart := strings.Index(string(source), "cfg")
	typeStart := strings.Index(string(source), "Config")
	tree := SyntaxNode{Type: "source_file", Named: true, End: uint32(len(source)), Children: []SyntaxNode{
		{Type: "function_declaration", Named: true, End: uint32(len(source) - 1), Children: []SyntaxNode{
			{Type: "identifier", Field: "name", Named: true, Start: 5, End: 8},
			{Type: "parameter_list", Field: "parameters", Named: true, Start: 8, End: 19, Children: []SyntaxNode{
				{Type: "parameter_declaration", Named: true, Start: uint32(cfgStart), End: uint32(typeStart + len("Config")), Children: []SyntaxNode{
					{Type: "identifier", Field: "name", Named: true, Start: uint32(cfgStart), End: uint32(cfgStart + 3)},
					{Type: "type_identifier", Field: "type", Named: true, Start: uint32(typeStart), End: uint32(typeStart + len("Config"))},
				}},
			}},
		}},
	}}
	got, err := ExtractSyntaxFacts("go", tree, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Usages) != 1 || got.Usages[0].Name != "Config" {
		t.Fatalf("usages=%#v", got.Usages)
	}
}

func TestUsageWalkSkipsKeywordArgumentLabels(t *testing.T) {
	source := []byte("render(name=state)\n")
	tree := SyntaxNode{Type: "module", Named: true, End: uint32(len(source)), Children: []SyntaxNode{
		{Type: "call", Named: true, End: uint32(len(source) - 1), Children: []SyntaxNode{
			{Type: "identifier", Field: "function", Named: true, End: 6},
			{Type: "argument_list", Named: true, Start: 6, End: uint32(len(source) - 1), Children: []SyntaxNode{
				{Type: "keyword_argument", Named: true, Start: 7, End: 17, Children: []SyntaxNode{
					{Type: "identifier", Field: "name", Named: true, Start: 7, End: 11},
					{Type: "identifier", Field: "value", Named: true, Start: 12, End: 17},
				}},
			}},
		}},
	}}
	got, err := ExtractSyntaxFacts("python", tree, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Usages) != 1 || got.Usages[0].Name != "state" {
		t.Fatalf("usages=%#v", got.Usages)
	}
}

func TestUsageWalkTreatsAugmentedAssignmentAsRead(t *testing.T) {
	source := []byte("state += next\n")
	tree := SyntaxNode{Type: "program", Named: true, End: uint32(len(source)), Children: []SyntaxNode{
		{Type: "augmented_assignment_expression", Named: true, End: uint32(len(source) - 1), Children: []SyntaxNode{
			{Type: "identifier", Field: "left", Named: true, End: 5},
			{Type: "identifier", Field: "right", Named: true, Start: 9, End: 13},
		}},
	}}
	got, err := ExtractSyntaxFacts("javascript", tree, source)
	if err != nil {
		t.Fatal(err)
	}
	// An augmented assignment reads its target, so `state` stays a usage, while
	// the assignment walk still records the write.
	if len(got.Writes) != 1 || got.Writes[0].Name != "state" {
		t.Fatalf("writes=%#v", got.Writes)
	}
	if len(got.Usages) != 2 || got.Usages[0].Name != "state" || got.Usages[1].Name != "next" {
		t.Fatalf("usages=%#v", got.Usages)
	}
}

func TestUsageWalkUsesLanguageSpecificReferenceNodes(t *testing.T) {
	source := []byte("$(watched)\n")
	tree := SyntaxNode{Type: "makefile", Named: true, End: uint32(len(source)), Children: []SyntaxNode{
		{Type: "variable_reference", Named: true, End: uint32(len(source) - 1)},
	}}
	got, err := ExtractSyntaxFacts("makefile", tree, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Usages) != 1 || got.Usages[0].Name != "watched" {
		t.Fatalf("makefile usages=%#v", got.Usages)
	}

	haskell := []byte("watched\n")
	haskellTree := SyntaxNode{Type: "haskell", Named: true, End: uint32(len(haskell)), Children: []SyntaxNode{
		{Type: "variable", Named: true, End: uint32(len(haskell) - 1)},
	}}
	got, err = ExtractSyntaxFacts("haskell", haskellTree, haskell)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Usages) != 1 || got.Usages[0].Name != "watched" {
		t.Fatalf("haskell usages=%#v", got.Usages)
	}
}

func TestUsageWalkSuppressesRustScopedIdentifierChildren(t *testing.T) {
	source := []byte("crate::watched\n")
	tree := SyntaxNode{Type: "source_file", Named: true, End: uint32(len(source)), Children: []SyntaxNode{
		{Type: "scoped_identifier", Named: true, End: uint32(len(source) - 1), Children: []SyntaxNode{
			{Type: "identifier", Named: true, End: 5},
			{Type: "identifier", Named: true, Start: 7, End: 14},
		}},
	}}
	got, err := ExtractSyntaxFacts("rust", tree, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Usages) != 1 || got.Usages[0].Name != "crate::watched" {
		t.Fatalf("rust usages=%#v", got.Usages)
	}
}

func TestIniSectionAndSettingNames(t *testing.T) {
	source := []byte("[supervisord]\nnodaemon=true\n")
	sectionName := SyntaxNode{Type: "section_name", Named: true, Start: 0, End: 13, Children: []SyntaxNode{
		{Type: "text", Named: true, Start: 1, End: 12},
	}}
	section := SyntaxNode{Type: "section", Named: true, End: uint32(len(source)), Children: []SyntaxNode{
		sectionName,
		{Type: "setting", Named: true, Start: 14, End: 27, Children: []SyntaxNode{
			{Type: "setting_name", Named: true, Start: 14, End: 22},
		}},
	}}
	tree := SyntaxNode{Type: "document", Named: true, End: uint32(len(source)), Children: []SyntaxNode{section}}
	got, err := ExtractSyntaxFacts("ini", tree, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Definitions) != 2 {
		t.Fatalf("definitions=%#v", got.Definitions)
	}
	if got.Definitions[0].Kind != "class" || got.Definitions[0].Name != "supervisord" || got.Definitions[0].Scope != "" {
		t.Fatalf("section=%#v", got.Definitions[0])
	}
	if got.Definitions[1].Kind != "variable" || got.Definitions[1].Name != "nodaemon" || got.Definitions[1].Scope != "" {
		t.Fatalf("setting=%#v", got.Definitions[1])
	}
}

func TestSyntaxNodeLabelMapsEnumDeclaration(t *testing.T) {
	if got := syntaxNodeLabel("typescript", SyntaxFact{Kind: "class", NodeType: "enum_declaration", Name: "ROUTES"}); got != "Enum" {
		t.Fatalf("label=%q", got)
	}
}

func TestSyntaxNodeLabelObjectLiteralMethodIsFunction(t *testing.T) {
	if got := syntaxNodeLabel("typescript", SyntaxFact{Kind: "function", NodeType: "method_definition", Scope: ""}); got != "Function" {
		t.Fatalf("object method label=%q", got)
	}
	if got := syntaxNodeLabel("typescript", SyntaxFact{Kind: "function", NodeType: "method_definition", Scope: "Widget"}); got != "Method" {
		t.Fatalf("class method label=%q", got)
	}
}

func TestGitBranchSlugCollapsesPathSeparators(t *testing.T) {
	if got := gitBranchSlug("feat/openplait-connectors-framework", false); got != "feat-openplait-connectors-framework" {
		t.Fatalf("slug=%q", got)
	}
	if got := gitBranchSlug("main", false); got != "main" {
		t.Fatalf("main slug=%q", got)
	}
}

func TestExtractSyntaxFactsCommonJSRequireAndObjectMethods(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	runtime := testSyntaxGrammarRuntime(t)
	root := t.TempDir()
	files := map[string]string{
		"req.ts": "const metrics = require('../otel/metrics');\nOpenAI = require('openai').default;\n",
		"obj.ts": "export function wrap() {\n  return {\n    [Symbol.asyncIterator]() {},\n    next() {},\n  };\n}\nclass Widget { run() {} }\n",
	}
	var paths []string
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, name)
	}
	repo, err := ParseSyntaxRepository(ctx, runtime, root, "fixture", paths, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	byFile := map[string]SyntaxExtraction{}
	for _, f := range repo.Files {
		byFile[f.File.Path] = f.Extraction
	}
	req := byFile["req.ts"]
	if len(req.Imports) != 2 {
		t.Fatalf("require imports=%#v", req.Imports)
	}
	if req.Imports[0].Name != "../otel/metrics" || req.Imports[0].LocalName != "metrics" {
		t.Fatalf("relative require=%#v", req.Imports[0])
	}
	if req.Imports[1].Name != "openai" || req.Imports[1].LocalName != "openai" {
		t.Fatalf("bare require=%#v", req.Imports[1])
	}
	obj := byFile["obj.ts"]
	defs := map[string]SyntaxFact{}
	for _, d := range obj.Definitions {
		defs[d.Name] = d
	}
	for _, name := range []string{"wrap", "[Symbol.asyncIterator]", "next"} {
		d, ok := defs[name]
		if !ok {
			t.Fatalf("missing %s in %#v", name, obj.Definitions)
		}
		if d.Scope != "" {
			t.Fatalf("%s must be module-flat, got scope=%q", name, d.Scope)
		}
		if syntaxNodeLabel("typescript", d) != "Function" {
			t.Fatalf("%s label=%q", name, syntaxNodeLabel("typescript", d))
		}
	}
	widgetRun, ok := defs["run"]
	if !ok || widgetRun.Scope != "Widget" || syntaxNodeLabel("typescript", widgetRun) != "Method" {
		t.Fatalf("class method run=%#v", widgetRun)
	}
}

func TestYamlJestAndEnumExtraction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	runtime := testSyntaxGrammarRuntime(t)
	root := t.TempDir()
	files := map[string]string{
		"default.yaml": "apiVersion: 1\n\ndatasources:\n  - name: x\n    url: http://prometheus:9090\n",
		"jest.js":      "const path = require('path');\nconst { grafanaESModules, nodeModulesToTransform } = require('./jest/utils');\n",
		"enum.ts":      "export enum ROUTES {\n  Overview = 'overview',\n  Fleet = 'fleet',\n}\n",
	}
	var paths []string
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, name)
	}
	repo, err := ParseSyntaxRepository(ctx, runtime, root, "fixture", paths, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	byFile := map[string]SyntaxExtraction{}
	for _, f := range repo.Files {
		byFile[f.File.Path] = f.Extraction
		t.Logf("FILE %s lang=%s", f.File.Path, f.File.Language)
		for _, d := range f.Extraction.Definitions {
			t.Logf("  def kind=%s name=%q scope=%q depth=%d type=%s", d.Kind, d.Name, d.Scope, d.VariableDepth, d.NodeType)
		}
		for _, r := range f.Extraction.Routes {
			t.Logf("  route kind=%s name=%q key=%q", r.Kind, r.Name, r.FirstStringArg)
		}
	}
	jest := byFile["jest.js"]
	names := map[string]bool{}
	for _, d := range jest.Definitions {
		names[d.Name] = true
	}
	if names["path"] || names["require"] || !names["grafanaESModules"] || !names["nodeModulesToTransform"] {
		t.Fatalf("jest defs=%v", names)
	}
	enumDefs := byFile["enum.ts"].Definitions
	if len(enumDefs) < 3 {
		t.Fatalf("enum defs=%#v", enumDefs)
	}
	yaml := byFile["default.yaml"]
	if yaml.Definitions[0].Kind != "variable" {
		t.Fatalf("yaml should stay yaml variables, got %#v lang from defs", yaml.Definitions)
	}
	if len(yaml.Routes) == 0 || yaml.Routes[0].Name != "http://prometheus:9090" {
		t.Fatalf("infra routes=%#v", yaml.Routes)
	}
}
