package engine

import (
	"context"
	"strings"
)

// Reparse script content that markup grammars keep as opaque raw_text. The
// content is parsed separately, then its facts are placed back in host-file
// coordinates so definitions and relationships share the surrounding file.
func enrichEmbeddedScripts(ctx context.Context, parser SyntaxParser, host string, body []byte, result FileResult) FileResult {
	defaultLanguage := "javascript"
	switch host {
	case "html", "vue", "svelte":
	case "astro":
		defaultLanguage = "typescript"
	default:
		return result
	}
	root, err := parser.Parse(ctx, host, body)
	if err != nil {
		return result
	}
	lines := sourceLineIndex(body)
	stack := []syntaxView{root}
	blocks := 0
	for len(stack) > 0 && blocks < 16 {
		last := len(stack) - 1
		node := stack[last]
		stack = stack[:last]
		language := defaultLanguage
		contentKind := "raw_text"
		if host == "astro" && node.Kind() == "frontmatter" {
			contentKind = "frontmatter_js_block"
		} else if node.Kind() != "script_element" {
			stack = viewPushChildrenReversed(node, stack, 1024)
			continue
		}
		var content syntaxView
		viewEachChild(node, func(child syntaxView) {
			if child.Kind() == contentKind {
				content = child
			}
		})
		if viewMissing(content) {
			continue
		}
		blocks++
		if node.Kind() == "script_element" && !embeddedScriptLanguage(node, body, &language) {
			continue
		}
		start, end := content.StartByte(), content.EndByte()
		if end <= start || int(end) > len(body) {
			continue
		}
		facts, err := extractSyntaxFile(ctx, parser, language, language, body[start:end])
		if err != nil {
			continue
		}
		shiftEmbeddedFacts(&facts, start, lines)
		appendEmbeddedFacts(&result, facts)
	}
	sortSyntaxFacts(&result)
	return result
}

func embeddedScriptLanguage(script syntaxView, body []byte, language *string) bool {
	stack := []syntaxView{script}
	for len(stack) > 0 {
		last := len(stack) - 1
		node := stack[last]
		stack = stack[:last]
		if node.Kind() != "attribute" {
			stack = viewPushChildrenReversed(node, stack, 128)
			continue
		}
		name := ""
		value := ""
		viewEachChild(node, func(child syntaxView) {
			if !child.IsNamed() {
				return
			}
			if name == "" {
				name = strings.TrimSpace(nodeText(child, body))
			}
			if child.Kind() == "quoted_attribute_value" {
				viewEachChild(child, func(inner syntaxView) {
					if inner.Kind() == "attribute_value" {
						value = strings.TrimSpace(nodeText(inner, body))
					}
				})
			} else if child.Kind() == "attribute_value" {
				value = strings.TrimSpace(nodeText(child, body))
			}
		})
		name = strings.ToLower(name)
		value = strings.ToLower(value)
		switch name {
		case "src":
			return false
		case "lang":
			switch value {
			case "js", "javascript":
				*language = "javascript"
			case "ts", "typescript":
				*language = "typescript"
			default:
				return false
			}
		case "type":
			if value != "" && value != "module" && value != "text/javascript" && value != "application/javascript" {
				return false
			}
		}
	}
	return true
}

func shiftEmbeddedFacts(facts *FileResult, start uint32, lines []uint32) {
	shift := func(f *SyntaxFact) {
		f.StartByte += start
		f.EndByte += start
		f.StartLine, f.StartColumn = bytePosition(lines, f.StartByte)
		f.EndLine, f.EndColumn = bytePosition(lines, f.EndByte)
	}
	for _, group := range [][]SyntaxFact{facts.Definitions, facts.Imports, facts.Calls, facts.CallableReferences, facts.Throws, facts.Decorators, facts.Inheritance, facts.TypeReferences, facts.TypeAssignments, facts.Implementations, facts.Routes, facts.Channels, facts.EnvironmentAccesses, facts.ConfigurationRefs, facts.StringReferences, facts.KubernetesFacts, facts.Sections, facts.Branches} {
		for i := range group {
			shift(&group[i])
		}
	}
	for _, group := range [][]OccurrenceFact{facts.Usages, facts.Bindings, facts.Writes} {
		for i := range group {
			f := &group[i]
			f.StartByte += start
			f.EndByte += start
			sl, sc := bytePosition(lines, f.StartByte)
			el, ec := bytePosition(lines, f.EndByte)
			f.StartLine, f.StartColumn = int32(sl), int32(sc)
			f.EndLine, f.EndColumn = int32(el), int32(ec)
		}
	}
}

func appendEmbeddedFacts(dst *FileResult, src FileResult) {
	dst.Definitions = append(dst.Definitions, src.Definitions...)
	dst.Imports = append(dst.Imports, src.Imports...)
	dst.Calls = append(dst.Calls, src.Calls...)
	dst.Usages = append(dst.Usages, src.Usages...)
	dst.CallableReferences = append(dst.CallableReferences, src.CallableReferences...)
	dst.Bindings = append(dst.Bindings, src.Bindings...)
	dst.Writes = append(dst.Writes, src.Writes...)
	dst.Throws = append(dst.Throws, src.Throws...)
	dst.Decorators = append(dst.Decorators, src.Decorators...)
	dst.Inheritance = append(dst.Inheritance, src.Inheritance...)
	dst.TypeReferences = append(dst.TypeReferences, src.TypeReferences...)
	dst.TypeAssignments = append(dst.TypeAssignments, src.TypeAssignments...)
	dst.Implementations = append(dst.Implementations, src.Implementations...)
	dst.Routes = append(dst.Routes, src.Routes...)
	dst.Channels = append(dst.Channels, src.Channels...)
	dst.EnvironmentAccesses = append(dst.EnvironmentAccesses, src.EnvironmentAccesses...)
	dst.ConfigurationRefs = append(dst.ConfigurationRefs, src.ConfigurationRefs...)
	dst.StringReferences = append(dst.StringReferences, src.StringReferences...)
	dst.KubernetesFacts = append(dst.KubernetesFacts, src.KubernetesFacts...)
	dst.Sections = append(dst.Sections, src.Sections...)
	dst.Branches = append(dst.Branches, src.Branches...)
	dst.Partial = dst.Partial || src.Partial
}
