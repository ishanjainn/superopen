package engine

import (
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// Shell steps in repository workflows can launch Makefile targets. A target
// edge points to the actual rule rather than to a generic process name.
func indexWorkflowMakeSpawns(files []ParsedSyntaxFile, graph *goGraph) {
	targets := map[string]string{}
	for _, node := range graph.nodes {
		if node.Label == "Function" && filepath.Base(node.Location.File) == "Makefile" {
			targets[node.Location.File+"\x00"+node.Name] = node.QualifiedName
		}
	}
	for _, parsed := range files {
		file := filepath.ToSlash(parsed.File.Path)
		if !strings.HasPrefix(file, ".github/workflows/") || parsed.File.Language != "yaml" {
			continue
		}
		job, cwd, runIndent := "", "", -1
		inJobs := false
		for index, line := range strings.Split(string(parsed.Body), "\n") {
			trimmed := strings.TrimSpace(line)
			indent := len(line) - len(strings.TrimLeft(line, " \t"))
			if trimmed == "jobs:" && indent == 0 {
				inJobs = true
				continue
			}
			if !inJobs || trimmed == "" {
				continue
			}
			if indent == 0 {
				inJobs = false
				continue
			}
			if indent == 2 && strings.HasSuffix(trimmed, ":") {
				job = strings.TrimSuffix(trimmed, ":")
				cwd, runIndent = "", -1
				continue
			}
			if strings.HasPrefix(trimmed, "working-directory:") {
				cwd = strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "working-directory:")), `"'`)
				continue
			}
			if strings.HasPrefix(trimmed, "run:") {
				runIndent = indent
				continue
			}
			if runIndent < 0 || indent <= runIndent {
				runIndent = -1
				continue
			}
			if strings.HasPrefix(trimmed, "cd ") {
				cwd = strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "cd ")), `"'`)
				continue
			}
			words := strings.Fields(trimmed)
			if len(words) < 2 || words[0] != "make" {
				continue
			}
			target := words[1]
			if strings.HasPrefix(target, "-") || strings.ContainsAny(target, "$=:;") {
				continue
			}
			makefile := filepath.ToSlash(filepath.Clean(filepath.Join(cwd, "Makefile")))
			qn := targets[makefile+"\x00"+target]
			if qn == "" {
				continue
			}
			graph.addEdge(pendingEdge{source: syntaxModuleQN(file), target: qn, kind: "SPAWNS",
				properties: api.Properties{"api": "shell", "program": "make", "path": makefile, "target": target,
					"ci": "github-actions", "job": job, "line": index + 1, "confidence": 1.0}})
		}
	}
}
