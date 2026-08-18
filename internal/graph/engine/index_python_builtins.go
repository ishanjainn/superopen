package engine

import (
	"sort"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

var pinnedPythonBuiltins = []struct {
	label, name, qn string
}{
	{"Class", "dict", "builtins.dict"},
	{"Class", "int", "builtins.int"},
	{"Class", "list", "builtins.list"},
	{"Class", "range", "builtins.range"},
	{"Class", "str", "builtins.str"},
	{"Function", "len", "builtins.len"},
	{"Function", "print", "builtins.print"},
	{"Method", "get", "builtins.dict.get"},
	{"Method", "append", "builtins.list.append"},
	{"Method", "pop", "builtins.list.pop"},
	{"Method", "lower", "builtins.str.lower"},
	{"Method", "upper", "builtins.str.upper"},
}

// seedPythonBuiltinNodes adds the pinned builtin symbol nodes, skipping any
// that a previous pass already published.
func seedPythonBuiltinNodes(project string, files []ParsedSyntaxFile, graph *goGraph) {
	hasPython := false
	for _, parsed := range files {
		hasPython = hasPython || parsed.File.Language == "python"
	}
	if !hasPython {
		return
	}
	existing := make(map[string]bool, len(graph.nodes))
	for _, node := range graph.nodes {
		existing[node.QualifiedName] = true
	}
	for _, builtin := range pinnedPythonBuiltins {
		if existing[builtin.qn] {
			continue
		}
		graph.nodes = append(graph.nodes, api.Node{Project: project, Label: builtin.label, Name: builtin.name,
			QualifiedName: builtin.qn, Location: api.Location{File: "<python-builtins>", StartLine: 1, EndLine: 1},
			Properties: pinnedBuiltinProperties(builtin.label)})
	}
}

// pinnedBuiltinProperties reproduces the zeroed metric blob Superopen writes for
// synthesized builtin symbols; callables carry the full function metric set.
func pinnedBuiltinProperties(label string) api.Properties {
	properties := api.Properties{"complexity": 0, "lines": 0,
		"is_exported": false, "is_test": false, "is_entry_point": false}
	if label != "Function" && label != "Method" {
		return properties
	}
	for key, value := range map[string]any{
		"cognitive": 0, "loop_count": 0, "loop_depth": 0, "self_recursive": false,
		"param_count": 0, "max_access_depth": 0, "linear_scan_in_loop": 0,
		"alloc_in_loop": 0, "recursion_in_loop": false, "unguarded_recursion": false,
	} {
		properties[key] = value
	}
	return properties
}

func indexPythonBuiltins(project string, files []ParsedSyntaxFile, graph *goGraph) {
	hasPython := false
	for _, parsed := range files {
		hasPython = hasPython || parsed.File.Language == "python"
	}
	if !hasPython {
		return
	}
	seedPythonBuiltinNodes(project, files, graph)
	targets := map[string]string{}
	for _, builtin := range pinnedPythonBuiltins {
		targets[builtin.name] = builtin.qn
	}
	byFileName := map[string][]api.Node{}
	for _, node := range graph.nodes {
		if node.Label == "Function" || node.Label == "Method" {
			key := node.Location.File + "\x00" + node.Name
			byFileName[key] = append(byFileName[key], node)
		}
	}
	for key := range byFileName {
		sort.Slice(byFileName[key], func(i, j int) bool { return byFileName[key][i].QualifiedName < byFileName[key][j].QualifiedName })
	}
	for _, parsed := range files {
		if parsed.File.Language == "python" {
			for _, builtin := range pinnedPythonBuiltins {
				graph.edges = append(graph.edges, pendingEdge{source: fileQualifiedName(parsed.File.Path), target: builtin.qn,
					kind: "DEFINES", evidence: &api.Evidence{Strategy: "python_builtins", Confidence: 1}})
			}
		}
		for _, call := range parsed.Extraction.Calls {
			target := targets[syntaxCallBase(call.Name)]
			if target == "" {
				continue
			}
			source := fileQualifiedName(parsed.File.Path)
			owner := ""
			if call.Scope != "" {
				parts := splitSyntaxScope(call.Scope)
				owner = parts[len(parts)-1]
			}
			if candidates := byFileName[parsed.File.Path+"\x00"+owner]; len(candidates) > 0 {
				source = candidates[0].QualifiedName
			}
			graph.edges = append(graph.edges, pendingEdge{source: source, target: target, kind: "CALLS",
				evidence: syntaxEvidence(parsed.File.Path, call, "python_builtin")})
		}
	}
	sortGraph(graph)
}

func splitSyntaxScope(scope string) []string {
	parts := []string{}
	start := 0
	for index := 0; index <= len(scope); index++ {
		if index == len(scope) || scope[index] == '.' {
			parts = append(parts, scope[start:index])
			start = index + 1
		}
	}
	return parts
}
