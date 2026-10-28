package engine

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

var dotNodeLine = regexp.MustCompile(`^\s*"([^"]+)"\s*\[.*\blabel="([^"]*)"`)
var dotArrowLine = regexp.MustCompile(`^\s*(?:"([^"]+)"|([A-Za-z_][\w.]*))\s*(?:->|--)\s*(?:"([^"]+)"|([A-Za-z_][\w.]*))`)
var dotArrowLabel = regexp.MustCompile(`\blabel="([^"]*)"`)
var dotDiamondHead = regexp.MustCompile(`\barrowhead="diamond"`)
var pumlDeclaration = regexp.MustCompile(`^\s*(file|database|actor|participant|class|interface|entity|component|node|rectangle)\s+(?:"([^"]+)"|([^\s{]+))(?:\s+as\s+([^\s{]+))?`)
var pumlArrowLine = regexp.MustCompile(`^\s*(\S+)\s+[-.<|o*]+(?:[a-z]+)?[-.>|o*]+\s+(\S+)(?:\s*:\s*(.*))?`)
var mermaidClassLine = regexp.MustCompile(`^\s*class\s+([A-Za-z_][\w.]*)`)
var mermaidArrowLine = regexp.MustCompile(`^\s*([A-Za-z_][\w.]*)\s+(<\|--\|>|<\|--|--\|>|<\|\.\.|\.\.\|>|\*--|--\*|o--|--o|<--|-->|<\.\.|\.\.>|--|\.\.)\s+([A-Za-z_][\w.]*)(?:\s*:\s*(.*))?`)

// Diagram-file routing and identities follow the document diagram extractor.
// The line parsers cover explicit DOT nodes and PlantUML declarations/arrows.
func indexDiagramFiles(files []ParsedSyntaxFile, graph *goGraph) {
	for _, parsed := range files {
		if parsed.File.Language != "diagram" && parsed.File.Language != "mermaid" {
			continue
		}
		file := filepath.ToSlash(parsed.File.Path)
		ext := strings.ToLower(filepath.Ext(file))
		format, kind := "dot", "digraph"
		if parsed.File.Language == "mermaid" {
			format, kind = "mermaid", "class"
		} else if ext != ".dot" && ext != ".gv" {
			format, kind = "plantuml", "sequence"
		}
		lines := strings.Split(string(parsed.Body), "\n")
		if format == "dot" && !strings.Contains(string(parsed.Body), "digraph") {
			kind = "graph"
		}
		if format == "plantuml" && strings.Contains(string(parsed.Body), "class ") {
			kind = "class"
		}
		name := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "@startuml ") {
				name = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "@startuml"))
				break
			}
		}
		qn := syntaxModuleQN(file) + "#diagram-1"
		first, last := 1, len(lines)
		if format == "plantuml" {
			for i, line := range lines {
				if strings.HasPrefix(strings.TrimSpace(line), "@startuml") {
					first = i + 1
				}
				if strings.HasPrefix(strings.TrimSpace(line), "@enduml") {
					last = i + 1
					break
				}
			}
		}
		diagram := api.Node{Project: parsed.File.Project, Label: "Diagram", Name: name,
			QualifiedName: qn, Location: api.Location{File: file, StartLine: first, EndLine: last},
			Properties: api.Properties{"format": format, "diagram_kind": kind}}
		ids := map[string]string{}
		var labels []string
		add := func(id, label, elementKind string, line int) {
			if id == "" {
				return
			}
			if _, exists := ids[id]; exists {
				return
			}
			segment := strings.NewReplacer(".", "_", " ", "_", "#", "_", "\\", "_").Replace(id)
			segment = strings.Trim(segment, `"`)
			elementQN := qn + "." + segment
			ids[id] = elementQN
			if label == "" {
				label = id
			}
			labels = append(labels, label)
			graph.nodes = append(graph.nodes, api.Node{Project: parsed.File.Project, Label: "DiagramElement", Name: label,
				QualifiedName: elementQN, Location: api.Location{File: file, StartLine: line, EndLine: line},
				Properties: api.Properties{"parent_class": qn, "element_id": id, "element_kind": elementKind}})
			graph.addEdge(pendingEdge{source: fileQualifiedName(file), target: elementQN, kind: "DEFINES"})
			graph.addEdge(pendingEdge{source: qn, target: elementQN, kind: "DEFINES"})
		}
		for i, line := range lines {
			if format == "dot" {
				if match := dotNodeLine.FindStringSubmatch(line); match != nil {
					label := strings.NewReplacer(`\l`, " ", `\n`, " ", `\r`, " ").Replace(match[2])
					add(match[1], strings.Join(strings.Fields(label), " "), "node", i+1)
				}
			} else if format == "mermaid" {
				if match := mermaidClassLine.FindStringSubmatch(line); match != nil {
					add(match[1], match[1], "class", i+1)
				}
			} else if match := pumlDeclaration.FindStringSubmatch(line); match != nil {
				id, label := match[2], match[2]
				if id == "" {
					id, label = match[3], match[3]
				}
				if match[4] != "" {
					id = match[4]
				}
				add(id, label, match[1], i+1)
			}
		}
		insideClass := false
		for i, line := range lines {
			var from, to, style, label string
			if format == "dot" {
				if match := dotArrowLine.FindStringSubmatch(line); match != nil {
					from, to, style = match[1]+match[2], match[3]+match[4], "arrow"
					add(from, "", "node", i+1)
					add(to, "", "node", i+1)
					if dotDiamondHead.MatchString(line) {
						from, to, style = to, from, "composition"
					}
					if labelMatch := dotArrowLabel.FindStringSubmatch(line); labelMatch != nil {
						label = labelMatch[1]
					}
				}
			} else if format == "mermaid" {
				if match := mermaidArrowLine.FindStringSubmatch(line); match != nil {
					from, to, label = match[1], match[3], strings.TrimSpace(match[4])
					var reverse bool
					style, reverse = mermaidRelation(match[2])
					if reverse {
						from, to = to, from
					}
				}
			} else if strings.Contains(line, "{") && strings.HasPrefix(strings.TrimSpace(line), "class ") {
				insideClass = true
			} else if insideClass && strings.TrimSpace(line) == "}" {
				insideClass = false
			} else if !insideClass {
				if match := pumlArrowLine.FindStringSubmatch(line); match != nil {
					from, to, style, label = match[1], match[2], "message", strings.TrimSpace(match[3])
					add(from, "", "participant", i+1)
					add(to, "", "participant", i+1)
				}
			}
			if ids[from] != "" && ids[to] != "" {
				if label != "" {
					labels = append(labels, label)
				}
				graph.addEdge(pendingEdge{source: ids[from], target: ids[to], kind: "DIAGRAM_ARROW",
					properties: diagramArrowProperties(style, label, i+1)})
			}
		}
		if len(labels) > 0 {
			text := strings.Join(labels, "; ")
			if len(text) > 2048 {
				text = text[:2048]
			}
			diagram.Properties["docstring"] = text
		}
		graph.nodes = append(graph.nodes, diagram)
		graph.addEdge(pendingEdge{source: fileQualifiedName(file), target: qn, kind: "DEFINES"})
	}
}

func mermaidRelation(operator string) (string, bool) {
	switch operator {
	case "<|--":
		return "inheritance", true
	case "<|--|>", "--|>":
		return "inheritance", false
	case "<|..":
		return "realization", true
	case "..|>":
		return "realization", false
	case "*--":
		return "composition", false
	case "--*":
		return "composition", true
	case "o--":
		return "aggregation", false
	case "--o":
		return "aggregation", true
	case "<--":
		return "association", true
	case "-->":
		return "association", false
	case "<..":
		return "dependency", true
	case "..>":
		return "dependency", false
	default:
		return "link", false
	}
}

func diagramArrowProperties(style, label string, line int) api.Properties {
	properties := api.Properties{"style": style, "line": line}
	if label != "" {
		properties["label"] = label
	}
	return properties
}
