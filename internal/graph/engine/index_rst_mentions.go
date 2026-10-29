package engine

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// These are the Python roles and directives accepted by the reST document-link
// scanner. Resolution follows the source module, class, then unique-name order.
var rstRolePattern = regexp.MustCompile(":(?:(py|std):)?(func|class|meth|attr|mod|exc|data|obj|const|deco):`([^`]+)`")
var rstSourceRolePattern = regexp.MustCompile(":source:`([^`]+)`")
var rstOpenRolePattern = regexp.MustCompile(":[A-Za-z:]+:`[^`]*$")
var rstDirectivePattern = regexp.MustCompile(`^\s*\.\.\s+(?:(py|std):)?([A-Za-z][\w-]*)::\s*(.*)$`)
var rstPythonName = regexp.MustCompile(`^[A-Za-z_]\w*(?:\.[A-Za-z_]\w*)*$`)
var rstLiteralPattern = regexp.MustCompile("``([^`\\n]+)``")
var rstOptionPattern = regexp.MustCompile(`^\s+:([a-z-]+):\s*(.*)$`)

var rstKinds = map[string][]string{
	"class": {"Class"}, "exc": {"Class"}, "exception": {"Class"},
	"meth": {"Method"}, "method": {"Method"}, "classmethod": {"Method"}, "staticmethod": {"Method"},
	"func": {"Function"}, "function": {"Function"}, "decorator": {"Function"}, "deco": {"Function"},
	"decoratormethod": {"Method"}, "attr": {"Variable", "Method"},
	"attribute": {"Variable", "Method"}, "property": {"Method", "Variable"},
	"data": {"Variable"}, "const": {"Variable"}, "mod": {"Module"}, "module": {"Module"},
}

type rstCodeIndex struct {
	byQN         map[string][]api.Node
	byName       map[string][]api.Node
	byFile       map[string][]api.Node
	aliases      map[string]string
	moduleFile   map[string]string
	moduleSuffix map[string][]api.Node
	normalized   map[string]string
}

func newRSTCodeIndex(nodes []api.Node) rstCodeIndex {
	x := rstCodeIndex{byQN: make(map[string][]api.Node), byName: make(map[string][]api.Node), byFile: make(map[string][]api.Node), aliases: make(map[string]string), moduleFile: make(map[string]string), moduleSuffix: make(map[string][]api.Node), normalized: make(map[string]string)}
	canonical := make(map[string]api.Node)
	for _, n := range nodes {
		if !strings.HasSuffix(n.Location.File, ".py") || n.Label == "File" || n.Label == "Section" {
			continue
		}
		if prior, ok := canonical[n.QualifiedName]; ok {
			if n.Location.File > prior.Location.File || n.Location.File == prior.Location.File && nodeCollisionPriority(n.Label) < nodeCollisionPriority(prior.Label) {
				continue
			}
		}
		canonical[n.QualifiedName] = n
	}
	for _, n := range canonical {
		qn := n.QualifiedName
		if filepath.Base(n.Location.File) == "__init__.py" {
			qn = strings.Replace(qn, ".__init__.", ".", 1)
			if n.Label == "Module" {
				qn = strings.TrimSuffix(qn, ".__init__")
			}
		}
		x.byQN[qn] = append(x.byQN[qn], n)
		x.normalized[n.QualifiedName] = qn
		x.byName[n.Name] = append(x.byName[n.Name], n)
		x.byFile[n.Location.File] = append(x.byFile[n.Location.File], n)
		if n.Label == "Module" {
			x.moduleFile[qn] = n.Location.File
			for suffix := qn; suffix != ""; {
				x.moduleSuffix[suffix] = append(x.moduleSuffix[suffix], n)
				cut := strings.IndexByte(suffix, '.')
				if cut < 0 {
					break
				}
				suffix = suffix[cut+1:]
			}
		}
	}
	return x
}

func rstKindAllows(kind, label string) bool {
	labels, ok := rstKinds[kind]
	if !ok {
		return label == "Class" || label == "Function" || label == "Method" || label == "Variable" || label == "Module"
	}
	for _, want := range labels {
		if want == label {
			return true
		}
	}
	return false
}

func rstCodeCandidate(nodes []api.Node, kind string) (api.Node, bool) {
	var found api.Node
	count := 0
	for _, n := range nodes {
		if !rstKindAllows(kind, n.Label) {
			continue
		}
		found = n
		count++
	}
	return found, count == 1
}

func rstExactCandidate(nodes []api.Node, kind string) (api.Node, bool) {
	var found api.Node
	count := 0
	moduleKind := kind == "mod" || kind == "module" || kind == "obj"
	for _, n := range nodes {
		if n.Label == "File" || n.Label == "Folder" || n.Label == "Section" || !moduleKind && n.Label == "Module" {
			continue
		}
		found = n
		count++
	}
	return found, count == 1
}

// Follow package re-exports at any dotted prefix. A re-export of a class also
// exposes its members through the package name.
func (x rstCodeIndex) dotted(form, kind string) (api.Node, bool) {
	for hop := 0; hop <= 12; hop++ {
		if n, ok := rstExactCandidate(x.byQN[form], kind); ok {
			return n, true
		}
		prefix := form
		for prefix != "" {
			if target, ok := x.aliases[prefix]; ok {
				form = target + strings.TrimPrefix(form, prefix)
				break
			}
			cut := strings.LastIndexByte(prefix, '.')
			if cut < 0 {
				prefix = ""
			} else {
				prefix = prefix[:cut]
			}
		}
		if prefix == "" {
			break
		}
	}
	return api.Node{}, false
}

func (x rstCodeIndex) resolve(target, kind, module, class string, refSpecific, directive bool) (api.Node, bool) {
	var forms []string
	if refSpecific {
		if module != "" && class != "" {
			forms = append(forms, module+"."+class+"."+target)
		}
		if module != "" {
			forms = append(forms, module+"."+target)
		}
		forms = append(forms, target)
	} else {
		forms = append(forms, target)
		if class != "" {
			forms = append(forms, class+"."+target)
		}
		if module != "" {
			forms = append(forms, module+"."+target)
		}
		if module != "" && class != "" {
			forms = append(forms, module+"."+class+"."+target)
		}
	}
	for _, form := range forms {
		if n, ok := x.dotted(form, kind); ok {
			return n, true
		}
	}
	if !strings.Contains(target, ".") && !directive && class == "" {
		switch kind {
		case "class", "exc", "func", "data", "const", "deco":
		default:
			return api.Node{}, false
		}
	}
	name := target
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		name = name[i+1:]
	}
	var candidates []api.Node
	for _, n := range x.byName[name] {
		if strings.Contains(n.Location.File, "/test") || strings.HasPrefix(n.Location.File, "test") {
			continue
		}
		if class != "" && !strings.Contains(target, ".") &&
			(kind == "meth" || kind == "method" || kind == "classmethod" || kind == "staticmethod" ||
				kind == "attr" || kind == "attribute" || kind == "property") &&
			!strings.HasSuffix(x.normalized[n.QualifiedName], "."+class+"."+target) {
			continue
		}
		if n.QualifiedName == target || strings.HasSuffix(n.QualifiedName, "."+target) {
			candidates = append(candidates, n)
		}
	}
	if module != "" && len(candidates) > 1 {
		var local []api.Node
		for _, n := range candidates {
			if strings.HasPrefix(n.QualifiedName, module+".") {
				local = append(local, n)
			}
		}
		if len(local) > 0 {
			candidates = local
		}
	}
	if directive && module != "" {
		var local []api.Node
		modulePath := x.moduleFile[module]
		for _, n := range candidates {
			qn := x.normalized[n.QualifiedName]
			if !strings.HasPrefix(qn, module+".") {
				continue
			}
			if modulePath != "" && n.Location.File == modulePath ||
				strings.Contains(target, ".") || kind == "class" || kind == "exc" || kind == "function" || kind == "func" ||
				class != "" && strings.HasSuffix(qn, "."+class+"."+target) || class == "" {
				local = append(local, n)
			}
		}
		candidates = local
	}
	return rstCodeCandidate(candidates, kind)
}

func (x rstCodeIndex) nearbyModule(doc, target string) (api.Node, bool) {
	if target == "" || strings.HasPrefix(target, ".") || !rstPythonName.MatchString(target) {
		return api.Node{}, false
	}
	path := strings.ReplaceAll(target, ".", "/")
	for dir := filepath.ToSlash(filepath.Dir(doc)); ; dir = filepath.ToSlash(filepath.Dir(dir)) {
		for _, suffix := range []string{path + ".py", path + "/__init__.py"} {
			candidate := filepath.ToSlash(filepath.Clean(filepath.Join(dir, suffix)))
			for _, node := range x.byFile[candidate] {
				if node.Label == "Module" {
					return node, true
				}
			}
		}
		if dir == "." {
			break
		}
	}
	return api.Node{}, false
}

func (x rstCodeIndex) nearbyCode(doc, target, kind string) (api.Node, bool) {
	base := target[strings.LastIndexByte(target, '.')+1:]
	dir := filepath.ToSlash(filepath.Dir(doc)) + "/"
	var candidates []api.Node
	for _, node := range x.byName[base] {
		if !strings.HasPrefix(node.Location.File, dir) || node.Label == "Module" && kind != "mod" && kind != "module" && kind != "obj" {
			continue
		}
		if strings.Contains(target, ".") && !strings.HasSuffix(x.normalized[node.QualifiedName], "."+target) {
			continue
		}
		candidates = append(candidates, node)
	}
	return rstExactCandidate(candidates, kind)
}

func rstTarget(content string) (string, bool, bool) {
	content = strings.ReplaceAll(content, `\ `, "")
	if strings.HasSuffix(content, ">") {
		if start := strings.LastIndexByte(content, '<'); start > 0 {
			content = content[start+1 : len(content)-1]
		}
	}
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "!") {
		return "", false, false
	}
	content = strings.TrimPrefix(content, "~")
	refSpecific := strings.HasPrefix(content, ".")
	content = strings.TrimPrefix(content, ".")
	content = strings.TrimSuffix(content, "()")
	content = strings.TrimSpace(content)
	return content, refSpecific, rstPythonName.MatchString(content)
}

func indexRSTMentions(files []ParsedSyntaxFile, importsByFile map[string]map[string]string, graph *goGraph) {
	x := newRSTCodeIndex(graph.nodes)
	for file, imports := range importsByFile {
		if !strings.HasSuffix(file, "/__init__.py") {
			continue
		}
		module := strings.TrimSuffix(strings.ReplaceAll(file, "/", "."), ".__init__.py")
		for local, target := range imports {
			x.aliases[module+"."+local] = target
		}
	}
	byPath := make(map[string]api.Node)
	for _, n := range graph.nodes {
		if n.Label == "File" || n.Label == "Folder" {
			byPath[n.Location.File] = n
		}
	}
	sections := make(map[string][]api.Node)
	for _, node := range graph.nodes {
		if node.Label == "Section" {
			sections[node.Location.File] = append(sections[node.Location.File], node)
		}
	}
	for _, parsed := range files {
		if parsed.File.Language != "rst" || len(parsed.Body) == 0 {
			continue
		}
		file := filepath.ToSlash(parsed.File.Path)
		module, class := "", ""
		type classFrame struct {
			indent, line int
			previous     string
		}
		var classFrames []classFrame
		lines := strings.Split(string(parsed.Body), "\n")
		for i, line := range lines {
			lineNo := i + 1
			if strings.TrimSpace(line) != "" {
				indent := len(line) - len(strings.TrimLeft(line, " \t"))
				for len(classFrames) > 0 && i > classFrames[len(classFrames)-1].line && indent <= classFrames[len(classFrames)-1].indent {
					class = classFrames[len(classFrames)-1].previous
					classFrames = classFrames[:len(classFrames)-1]
				}
			}
			if match := rstDirectivePattern.FindStringSubmatch(line); match != nil {
				kind, arg := match[2], strings.TrimSpace(match[3])
				if kind == "literalinclude" || kind == "include" {
					if target, ok := rstIncludedTarget(file, arg, lines[i+1:], byPath, x); ok {
						if kind == "literalinclude" {
							rstMention(graph, sections[file], file, lineNo, target, "literalinclude")
						}
					}
					continue
				}
				switch kind {
				case "currentmodule", "module":
					parts := strings.Fields(arg)
					if len(parts) == 0 {
						continue
					}
					module = parts[0]
					class = ""
					classFrames = nil
					if kind == "module" {
						n, ok := x.resolve(module, "module", "", "", false, true)
						if !ok {
							n, ok = x.nearbyModule(file, module)
						}
						if !ok {
							n, ok = byPath[strings.ReplaceAll(module, ".", "/")]
							ok = ok && n.Label == "Folder"
						}
						if ok {
							rstMention(graph, sections[file], file, lineNo, n, "object")
						}
					}
					continue
				case "class":
					classFrames = append(classFrames, classFrame{indent: len(line) - len(strings.TrimLeft(line, " \t")), line: i, previous: class})
					class = strings.TrimSpace(strings.SplitN(arg, "(", 2)[0])
				}
				syntax := "object"
				if strings.HasPrefix(kind, "auto") {
					kind = strings.TrimPrefix(kind, "auto")
					syntax = "autodoc"
					arg = strings.TrimSpace(strings.SplitN(arg, "(", 2)[0])
				}
				if _, ok := rstKinds[kind]; ok {
					declaration := strings.TrimSpace(strings.SplitN(arg, "(", 2)[0])
					if kind == "attribute" || kind == "data" || kind == "property" {
						if cut := strings.IndexAny(declaration, ":="); cut >= 0 {
							declaration = strings.TrimSpace(declaration[:cut])
						}
					}
					name := strings.Fields(declaration)
					if len(name) > 0 {
						target := name[0]
						if kind == "attribute" || kind == "method" || kind == "classmethod" || kind == "staticmethod" || kind == "property" {
							if dot := strings.LastIndexByte(target, '.'); dot > 0 {
								classFrames = append(classFrames, classFrame{indent: len(line) - len(strings.TrimLeft(line, " \t")), line: i, previous: class})
								class = target[:dot]
							}
						}
						if strings.HasPrefix(target, "~") {
							target = target[1:]
						}
						if rstPythonName.MatchString(target) {
							n, ok := x.resolve(target, kind, module, class, false, true)
							if !ok && kind == "module" {
								n, ok = x.nearbyModule(file, target)
							}
							if !ok {
								n, ok = x.nearbyCode(file, target, kind)
							}
							if ok {
								rstMention(graph, sections[file], file, lineNo, n, syntax)
							}
						}
					}
				}
			}
			roleText := line
			if (strings.Count(line, "`")%2 == 1 || rstOpenRolePattern.MatchString(line)) && i+1 < len(lines) {
				roleText += " " + strings.TrimSpace(lines[i+1])
			}
			for _, match := range rstRolePattern.FindAllStringSubmatch(roleText, -1) {
				target, specific, valid := rstTarget(match[3])
				if !valid {
					continue
				}
				n, ok := x.resolve(target, match[2], module, class, specific, false)
				if match[2] == "mod" && !strings.Contains(target, ".") {
					if folder, exists := byPath[target]; exists && folder.Label == "Folder" {
						n, ok = folder, true
					}
				}
				if !ok && match[2] == "mod" {
					n, ok = x.nearbyModule(file, target)
					if !ok {
						n, ok = byPath[strings.ReplaceAll(target, ".", "/")]
						ok = ok && n.Label == "Folder"
					}
				}
				if !ok {
					n, ok = x.nearbyCode(file, target, match[2])
				}
				if ok {
					rstMention(graph, sections[file], file, lineNo, n, "role")
				}
			}
			for _, match := range rstSourceRolePattern.FindAllStringSubmatch(roleText, -1) {
				written := strings.TrimSpace(match[1])
				if strings.HasSuffix(written, ">") {
					if start := strings.LastIndexByte(written, '<'); start >= 0 {
						written = strings.TrimSpace(written[start+1 : len(written)-1])
					}
				}
				if target, ok := rstPathTarget(file, written, byPath); ok {
					rstMention(graph, sections[file], file, lineNo, target, "role")
				}
			}
			for _, match := range rstLiteralPattern.FindAllStringSubmatch(line, -1) {
				written := strings.TrimSpace(match[1])
				if target, ok := rstPathTarget(file, written, byPath); ok {
					rstMention(graph, sections[file], file, lineNo, target, "code_path")
					continue
				}
				if open := strings.IndexByte(written, '('); open >= 0 && strings.HasSuffix(written, ")") {
					written = written[:open]
				}
				if strings.Contains(written, "/") || !strings.Contains(written, ".") || !rstPythonName.MatchString(written) {
					continue
				}
				n, ok := x.codeName(written)
				if !ok {
					packageFile := strings.ReplaceAll(written, ".", "/") + "/__init__.py"
					n, ok = byPath[packageFile]
					ok = ok && n.Label == "File"
				}
				if ok {
					if n.Label == "Module" {
						if fileNode, exists := byPath[n.Location.File]; exists {
							n = fileNode
						}
					}
					rstMention(graph, sections[file], file, lineNo, n, "code_name")
				}
			}
		}
	}
}

func documentSections(graph *goGraph) map[string][]api.Node {
	canonical := make(map[string]api.Node)
	for _, node := range graph.nodes {
		if node.Label == "Section" {
			// A repeated heading has one final node identity; the last occurrence
			// wins. Link to that same occurrence before graph publication.
			canonical[node.QualifiedName] = node
		}
	}
	sections := make(map[string][]api.Node)
	for _, node := range canonical {
		sections[node.Location.File] = append(sections[node.Location.File], node)
	}
	for file := range sections {
		sort.Slice(sections[file], func(i, j int) bool {
			return sections[file][i].Location.StartLine < sections[file][j].Location.StartLine
		})
	}
	return sections
}

func rstPathTarget(doc, written string, byPath map[string]api.Node) (api.Node, bool) {
	if written == "" || strings.ContainsAny(written, " \t\n<>:`") {
		return api.Node{}, false
	}
	if !strings.Contains(written, "/") {
		switch strings.ToLower(filepath.Ext(written)) {
		case ".py", ".rst", ".txt", ".md", ".json", ".yaml", ".yml", ".toml", ".go", ".js", ".ts", ".c", ".h":
		default:
			return api.Node{}, false
		}
	}
	tries := []string{filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(doc), written)))}
	if !strings.HasPrefix(written, "./") && !strings.HasPrefix(written, "../") {
		tries = append(tries, filepath.ToSlash(filepath.Clean(strings.TrimPrefix(written, "/"))))
	}
	for _, candidate := range tries {
		if candidate == doc || candidate == "." || strings.HasPrefix(candidate, "../") {
			continue
		}
		if n, ok := byPath[candidate]; ok {
			return n, true
		}
	}
	return api.Node{}, false
}

func rstIncludedTarget(doc, arg string, following []string, byPath map[string]api.Node, x rstCodeIndex) (api.Node, bool) {
	if arg == "" || strings.Contains(arg, "$") {
		return api.Node{}, false
	}
	path := arg
	if strings.HasPrefix(arg, "/") {
		root := ""
		for dir := filepath.ToSlash(filepath.Dir(doc)); dir != "."; dir = filepath.ToSlash(filepath.Dir(dir)) {
			if _, ok := byPath[filepath.ToSlash(filepath.Join(dir, "conf.py"))]; ok {
				root = dir
				break
			}
		}
		path = filepath.ToSlash(filepath.Join(root, strings.TrimPrefix(arg, "/")))
	} else {
		path = filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(doc), arg)))
	}
	file, ok := byPath[path]
	if !ok || file.Label != "File" {
		return api.Node{}, false
	}
	options := map[string]string{}
	for _, line := range following {
		if strings.TrimSpace(line) == "" {
			continue
		}
		match := rstOptionPattern.FindStringSubmatch(line)
		if match == nil {
			break
		}
		options[match[1]] = strings.TrimSpace(match[2])
	}
	if object := options["pyobject"]; object != "" {
		if !strings.HasSuffix(path, ".py") {
			return api.Node{}, false
		}
		base := strings.TrimSuffix(strings.ReplaceAll(path, "/", "."), ".py")
		base = strings.TrimSuffix(base, ".__init__")
		if n, found := rstCodeCandidate(x.byQN[base+"."+object], "obj"); found {
			return n, true
		}
		return api.Node{}, false
	}
	if span := options["lines"]; span != "" {
		start, end := rstLineSpan(span)
		if start > 0 {
			var best api.Node
			bestSpan := int(^uint(0) >> 1)
			for _, n := range x.byFile[path] {
				if n.Label == "Module" {
					continue
				}
				width := n.Location.EndLine - n.Location.StartLine
				if n.Location.StartLine <= start && n.Location.EndLine >= end && width < bestSpan {
					best, bestSpan = n, width
				}
			}
			if best.QualifiedName != "" {
				return best, true
			}
		}
	}
	return file, true
}

func (x rstCodeIndex) codeName(written string) (api.Node, bool) {
	for _, instance := range []string{"self.", "this.", "cls."} {
		if strings.HasPrefix(written, instance) {
			return x.codeName(strings.TrimPrefix(written, instance))
		}
	}
	if n, ok := rstCodeCandidate(x.byQN[written], "obj"); ok {
		return n, true
	}
	if alias, ok := x.aliases[written]; ok {
		if n, found := rstCodeCandidate(x.byQN[alias], "obj"); found {
			return n, true
		}
	}
	if n, ok := rstCodeCandidate(x.moduleSuffix[written], "module"); ok {
		return n, true
	}
	name := written[strings.LastIndexByte(written, '.')+1:]
	var candidates []api.Node
	for _, n := range x.byName[name] {
		if strings.HasSuffix(x.normalized[n.QualifiedName], "."+written) && !strings.Contains(n.Location.File, "/test") && !strings.HasPrefix(n.Location.File, "test") {
			candidates = append(candidates, n)
		}
	}
	if n, ok := rstCodeCandidate(candidates, "obj"); ok {
		return n, true
	}
	if len(candidates) > 0 {
		return api.Node{}, false
	}
	for _, n := range x.byName[name] {
		qn := x.normalized[n.QualifiedName]
		if len(qn) > len(written) && qn[len(qn)-len(written)-1] == '.' && strings.EqualFold(qn[len(qn)-len(written):], written) &&
			!strings.Contains(n.Location.File, "/test") && !strings.HasPrefix(n.Location.File, "test") {
			candidates = append(candidates, n)
		}
	}
	return rstCodeCandidate(candidates, "obj")
}

func rstLineSpan(span string) (int, int) {
	start, end := 0, 0
	for _, part := range strings.Split(span, ",") {
		part = strings.TrimSpace(part)
		var a, b int
		if strings.Contains(part, "-") {
			pieces := strings.SplitN(part, "-", 2)
			if pieces[0] != "" {
				_, _ = fmt.Sscanf(pieces[0], "%d", &a)
			} else {
				a = 1
			}
			if pieces[1] != "" {
				_, _ = fmt.Sscanf(pieces[1], "%d", &b)
			} else {
				b = a
			}
		} else {
			_, _ = fmt.Sscanf(part, "%d", &a)
			b = a
		}
		if a > 0 && b >= a {
			if start == 0 || a < start {
				start = a
			}
			if b > end {
				end = b
			}
		}
	}
	return start, end
}

func rstMention(graph *goGraph, sections []api.Node, file string, line int, target api.Node, syntax string) {
	source := fileQualifiedName(file)
	var nearest *api.Node
	for i := range sections {
		section := &sections[i]
		if section.Location.StartLine <= line && (nearest == nil || section.Location.StartLine > nearest.Location.StartLine) {
			nearest = section
		}
	}
	if nearest != nil {
		shared := false
		for i := range sections {
			if &sections[i] != nearest && sections[i].QualifiedName == nearest.QualifiedName {
				shared = true
				break
			}
		}
		if !shared {
			source = nearest.QualifiedName
		}
	}
	addDocumentMention(graph, source, file, line, target, "rst", syntax)
}

func documentMention(graph *goGraph, sections []api.Node, file string, line int, target api.Node, via, syntax string) {
	source := fileQualifiedName(file)
	for _, section := range sections {
		if section.Location.StartLine <= line && line <= section.Location.EndLine {
			source = section.QualifiedName
		}
	}
	addDocumentMention(graph, source, file, line, target, via, syntax)
}

func addDocumentMention(graph *goGraph, source, file string, line int, target api.Node, via, syntax string) {
	graph.addEdge(pendingEdge{source: source, target: target.QualifiedName, kind: "MENTIONS",
		properties: api.Properties{"via": via, "syntax": syntax, "tier": "exact", "line": line, "count": 1},
		evidence: &api.Evidence{Strategy: via + "_document_link", Confidence: 1,
			Location: &api.Location{File: file, StartLine: line, EndLine: line}}})
}
