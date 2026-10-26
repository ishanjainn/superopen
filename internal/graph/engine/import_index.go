package engine

import (
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

type importTargetIndex struct {
	nodes          []api.Node
	byQN           map[string][]int
	byName         map[string][]int
	foldersByPath  map[string][]int
	folderSuffixes map[string][]int
	modulesByPath  map[string][]int
	modulePaths    map[string][]int
	moduleSuffixes map[string][]string
	projectQN      string
}

func newImportTargetIndex(nodes []api.Node) *importTargetIndex {
	index := &importTargetIndex{
		nodes:          nodes,
		byQN:           map[string][]int{},
		byName:         map[string][]int{},
		foldersByPath:  map[string][]int{},
		folderSuffixes: map[string][]int{},
		modulesByPath:  map[string][]int{},
		modulePaths:    map[string][]int{},
		moduleSuffixes: map[string][]string{},
	}
	for i, node := range nodes {
		index.byQN[node.QualifiedName] = append(index.byQN[node.QualifiedName], i)
		if node.Name != "" {
			index.byName[node.Name] = append(index.byName[node.Name], i)
		}
		if node.Label == "Project" && index.projectQN == "" {
			index.projectQN = node.QualifiedName
		}
		path := filepath.ToSlash(node.Location.File)
		if path == "" {
			continue
		}
		switch node.Label {
		case "Folder":
			index.foldersByPath[path] = append(index.foldersByPath[path], i)
			index.addPathSuffixes(index.folderSuffixes, path, i)
		case "Module":
			trimmed := strings.TrimSuffix(path, filepath.Ext(path))
			index.modulesByPath[trimmed] = append(index.modulesByPath[trimmed], i)
			index.addPathSuffixes(index.modulePaths, trimmed, i)
			withoutIndex := strings.TrimSuffix(trimmed, "/index")
			if withoutIndex != trimmed {
				index.modulesByPath[withoutIndex] = append(index.modulesByPath[withoutIndex], i)
				index.addPathSuffixes(index.modulePaths, withoutIndex, i)
			}
			qn := node.QualifiedName
			for work := qn; work != ""; {
				index.moduleSuffixes[work] = append(index.moduleSuffixes[work], qn)
				dot := strings.IndexByte(work, '.')
				if dot < 0 {
					break
				}
				work = work[dot+1:]
			}
		}
	}
	return index
}

func (index *importTargetIndex) addPathSuffixes(table map[string][]int, path string, id int) {
	for work := path; ; {
		slash := strings.IndexByte(work, '/')
		if slash < 0 {
			return
		}
		work = work[slash+1:]
		table[work] = append(table[work], id)
	}
}

func (index *importTargetIndex) uniquePathMatches(exact, suffixes map[string][]int, clean, imported string) []string {
	var matches []string
	seen := map[string]bool{}
	add := func(ids []int) {
		for _, id := range ids {
			qn := index.nodes[id].QualifiedName
			if seen[qn] {
				continue
			}
			seen[qn] = true
			matches = append(matches, qn)
		}
	}
	add(exact[clean])
	if !strings.HasPrefix(imported, ".") {
		add(suffixes[clean])
	}
	return matches
}

func localSyntaxImportTarget(sourceFile, imported string, nodes []api.Node) string {
	return localSyntaxImportTargetForLanguage("", sourceFile, imported, "", newImportTargetIndex(nodes))
}

func localSyntaxImportTargetForLanguage(language, sourceFile, imported, goModule string, index *importTargetIndex) string {
	if index == nil {
		return ""
	}
	imported = strings.TrimSpace(strings.Trim(imported, "\"'`"))
	if imported == "" {
		return ""
	}
	if isJSLanguage(language) {
		return jsSyntaxImportTarget(sourceFile, imported, index)
	}
	if language == "python" {
		return pythonSyntaxImportTarget(sourceFile, imported, index)
	}
	clean := filepath.ToSlash(imported)
	if language == "go" && goModule != "" {
		if clean == goModule {
			clean = ""
		} else if strings.HasPrefix(clean, goModule+"/") {
			clean = strings.TrimPrefix(clean, goModule+"/")
		}
	}
	if strings.HasPrefix(clean, ".") {
		clean = filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(sourceFile), clean)))
	} else {
		clean = strings.TrimPrefix(clean, "@/")
	}
	if language == "go" && clean == "" {
		return index.projectQN
	}
	clean = strings.TrimSuffix(clean, filepath.Ext(clean))
	folderMatches := index.uniquePathMatches(index.foldersByPath, index.folderSuffixes, clean, imported)
	if len(folderMatches) == 1 {
		return folderMatches[0]
	}
	moduleMatches := index.uniquePathMatches(index.modulesByPath, index.modulePaths, clean, imported)
	if len(moduleMatches) == 1 {
		return moduleMatches[0]
	}
	if isJSLanguage(language) && !strings.HasPrefix(strings.TrimSpace(imported), ".") &&
		!strings.HasPrefix(strings.TrimSpace(imported), "@/") {
		return ""
	}
	if target := syntaxImportSymbolFallback(sourceFile, imported, index); target != "" {
		return target
	}
	work := strings.Trim(strings.NewReplacer("::", "/", "\\", "/").Replace(imported), " */\"'`")
	for _, prefix := range []string{"crate/", "self/", "super/"} {
		work = strings.TrimPrefix(work, prefix)
	}
	own := fileQualifiedName(sourceFile)
	for work != "" {
		candidate := strings.ReplaceAll(strings.TrimSuffix(work, filepath.Ext(work)), "/", ".")
		for _, id := range index.byQN[candidate] {
			node := index.nodes[id]
			if syntaxImportTargetable(node.Label) && node.QualifiedName != own {
				return node.QualifiedName
			}
		}
		slash := strings.LastIndexByte(work, '/')
		if slash < 0 {
			break
		}
		work = work[:slash]
	}
	return ""
}

// jsSyntaxImportTarget addresses JS/TS specifiers by qualified name instead of
// by file path: the resolved path is dotted and matched against node qualified
// names, retrying progressively shorter left-trimmed suffixes.
func jsSyntaxImportTarget(sourceFile, imported string, index *importTargetIndex) string {
	clean := filepath.ToSlash(imported)
	if strings.HasPrefix(clean, ".") {
		clean = filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(sourceFile), clean)))
	} else {
		clean = strings.TrimPrefix(clean, "@/")
	}
	clean = strings.Trim(strings.TrimSuffix(clean, filepath.Ext(clean)), "/")
	own := fileQualifiedName(sourceFile)
	for work := clean; work != ""; {
		dotted := strings.ReplaceAll(work, "/", ".")
		for _, candidate := range []string{dotted, dotted + ".index"} {
			for _, id := range index.byQN[candidate] {
				node := index.nodes[id]
				if node.QualifiedName == own {
					continue
				}
				if node.Label == "Module" || node.Label == "Folder" {
					return node.QualifiedName
				}
			}
		}
		slash := strings.IndexByte(work, '/')
		if slash < 0 {
			break
		}
		work = work[slash+1:]
	}
	if !strings.HasPrefix(imported, ".") {
		if target := jsBareImportModuleTarget(clean, own, index); target != "" {
			return target
		}
	}
	return jsImportTypeSymbolTarget(sourceFile, clean, index)
}

func jsBareImportModuleTarget(clean, own string, index *importTargetIndex) string {
	dotted := strings.ReplaceAll(clean, "/", ".")
	if dotted == "" {
		return ""
	}
	var matches []string
	seen := map[string]bool{}
	for _, qn := range index.moduleSuffixes[dotted] {
		if qn == own || seen[qn] {
			continue
		}
		seen[qn] = true
		matches = append(matches, qn)
	}
	if len(matches) != 1 {
		return ""
	}
	return matches[0]
}

func jsImportTypeSymbolTarget(sourceFile, clean string, index *importTargetIndex) string {
	base := clean
	if slash := strings.LastIndexByte(base, '/'); slash >= 0 {
		base = base[slash+1:]
	}
	if base == "" {
		return ""
	}
	own := fileQualifiedName(sourceFile)
	family := pathLanguageFamily(sourceFile)
	var candidates []string
	for _, id := range index.byName[base] {
		node := index.nodes[id]
		if node.QualifiedName == own || !sameLanguageFamily(family, node.Location.File) {
			continue
		}
		if syntaxImportTargetable(node.Label) && node.Label != "Module" && node.Label != "File" {
			candidates = append(candidates, node.QualifiedName)
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	sort.Strings(candidates)
	return candidates[0]
}

func syntaxImportSymbolFallback(sourceFile, imported string, index *importTargetIndex) string {
	trimmed := strings.TrimSpace(strings.Trim(imported, "\"'`"))
	segments := strings.FieldsFunc(trimmed, func(value rune) bool {
		return value == '.' || value == '/' || value == '\\' || value == ':'
	})
	own := fileQualifiedName(sourceFile)
	family := pathLanguageFamily(sourceFile)
	for i := len(segments) - 1; i >= 0; i-- {
		var candidates []string
		for _, id := range index.byName[segments[i]] {
			node := index.nodes[id]
			if node.QualifiedName == own || !syntaxImportTargetable(node.Label) || !sameLanguageFamily(family, node.Location.File) {
				continue
			}
			candidates = append(candidates, node.QualifiedName)
		}
		if len(candidates) > 0 {
			sort.Strings(candidates)
			return candidates[0]
		}
	}
	return ""
}

// pythonSyntaxImportTarget resolves a dotted Python import (pkg.mod.Name,
// .sibling.Name) to the qualified name it binds. Qualified names follow file
// paths, with a package's own symbols under pkg.__init__, so the path is
// tried as written, with __init__ inserted, and then one segment shorter
// until a module or package matches. A path that matches nothing in the
// project is a standard-library or third-party import and stays unresolved:
// guessing by the last segment bound "from django.db import connections" to
// a method named db and "from collections import Counter" to an unrelated
// module called collections.
func pythonSyntaxImportTarget(sourceFile, imported string, index *importTargetIndex) string {
	dotted := imported
	if strings.HasPrefix(dotted, ".") {
		rest := strings.TrimLeft(dotted, ".")
		pkg := path.Dir(filepath.ToSlash(sourceFile))
		for up := len(dotted) - len(rest); up > 1; up-- {
			pkg = path.Dir(pkg)
		}
		dotted = rest
		if pkg != "." && pkg != "/" {
			base := strings.ReplaceAll(pkg, "/", ".")
			if dotted == "" {
				dotted = base
			} else {
				dotted = base + "." + dotted
			}
		}
	}
	if dotted == "" {
		return ""
	}
	own := fileQualifiedName(sourceFile)
	lookup := func(qn string) string {
		candidates := []string{qn, qn + ".__init__"}
		if dot := strings.LastIndexByte(qn, '.'); dot > 0 {
			candidates = append(candidates, qn[:dot]+".__init__"+qn[dot:])
		}
		for _, candidate := range candidates {
			for _, id := range index.byQN[candidate] {
				node := index.nodes[id]
				if node.QualifiedName != own && pythonImportTargetable(node.Label) {
					return node.QualifiedName
				}
			}
		}
		return ""
	}
	for work := dotted; work != ""; {
		if target := lookup(work); target != "" {
			return target
		}
		// A src/ or lib/ layout puts the package under a directory that is
		// not itself a package; its qualified names carry that prefix.
		if module := index.pythonRootedModule(work); module != "" {
			if rest := strings.TrimPrefix(dotted, work); rest != "" {
				if target := lookup(module + rest); target != "" {
					return target
				}
			}
			return module
		}
		dot := strings.LastIndexByte(work, '.')
		if dot < 0 {
			break
		}
		work = work[:dot]
	}
	return ""
}

// pythonRootedModule finds the one module whose qualified name is work
// under a directory prefix that is not a Python package (no __init__ module).
func (index *importTargetIndex) pythonRootedModule(work string) string {
	match := ""
	for _, key := range []string{work, work + ".__init__"} {
		for _, qn := range index.moduleSuffixes[key] {
			if qn == key || !strings.HasSuffix(qn, "."+key) {
				continue
			}
			prefix := strings.TrimSuffix(qn, "."+key)
			if len(index.byQN[prefix+".__init__"]) > 0 {
				continue
			}
			if match != "" && match != qn {
				return ""
			}
			match = qn
		}
	}
	return match
}

func pythonImportTargetable(label string) bool {
	return syntaxImportTargetable(label) || label == "Variable" || label == "Folder"
}

// sameLanguageFamily is true when a candidate file could define what a file
// of the given family imports by name. Unknown families match anything.
func sameLanguageFamily(family, candidateFile string) bool {
	other := pathLanguageFamily(candidateFile)
	return family == "" || other == "" || other == family
}
