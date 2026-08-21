package engine

import (
	"path/filepath"
	"strings"
)

func appendTestRelationships(graph *goGraph) {
	nodes := make(map[string]struct {
		name, file string
		test       bool
	}, len(graph.nodes))
	files := make(map[string]string)
	for _, node := range graph.nodes {
		isTest, _ := node.Properties["is_test"].(bool)
		nodes[node.QualifiedName] = struct {
			name, file string
			test       bool
		}{node.Name, filepath.ToSlash(node.Location.File), isTest || isTestPath(node.Location.File)}
		if node.Label == "File" {
			files[filepath.ToSlash(node.Location.File)] = node.QualifiedName
		}
	}
	for _, edge := range append([]pendingEdge(nil), graph.edges...) {
		if edge.kind != "CALLS" {
			continue
		}
		source, sourceOK := nodes[edge.source]
		target, targetOK := nodes[edge.target]
		if !sourceOK || !targetOK || !source.test || target.test || !isTestFunctionName(source.name) {
			continue
		}
		graph.edges = append(graph.edges, pendingEdge{source: edge.source, target: edge.target, kind: "TESTS",
			properties: edge.properties, evidence: edge.evidence})
	}
	for path, source := range files {
		if production := productionTestPath(path); production != "" {
			if target := files[production]; target != "" && target != source {
				graph.edges = append(graph.edges, pendingEdge{source: source, target: target, kind: "TESTS_FILE",
					evidence: layoutEvidence(path)})
			}
		}
	}
}

func isTestPath(path string) bool {
	path = filepath.ToSlash(path)
	base := filepath.Base(path)
	for _, suffix := range []string{"_test.go", "_test.py", "_test.rs", "_test.cpp", "_test.lua", "_spec.rb",
		"Test.java", "Test.kt", "Test.cs", "Test.php", "Spec.scala"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	if strings.HasPrefix(base, "test_") || strings.Contains(path, ".test.ts") || strings.Contains(path, ".spec.ts") ||
		strings.Contains(path, ".test.js") || strings.Contains(path, ".spec.js") ||
		strings.Contains(path, "/__tests__/") || strings.Contains(path, "/tests/") || strings.Contains(path, "/test/") || strings.Contains(path, "/spec/") {
		return true
	}
	return strings.HasPrefix(path, "tests/") || strings.HasPrefix(path, "test/") || strings.HasPrefix(path, "spec/") || strings.HasPrefix(path, "__tests__/")
}

func isTestFunctionName(name string) bool {
	for _, prefix := range []string{"Test", "Benchmark", "Example"} {
		if strings.HasPrefix(name, prefix) && (len(name) == len(prefix) || name[len(prefix)] >= 'A' && name[len(prefix)] <= 'Z') {
			return true
		}
	}
	if strings.HasPrefix(name, "test_") || strings.HasPrefix(name, "test") && len(name) > 4 && name[4] >= 'A' && name[4] <= 'Z' {
		return true
	}
	switch name {
	case "test", "it", "describe", "beforeAll", "afterAll", "beforeEach", "afterEach", "@testset", "@test":
		return true
	default:
		return false
	}
}

func productionTestPath(path string) string {
	dir, base := filepath.ToSlash(filepath.Dir(path)), filepath.Base(path)
	join := func(name string) string {
		if dir == "." || dir == "" {
			return name
		}
		return dir + "/" + name
	}
	if strings.HasSuffix(base, "_test.go") {
		return join(strings.TrimSuffix(base, "_test.go") + ".go")
	}
	if strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py") {
		return join(strings.TrimPrefix(base, "test_"))
	}
	for _, marker := range []string{".test.", ".spec."} {
		if index := strings.Index(base, marker); index >= 0 {
			return join(base[:index] + "." + base[index+len(marker):])
		}
	}
	return ""
}
