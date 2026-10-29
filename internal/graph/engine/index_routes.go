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
	registry := newSymbolRegistry(graph.nodes)
	importIndex := newImportTargetIndex(graph.nodes)
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
		imports := map[string]string{}
		for _, fact := range parsed.Extraction.Imports {
			if fact.LocalName != "" {
				if target := localSyntaxImportTargetForLanguage(parsed.File.Language, parsed.File.Path, fact.Name, "", importIndex); target != "" {
					imports[fact.LocalName] = target
				}
			}
		}
		for _, call := range parsed.Extraction.Calls {
			if len(call.Arguments) == 0 {
				continue
			}
			registrationPath := routeRegistrationPath(call, parsed.File.Language)
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
			if registrationPath != "" && serviceRouteMethod(call.Name) != "" &&
				isJSSupertestRequest(call.Name, parsed.Extraction.Imports) {
				addHTTPRoute(project, graph, source, call, registrationPath, serviceRouteMethod(call.Name), "service_pattern")
				continue
			}
			if registrationPath != "" && serviceRouteMethod(call.Name) != "" &&
				(importKind == serviceRouteRegistration || importKind == serviceHTTP ||
					parsed.File.Language == "go" || isJSLanguage(parsed.File.Language)) {
				method := serviceRouteMethod(call.Name)
				addRegisteredRoute(project, graph, source, parsed.File.Path, call, registrationPath, method)
				addRegisteredRouteHandler(graph, registry, parsed, source, call, registrationPath, method, imports)
				if resolved != "" {
					suppressResolved[source+"\x00"+call.Name+"\x00"+resolved] = true
				}
				continue
			}
			if kind == serviceHTTP && call.FirstStringArg != "" && isHTTPRouteLiteral(call.FirstStringArg, call.Name) {
				addHTTPRoute(project, graph, source, call, call.FirstStringArg, serviceHTTPMethod(call.Name), "service_pattern")
				continue
			}
			if method := pythonTestClientMethod(parsed.File.Path, parsed.File.Language, call.Name); method != "" &&
				strings.HasPrefix(call.FirstStringArg, "/") && !strings.Contains(call.FirstStringArg, "://") {
				addHTTPRoute(project, graph, source, call, call.FirstStringArg, method, "test_client")
				continue
			}
			if isJSLanguage(parsed.File.Language) && expressRedirectCallee(call.Name) &&
				routePathShaped(call.FirstStringArg) {
				addRedirectRoute(project, graph, source, parsed.File.Path, call)
				continue
			}
			if resolved != "" {
				if isJSLanguage(parsed.File.Language) && isClientNavigationCall(call.Name) {
					continue
				}
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
					if normalized, ok := normalizeArgumentURL(candidate); ok && isHTTPRouteLiteral(normalized, call.Name) {
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

// routeRegistrationPath follows the route-specific argument check: a literal
// string is preferred, then a slash-shaped first or named path argument. A
// JavaScript regex is accepted only on an Express-style registration call.
func routeRegistrationPath(call SyntaxFact, language string) string {
	if routePathShaped(call.FirstStringArg) {
		return call.FirstStringArg
	}
	for _, argument := range call.Arguments {
		if argument.Index != 0 && !routePathKeyword(argument.Keyword) {
			continue
		}
		path := argument.Value
		if path == "" {
			path = argument.Expr
		}
		if !routePathShaped(path) {
			continue
		}
		if !argument.Literal && isJSLanguage(language) && looksLikeJavaScriptRegex(path) && !expressRouteCallee(call.Name) {
			continue
		}
		return path
	}
	return ""
}

func routePathShaped(path string) bool {
	return strings.HasPrefix(path, "/") && !strings.ContainsAny(path, "\r\n") &&
		!(strings.HasPrefix(path, "/*") && strings.HasSuffix(path, "*/")) &&
		!strings.HasPrefix(path, "// ")
}

func routePathKeyword(keyword string) bool {
	switch keyword {
	case "prefix", "path", "route", "pattern", "url", "endpoint", "rule", "mount_path", "route_path", "url_path":
		return true
	}
	return false
}

func expressRouteCallee(callee string) bool {
	for _, receiver := range []string{"app.", "router.", "route."} {
		if strings.HasPrefix(callee, receiver) {
			return true
		}
	}
	return false
}

func expressRedirectCallee(callee string) bool {
	return callee == "res.redirect" || callee == "response.redirect"
}

func addRedirectRoute(project string, graph *goGraph, source, file string, call SyntaxFact) {
	path := canonicalRoutePath(call.FirstStringArg)
	target := "__route__ANY__" + path
	graph.nodes = append(graph.nodes, api.Node{Project: project, Label: "Route", Name: path,
		QualifiedName: target, Location: api.Location{File: file, StartLine: call.StartLine, EndLine: call.EndLine},
		Properties: api.Properties{"source": "redirect"}})
	graph.edges = append(graph.edges, pendingEdge{source: source, target: target, kind: "REDIRECTS_TO",
		properties: api.Properties{"callee": call.Name, "url_path": call.FirstStringArg},
		evidence:   syntaxEvidence(file, call, "redirect", 1)})
}

func isClientNavigationCall(callee string) bool {
	method := callee
	receiver := ""
	if dot := strings.LastIndexByte(callee, '.'); dot >= 0 {
		receiver, method = callee[:dot], callee[dot+1:]
	}
	switch method {
	case "navigate", "navigateByUrl", "navigateTo", "navigateForward", "navigateBack", "navigateRoot", "pushState", "replaceState":
		return true
	case "goto", "redirect", "permanentRedirect":
		return receiver == ""
	case "push", "replace", "prefetch":
		receiver = strings.ToLower(receiver)
		return strings.HasSuffix(receiver, "router") || strings.HasSuffix(receiver, "history") ||
			(method == "replace" && strings.HasSuffix(receiver, "location"))
	case "go", "assign":
		return strings.HasSuffix(strings.ToLower(receiver), "location")
	}
	return false
}

func pythonTestClientMethod(file, language, callee string) string {
	if language != "python" || !(strings.HasPrefix(file, "tests/") || strings.Contains(file, "/tests/") ||
		strings.HasPrefix(filepath.Base(file), "test_")) {
		return ""
	}
	method := serviceHTTPMethod(callee)
	if method == "" {
		return ""
	}
	receiver := strings.TrimSuffix(callee, "."+strings.ToLower(method))
	if receiver == callee {
		return ""
	}
	for _, allowed := range []string{"self.client", "self.rf", "self.request_factory", "self.factory", "cls.request_factory",
		"client", "rf", "factory", "request_factory", "RequestFactory()"} {
		if receiver == allowed {
			return method
		}
	}
	return ""
}

func isJSSupertestRequest(callee string, imports []SyntaxFact) bool {
	open := strings.IndexByte(callee, '(')
	if open <= 0 {
		return false
	}
	root := strings.TrimSpace(callee[:open])
	for _, imported := range imports {
		if imported.LocalName == root && (imported.Name == "supertest" || imported.Name == "superagent") {
			return true
		}
	}
	return false
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

func addRegisteredRoute(project string, graph *goGraph, source, file string, call SyntaxFact, routePath, method string) {
	target := "__route__" + method + "__" + canonicalRoutePath(routePath)
	graph.nodes = append(graph.nodes, api.Node{Project: project, Label: "Route", Name: routePath,
		QualifiedName: target, Location: api.Location{File: file, StartLine: call.StartLine, EndLine: call.EndLine},
		Properties: api.Properties{"method": method}})
	graph.edges = append(graph.edges, pendingEdge{source: source, target: target, kind: "CALLS",
		properties: api.Properties{"callee": call.Name, "url_path": routePath, "via": "route_registration"},
		evidence:   &api.Evidence{Strategy: "service_pattern", Confidence: .8}})
}

// A route's second callable argument is its handler. Resolve the same local,
// imported, or receiver-owned symbol used by the call graph before linking it.
func addRegisteredRouteHandler(graph *goGraph, registry symbolRegistry, parsed ParsedSyntaxFile, source string, call SyntaxFact, routePath, method string, imports map[string]string) {
	ref := ""
	for _, arg := range call.Arguments {
		if arg.Index > 0 && arg.Keyword == "" {
			candidate := strings.TrimSpace(arg.Expr)
			if routeHandlerName(candidate) {
				ref = candidate
			}
		}
	}
	if ref == "" {
		return
	}
	var resolution symbolResolution
	module := syntaxDefinitionModuleQN(parsed.File.Language, parsed.File.Path)
	if candidate := joinSyntaxScope(module, ref); candidate != "" {
		if node, ok := registry.exact[candidate]; ok && (node.Label == "Function" || node.Label == "Method") {
			resolution = symbolResolution{qn: candidate, strategy: "same_module", confidence: 1}
		}
	}
	if parsed.File.Language == "go" && strings.Contains(ref, ".") && strings.Contains(source, ".") {
		candidate := packagePrefix(source) + "." + ref[strings.LastIndexByte(ref, '.')+1:]
		if node, ok := registry.exact[candidate]; ok && (node.Label == "Method" || node.Label == "Function") {
			resolution = symbolResolution{qn: candidate, strategy: "receiver_method", confidence: 1}
		}
	}
	if resolution.qn == "" {
		session := newResolveSession(module, imports)
		resolution = registry.resolveWith(session, ref)
	}
	if resolution.qn == "" || guessStrategy(resolution.strategy) {
		return
	}
	target, ok := registry.exact[resolution.qn]
	if !ok || (target.Label != "Function" && target.Label != "Method") {
		return
	}
	route := "__route__" + method + "__" + canonicalRoutePath(routePath)
	graph.edges = append(graph.edges, pendingEdge{source: resolution.qn, target: route, kind: "HANDLES",
		properties: api.Properties{"handler": resolution.qn}, evidence: syntaxEvidence(parsed.File.Path, call, "route_handler", resolution.confidence)})
}

func routeHandlerName(value string) bool {
	for _, part := range strings.Split(value, ".") {
		if part == "" || !isRouteIdentifier(part[0]) || part[0] >= '0' && part[0] <= '9' {
			return false
		}
		for i := 1; i < len(part); i++ {
			if !isRouteIdentifier(part[i]) {
				return false
			}
		}
	}
	return true
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
	last := strings.LastIndexByte(value, '/')
	if last <= 0 {
		return false
	}
	backslashes := 0
	for i := last - 1; i > 0 && value[i] == '\\'; i-- {
		backslashes++
	}
	if backslashes%2 != 0 {
		return false
	}
	for _, flag := range value[last+1:] {
		if !strings.ContainsRune("dgimsuvy", flag) {
			return false
		}
	}
	return true
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
