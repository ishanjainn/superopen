package engine

import "strings"

// Python modules bind imported names into their namespace. Package initializers
// expose that namespace under the package name. Direct definitions win over
// imported names; bounded passes follow chains of re-exports.
func collectPythonPackageExports(files []ParsedSyntaxFile, registry symbolRegistry, index *importTargetIndex) map[string]string {
	exports := map[string]string{}
	type pendingExport struct{ key, target string }
	var pending []pendingExport
	for _, parsed := range files {
		if parsed.File.Language != "python" {
			continue
		}
		moduleQN := syntaxDefinitionModuleQN("python", parsed.File.Path)
		packageQN := strings.TrimSuffix(moduleQN, ".__init__")
		for _, fact := range parsed.Extraction.Imports {
			if fact.LocalName == "" {
				continue
			}
			key := packageQN + "." + fact.LocalName
			if _, own := registry.exact[moduleQN+"."+fact.LocalName]; own {
				continue
			}
			if _, own := registry.exact[key]; own {
				continue
			}
			target := localSyntaxImportTargetForLanguage("python", parsed.File.Path, fact.Name, "", index)
			if node, ok := registry.exact[target]; ok && jsxCallableLabel(node.Label) {
				exports[key] = target
			} else {
				pending = append(pending, pendingExport{key: key, target: pythonAbsoluteImportName(parsed.File.Path, fact.Name)})
			}
		}
	}
	for pass := 0; pass < len(pending); pass++ {
		changed := false
		for _, item := range pending {
			if target := exports[item.target]; target != "" && exports[item.key] != target {
				exports[item.key] = target
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return exports
}
