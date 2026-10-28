package engine

import (
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestSymbolRegistryResolutionOrder(t *testing.T) {
	registry := newSymbolRegistry([]api.Node{
		{Label: "Function", Name: "Run", QualifiedName: "pkg.worker.Run"},
		{Label: "Function", Name: "Run", QualifiedName: "other.Run"},
		{Label: "Function", Name: "Close", QualifiedName: "pkg.Close"},
	})
	if got := registry.resolve("worker.Run", "caller", map[string]string{"worker": "pkg.worker"}); got.qn != "pkg.worker.Run" || got.strategy != "import_map" {
		t.Fatalf("import resolution=%+v", got)
	}
	if got := registry.resolve("Close", "pkg", nil); got.qn != "pkg.Close" || got.strategy != "same_module" {
		t.Fatalf("same module resolution=%+v", got)
	}
}

func TestRootPackageFunctionBeatsSameNamedField(t *testing.T) {
	registry := newSymbolRegistry([]api.Node{
		{Label: "Function", Name: "chain", QualifiedName: "chain"},
		{Label: "Field", Name: "chain", QualifiedName: "ChainHandler.chain"},
	})
	if got := registry.resolve("chain", "", nil); got.qn != "chain" || got.strategy != "same_module" {
		t.Fatalf("root package call resolved to %+v", got)
	}
}

func TestImportedPackageResolvesInitModuleMember(t *testing.T) {
	registry := newSymbolRegistry([]api.Node{
		{Label: "Function", Name: "gettext", QualifiedName: "pkg.translation.__init__.gettext"},
		{Label: "Function", Name: "gettext", QualifiedName: "pkg.translation.other.gettext"},
	})
	got := registry.resolve("translation.gettext", "caller", map[string]string{"translation": "pkg.translation"})
	if got.qn != "pkg.translation.__init__.gettext" || got.strategy != "import_map" {
		t.Fatalf("package member resolution=%+v", got)
	}
}

func TestFieldHintDoesNotRouteProductionCallToTestFixture(t *testing.T) {
	registry := newSymbolRegistry([]api.Node{
		{Label: "Function", Name: "run", QualifiedName: "pkg.run", Location: api.Location{File: "pkg/run.py"}},
		{Label: "Method", Name: "write", QualifiedName: "tests.fixture.File.write", Location: api.Location{File: "tests/fixture.py"}},
	})
	before := symbolResolution{qn: "other.write", strategy: "suffix_match", confidence: .2, candidates: 2}
	for _, source := range []struct{ qn, file string }{{"pkg.run", "pkg/run.py"}, {"pkg", "pkg/__init__.py"}} {
		if got := registry.applyFieldTypeHint("file.write", source.qn, source.file, before); got.qn != before.qn || got.strategy != before.strategy {
			t.Fatalf("test fixture selected for production call from %s: %+v", source.file, got)
		}
	}
}

func TestSymbolRegistryGuessesSkipImpossibleTargets(t *testing.T) {
	registry := newSymbolRegistry([]api.Node{
		{Label: "Method", Name: "t", QualifiedName: "utils.dateformat.DateFormat.t", Location: api.Location{File: "utils/dateformat.py"}},
		{Label: "Method", Name: "db_for_read", QualifiedName: "tests.routers.OtherRouter.db_for_read", Location: api.Location{File: "tests/routers.py"}},
		{Label: "Function", Name: "get_runner", QualifiedName: "pkg.test.utils.get_runner", Location: api.Location{File: "pkg/test/utils.py"}},
	})
	python := func(production bool) *resolveSession {
		session := newResolveSession("pkg.sql.query", map[string]string{"get_runner": "pkg.test.utils"})
		session.receiverMethods = true
		session.sourceIsProduction = production
		return session
	}
	// A bare name in Python is a local or a function, never a method.
	if got := registry.resolveWith(python(true), "t"); got.qn != "" {
		t.Fatalf("bare name resolved to a method: %+v", got)
	}
	// Production code does not reach tests by a name guess...
	if got := registry.resolveWith(python(true), "router.db_for_read"); got.qn != "" {
		t.Fatalf("production guess resolved into tests: %+v", got)
	}
	// ...but a test file may, and an explicit import always does.
	if got := registry.resolveWith(python(false), "router.db_for_read"); got.qn != "tests.routers.OtherRouter.db_for_read" {
		t.Fatalf("test source lost its guess: %+v", got)
	}
	if got := registry.resolveWith(python(true), "get_runner"); got.qn != "pkg.test.utils.get_runner" || got.strategy != "import_map" {
		t.Fatalf("explicit import into a test path: %+v", got)
	}
	// Languages that call own methods by bare name keep that resolution.
	java := newResolveSession("pkg", nil)
	if got := registry.resolveWith(java, "t"); got.qn != "utils.dateformat.DateFormat.t" {
		t.Fatalf("java bare method call lost: %+v", got)
	}
}

func TestReceiverGuessIsWeak(t *testing.T) {
	registry := newSymbolRegistry([]api.Node{
		{Label: "Class", Name: "MySQLOperations", QualifiedName: "gis.ops.MySQLOperations"},
		{Label: "Method", Name: "select", QualifiedName: "gis.ops.MySQLOperations.select"},
		{Label: "Class", Name: "QuerySet", QualifiedName: "models.query.QuerySet"},
		{Label: "Method", Name: "order_by", QualifiedName: "models.query.QuerySet.order_by"},
		{Label: "Class", Name: "SQLCompiler", QualifiedName: "sql.compiler.SQLCompiler"},
		{Label: "Method", Name: "execute_sql", QualifiedName: "sql.compiler.SQLCompiler.execute_sql"},
		{Label: "Class", Name: "ChoicesMeta", QualifiedName: "enums.ChoicesMeta"},
		{Label: "Method", Name: "values", QualifiedName: "enums.ChoicesMeta.values"},
		{Label: "Function", Name: "get_converters", QualifiedName: "urls.converters.get_converters"},
	})
	guess := func(qn string) symbolResolution {
		return symbolResolution{qn: qn, strategy: "unique_name", confidence: .75, candidates: 1}
	}
	cases := []struct {
		callee, target string
		call, weak     bool
	}{
		// Reading self.query.order_by is Query's attribute, not QuerySet.order_by.
		{"self.query.order_by", "models.query.QuerySet.order_by", false, true},
		// An accessor name alone does not prove the receiver's type.
		{"inner.select", "gis.ops.MySQLOperations.select", false, true},
		// A call on another receiver keeps its resolver confidence...
		{"compiler.execute_sql", "sql.compiler.SQLCompiler.execute_sql", true, false},
		// ...unless the name is a builtin type's method or a module function.
		{"mask.values", "enums.ChoicesMeta.values", true, true},
		{"compiler.get_converters", "urls.converters.get_converters", true, true},
	}
	for _, c := range cases {
		if got := registry.receiverGuessIsWeak("python", c.callee, nil, guess(c.target), c.call); got != c.weak {
			t.Fatalf("%s -> %s call=%v: weak=%v want %v", c.callee, c.target, c.call, got, c.weak)
		}
	}
	if registry.receiverGuessIsWeak("python", "enums.values", map[string]string{"enums": "enums"}, guess("enums.ChoicesMeta.values"), true) {
		t.Fatal("an imported receiver is not an untyped one")
	}
	if registry.receiverGuessIsWeak("python", "inner.select", nil, symbolResolution{qn: "models.query.QuerySet.order_by", strategy: "import_map"}, false) {
		t.Fatal("an import resolution is not a guess")
	}
	if registry.receiverGuessIsWeak("java", "self.query.order_by", nil, guess("models.query.QuerySet.order_by"), false) {
		t.Fatal("the rule is Python's")
	}
}

func TestSymbolRegistryJavaBareMethod(t *testing.T) {
	registry := newSymbolRegistry([]api.Node{
		{Label: "Method", Name: "t", QualifiedName: "utils.dateformat.DateFormat.t", Location: api.Location{File: "utils/dateformat.py"}},
	})
	java := newResolveSession("pkg", nil)
	if got := registry.resolveWith(java, "t"); got.qn != "utils.dateformat.DateFormat.t" {
		t.Fatalf("java bare method call lost: %+v", got)
	}
}

func TestPythonImportTargetsFollowQualifiedNames(t *testing.T) {
	nodes := []api.Node{
		{Label: "Folder", Name: "db", QualifiedName: "django.db", Location: api.Location{File: "django/db"}},
		{Label: "Module", Name: "__init__", QualifiedName: "django.db.__init__", Location: api.Location{File: "django/db/__init__.py"}},
		{Label: "Variable", Name: "connections", QualifiedName: "django.db.__init__.connections", Location: api.Location{File: "django/db/__init__.py"}},
		{Label: "Method", Name: "db", QualifiedName: "django.db.models.manager.BaseManager.db", Location: api.Location{File: "django/db/models/manager.py"}},
		{Label: "Module", Name: "constants", QualifiedName: "django.db.models.sql.constants", Location: api.Location{File: "django/db/models/sql/constants.py"}},
		{Label: "Variable", Name: "SINGLE", QualifiedName: "django.db.models.sql.constants.SINGLE", Location: api.Location{File: "django/db/models/sql/constants.py"}},
		{Label: "Module", Name: "collections", QualifiedName: "django.contrib.gis.geos.collections", Location: api.Location{File: "django/contrib/gis/geos/collections.py"}},
		{Label: "Module", Name: "__init__", QualifiedName: "django.contrib.gis.geos.__init__", Location: api.Location{File: "django/contrib/gis/geos/__init__.py"}},
		{Label: "Module", Name: "util", QualifiedName: "src.pkg.util", Location: api.Location{File: "src/pkg/util.py"}},
		{Label: "Module", Name: "__init__", QualifiedName: "src.pkg.__init__", Location: api.Location{File: "src/pkg/__init__.py"}},
		{Label: "Function", Name: "helper", QualifiedName: "src.pkg.util.helper", Location: api.Location{File: "src/pkg/util.py"}},
	}
	index := newImportTargetIndex(nodes)
	cases := map[string]string{
		"django.db.connections":                 "django.db.__init__.connections",
		"django.db.models.sql.constants.SINGLE": "django.db.models.sql.constants.SINGLE",
		".constants.SINGLE":                     "django.db.models.sql.constants.SINGLE",
		"..sql.constants":                       "django.db.models.sql.constants",
		"collections.Counter":                   "",
		"itertools.count":                       "",
		"pkg.util.helper":                       "src.pkg.util.helper",
	}
	for imported, want := range cases {
		if got := localSyntaxImportTargetForLanguage("python", "django/db/models/sql/query.py", imported, "", index); got != want {
			t.Fatalf("%s: got %q want %q", imported, got, want)
		}
	}
	registry := newSymbolRegistry([]api.Node{
		{Label: "Class", Name: "Q", QualifiedName: "django.db.models.query_utils.Q"},
		{Label: "Class", Name: "Q", QualifiedName: "other.Q"},
	})
	if got := registry.resolve("Q", "app.views", map[string]string{"Q": "django.db.models"}); got.qn != "django.db.models.query_utils.Q" || got.strategy != "import_map_suffix" {
		t.Fatalf("re-exported name: %+v", got)
	}
}

func TestOwnMemberWalksTheClassHierarchy(t *testing.T) {
	registry := newSymbolRegistry([]api.Node{
		{Label: "Class", Name: "Query", QualifiedName: "sql.query.Query", Properties: api.Properties{"base_classes": []string{"BaseExpression"}}},
		{Label: "Method", Name: "clone", QualifiedName: "sql.query.Query.clone"},
		{Label: "Method", Name: "get_count", QualifiedName: "sql.query.Query.get_count"},
		{Label: "Class", Name: "RawQuery", QualifiedName: "sql.query.RawQuery"},
		{Label: "Method", Name: "clone", QualifiedName: "sql.query.RawQuery.clone"},
		{Label: "Class", Name: "BaseExpression", QualifiedName: "expressions.BaseExpression", Properties: api.Properties{"base_classes": []any{"Combinable"}}},
		{Label: "Class", Name: "Combinable", QualifiedName: "expressions.Combinable"},
		{Label: "Method", Name: "bitand", QualifiedName: "expressions.Combinable.bitand"},
		{Label: "Class", Name: "AppConfig", QualifiedName: "apps.config.AppConfig"},
		{Label: "Method", Name: "create", QualifiedName: "apps.config.AppConfig.create"},
	})
	src := "sql.query.Query.get_count"
	cases := map[string]string{
		"self.clone":  "sql.query.Query.clone",
		"cls.bitand":  "expressions.Combinable.bitand",
		"self.create": "",
	}
	for callee, want := range cases {
		got, self := registry.resolveOwnMember("python", callee, src)
		if !self || got.qn != want {
			t.Fatalf("%s: got %+v self=%v, want %q", callee, got, self, want)
		}
	}
	if got, _ := registry.resolveOwnMember("typescript", "this.clone", src); got.qn != "sql.query.Query.clone" || got.strategy != "own_member" {
		t.Fatalf("this.clone: %+v", got)
	}
	// A closure inside the method still belongs to the class.
	if got, _ := registry.resolveOwnMember("python", "self.clone", src+".inner"); got.qn != "sql.query.Query.clone" {
		t.Fatalf("closure: %+v", got)
	}
	for _, callee := range []string{"other.clone", "self.a.clone"} {
		if got, self := registry.resolveOwnMember("python", callee, src); got.qn != "" || self {
			t.Fatalf("%s resolved to %+v", callee, got)
		}
	}
}

func TestExternalImportNamesAreNotGuessed(t *testing.T) {
	registry := newSymbolRegistry([]api.Node{
		{Label: "Method", Name: "splitext", QualifiedName: "templates.TemplateCommand.splitext", Location: api.Location{File: "core/templates.py"}},
		{Label: "Function", Name: "join", QualifiedName: "pkg.text.join", Location: api.Location{File: "pkg/text.py"}},
	})
	session := newResolveSession("pkg.cmd", nil)
	session.external = map[string]bool{"os": true, "join": true}
	for _, callee := range []string{"os.path.splitext", "join"} {
		if got := registry.resolveWith(session, callee); got.qn != "" {
			t.Fatalf("%s from a stdlib import resolved to %+v", callee, got)
		}
	}
	if got := registry.resolveWith(newResolveSession("pkg.cmd", nil), "join"); got.qn != "pkg.text.join" {
		t.Fatalf("an unbound name still resolves: %+v", got)
	}
}

func TestSuperAndModuleLevelReceiverRules(t *testing.T) {
	registry := newSymbolRegistry([]api.Node{
		{Label: "Class", Name: "Lookup", QualifiedName: "lookups.Lookup"},
		{Label: "Method", Name: "process_lhs", QualifiedName: "lookups.Lookup.process_lhs"},
		{Label: "Class", Name: "SearchLookup", QualifiedName: "search.SearchLookup", Properties: api.Properties{"base_classes": []string{"Lookup"}}},
		{Label: "Method", Name: "process_lhs", QualifiedName: "search.SearchLookup.process_lhs"},
		{Label: "Class", Name: "RangeContainedBy", QualifiedName: "ranges.RangeContainedBy", Properties: api.Properties{"base_classes": []string{"lookups.Lookup"}}},
		{Label: "Method", Name: "process_lhs", QualifiedName: "ranges.RangeContainedBy.process_lhs"},
		{Label: "Variable", Name: "connection", QualifiedName: "db.__init__.connection"},
		{Label: "Module", Name: "__init__", QualifiedName: "db.__init__"},
	})
	got, self := registry.resolveOwnMember("python", "super().process_lhs", "ranges.RangeContainedBy.process_lhs")
	if !self || got.qn != "lookups.Lookup.process_lhs" || got.strategy != "inherited_member" {
		t.Fatalf("super() must skip the class and follow its bases: %+v", got)
	}
	guess := symbolResolution{qn: "db.__init__.connection", strategy: "suffix_match", confidence: .55}
	if !registry.receiverGuessIsWeak("python", "schema_editor.connection", nil, guess, false) {
		t.Fatal("an attribute of an object is not a module-level variable")
	}
	if registry.receiverGuessIsWeak("python", "db.connection", map[string]string{"db": "db"}, guess, false) {
		t.Fatal("an imported module's attribute is")
	}
}
