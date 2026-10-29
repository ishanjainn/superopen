package engine

import (
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func indexDependencyManifests(files []ParsedSyntaxFile, project string, graph *goGraph) {
	seen := map[string]bool{}
	for _, parsed := range files {
		base := filepath.Base(parsed.File.Path)
		if base != "requirements.txt" && base != "go.mod" {
			continue
		}
		ecosystem := "pypi"
		if base == "go.mod" {
			ecosystem = "gomod"
		}
		inRequire := false
		for _, raw := range strings.Split(string(parsed.Body), "\n") {
			line := strings.TrimSpace(raw)
			name := ""
			if base == "requirements.txt" {
				if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") || strings.Contains(line, "://") {
					continue
				}
				if cut := strings.IndexAny(line, "=<>!~[; \t\r"); cut >= 0 {
					name = line[:cut]
				} else {
					name = line
				}
			} else if inRequire {
				if strings.HasPrefix(line, ")") {
					inRequire = false
					continue
				}
				if line != "" && !strings.HasPrefix(line, "//") {
					name = strings.Fields(line)[0]
				}
			} else if strings.HasPrefix(line, "require ") {
				rest := strings.TrimSpace(strings.TrimPrefix(line, "require "))
				if strings.HasPrefix(rest, "(") {
					inRequire = true
				} else if rest != "" {
					name = strings.Fields(rest)[0]
				}
			}
			if name == "" || len(name) >= 256 {
				continue
			}
			qn := project + ".__" + ecosystem + "_dep__." + name
			if !seen[qn] {
				graph.nodes = append(graph.nodes, api.Node{Project: project, Label: "Package", Name: name,
					QualifiedName: qn, Location: api.Location{File: filepath.ToSlash(parsed.File.Path), StartLine: 1},
					Properties: api.Properties{"source": ecosystem, "external": true}})
				seen[qn] = true
			}
			graph.addEdge(pendingEdge{source: fileQualifiedName(parsed.File.Path), target: qn, kind: "DEPENDS_ON"})
		}
	}
}
