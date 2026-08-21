package engine

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func indexHTTPRoutes(project string, files []ParsedSyntaxFile, graph *goGraph) {
	byFileName := map[string][]api.Node{}
	modules := map[string]string{}
	for _, node := range graph.nodes {
		switch node.Label {
		case "Function", "Method":
			key := node.Location.File + "\x00" + node.Name
			byFileName[key] = append(byFileName[key], node)
		case "Module":
			modules[node.Location.File] = node.QualifiedName
		}
	}
	for key := range byFileName {
		sort.Slice(byFileName[key], func(i, j int) bool { return byFileName[key][i].QualifiedName < byFileName[key][j].QualifiedName })
	}
	resolvedCalls := map[string]string{}
	for _, edge := range graph.edges {
		if edge.kind != "CALLS" {
			continue
		}
		callee := edge.Callee()
		if callee != "" {
			resolvedCalls[edge.source+"\x00"+callee] = edge.target
		}
	}
	suppressResolved := map[string]bool{}
	for _, parsed := range files {
		for _, call := range parsed.Extraction.Calls {
			if len(call.Arguments) == 0 {
				continue
			}
			source := enclosingRouteSource(parsed, call, modules, byFileName)
			resolved := resolvedCalls[source+"\x00"+call.Name]
			importIdentity := importedCallIdentity(parsed.File.Language, call.Name, parsed.Extraction.Imports)
			identity := resolved
			if identity == "" {
				identity = call.Name
			}
			kind, _ := matchService(identity)
			calleeKind, _ := matchService(call.Name)
			if calleeKind == serviceHTTP || calleeKind == serviceAsync {
				kind = calleeKind
			}
			if kind == serviceNone && call.Name == "fetch" {
				kind = serviceHTTP
			}
			importKind, _ := matchService(importIdentity)
			if call.FirstStringArg != "" && serviceRouteMethod(call.Name) != "" &&
				(importKind == serviceRouteRegistration || importKind == serviceHTTP) {
				addRegisteredRoute(project, graph, source, call, call.FirstStringArg, serviceRouteMethod(call.Name))
				if resolved != "" {
					suppressResolved[source+"\x00"+call.Name+"\x00"+resolved] = true
				}
				continue
			}
			if kind == serviceHTTP && call.FirstStringArg != "" && isHTTPRouteLiteral(call.FirstStringArg, call.Name) {
				addHTTPRoute(project, graph, source, call, call.FirstStringArg, serviceHTTPMethod(call.Name), "service_pattern")
				continue
			}
			if resolved != "" {
				for _, argument := range call.Arguments {
					// Tree-sitter's JS regex node is not present in Superopen's
					// call argument argument surface. Our generic argument collector does
					// retain it, so suppress that non-literal occurrence here while
					// preserving slash-terminated string literals such as "/rules/".
					if !argument.Literal && looksLikeJavaScriptRegex(strings.TrimSpace(argument.Expr)) {
						continue
					}
					candidate := argument.Value
					if candidate == "" {
						candidate = argument.Expr
					}
					if normalized, ok := normalizeArgumentURL(candidate); ok {
						addArgumentURLRoute(project, graph, source, call, normalized)
						break
					}
				}
			}
		}
	}
	if len(suppressResolved) > 0 {
		filtered := graph.edges[:0]
		for _, edge := range graph.edges {
			callee := edge.Callee()
			if edge.kind == "CALLS" && suppressResolved[edge.source+"\x00"+callee+"\x00"+edge.target] {
				continue
			}
			filtered = append(filtered, edge)
		}
		graph.edges = filtered
	}
	indexInfraURLRoutes(project, files, graph)
}

// indexInfraURLRoutes mirrors try_upsert_infra_route for YAML/config URL values.
func indexInfraURLRoutes(project string, files []ParsedSyntaxFile, graph *goGraph) {
	seen := map[string]bool{}
	for _, parsed := range files {
		if isToolingConfigFile(parsed.File.Path) {
			continue
		}
		for _, route := range parsed.Extraction.Routes {
			if route.Kind != "infra_url" || route.Name == "" {
				continue
			}
			if strings.Contains(route.Name, " ") || !strings.Contains(route.Name, "://") {
				continue
			}
			keyPath := route.FirstStringArg
			if keyPath == "" {
				keyPath = route.Scope
			}
			if keyPath == "" || isUpstreamConfigKey(keyPath) {
				continue
			}
			target := "__route__infra__" + route.Name
			if seen[target] {
				continue
			}
			seen[target] = true
			properties := api.Properties{"source": "infra", "key_path": keyPath}
			graph.nodes = append(graph.nodes, api.Node{Project: project, Label: "Route", Name: route.Name,
				QualifiedName: target, Location: api.Location{File: parsed.File.Path}, Properties: properties})
		}
	}
}

func isToolingConfigFile(rel string) bool {
	base := filepath.Base(rel)
	for _, name := range []string{
		".pre-commit-config.yaml", ".pre-commit-hooks.yaml", ".gitlab-ci.yml", ".travis.yml",
		"azure-pipelines.yml", "appveyor.yml", "bitbucket-pipelines.yml", ".readthedocs.yaml",
		".readthedocs.yml", "codecov.yml", ".codecov.yml", ".goreleaser.yaml", ".goreleaser.yml",
		".golangci.yml", ".golangci.yaml",
	} {
		if base == name {
			return true
		}
	}
	return strings.HasPrefix(filepath.ToSlash(rel), ".github/workflows/")
}

// addArgumentURLRoute mirrors detect_url_in_args in the pinned pipeline. This
// deliberately treats any sufficiently URL-shaped argument as a route, even
// when it is really a filesystem path: that behavior is observable Superopen.
func addArgumentURLRoute(project string, graph *goGraph, source string, call SyntaxFact, routePath string) {
	target := "__route__ANY__" + canonicalRoutePath(routePath)
	graph.nodes = append(graph.nodes, api.Node{Project: project, Label: "Route", Name: routePath,
		QualifiedName: target, Properties: api.Properties{"source": "arg_url"}})
	graph.edges = append(graph.edges, pendingEdge{source: source, target: target, kind: "HTTP_CALLS",
		properties: api.Properties{"callee": call.Name, "url_path": routePath, "via": "arg_url"},
		evidence:   &api.Evidence{Strategy: "arg_url", Confidence: .8}})
}

func serviceResolutionMatches(resolved, identity string) bool {
	if resolved == "" {
		return false
	}
	resolved = strings.TrimPrefix(resolved, "external:")
	return resolved == identity || strings.ReplaceAll(resolved, ".", "/") == strings.ReplaceAll(identity, ".", "/")
}

func enclosingRouteSource(parsed ParsedSyntaxFile, call SyntaxFact, modules map[string]string, byFileName map[string][]api.Node) string {
	source := modules[parsed.File.Path]
	if source == "" {
		source = fileQualifiedName(parsed.File.Path)
	}
	ownerName, ownerSpan := "", ^uint32(0)
	for _, fact := range parsed.Extraction.Definitions {
		if fact.Kind != "function" || fact.StartByte > call.StartByte || call.StartByte >= fact.EndByte {
			continue
		}
		if span := fact.EndByte - fact.StartByte; span < ownerSpan {
			ownerName, ownerSpan = fact.Name, span
		}
	}
	if candidates := byFileName[parsed.File.Path+"\x00"+ownerName]; ownerName != "" && len(candidates) > 0 {
		source = candidates[0].QualifiedName
	}
	return source
}

func addHTTPRoute(project string, graph *goGraph, source string, call SyntaxFact, routePath, method, strategy string) {
	prefix := method
	if prefix == "" {
		prefix = "ANY"
	}
	canonical := canonicalRoutePath(routePath)
	target := "__route__" + prefix + "__" + canonical
	properties := api.Properties{}
	if method != "" {
		properties["method"] = method
	}
	// Superopen names the route node by its canonical path, so a templated URL
	// and its interpolated siblings collapse onto one identity.
	graph.nodes = append(graph.nodes, api.Node{Project: project, Label: "Route", Name: canonical,
		QualifiedName: target, Properties: properties})
	edgeProperties := api.Properties{"callee": call.Name, "url_path": routePath}
	if method != "" {
		edgeProperties["method"] = method
	}
	graph.edges = append(graph.edges, pendingEdge{source: source, target: target, kind: "HTTP_CALLS",
		properties: edgeProperties,
		evidence:   &api.Evidence{Strategy: strategy, Confidence: .8}})
}

func addRegisteredRoute(project string, graph *goGraph, source string, call SyntaxFact, routePath, method string) {
	target := "__route__" + method + "__" + canonicalRoutePath(routePath)
	graph.nodes = append(graph.nodes, api.Node{Project: project, Label: "Route", Name: routePath,
		QualifiedName: target, Properties: api.Properties{"method": method}})
	graph.edges = append(graph.edges, pendingEdge{source: source, target: target, kind: "CALLS",
		properties: api.Properties{"callee": call.Name, "url_path": routePath, "via": "route_registration"},
		evidence:   &api.Evidence{Strategy: "service_pattern", Confidence: .8}})
}

func canonicalRoutePath(input string) string {
	var result strings.Builder
	segmentStart := true
	for index := 0; index < len(input); {
		switch {
		case input[index] == ':' && segmentStart && index+1 < len(input) && isRouteIdentifier(input[index+1]):
			index++
			for index < len(input) && isRouteIdentifier(input[index]) {
				index++
			}
			result.WriteString("{}")
			segmentStart = false
		case input[index] == '{':
			index = skipRoutePlaceholder(input, index+1, '}')
			result.WriteString("{}")
			segmentStart = false
		case input[index] == '<':
			index = skipRoutePlaceholder(input, index+1, '>')
			result.WriteString("{}")
			segmentStart = false
		case input[index] == '$' && index+1 < len(input) && input[index+1] == '{':
			index = skipRoutePlaceholder(input, index+2, '}')
			result.WriteString("{}")
			segmentStart = false
		default:
			result.WriteByte(input[index])
			segmentStart = input[index] == '/'
			index++
		}
	}
	return result.String()
}

func skipRoutePlaceholder(input string, index int, terminator byte) int {
	for index < len(input) && input[index] != terminator && input[index] != '/' {
		index++
	}
	if index < len(input) && input[index] == terminator {
		index++
	}
	return index
}

func isRouteIdentifier(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '_'
}

func normalizeArgumentURL(input string) (string, bool) {
	input = strings.TrimSpace(input)
	if len(input) > 1 && strings.ContainsRune("\"'`", rune(input[0])) {
		input = input[1:]
	}
	if input == "" || input[0] != '/' {
		return "", false
	}
	var result strings.Builder
	for index := 0; index < len(input); {
		if input[index] == '$' && index+1 < len(input) && input[index+1] == '{' {
			result.WriteByte(':')
			index += 2
			for index < len(input) && input[index] != '}' {
				result.WriteByte(input[index])
				index++
			}
			if index < len(input) {
				index++
			}
			continue
		}
		if strings.ContainsRune("\"'`?", rune(input[index])) {
			break
		}
		result.WriteByte(input[index])
		index++
	}
	value := result.String()
	if !validGenericArgURL(value) {
		return "", false
	}
	return value, true
}

func looksLikeJavaScriptRegex(value string) bool {
	if len(value) < 3 || value[0] != '/' {
		return false
	}
	last := strings.LastIndexByte(value[1:], '/')
	if last < 0 {
		return false
	}
	last++
	for _, flag := range value[last+1:] {
		if !strings.ContainsRune("dgimsuvy", flag) {
			return false
		}
	}
	return last < len(value)-1 || !strings.Contains(value[1:last], "/")
}

func validGenericArgURL(routePath string) bool {
	if len(routePath) < 4 || routePath[0] != '/' || !strings.Contains(routePath[1:], "/") {
		return false
	}
	return !strings.Contains(routePath, "//") && !strings.ContainsAny(routePath, `\^$*+()[]| `)
}

func isHTTPRouteLiteral(routePath, callee string) bool {
	routePath = strings.TrimSpace(strings.Trim(routePath, "\"'`"))
	if strings.HasPrefix(routePath, "http://") || strings.HasPrefix(routePath, "https://") {
		return true
	}
	if routePath == "" || routePath[0] != '/' || strings.Contains(routePath, "://") {
		return false
	}
	method := callee
	if index := strings.LastIndex(method, "."); index >= 0 {
		method = method[index+1:]
	}
	if index := strings.LastIndex(method, "::"); index >= 0 {
		method = method[index+2:]
	}
	if method == "split" || method == "rsplit" || method == "partition" || method == "join" ||
		strings.Contains(callee, "os.path.join") || strings.Contains(callee, "path.join") {
		return false
	}
	first := strings.TrimPrefix(routePath, "/")
	if slash := strings.IndexByte(first, '/'); slash >= 0 {
		first = first[:slash]
	}
	for _, root := range []string{"etc", "root", "var", "usr", "home", "tmp", "private", "opt", "bin", "sbin", "dev", "proc", "sys", "run", "lib", "lib64", "mnt", "media", "boot", "srv", "Users", "Volumes"} {
		if first == root {
			return false
		}
	}
	for _, hidden := range []string{".aws", ".azure", ".config", ".docker", ".env", ".git", ".gnupg", ".kube", ".ssh"} {
		if containsPathSegment(routePath, hidden) {
			return false
		}
	}
	return !hasFilesystemRouteExtension(routePath)
}

func containsPathSegment(value, segment string) bool {
	for _, item := range strings.Split(value, "/") {
		if item == segment {
			return true
		}
	}
	return false
}

func hasFilesystemRouteExtension(routePath string) bool {
	base := routePath
	if index := strings.IndexAny(base, "?#"); index >= 0 {
		base = base[:index]
	}
	ext := filepath.Ext(base)
	for _, hard := range []string{".cfg", ".conf", ".credentials", ".crt", ".db", ".env", ".ini", ".key", ".pem", ".pid", ".properties", ".service", ".sock", ".socket", ".sqlite", ".toml"} {
		if ext == hard {
			return true
		}
	}
	if ext == ".json" || ext == ".yaml" || ext == ".yml" || ext == ".xml" {
		return !hasHTTPRouteMarker(routePath)
	}
	return false
}

func hasHTTPRouteMarker(routePath string) bool {
	first := strings.TrimPrefix(routePath, "/")
	if slash := strings.IndexByte(first, '/'); slash >= 0 {
		first = first[:slash]
	}
	if first == "api" || first == "apis" || first == "graphql" || first == "health" || first == "metrics" {
		return true
	}
	return len(first) >= 2 && first[0] == 'v' && first[1] >= '0' && first[1] <= '9'
}
