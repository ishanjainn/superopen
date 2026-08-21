package engine

import (
	"bufio"
	"bytes"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// indexGoModDependencies reproduces the pinned engine's observable go.mod
// package surface. Package identity is repository-global while every go.mod
// file retains its own DEPENDS_ON relationship.
func indexGoModDependencies(project, root string, files []ParsedSyntaxFile, graph *goGraph) {
	for _, parsed := range files {
		rel := filepath.ToSlash(parsed.File.Path)
		if filepath.Base(rel) != "go.mod" {
			continue
		}
		body := parsedSource(root, parsed)
		if len(body) == 0 {
			continue
		}
		for _, dependency := range goModRequirements(body) {
			variableQN := syntaxModuleQN(rel) + "." + dependency
			graph.nodes = append(graph.nodes, api.Node{Project: project, Label: "Variable", Name: dependency,
				QualifiedName: variableQN, Location: api.Location{File: rel, StartLine: 1},
				Properties: api.Properties{"complexity": 0, "lines": 0, "is_exported": true,
					"is_test": false, "is_entry_point": false}})
			graph.edges = append(graph.edges, pendingEdge{source: fileQualifiedName(rel), target: variableQN, kind: "DEFINES",
				evidence: layoutEvidence(rel)})
			qn := "__gomod_dep__." + dependency
			graph.nodes = append(graph.nodes, api.Node{
				Project: project, Label: "Package", Name: dependency, QualifiedName: qn,
				Location:   api.Location{File: "go.mod", StartLine: 1},
				Properties: api.Properties{"source": "gomod", "external": true},
			})
			graph.edges = append(graph.edges, pendingEdge{source: fileQualifiedName(rel), target: qn, kind: "DEPENDS_ON",
				properties: api.Properties{"source": "gomod"}, evidence: layoutEvidence(rel)})
		}
	}
}

func goModRequirements(body []byte) []string {
	seen := map[string]bool{}
	result := []string{}
	inBlock := false
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(strings.SplitN(scanner.Text(), "//", 2)[0])
		if line == "require (" {
			inBlock = true
			continue
		}
		if inBlock && line == ")" {
			inBlock = false
			continue
		}
		fields := strings.Fields(line)
		if !inBlock {
			if len(fields) < 3 || fields[0] != "require" {
				continue
			}
			fields = fields[1:]
		}
		if len(fields) < 2 || seen[fields[0]] {
			continue
		}
		seen[fields[0]] = true
		result = append(result, fields[0])
	}
	return result
}
