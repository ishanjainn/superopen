package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestRouteRegistrationLinksNamedHandler(t *testing.T) {
	for _, tc := range []struct{ file, body, handler, route string }{
		{"app.js", "const express = require('express');\nconst app = express();\nfunction show(req, res) {}\napp.get('/users', show);\n", "app.show", "__route__GET__/users"},
		{"app.go", "package app\nfunc show() {}\nfunc routes(r Router) { r.Get(\"/users\", show) }\n", "show", "__route__GET__/users"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, tc.file), []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{tc.file}, nil, 1)
			if err != nil {
				t.Fatal(err)
			}
			graph, _ := AssembleSyntaxGraph(repository, "fixture")
			indexHTTPRoutes("fixture", repository.Files, &graph)
			if !hasNode(graph.nodes, tc.route) || !hasPendingEdge(graph.edges, tc.handler, "HANDLES", tc.route) {
				t.Fatalf("route handler missing: calls=%#v edges=%#v", repository.Files[0].Extraction.Calls, graph.edges)
			}
		})
	}
}

func TestMultilineSupertestCallRoutesAsHTTPClient(t *testing.T) {
	root := t.TempDir()
	code := "const request = require('supertest');\nrequest(app)\n  .get('/users/1');\n"
	if err := os.WriteFile(filepath.Join(root, "client.js"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"client.js"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	indexHTTPRoutes("fixture", repository.Files, &graph)
	if !hasPendingEdge(graph.edges, "client", "HTTP_CALLS", "__route__GET__/users/1") {
		t.Fatalf("missing HTTP client route: calls=%#v", repository.Files[0].Extraction.Calls)
	}
}

func TestUnresolvedTestClientCallStillRoutesLiteralURL(t *testing.T) {
	root := t.TempDir()
	code := "class ViewTests:\n    def test_head(self):\n        self.rf.head('/about/')\n        self.client.get('/sitemap.xml')\n        self.assertEqual('/unrelated/', 'value')\n"
	if err := os.WriteFile(filepath.Join(root, "test_views.py"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"test_views.py"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	indexHTTPRoutes("fixture", repository.Files, &graph)
	if !hasPendingEdge(graph.edges, "test_views.ViewTests.test_head", "HTTP_CALLS", "__route__HEAD__/about/") {
		t.Fatalf("missing literal route from unresolved client call: calls=%#v", repository.Files[0].Extraction.Calls)
	}
	if !hasPendingEdge(graph.edges, "test_views.ViewTests.test_head", "HTTP_CALLS", "__route__GET__/sitemap.xml") {
		t.Fatal("an explicit test client request to an XML endpoint must be routed")
	}
	if hasNode(graph.nodes, "__route__ANY__/unrelated/") {
		t.Fatal("an assertion argument must not create a route")
	}
}

func TestExpressRegexRegistrationLinksLastNamedHandler(t *testing.T) {
	root := t.TempDir()
	code := "const express = require('express');\nconst request = require('supertest');\nconst app = express();\nconst router = express.Router();\nfunction middleware(req, res, next) { next(); }\nfunction show(req, res) { res.end('ok'); }\napp.get(/^\\/api\\/users\\/\\d+$/, middleware, show);\napp.use(/\\/admin.*/, show);\napp.get('/*path', show);\napp.get('/*splat', show);\nrouter.get('/thing' + i, show);\nrequest(app).get('/?q=' + encodeURIComponent(inputUrl));\nrequest(app).get('/' + dest + '/todo.txt');\ncache.get(/\\/not-a-route/, show);\n"
	if err := os.WriteFile(filepath.Join(root, "app.js"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"app.js"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	indexHTTPRoutes("fixture", repository.Files, &graph)
	getRoute := "__route__GET__/^\\/api\\/users\\/\\d+$/"
	useRoute := "__route__ANY__/\\/admin.*/"
	if !hasPendingEdge(graph.edges, "app.show", "HANDLES", getRoute) ||
		!hasPendingEdge(graph.edges, "app.show", "HANDLES", useRoute) {
		t.Fatalf("regex registrations did not reach the handler: calls=%#v edges=%#v", repository.Files[0].Extraction.Calls, graph.edges)
	}
	for _, route := range []string{"__route__GET__/*path", "__route__GET__/*splat", "__route__GET__/thing{}", "__route__GET__/?q={}", "__route__GET__/{}/todo.txt"} {
		if !hasNode(graph.nodes, route) {
			t.Errorf("missing Express route or client path %s", route)
		}
	}
	if hasNode(graph.nodes, "__route__GET__/\\/not-a-route/") {
		t.Fatal("unrelated regex call became a route")
	}
	for _, node := range graph.nodes {
		if node.QualifiedName == getRoute && (node.Location.File != "app.js" || node.Location.StartLine != 7) {
			t.Fatalf("registration location missing: %+v", node.Location)
		}
	}
}

func TestExpressAllRouteAndRedirectTargets(t *testing.T) {
	root := t.TempDir()
	code := "const express = require('express');\nconst app = express();\nfunction show(req, res) { res.redirect('/user/0'); res.redirect('/pet/' + req.params.id); }\napp.all('/user/:id{/:op}', show);\n"
	if err := os.WriteFile(filepath.Join(root, "app.js"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"app.js"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	indexHTTPRoutes("fixture", repository.Files, &graph)
	if !hasPendingEdge(graph.edges, "app.show", "HANDLES", "__route__ANY__/user/{}{}/{}}") {
		t.Fatalf("app.all route is missing: calls=%#v", repository.Files[0].Extraction.Calls)
	}
	for _, route := range []string{"__route__ANY__/user/0", "__route__ANY__/pet/{}"} {
		if !hasPendingEdge(graph.edges, "app.show", "REDIRECTS_TO", route) ||
			hasPendingEdge(graph.edges, "app.show", "HTTP_CALLS", route) {
			t.Errorf("redirect target has wrong relationship: %s", route)
		}
	}
}

func TestExpressRouteCanBeQueriedReadAndTraced(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	code := "function show(req, res) { res.end('user'); }\napp.get(/^\\/user\\/[0-9]+$/, show);\n"
	if err := os.WriteFile(filepath.Join(root, "app.js"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenWritable(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	qn := "__route__GET__/^\\/user\\/[0-9]+$/"
	err = store.Build(ctx, func(b *Builder) error {
		if err := b.PutProject(ProjectRecord{Name: "fixture", RootPath: root, Generation: "one", EngineVersion: "test", IndexedAt: time.Now().UTC()}); err != nil {
			return err
		}
		source, err := b.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "show", QualifiedName: "app.show", Location: api.Location{File: "app.js", StartLine: 1, EndLine: 1}})
		if err != nil {
			return err
		}
		target, err := b.PutNode(api.Node{Project: "fixture", Label: "Route", Name: "/^\\/user\\/[0-9]+$/", QualifiedName: qn, Location: api.Location{File: "app.js", StartLine: 2, EndLine: 2}})
		if err != nil {
			return err
		}
		_, err = b.PutEdge(api.Edge{Project: "fixture", SourceID: source, TargetID: target, Type: "HANDLES"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	snippet, err := store.Snippet(ctx, api.SnippetRequest{Project: "fixture", QualifiedName: qn})
	if err != nil || !strings.Contains(snippet.Code, "app.get(") || snippet.Callers != 1 {
		t.Fatalf("route snippet: %+v, %v", snippet, err)
	}
	trace, err := store.Trace(ctx, api.TraceRequest{Project: "fixture", Start: qn, Direction: "incoming"})
	if err != nil || len(trace.Paths) != 1 || trace.Paths[0][1].Node.QualifiedName != "app.show" {
		t.Fatalf("route trace: %+v, %v", trace, err)
	}
	query, err := store.Query(ctx, api.QueryRequest{Project: "fixture", Question: "regex user route handler"})
	if err != nil || len(query.Rows) == 0 || query.Rows[0].QualifiedName != qn || len(query.Bodies) == 0 || !strings.Contains(query.Bodies[0].Code, "app.get(") {
		t.Fatalf("route query: %+v, %v", query, err)
	}
}
