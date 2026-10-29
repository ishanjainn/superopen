package engine

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

var rstGraphvizDirective = regexp.MustCompile(`^\.\.\s+(?:\|[^|]+\|\s+)?(?:di)?graph::`)
var rstCaptionOption = regexp.MustCompile(`^\s+:caption:\s*(.+)$`)
var rstDotArrow = regexp.MustCompile(`([A-Za-z_][\w]*)\s*->\s*([A-Za-z_][\w]*)`)
var rstDotIdentifier = regexp.MustCompile(`^[A-Za-z_][\w]*$`)

func indexRSTDiagrams(files []ParsedSyntaxFile, graph *goGraph) {
	for _, parsed := range files {
		if parsed.File.Language != "rst" {
			continue
		}
		file := filepath.ToSlash(parsed.File.Path)
		lines := strings.Split(string(parsed.Body), "\n")
		ordinal := 0
		for i := 0; i < len(lines); i++ {
			if !rstGraphvizDirective.MatchString(lines[i]) {
				continue
			}
			ordinal++
			name := "dot digraph"
			end := i + 1
			for j := i + 1; j < len(lines); j++ {
				line := lines[j]
				if line != "" && line[0] != ' ' && line[0] != '\t' {
					break
				}
				if caption := rstCaptionOption.FindStringSubmatch(line); caption != nil {
					name = strings.TrimSpace(caption[1])
				}
				if strings.TrimSpace(line) != "" {
					end = j + 1
				}
			}
			qn := syntaxModuleQN(file) + "#diagram-" + strconv.Itoa(ordinal)
			graph.nodes = append(graph.nodes, api.Node{Project: parsed.File.Project, Label: "Diagram", Name: name, QualifiedName: qn,
				Location: api.Location{File: file, StartLine: i + 1, EndLine: end}, Properties: api.Properties{"format": "dot", "diagram_kind": "digraph"}})
			graph.addEdge(pendingEdge{source: fileQualifiedName(file), target: qn, kind: "DEFINES"})
			ids := map[string]string{}
			for j := i + 1; j < end; j++ {
				standalone := strings.TrimSpace(lines[j])
				if rstDotIdentifier.MatchString(standalone) && ids[standalone] == "" {
					item := qn + "." + standalone
					ids[standalone] = item
					graph.nodes = append(graph.nodes, api.Node{Project: parsed.File.Project, Label: "DiagramElement", Name: standalone, QualifiedName: item,
						Location: api.Location{File: file, StartLine: j + 1, EndLine: j + 1}, Properties: api.Properties{"parent_class": qn, "element_id": standalone, "element_kind": "node"}})
					graph.addEdge(pendingEdge{source: fileQualifiedName(file), target: item, kind: "DEFINES"})
					graph.addEdge(pendingEdge{source: qn, target: item, kind: "DEFINES"})
				}
				for _, arrow := range rstDotArrow.FindAllStringSubmatch(lines[j], -1) {
					for _, id := range arrow[1:3] {
						if ids[id] != "" {
							continue
						}
						item := qn + "." + id
						ids[id] = item
						graph.nodes = append(graph.nodes, api.Node{Project: parsed.File.Project, Label: "DiagramElement", Name: id, QualifiedName: item,
							Location: api.Location{File: file, StartLine: j + 1, EndLine: j + 1}, Properties: api.Properties{"parent_class": qn, "element_id": id, "element_kind": "node"}})
						graph.addEdge(pendingEdge{source: fileQualifiedName(file), target: item, kind: "DEFINES"})
						graph.addEdge(pendingEdge{source: qn, target: item, kind: "DEFINES"})
					}
					graph.addEdge(pendingEdge{source: ids[arrow[1]], target: ids[arrow[2]], kind: "DIAGRAM_ARROW",
						properties: api.Properties{"style": "arrow", "line": j + 1}})
				}
			}
		}
	}
}
