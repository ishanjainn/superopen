package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"sort"
	"strconv"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

var jsEnvironmentAccessPatterns = []*regexp.Regexp{
	regexp.MustCompile(`process\.env\.([A-Za-z_][A-Za-z0-9_]*)`),
	regexp.MustCompile(`process\.env\s*\[\s*["']([A-Za-z_][A-Za-z0-9_]*)["']\s*\]`),
}

var pythonEnvironmentAccessPatterns = []*regexp.Regexp{
	regexp.MustCompile(`os\.(?:getenv|environ\.get)\s*\(\s*["']([A-Za-z_][A-Za-z0-9_]*)["']`),
	regexp.MustCompile(`os\.environ\s*\[\s*["']([A-Za-z_][A-Za-z0-9_]*)["']\s*\]`),
}

type environmentAccess struct {
	name   string
	offset uint32
	owner  string
}

func indexEnvironmentAccesses(project string, files []ParsedSyntaxFile, graph *goGraph) {
	byFileName := map[string][]api.Node{}
	modules := map[string]string{}
	for _, node := range graph.nodes {
		if node.Location.File == "" {
			continue
		}
		switch node.Label {
		case "Function", "Method":
			key := node.Location.File + "\x00" + node.Name
			byFileName[key] = append(byFileName[key], node)
		case "Module":
			modules[node.Location.File] = node.QualifiedName
		}
	}
	for key := range byFileName {
		sort.Slice(byFileName[key], func(i, j int) bool { return byFileName[key][i].QualifiedName < byFileName[key][j].QualifiedName })
	}
	for _, parsed := range files {
		if len(parsed.Body) == 0 {
			continue
		}
		for _, access := range findEnvironmentAccesses(parsed.File.Language, parsed.Body) {
			target := "__env__" + access.name
			graph.nodes = append(graph.nodes, api.Node{Project: project, Label: "EnvVar", Name: access.name,
				QualifiedName: target, Properties: api.Properties{"env_key": access.name}})
			source := modules[parsed.File.Path]
			if source == "" {
				source = fileQualifiedName(parsed.File.Path)
			}
			ownerName, ownerSpan := access.owner, ^uint32(0)
			for _, fact := range parsed.Extraction.Definitions {
				if ownerName != "" {
					break
				}
				if fact.Kind != "function" || fact.StartByte > access.offset || access.offset >= fact.EndByte {
					continue
				}
				if span := fact.EndByte - fact.StartByte; span < ownerSpan {
					ownerName, ownerSpan = fact.Name, span
				}
			}
			if candidates := byFileName[parsed.File.Path+"\x00"+ownerName]; ownerName != "" && len(candidates) > 0 {
				source = candidates[0].QualifiedName
			}
			graph.edges = append(graph.edges, pendingEdge{source: source, target: target, kind: "CONFIGURES",
				properties: api.Properties{"strategy": "env_access"},
				evidence:   &api.Evidence{Strategy: "env_access", Confidence: 1}})
		}
	}
	sortGraph(graph)
}

func findEnvironmentAccesses(language string, body []byte) []environmentAccess {
	if language == "go" {
		return findGoEnvironmentAccesses(body)
	}
	result := []environmentAccess{}
	var patterns []*regexp.Regexp
	switch language {
	case "javascript", "typescript", "tsx":
		patterns = jsEnvironmentAccessPatterns
	case "python":
		patterns = pythonEnvironmentAccessPatterns
	}
	for _, pattern := range patterns {
		for _, match := range pattern.FindAllSubmatchIndex(body, -1) {
			if len(match) < 4 {
				continue
			}
			name := string(body[match[2]:match[3]])
			result = append(result, environmentAccess{name: name, offset: uint32(match[0])})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].offset != result[j].offset {
			return result[i].offset < result[j].offset
		}
		return result[i].name < result[j].name
	})
	return result
}

func findGoEnvironmentAccesses(body []byte) []environmentAccess {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "source.go", body, 0)
	if err != nil && file == nil {
		return nil
	}
	var result []environmentAccess
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (selector.Sel.Name != "Getenv" && selector.Sel.Name != "LookupEnv") {
				return true
			}
			pkg, ok := selector.X.(*ast.Ident)
			literal, literalOK := call.Args[0].(*ast.BasicLit)
			if !ok || pkg.Name != "os" || !literalOK || literal.Kind != token.STRING {
				return true
			}
			name, unquoteErr := strconv.Unquote(literal.Value)
			if unquoteErr != nil || name == "" {
				return true
			}
			result = append(result, environmentAccess{name: name, offset: uint32(fset.Position(call.Pos()).Offset), owner: function.Name.Name})
			return true
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].offset != result[j].offset {
			return result[i].offset < result[j].offset
		}
		return result[i].name < result[j].name
	})
	return result
}
