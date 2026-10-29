package engine

import (
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestRSTCodeResolutionUsesContextAndRejectsAmbiguousMembers(t *testing.T) {
	nodes := []api.Node{
		{Label: "Method", Name: "render", QualifiedName: "pkg.a.View.render", Location: api.Location{File: "pkg/a.py"}},
		{Label: "Method", Name: "render", QualifiedName: "pkg.b.View.render", Location: api.Location{File: "pkg/b.py"}},
		{Label: "Class", Name: "View", QualifiedName: "pkg.a.View", Location: api.Location{File: "pkg/a.py"}},
	}
	x := newRSTCodeIndex(nodes)
	if _, ok := x.resolve("render", "meth", "", "", false, false); ok {
		t.Fatal("bare member resolved without context")
	}
	if n, ok := x.resolve("render", "meth", "pkg.a", "View", true, false); !ok || n.QualifiedName != "pkg.a.View.render" {
		t.Fatalf("contextual method: %q, %v", n.QualifiedName, ok)
	}
	if n, ok := x.resolve("View", "class", "", "", false, false); !ok || n.QualifiedName != "pkg.a.View" {
		t.Fatalf("unique class: %q, %v", n.QualifiedName, ok)
	}
	if target, _, ok := rstTarget("label <~pkg.a.View.render()>"); !ok || target != "pkg.a.View.render" {
		t.Fatalf("role target: %q, %v", target, ok)
	}
}

func TestRSTDirectiveResolvesDottedMemberWithoutModuleContext(t *testing.T) {
	method := api.Node{Label: "Method", Name: "get_port", QualifiedName: "django.http.request.HttpRequest.get_port", Location: api.Location{File: "django/http/request.py", StartLine: 124}}
	graph := goGraph{nodes: []api.Node{method}}
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "docs/ref/request-response.txt", Language: "rst"}, Body: []byte(".. method:: HttpRequest.get_port()\n")}}
	indexRSTMentions(files, nil, &graph)
	if len(graph.edges) != 1 || graph.edges[0].target != method.QualifiedName {
		t.Fatalf("dotted method directive: %+v", graph.edges)
	}
}

func TestRSTPackageReexportResolvesDottedMemberInModuleContext(t *testing.T) {
	method := api.Node{Label: "Method", Name: "get_port", QualifiedName: "django.http.request.HttpRequest.get_port", Location: api.Location{File: "django/http/request.py", StartLine: 124}}
	graph := goGraph{nodes: []api.Node{method, {Label: "Module", Name: "http", QualifiedName: "django.http.__init__", Location: api.Location{File: "django/http/__init__.py"}}}}
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "docs/ref/request-response.txt", Language: "rst"}, Body: []byte(".. module:: django.http\n\n.. class:: HttpRequest\n\n.. method:: HttpRequest.get_port()\n:meth:`~django.http.HttpRequest.get_port()`\n")}}
	indexRSTMentions(files, map[string]map[string]string{"django/http/__init__.py": {"HttpRequest": "django.http.request.HttpRequest"}}, &graph)
	count := 0
	for _, edge := range graph.edges {
		if edge.target == method.QualifiedName {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("package re-export member: %+v", graph.edges)
	}
}

func TestRSTMentionsAttachToEnclosingSection(t *testing.T) {
	graph := goGraph{nodes: []api.Node{
		{Label: "Class", Name: "View", QualifiedName: "pkg.a.View", Location: api.Location{File: "pkg/a.py"}},
		{Label: "Section", Name: "API", QualifiedName: "doc.api.API", Location: api.Location{File: "doc/api.rst", StartLine: 1, EndLine: 5}},
	}}
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "doc/api.rst", Language: "rst"}, Body: []byte("API\n===\n\n.. currentmodule:: pkg.a\n:class:`View`\n")}}
	indexRSTMentions(files, nil, &graph)
	if len(graph.edges) != 1 || graph.edges[0].source != "doc.api.API" || graph.edges[0].target != "pkg.a.View" {
		t.Fatalf("mentions: %+v", graph.edges)
	}
}

func TestRSTLiteralIncludeBindsRequestedPythonDefinition(t *testing.T) {
	graph := goGraph{nodes: []api.Node{
		{Label: "File", Name: "example.py", QualifiedName: fileQualifiedName("doc/example.py"), Location: api.Location{File: "doc/example.py"}},
		{Label: "Function", Name: "setup", QualifiedName: "doc.example.setup", Location: api.Location{File: "doc/example.py", StartLine: 5, EndLine: 9}},
	}}
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "doc/api.rst", Language: "rst"}, Body: []byte(".. literalinclude:: example.py\n   :pyobject: setup\n")}}
	indexRSTMentions(files, nil, &graph)
	if len(graph.edges) != 1 || graph.edges[0].target != "doc.example.setup" || graph.edges[0].properties["syntax"] != "literalinclude" {
		t.Fatalf("literalinclude edges: %+v", graph.edges)
	}
}

func TestRSTPackageExportResolvesToDefinition(t *testing.T) {
	graph := goGraph{nodes: []api.Node{{Label: "Class", Name: "Thing", QualifiedName: "pkg.impl.Thing", Location: api.Location{File: "pkg/impl.py", StartLine: 7, EndLine: 12}}}}
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "doc/api.rst", Language: "rst"}, Body: []byte(":class:`pkg.Thing`\n")}}
	indexRSTMentions(files, map[string]map[string]string{"pkg/__init__.py": {"Thing": "pkg.impl.Thing"}}, &graph)
	if len(graph.edges) != 1 || graph.edges[0].target != "pkg.impl.Thing" {
		t.Fatalf("package export: %+v", graph.edges)
	}
}

func TestRSTCodeNameRequiresUniqueQualifiedSuffix(t *testing.T) {
	x := newRSTCodeIndex([]api.Node{
		{Label: "Method", Name: "run", QualifiedName: "pkg.one.Task.run", Location: api.Location{File: "pkg/one.py"}},
		{Label: "Method", Name: "run", QualifiedName: "pkg.two.Task.run", Location: api.Location{File: "pkg/two.py"}},
	})
	if _, ok := x.codeName("Task.run"); ok {
		t.Fatal("ambiguous suffix resolved")
	}
	if n, ok := x.codeName("one.Task.run"); !ok || n.QualifiedName != "pkg.one.Task.run" {
		t.Fatalf("qualified suffix: %q, %v", n.QualifiedName, ok)
	}
}

func TestRSTMethodDirectiveUsesSignatureName(t *testing.T) {
	graph := goGraph{nodes: []api.Node{{Label: "Module", Name: "a", QualifiedName: "pkg.a", Location: api.Location{File: "pkg/a.py", StartLine: 1}}, {Label: "Method", Name: "render", QualifiedName: "pkg.a.View.render", Location: api.Location{File: "pkg/a.py", StartLine: 8}}}}
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "doc/api.rst", Language: "rst"}, Body: []byte(".. currentmodule:: pkg.a\n\n.. method:: render(name, options=None)\n")}}
	indexRSTMentions(files, nil, &graph)
	if len(graph.edges) != 1 || graph.edges[0].target != "pkg.a.View.render" {
		t.Fatalf("method signature: %+v", graph.edges)
	}
}

func TestRSTClassScopeEndsAtDirectiveIndent(t *testing.T) {
	graph := goGraph{nodes: []api.Node{
		{Label: "Class", Name: "View", QualifiedName: "pkg.View", Location: api.Location{File: "pkg.py"}},
		{Label: "Method", Name: "run", QualifiedName: "pkg.View.run", Location: api.Location{File: "pkg.py"}},
		{Label: "Method", Name: "run", QualifiedName: "other.View.run", Location: api.Location{File: "other.py"}},
	}}
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "doc/api.rst", Language: "rst"}, Body: []byte(".. module:: pkg\n\n.. class:: View\n\n   :meth:`run`\n\n:meth:`run`\n")}}
	indexRSTMentions(files, nil, &graph)
	count := 0
	for _, edge := range graph.edges {
		if edge.kind == "MENTIONS" && edge.properties["syntax"] == "role" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("class scope leaked: %+v", graph.edges)
	}
}

func TestRSTMemberDirectiveRestoresEnclosingClassForSibling(t *testing.T) {
	graph := goGraph{nodes: []api.Node{
		{Label: "Class", Name: "Widget", QualifiedName: "pkg.widgets.Widget", Location: api.Location{File: "pkg/widgets.py"}},
		{Label: "Variable", Name: "attrs", QualifiedName: "pkg.widgets.Widget.attrs", Location: api.Location{File: "pkg/widgets.py", StartLine: 4}},
		{Label: "Method", Name: "format_value", QualifiedName: "pkg.widgets.Widget.format_value", Location: api.Location{File: "pkg/widgets.py", StartLine: 8}},
		{Label: "Method", Name: "format_value", QualifiedName: "pkg.widgets.Other.format_value", Location: api.Location{File: "pkg/widgets.py", StartLine: 20}},
	}}
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "doc/widgets.rst", Language: "rst"}, Body: []byte(".. module:: pkg.widgets\n\n.. class:: Widget\n\n    .. attribute:: Widget.attrs\n\n        Description.\n\n    .. method:: format_value(value)\n")}}
	indexRSTMentions(files, nil, &graph)
	if !hasPendingEdge(graph.edges, fileQualifiedName("doc/widgets.rst"), "MENTIONS", "pkg.widgets.Widget.format_value") {
		t.Fatalf("sibling method lost enclosing class: %+v", graph.edges)
	}
}

func TestRSTAdjacentWrappedRolesResolveBothTargets(t *testing.T) {
	graph := goGraph{nodes: []api.Node{
		{Label: "Method", Name: "raw", QualifiedName: "pkg.QuerySet.raw", Location: api.Location{File: "pkg.py", StartLine: 4}},
		{Label: "Method", Name: "extra", QualifiedName: "pkg.QuerySet.extra", Location: api.Location{File: "pkg.py", StartLine: 8}},
	}}
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "doc/api.rst", Language: "rst"}, Body: []byte("See :meth:`raw()\n<pkg.QuerySet.raw>` and :meth:`extra()\n<pkg.QuerySet.extra>`.\n")}}
	indexRSTMentions(files, nil, &graph)
	if !hasPendingEdge(graph.edges, fileQualifiedName("doc/api.rst"), "MENTIONS", "pkg.QuerySet.raw") ||
		!hasPendingEdge(graph.edges, fileQualifiedName("doc/api.rst"), "MENTIONS", "pkg.QuerySet.extra") {
		t.Fatalf("adjacent wrapped roles: %+v", graph.edges)
	}
}

func TestRSTPackageModuleTargetsFolder(t *testing.T) {
	folder := api.Node{Label: "Folder", Name: "middleware", QualifiedName: "pkg.middleware", Location: api.Location{File: "pkg/middleware"}}
	graph := goGraph{nodes: []api.Node{folder}}
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "doc/api.rst", Language: "rst"}, Body: []byte(".. module:: pkg.middleware\n\nSee :mod:`pkg.middleware`.\n")}}
	indexRSTMentions(files, nil, &graph)
	if !hasPendingEdge(graph.edges, fileQualifiedName("doc/api.rst"), "MENTIONS", folder.QualifiedName) {
		t.Fatalf("package folder link missing: %+v", graph.edges)
	}
}

func TestRSTNonemptyPackageTargetsModule(t *testing.T) {
	folder := api.Node{Label: "Folder", Name: "models", QualifiedName: "pkg.models", Location: api.Location{File: "pkg/models"}}
	module := api.Node{Label: "Module", Name: "__init__.py", QualifiedName: "pkg.models.__init__", Location: api.Location{File: "pkg/models/__init__.py"}}
	graph := goGraph{nodes: []api.Node{folder, module}}
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "doc/api.rst", Language: "rst"}, Body: []byte(".. module:: pkg.models\n\nSee :mod:`pkg.models`.\n")}}
	indexRSTMentions(files, nil, &graph)
	if !hasPendingEdge(graph.edges, fileQualifiedName("doc/api.rst"), "MENTIONS", module.QualifiedName) {
		t.Fatalf("package module link missing: %+v", graph.edges)
	}
}

func TestRSTShortModuleRolePrefersExactFolderPath(t *testing.T) {
	folder := api.Node{Label: "Folder", Name: "utils", QualifiedName: "utils", Location: api.Location{File: "utils"}}
	module := api.Node{Label: "Module", Name: "utils.py", QualifiedName: "tests.utils", Location: api.Location{File: "tests/utils.py"}}
	graph := goGraph{nodes: []api.Node{folder, module}}
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "doc/api.rst", Language: "rst"}, Body: []byte(":mod:`utils`\n")}}
	indexRSTMentions(files, nil, &graph)
	if !hasPendingEdge(graph.edges, fileQualifiedName("doc/api.rst"), "MENTIONS", folder.QualifiedName) {
		t.Fatalf("exact folder role: %+v", graph.edges)
	}
}

func TestRSTInlineEmptyPackageTargetsInitializerFile(t *testing.T) {
	file := api.Node{Label: "File", Name: "__init__.py", QualifiedName: fileQualifiedName("pkg/empty/__init__.py"), Location: api.Location{File: "pkg/empty/__init__.py"}}
	graph := goGraph{nodes: []api.Node{file}}
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "doc/api.rst", Language: "rst"}, Body: []byte("Use ``pkg.empty`` here.\n")}}
	indexRSTMentions(files, nil, &graph)
	if !hasPendingEdge(graph.edges, fileQualifiedName("doc/api.rst"), "MENTIONS", file.QualifiedName) {
		t.Fatalf("empty package literal: %+v", graph.edges)
	}
}

func TestRSTInlineInstanceMemberUsesUniqueTypeMethod(t *testing.T) {
	graph := goGraph{nodes: []api.Node{
		{Label: "Method", Name: "render", QualifiedName: "pkg.NodeList.render", Location: api.Location{File: "pkg.py", StartLine: 8}},
		{Label: "Method", Name: "render", QualifiedName: "pkg.Other.render", Location: api.Location{File: "pkg.py", StartLine: 20}},
	}}
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "doc/api.rst", Language: "rst"}, Body: []byte("Call ``self.nodelist.render(context)``.\n")}}
	indexRSTMentions(files, nil, &graph)
	if !hasPendingEdge(graph.edges, fileQualifiedName("doc/api.rst"), "MENTIONS", "pkg.NodeList.render") {
		t.Fatalf("instance member link: %+v", graph.edges)
	}
}

func TestRSTInlinePathUsesDocumentThenRepositoryRoot(t *testing.T) {
	byPath := map[string]api.Node{
		"docs/guide.txt": {Label: "File", QualifiedName: "docs.guide.txt.__file__", Location: api.Location{File: "docs/guide.txt"}},
		"pkg/data":       {Label: "Folder", QualifiedName: "pkg.data", Location: api.Location{File: "pkg/data"}},
	}
	if n, ok := rstPathTarget("README.rst", "docs/guide.txt", byPath); !ok || n.Label != "File" {
		t.Fatalf("root path: %q, %v", n.QualifiedName, ok)
	}
	if n, ok := rstPathTarget("docs/api.rst", "../pkg/data", byPath); !ok || n.Label != "Folder" {
		t.Fatalf("relative folder: %q, %v", n.QualifiedName, ok)
	}
}

func TestRSTInlineCodeNamesReachPackageAndClass(t *testing.T) {
	graph := goGraph{nodes: []api.Node{
		{Label: "Variable", Name: "BuildEnvironment", QualifiedName: "sphinx.environment.__init__.BuildEnvironment", Location: api.Location{File: "sphinx/environment/__init__.py", StartLine: 15}},
		{Label: "Class", Name: "BuildEnvironment", QualifiedName: "sphinx.environment.__init__.BuildEnvironment", Location: api.Location{File: "sphinx/environment/__init__.py", StartLine: 85}},
		{Label: "Class", Name: "PygmentsBridge", QualifiedName: "sphinx.highlighting.PygmentsBridge", Location: api.Location{File: "sphinx/highlighting.py", StartLine: 71}},
	}}
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "doc/api.rst", Language: "rst"}, Body: []byte("``sphinx.environment.BuildEnvironment``\n``sphinx.highlighting.PygmentsBridge``\n")}}
	indexRSTMentions(files, nil, &graph)
	if len(graph.edges) != 2 {
		t.Fatalf("inline code names: %+v", graph.edges)
	}
}

func TestRSTClassConstructorDoesNotCollideWithClassName(t *testing.T) {
	nodes := []api.Node{
		{Label: "Class", Name: "PygmentsBridge", QualifiedName: "sphinx.highlighting.PygmentsBridge", Location: api.Location{File: "sphinx/highlighting.py", StartLine: 71}},
		{Label: "Method", Name: "__init__", QualifiedName: "sphinx.highlighting.PygmentsBridge.__init__", Location: api.Location{File: "sphinx/highlighting.py", StartLine: 77}},
	}
	x := newRSTCodeIndex(nodes)
	if n, ok := x.codeName("sphinx.highlighting.PygmentsBridge"); !ok || n.Label != "Class" {
		t.Fatalf("class resolved ambiguously: %q, %v", n.Label, ok)
	}
}

func TestRSTExactDottedRoleUsesExactCodeEvenWithWrongRoleKind(t *testing.T) {
	x := newRSTCodeIndex([]api.Node{{Label: "Method", Name: "resolve_xref", QualifiedName: "sphinx.domains.__init__.Domain.resolve_xref", Location: api.Location{File: "sphinx/domains/__init__.py", StartLine: 319}}})
	if n, ok := x.resolve("sphinx.domains.Domain.resolve_xref", "class", "", "", false, false); !ok || n.Label != "Method" {
		t.Fatalf("exact role: %q, %v", n.Label, ok)
	}
}

func TestRSTAutodocModuleCanResolveBesideDocument(t *testing.T) {
	module := api.Node{Label: "Module", Name: "dummy.py", QualifiedName: "tests.roots.fixture.dummy", Location: api.Location{File: "tests/roots/fixture/dummy.py", StartLine: 1}}
	graph := goGraph{nodes: []api.Node{module}}
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "tests/roots/fixture/index.rst", Language: "rst"}, Body: []byte(".. automodule:: dummy\n")}}
	indexRSTMentions(files, nil, &graph)
	if len(graph.edges) != 1 || graph.edges[0].target != module.QualifiedName {
		t.Fatalf("nearby autodoc module: %+v", graph.edges)
	}
}

func TestRSTModuleResolutionWalksDocumentationAncestors(t *testing.T) {
	module := api.Node{Label: "Module", Name: "bar", QualifiedName: "tests.roots.fixture.dir.bar", Location: api.Location{File: "tests/roots/fixture/dir/bar.py", StartLine: 1}}
	graph := goGraph{nodes: []api.Node{module}}
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "tests/roots/fixture/dir/inner.rst", Language: "rst"}, Body: []byte(".. automodule:: dir.bar\n")}}
	indexRSTMentions(files, nil, &graph)
	if len(graph.edges) != 1 || graph.edges[0].target != module.QualifiedName {
		t.Fatalf("ancestor module: %+v", graph.edges)
	}
	graph.edges = nil
	graph.edgeSeen = nil
	files[0].Body = []byte(".. py:module:: dir.bar\n")
	indexRSTMentions(files, nil, &graph)
	if len(graph.edges) != 1 || graph.edges[0].target != module.QualifiedName {
		t.Fatalf("module directive: %+v", graph.edges)
	}
}

func TestRSTCodeNameAcceptsUniqueCaseVariant(t *testing.T) {
	x := newRSTCodeIndex([]api.Node{{Label: "Method", Name: "note", QualifiedName: "sphinx.environment.adapters.toctree.TocTree.note", Location: api.Location{File: "sphinx/environment/adapters/toctree.py", StartLine: 34}}})
	if n, ok := x.codeName("Toctree.note"); !ok || n.Name != "note" {
		t.Fatalf("case variant: %q, %v", n.QualifiedName, ok)
	}
}

func TestRSTCodeNameOmitsPackageInitFromSuffix(t *testing.T) {
	x := newRSTCodeIndex([]api.Node{{Label: "Method", Name: "get_doc", QualifiedName: "sphinx.ext.autodoc.__init__.ClassDocumenter.get_doc", Location: api.Location{File: "sphinx/ext/autodoc/__init__.py", StartLine: 1681}}})
	if n, ok := x.codeName("autodoc.ClassDocumenter.get_doc"); !ok || n.Name != "get_doc" {
		t.Fatalf("package method: %q, %v", n.QualifiedName, ok)
	}
}

func TestRSTFixtureDirectiveResolvesUniqueNearbyCode(t *testing.T) {
	class := api.Node{Label: "Class", Name: "Class1", QualifiedName: "tests.roots.fixture.spam.mod1.Class1", Location: api.Location{File: "tests/roots/fixture/spam/mod1.py", StartLine: 19}}
	graph := goGraph{nodes: []api.Node{class}}
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "tests/roots/fixture/index.rst", Language: "rst"}, Body: []byte(".. autofunction:: Class1\n")}}
	indexRSTMentions(files, nil, &graph)
	if len(graph.edges) != 1 || graph.edges[0].target != class.QualifiedName {
		t.Fatalf("fixture code: %+v", graph.edges)
	}
}

func TestRSTWrappedRoleUsesExplicitDottedTarget(t *testing.T) {
	class := api.Node{Label: "Class", Name: "LaTeXBuilder", QualifiedName: "sphinx.builders.latex.__init__.LaTeXBuilder", Location: api.Location{File: "sphinx/builders/latex/__init__.py", StartLine: 108}}
	graph := goGraph{nodes: []api.Node{class}}
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "doc/usage/quickstart.rst", Language: "rst"}, Body: []byte("See :mod:`LaTeX builder\n   <sphinx.builders.latex.LaTeXBuilder>` for PDFs.\n")}}
	indexRSTMentions(files, nil, &graph)
	if len(graph.edges) != 1 || graph.edges[0].target != class.QualifiedName {
		t.Fatalf("wrapped role: %+v", graph.edges)
	}
}

func TestRSTInlineModuleNameTargetsSourceFile(t *testing.T) {
	file := api.Node{Label: "File", Name: "utils.py", QualifiedName: fileQualifiedName("pkg/utils.py"), Location: api.Location{File: "pkg/utils.py"}}
	module := api.Node{Label: "Module", Name: "utils", QualifiedName: "pkg.utils", Location: api.Location{File: "pkg/utils.py"}}
	graph := goGraph{nodes: []api.Node{file, module}}
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "doc/api.rst", Language: "rst"}, Body: []byte("``pkg.utils``\n")}}
	indexRSTMentions(files, nil, &graph)
	if len(graph.edges) != 1 || graph.edges[0].target != file.QualifiedName {
		t.Fatalf("inline module: %+v", graph.edges)
	}
}

func TestRSTInlineModuleSuffixTargetsUniquePackageFile(t *testing.T) {
	file := api.Node{Label: "File", Name: "__init__.py", QualifiedName: fileQualifiedName("django/contrib/auth/__init__.py"), Location: api.Location{File: "django/contrib/auth/__init__.py"}}
	module := api.Node{Label: "Module", Name: "__init__.py", QualifiedName: "django.contrib.auth.__init__", Location: api.Location{File: file.Location.File}}
	graph := goGraph{nodes: []api.Node{file, module}}
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "docs/releases/1.11.txt", Language: "rst"}, Body: []byte("Use ``contrib.auth`` here.\n")}}
	indexRSTMentions(files, nil, &graph)
	if len(graph.edges) != 1 || graph.edges[0].target != file.QualifiedName {
		t.Fatalf("package suffix: %+v", graph.edges)
	}
}

func TestRSTRepeatedHeadingUsesPublishedSectionLocation(t *testing.T) {
	sectionQN := "doc.api.Fields"
	graph := goGraph{nodes: []api.Node{
		{Label: "Class", Name: "Thing", QualifiedName: "pkg.Thing", Location: api.Location{File: "pkg.py", StartLine: 1}},
		{Label: "Section", Name: "Fields", QualifiedName: sectionQN, Location: api.Location{File: "doc/api.rst", StartLine: 5, EndLine: 8}},
		{Label: "Section", Name: "Fields", QualifiedName: sectionQN, Location: api.Location{File: "doc/api.rst", StartLine: 30, EndLine: 40}},
	}}
	content := make([]string, 42)
	content[6] = ":class:`pkg.Thing`"
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "doc/api.rst", Language: "rst"}, Body: []byte(strings.Join(content, "\n"))}}
	indexRSTMentions(files, nil, &graph)
	if len(graph.edges) != 1 || graph.edges[0].source != fileQualifiedName("doc/api.rst") {
		t.Fatalf("repeated heading link source: %+v", graph.edges)
	}
}

func TestRSTReferenceUsesPrecedingUniqueHeading(t *testing.T) {
	graph := goGraph{nodes: []api.Node{
		{Label: "Class", Name: "Thing", QualifiedName: "pkg.Thing", Location: api.Location{File: "pkg.py", StartLine: 1}},
		{Label: "Section", Name: "Topic", QualifiedName: "doc.api.Topic", Location: api.Location{File: "doc/api.rst", StartLine: 2, EndLine: 3}},
	}}
	content := []byte("header\nTopic\n=====\n\n\n:class:`pkg.Thing`\n")
	indexRSTMentions([]ParsedSyntaxFile{{File: FileRecord{Path: "doc/api.rst", Language: "rst"}, Body: content}}, nil, &graph)
	if len(graph.edges) != 1 || graph.edges[0].source != "doc.api.Topic" {
		t.Fatalf("preceding heading: %+v", graph.edges)
	}
}

func TestRSTDirectiveDoesNotBorrowMemberFromPackageChild(t *testing.T) {
	nodes := []api.Node{
		{Label: "Module", Name: "pkg/__init__.py", QualifiedName: "pkg.__init__", Location: api.Location{File: "pkg/__init__.py"}},
		{Label: "Method", Name: "groups", QualifiedName: "pkg.models.Other.groups", Location: api.Location{File: "pkg/models.py"}},
	}
	x := newRSTCodeIndex(nodes)
	if _, ok := x.resolve("groups", "attribute", "pkg", "models.User", false, true); ok {
		t.Fatal("borrowed unrelated package child member")
	}
}

func TestRSTDirectiveFindsDocumentedClassInsidePackage(t *testing.T) {
	nodes := []api.Node{
		{Label: "Module", Name: "gdal", QualifiedName: "django.contrib.gis.gdal.__init__", Location: api.Location{File: "django/contrib/gis/gdal/__init__.py"}},
		{Label: "Class", Name: "Layer", QualifiedName: "django.contrib.gis.gdal.layer.Layer", Location: api.Location{File: "django/contrib/gis/gdal/layer.py"}},
		{Label: "Method", Name: "extent", QualifiedName: "django.contrib.gis.gdal.layer.Layer.extent", Location: api.Location{File: "django/contrib/gis/gdal/layer.py"}},
	}
	x := newRSTCodeIndex(nodes)
	if n, ok := x.resolve("Layer", "class", "django.contrib.gis.gdal", "Layer", false, true); !ok || n.Label != "Class" {
		t.Fatalf("package class: %q, %v", n.QualifiedName, ok)
	}
	if n, ok := x.resolve("extent", "attribute", "django.contrib.gis.gdal", "Layer", false, true); !ok || n.Name != "extent" {
		t.Fatalf("class member: %q, %v", n.QualifiedName, ok)
	}
}

func TestRSTSourceRoleAndPackageFolder(t *testing.T) {
	folder := api.Node{Label: "Folder", Name: "middleware", QualifiedName: "django.middleware", Location: api.Location{File: "django/middleware"}}
	file := api.Node{Label: "File", Name: "models.py", QualifiedName: fileQualifiedName("django/contrib/flatpages/models.py"), Location: api.Location{File: "django/contrib/flatpages/models.py"}}
	graph := goGraph{nodes: []api.Node{folder, file}}
	content := []byte(".. module:: django.middleware\n:mod:`django.middleware`\n:source:`flatpage model <django/contrib/flatpages/models.py>`\n")
	indexRSTMentions([]ParsedSyntaxFile{{File: FileRecord{Path: "docs/ref/api.rst", Language: "rst"}, Body: content}}, nil, &graph)
	if !hasPendingEdge(graph.edges, fileQualifiedName("docs/ref/api.rst"), "MENTIONS", folder.QualifiedName) ||
		!hasPendingEdge(graph.edges, fileQualifiedName("docs/ref/api.rst"), "MENTIONS", file.QualifiedName) {
		t.Fatalf("folder and source role: %+v", graph.edges)
	}
}

func TestRSTMemberDirectiveProvidesClassContextForNestedRole(t *testing.T) {
	method := api.Node{Label: "Method", Name: "add_view", QualifiedName: "django.contrib.admin.options.ModelAdmin.add_view", Location: api.Location{File: "django/contrib/admin/options.py"}}
	graph := goGraph{nodes: []api.Node{method}}
	content := []byte(".. module:: django.contrib.admin\n\n.. attribute:: ModelAdmin.add_form_template\n\n    Use :meth:`add_view` for the add form.\n")
	indexRSTMentions([]ParsedSyntaxFile{{File: FileRecord{Path: "docs/ref/admin.rst", Language: "rst"}, Body: content}}, nil, &graph)
	if !hasPendingEdge(graph.edges, fileQualifiedName("docs/ref/admin.rst"), "MENTIONS", method.QualifiedName) {
		t.Fatalf("member role context: %+v", graph.edges)
	}
}

func TestRSTUniquePackageClassmethodDirective(t *testing.T) {
	method := api.Node{Label: "Method", Name: "from_queryset", QualifiedName: "django.db.models.manager.BaseManager.from_queryset", Location: api.Location{File: "django/db/models/manager.py"}}
	x := newRSTCodeIndex([]api.Node{method})
	if got, ok := x.resolve("from_queryset", "classmethod", "django.db.models", "", false, true); !ok || got.QualifiedName != method.QualifiedName {
		t.Fatalf("package classmethod: %q, %v", got.QualifiedName, ok)
	}
}
