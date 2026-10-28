package engine

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// Docker's last stage supplies its base image and executable. ENTRYPOINT takes
// precedence over CMD, as in the container runtime pass.
func indexDockerRuntime(files []ParsedSyntaxFile, graph *goGraph) {
	for _, parsed := range files {
		if parsed.File.Language != "dockerfile" || len(parsed.Body) == 0 {
			continue
		}
		module := syntaxModuleQN(parsed.File.Path)
		stage := -1
		image := ""
		imageLine := 0
		entry, command := dockerInstruction{}, dockerInstruction{}
		for index, line := range strings.Split(string(parsed.Body), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			switch strings.ToUpper(fields[0]) {
			case "FROM":
				stage++
				image, imageLine = fields[1], index+1
				entry, command = dockerInstruction{}, dockerInstruction{}
			case "ENTRYPOINT":
				entry = dockerInstruction{value: strings.TrimSpace(line[strings.Index(strings.ToUpper(line), "ENTRYPOINT")+len("ENTRYPOINT"):]), line: index + 1, name: "ENTRYPOINT"}
			case "CMD":
				command = dockerInstruction{value: strings.TrimSpace(line[strings.Index(strings.ToUpper(line), "CMD")+len("CMD"):]), line: index + 1, name: "CMD"}
			}
		}
		if stage < 0 {
			continue
		}
		if repository := dockerImageRepository(image); repository != "" {
			qn := "__image__" + repository
			graph.nodes = append(graph.nodes, api.Node{Project: parsed.File.Project, Label: "Image", Name: repository,
				QualifiedName: qn, Properties: api.Properties{"image": image}})
			graph.addEdge(pendingEdge{source: module, target: qn, kind: "DEPENDS_ON",
				properties: api.Properties{"via": "from", "image": image, "stage": stage, "line": imageLine}})
		}
		selected := entry
		if selected.name == "" {
			selected = command
		}
		argv := dockerArgv(selected.value)
		if len(argv) == 0 || strings.Contains(argv[0], "$") || strings.HasPrefix(argv[0], "-") {
			continue
		}
		program := filepath.Base(argv[0])
		if (program == "sh" || program == "bash") && len(argv) > 1 && argv[1] == "-c" {
			continue
		}
		qn := "__process__" + program
		graph.nodes = append(graph.nodes, api.Node{Project: parsed.File.Project, Label: "Process", Name: program,
			QualifiedName: qn, Properties: api.Properties{"program": program}})
		graph.addEdge(pendingEdge{source: module, target: qn, kind: "SPAWNS",
			properties: api.Properties{"api": "dockerfile", "instruction": selected.name, "program": argv[0], "line": selected.line, "confidence": 1.0}})
	}
}

type dockerInstruction struct {
	value string
	line  int
	name  string
}

func dockerImageRepository(image string) string {
	if image == "" || strings.Contains(image, "$") {
		return ""
	}
	if i := strings.IndexByte(image, '@'); i >= 0 {
		image = image[:i]
	}
	if i := strings.LastIndexByte(image, ':'); i >= 0 && i > strings.LastIndexByte(image, '/') {
		image = image[:i]
	}
	return image
}

func dockerArgv(value string) []string {
	if strings.HasPrefix(value, "[") {
		var argv []string
		if json.Unmarshal([]byte(value), &argv) == nil {
			return argv
		}
		return nil
	}
	return strings.Fields(value)
}
