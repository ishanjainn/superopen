package engine

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func putGraphSemantics(builder *Builder, graph goGraph, ids map[string]int64, project string, model *pretrainedVectors) error {
	type document struct {
		qn     string
		tokens []string
	}
	documents := make([]document, 0, len(graph.nodes))
	corpus := newSemanticCorpus()
	nodes := make(map[string]api.Node, len(graph.nodes))
	for _, node := range graph.nodes {
		nodes[node.QualifiedName] = node
	}
	outbound := make(map[string][]string)
	inbound := make(map[string][]string)
	for _, edge := range graph.edges {
		if edge.kind != "CALLS" {
			continue
		}
		if target, ok := nodes[edge.target]; ok {
			outbound[edge.source] = append(outbound[edge.source], target.Name)
		}
		if source, ok := nodes[edge.source]; ok {
			inbound[edge.target] = append(inbound[edge.target], source.Name)
		}
	}
	for key := range outbound {
		sort.Strings(outbound[key])
	}
	for key := range inbound {
		sort.Strings(inbound[key])
	}
	for _, node := range graph.nodes {
		if !semanticNodeLabel(node.Label) || semanticPropertyBool(node.Properties, "external") {
			continue
		}
		tokens := semanticNodeTokens(node, outbound[node.QualifiedName], inbound[node.QualifiedName])
		if len(tokens) == 0 {
			continue
		}
		corpus.Add(tokens)
		documents = append(documents, document{qn: node.QualifiedName, tokens: tokens})
	}
	corpus.Finalize(model)
	tokens := append([]string(nil), corpus.tokens...)
	sort.Strings(tokens)
	for _, token := range tokens {
		vector, ok := corpus.Vector(token)
		if !ok {
			continue
		}
		idf := corpus.IDF(token)
		if idf <= .01 {
			continue
		}
		if err := builder.PutSemanticToken(project, token, vector, idf); err != nil {
			return err
		}
	}
	features := make(map[string]semanticFeatures, len(documents))
	for _, document := range documents {
		var vector semanticVector
		weights := make(map[int]float32, len(document.tokens))
		for index, token := range document.tokens {
			tokenVector, ok := corpus.Vector(token)
			if !ok {
				tokenVector = semanticIndex(token, model)
			}
			weight := corpus.IDF(token)
			if weight <= 0 {
				continue
			}
			weights[index] = weight
			addScaledSemantic(&vector, &tokenVector, weight)
		}
		normalizeSemantic(&vector)
		node := nodes[document.qn]
		features[document.qn] = buildSemanticFeatures(node, vector, weights,
			outbound[document.qn], model)
		if semanticCosine(vector, vector) == 0 {
			continue
		}
		if err := builder.PutSemanticVector(ids[document.qn], project, vector); err != nil {
			return err
		}
	}
	semanticFeatureCache = features
	return emitSimilarityEdges(builder, graph.nodes, ids, project)
}

// semanticFeatureCache carries the per-node feature vectors from the semantic
// publish step to the SEMANTICALLY_RELATED pass, which Superopen runs as a
// post-pass over the same corpus rather than recomputing it.
var semanticFeatureCache map[string]semanticFeatures

func buildSemanticFeatures(node api.Node, ri semanticVector, tfidf map[int]float32,
	callees []string, model *pretrainedVectors) semanticFeatures {
	features := semanticFeatures{FilePath: node.Location.File, TFIDF: tfidf, RI: encodeRotSQ(ri[:])}
	var api semanticVector
	for _, callee := range callees {
		vector := semanticIndex(callee, model)
		addScaledSemantic(&api, &vector, 1)
	}
	normalizeSemantic(&api)
	features.API = encodeRotSQ(api[:])

	var types semanticVector
	if returnType := semanticPropertyString(node.Properties, "return_type"); returnType != "" {
		vector := semanticIndex(returnType, model)
		addScaledSemantic(&types, &vector, 1)
	}
	for _, paramType := range semanticPropertyStrings(node.Properties, "param_types") {
		vector := semanticIndex(paramType, model)
		addScaledSemantic(&types, &vector, 1)
	}
	normalizeSemantic(&types)
	features.Type = encodeRotSQ(types[:])

	var decorators semanticVector
	for _, decorator := range semanticPropertyStrings(node.Properties, "decorators") {
		vector := semanticIndex(decorator, model)
		addScaledSemantic(&decorators, &vector, 1)
	}
	normalizeSemantic(&decorators)
	features.Decorator = encodeRotSQ(decorators[:])

	if encoded := semanticPropertyString(node.Properties, "sp"); encoded != "" {
		if profile, ok := parseASTProfile(encoded); ok {
			features.Profile = profile.Vector()
		}
	}
	if fingerprint := semanticPropertyString(node.Properties, "fp"); fingerprint != "" {
		if parsed, err := parseMinHashHex(fingerprint); err == nil {
			features.MinHash, features.HasMinHash = parsed, true
		}
	}
	return features
}

func emitSimilarityEdges(builder *Builder, nodes []api.Node, ids map[string]int64, project string) error {
	entries := make([]lshEntry, 0)
	for _, node := range nodes {
		if !semanticNodeLabel(node.Label) {
			continue
		}
		raw := semanticPropertyString(node.Properties, "fp")
		if raw == "" {
			continue
		}
		fingerprint, err := parseMinHashHex(raw)
		if err != nil {
			continue
		}
		entries = append(entries, lshEntry{NodeID: ids[node.QualifiedName], Fingerprint: fingerprint, FilePath: node.Location.File, FileExtension: filepath.Ext(node.Location.File), QualifiedName: node.QualifiedName})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].QualifiedName != entries[j].QualifiedName {
			return entries[i].QualifiedName < entries[j].QualifiedName
		}
		return entries[i].NodeID < entries[j].NodeID
	})
	if len(entries) < 2 {
		return nil
	}
	index := newLSHIndex()
	for _, entry := range entries {
		index.Insert(entry)
	}
	for _, source := range entries {
		emitted := 0
		for _, candidate := range index.Candidates(source.Fingerprint, 4096) {
			if candidate.NodeID == source.NodeID || source.FileExtension != candidate.FileExtension || source.QualifiedName >= candidate.QualifiedName {
				continue
			}
			if emitted >= 10 {
				break
			}
			jaccard := minHashJaccard(source.Fingerprint, candidate.Fingerprint)
			if jaccard < .95 {
				continue
			}
			properties := api.Properties{"jaccard": roundThree(jaccard), "same_file": source.FilePath == candidate.FilePath}
			if _, err := builder.PutEdge(api.Edge{Project: project, SourceID: source.NodeID, TargetID: candidate.NodeID, Type: "SIMILAR_TO", Properties: properties}); err != nil {
				return err
			}
			emitted++
		}
	}
	return nil
}

func roundThree(value float64) float64 { return float64(int(value*1000+.5)) / 1000 }

func roundTwo(value float64) float64 { return float64(int(value*100+.5)) / 100 }

// semanticNodeTokens mirrors the pinned metadata/call-neighbor tokenizer. The
// order matters because Superopen caps each node at 512 tokens.
func semanticNodeTokens(node api.Node, callees, callers []string) []string {
	tokens := make([]string, 0, 64)
	appendText := func(text string) {
		if len(tokens) >= semanticTokenLimit || text == "" {
			return
		}
		tokens = append(tokens, semanticTokens(text, semanticTokenLimit-len(tokens))...)
	}
	appendText(node.Name)
	appendText(node.QualifiedName)
	appendText(node.Location.File)
	for _, key := range []string{"signature", "return_type", "docstring"} {
		appendText(semanticPropertyString(node.Properties, key))
	}
	for _, key := range []string{"param_names", "param_types", "decorators"} {
		for _, value := range semanticPropertyStrings(node.Properties, key) {
			appendText(value)
		}
	}
	bodyTokens := semanticPropertyString(node.Properties, "bt")
	if len(bodyTokens) > 511 {
		bodyTokens = bodyTokens[:511]
	}
	appendText(bodyTokens)
	for index, name := range callees {
		if index >= 64 {
			break
		}
		appendText(name)
	}
	for index, name := range callers {
		if index >= 64 {
			break
		}
		appendText(name)
	}
	return injectSemanticPatternTokens(tokens, node, callees)
}

func semanticPropertyString(properties api.Properties, key string) string {
	if properties == nil {
		return ""
	}
	value, ok := properties[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func semanticPropertyStrings(properties api.Properties, key string) []string {
	if properties == nil {
		return nil
	}
	switch values := properties[key].(type) {
	case []string:
		return values
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok {
				result = append(result, text)
			}
		}
		return result
	case string:
		return []string{values}
	default:
		return nil
	}
}

func injectSemanticPatternTokens(tokens []string, node api.Node, callees []string) []string {
	push := func(values ...string) {
		for _, value := range values {
			if len(tokens) >= semanticTokenLimit {
				return
			}
			tokens = append(tokens, value)
		}
	}
	containsAny := func(text string, needles ...string) bool {
		for _, needle := range needles {
			if strings.Contains(text, needle) {
				return true
			}
		}
		return false
	}
	body := semanticPropertyString(node.Properties, "bt")
	if containsAny(body, "except", "catch", "rescue") {
		push("error", "handling", "exception")
	}
	if containsAny(body, "raise", "throw") {
		push("error", "exception", "throw")
	}
	if containsAny(body, "logger", "logging", "log_") {
		push("logging", "log")
	}
	for _, callee := range callees {
		if containsAny(callee, "log", "Log", "warn", "debug", "info") {
			push("logging", "log")
		}
		if containsAny(callee, "Error", "error", "Errorf", "panic") {
			push("error", "handling")
		}
		if containsAny(callee, "open", "read", "write", "close", "Open", "Read") {
			push("io", "file")
		}
	}
	decorators := strings.Join(semanticPropertyStrings(node.Properties, "decorators"), " ")
	if containsAny(decorators, "route", "Route", "app.") {
		push("routing", "endpoint", "handler")
	}
	if containsAny(decorators, "middleware", "Middleware") {
		push("middleware")
	}
	if containsAny(decorators, "test", "Test", "pytest") {
		push("test", "testing")
	}
	if containsAny(node.Name, "test_", "Test") {
		push("test", "testing")
	}
	if containsAny(node.Name, "middleware", "Middleware") {
		push("middleware")
	}
	if containsAny(node.Name, "handler", "Handler") {
		push("handler")
	}
	if containsAny(node.Name, "validator", "Validator", "validate", "Validate") {
		push("validation")
	}
	return tokens
}

func semanticNodeLabel(label string) bool {
	switch label {
	case "Function", "Method":
		return true
	default:
		return false
	}
}

func semanticPropertyBool(properties api.Properties, key string) bool {
	if properties == nil {
		return false
	}
	value, _ := properties[key].(bool)
	return value
}
