package engine

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

type packageMap struct {
	aliases map[string]string
	modules map[string]string
}

func buildPackageMap(root string, files []ParsedSyntaxFile) packageMap {
	result := packageMap{aliases: map[string]string{}, modules: map[string]string{}}
	for _, parsed := range files {
		rel := filepath.ToSlash(parsed.File.Path)
		body := parsedSource(root, parsed)
		if len(body) == 0 {
			continue
		}
		switch filepath.Base(rel) {
		case "package.json":
			indexPackageJSON(root, rel, body, &result)
		case "go.mod":
			indexGoModPackageMap(body, &result)
		case "Cargo.toml":
			indexCargoToml(body, &result)
		case "pyproject.toml":
			indexPyProjectToml(body, &result)
		case "composer.json":
			indexComposerJSON(body, &result)
		case "pubspec.yaml":
			indexPubspecYAML(body, &result)
		case "pom.xml":
			indexPomXML(body, &result)
		case "build.gradle", "build.gradle.kts":
			indexGradle(body, &result)
		case "mix.exs":
			indexMixExs(body, &result)
		case "Package.swift":
			indexPackageSwift(body, &result)
		}
		if strings.HasSuffix(rel, ".gemspec") {
			indexGemspec(body, &result)
		}
	}
	return result
}

func indexPackageJSON(root, rel string, body []byte, result *packageMap) {
	var payload struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Name == "" {
		return
	}
	dir := filepath.ToSlash(filepath.Dir(rel))
	if dir == "." {
		dir = ""
	}
	result.modules[payload.Name] = dir
}

func indexGoModPackageMap(body []byte, result *packageMap) {
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "//", 2)[0])
		if strings.HasPrefix(line, "module ") {
			result.modules[strings.TrimSpace(strings.TrimPrefix(line, "module "))] = ""
		}
	}
}

func indexCargoToml(body []byte, result *packageMap) {
	inPackage := false
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[package]" {
			inPackage = true
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			inPackage = false
		}
		if inPackage && strings.HasPrefix(trimmed, "name = ") {
			name := strings.Trim(trimmed[len("name = "):], `"`)
			if name != "" {
				result.modules[name] = ""
			}
		}
	}
}

func indexPyProjectToml(body []byte, result *packageMap) {
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "name = ") {
			name := strings.Trim(trimmed[len("name = "):], `"'`)
			if name != "" {
				result.modules[name] = ""
			}
		}
	}
}

func indexComposerJSON(body []byte, result *packageMap) {
	var payload struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Name == "" {
		return
	}
	result.modules[payload.Name] = ""
}

func indexPubspecYAML(body []byte, result *packageMap) {
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "name:") {
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, "name:"))
			if name != "" {
				result.modules[name] = ""
			}
		}
	}
}

func indexPomXML(body []byte, result *packageMap) {
	text := string(body)
	if start := strings.Index(text, "<artifactId>"); start >= 0 {
		start += len("<artifactId>")
		if end := strings.Index(text[start:], "</artifactId>"); end >= 0 {
			name := text[start : start+end]
			if name != "" {
				result.modules[name] = ""
			}
		}
	}
}

func indexGradle(body []byte, result *packageMap) {
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		for _, key := range []string{"rootProject.name", "archivesBaseName"} {
			if strings.Contains(trimmed, key) {
				if parts := strings.Split(trimmed, "="); len(parts) == 2 {
					name := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
					if name != "" {
						result.modules[name] = ""
					}
				}
			}
		}
	}
}

func indexMixExs(body []byte, result *packageMap) {
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "app:") {
			name := strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "app:")), `"'`)
			if name != "" {
				result.modules[name] = ""
			}
		}
	}
}

func indexPackageSwift(body []byte, result *packageMap) {
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "name:") {
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, "name:"))
			if name != "" {
				result.modules[name] = ""
			}
		}
	}
}

func indexGemspec(body []byte, result *packageMap) {
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "spec.name") {
			if parts := strings.Split(trimmed, "="); len(parts) == 2 {
				name := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
				if name != "" {
					result.modules[name] = ""
				}
			}
		}
	}
}

func (m packageMap) resolveBareImport(specifier, sourceFile string, index *importTargetIndex) string {
	specifier = strings.TrimSpace(strings.Trim(specifier, "\"'`"))
	if specifier == "" || strings.HasPrefix(specifier, ".") || strings.HasPrefix(specifier, "/") {
		return localSyntaxImportTargetForLanguage("", sourceFile, specifier, "", index)
	}
	if path, ok := m.modules[specifier]; ok && index != nil {
		if path == "" {
			for _, node := range index.nodes {
				if node.Label == "Module" || node.Label == "Folder" {
					return node.QualifiedName
				}
			}
		} else {
			for _, node := range index.nodes {
				if node.Label != "Module" && node.Label != "Folder" {
					continue
				}
				candidate := filepath.ToSlash(node.Location.File)
				if candidate == path || strings.HasSuffix(candidate, "/"+path) {
					return node.QualifiedName
				}
			}
		}
	}
	if alias, ok := m.aliases[specifier]; ok {
		return alias
	}
	return ""
}

func applyPackageMap(_ string, repository SyntaxRepository, graph *goGraph) {
	mapping := buildPackageMap(repository.Root, repository.Files)
	if len(mapping.modules) == 0 && len(mapping.aliases) == 0 {
		return
	}
	index := newImportTargetIndex(graph.nodes)
	for i := range graph.edges {
		edge := &graph.edges[i]
		if edge.kind != "IMPORTS" {
			continue
		}
		localName, _ := edge.properties["local_name"].(string)
		specifier := localName
		if specifier == "" {
			specifier = edge.target
		}
		if resolved := mapping.resolveBareImport(specifier, "", index); resolved != "" && resolved != edge.target {
			edge.target = resolved
			if edge.evidence == nil {
				edge.evidence = &api.Evidence{Strategy: "pkgmap", Confidence: .95}
			} else {
				edge.evidence.Strategy = "pkgmap"
				edge.evidence.Confidence = .95
			}
		}
	}
}
