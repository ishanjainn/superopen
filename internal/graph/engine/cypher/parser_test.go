package cypher

import (
	"fmt"
	"strings"
	"testing"
)

func TestParsePatternWhereProjection(t *testing.T) {
	q, err := Parse(`MATCH (f:Function {name: "Foo"})-[r:CALLS|HTTP_CALLS*1..3]->(g:Function) WHERE (f.name CONTAINS "Fo" OR g.depth >= 2) AND r.confidence IS NOT NULL RETURN DISTINCT f.name AS name, COUNT(DISTINCT g) AS cnt ORDER BY cnt DESC, name ASC SKIP 1 LIMIT 5`)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Matches) != 1 || len(q.Matches[0].Patterns) != 1 {
		t.Fatalf("matches = %#v", q.Matches)
	}
	p := q.Matches[0].Patterns[0]
	if len(p.Nodes) != 2 || len(p.Relationships) != 1 || p.Relationships[0].Direction != Outbound {
		t.Fatalf("pattern = %#v", p)
	}
	r := p.Relationships[0]
	if !r.VariableLength || r.MinHops != 1 || r.MaxHops != 3 || len(r.Types) != 2 {
		t.Fatalf("relationship = %#v", r)
	}
	if q.Where == nil || q.Return == nil || len(q.Return.Items) != 2 || len(q.Return.OrderBy) != 2 || q.Return.Skip != 1 || q.Return.Limit != 5 {
		t.Fatalf("query = %#v", q)
	}
}

func TestParseOptionalMultiMatchWithUnionUnwind(t *testing.T) {
	q, err := Parse(`UNWIND ["a", "b"] AS wanted MATCH (f:Function) OPTIONAL MATCH (f)-[:CALLS]->(g) WITH DISTINCT f, COUNT(g) AS calls WHERE calls > 0 RETURN f.name UNION ALL MATCH (m:Method) RETURN m.name`)
	if err != nil {
		t.Fatal(err)
	}
	if q.Unwind == nil || len(q.Matches) != 2 || !q.Matches[1].Optional || q.With == nil || q.PostWhere == nil || q.Union == nil || !q.UnionAll {
		t.Fatalf("query = %#v", q)
	}
}

func TestParseDirectionsAndRanges(t *testing.T) {
	for query, direction := range map[string]Direction{
		`MATCH (a)-[:CALLS]-(b)`:  Any,
		`MATCH (a)-[:CALLS]->(b)`: Outbound,
		`MATCH (a)<-[:CALLS]-(b)`: Inbound,
	} {
		q, err := Parse(query)
		if err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		if got := q.Matches[0].Patterns[0].Relationships[0].Direction; got != direction {
			t.Errorf("%s direction = %v, want %v", query, got, direction)
		}
	}
	q, err := Parse(`MATCH (a)-[:CALLS*]->(b)`)
	if err != nil || q.Matches[0].Patterns[0].Relationships[0].MaxHops != 0 {
		t.Fatalf("unbounded range: %#v, %v", q, err)
	}
}

func TestParseBooleanPrecedence(t *testing.T) {
	q, err := Parse(`MATCH (n) WHERE NOT n.a = 1 OR n.b = 2 XOR n.c = 3 AND n.d IN [4, 5] RETURN CASE WHEN n.a <> 0 THEN toString(n.a) ELSE "none" END AS value`)
	if err != nil {
		t.Fatal(err)
	}
	root, ok := q.Where.(BinaryExpr)
	if !ok || root.Op != "OR" {
		t.Fatalf("root = %#v", q.Where)
	}
	if _, ok := q.Return.Items[0].Expression.(CaseExpr); !ok {
		t.Fatalf("return = %#v", q.Return.Items[0].Expression)
	}
	left, ok := root.Left.(UnaryExpr)
	if !ok {
		t.Fatalf("NOT expression = %#v", root.Left)
	}
	if comparison, ok := left.Value.(BinaryExpr); !ok || comparison.Op != "=" {
		t.Fatalf("NOT operand = %#v", left.Value)
	}
}

func TestParseLabelAlternationAndTest(t *testing.T) {
	q, err := Parse(`MATCH (n:Function|Method) WHERE NOT n:Class RETURN n.name`)
	if err != nil {
		t.Fatal(err)
	}
	labels := q.Matches[0].Patterns[0].Nodes[0].Labels
	if len(labels) != 2 || labels[0] != "Function" || labels[1] != "Method" {
		t.Fatalf("labels = %#v", labels)
	}
	unary, ok := q.Where.(UnaryExpr)
	if !ok {
		t.Fatalf("where = %#v", q.Where)
	}
	if _, ok := unary.Value.(LabelTest); !ok {
		t.Fatalf("label test = %#v", unary.Value)
	}
}

func TestParseExistsPattern(t *testing.T) {
	q, err := Parse(`MATCH (f:Function) WHERE NOT EXISTS { (f)<-[:CALLS]-() } RETURN f.name`)
	if err != nil {
		t.Fatal(err)
	}
	unary, ok := q.Where.(UnaryExpr)
	if !ok {
		t.Fatalf("where = %#v", q.Where)
	}
	if _, ok := unary.Value.(ExistsPattern); !ok {
		t.Fatalf("exists = %#v", unary.Value)
	}
}

func TestParseRejectsWritesTrailingInjectionAndComplexity(t *testing.T) {
	for _, query := range []string{
		`CREATE (n)`,
		`MATCH (n) RETURN n; DROP TABLE nodes`,
		`MATCH (n) RETURN n UNION SELECT sql FROM sqlite_master`,
		`MATCH (a)-[:CALLS*3..1]->(b)`,
	} {
		tokens, lexErr := Lex(query)
		if lexErr == nil {
			_ = RejectWrites(tokens)
		}
		if _, err := Parse(query); err == nil {
			t.Errorf("Parse(%q) unexpectedly succeeded", query)
		}
	}
	keys := make([]string, maxOrderKeys+1)
	for i := range keys {
		keys[i] = fmt.Sprintf("n.p%d", i)
	}
	if _, err := Parse(`MATCH (n) RETURN n ORDER BY ` + strings.Join(keys, ",")); err == nil {
		t.Fatal("too many ORDER BY keys unexpectedly succeeded")
	}
}

func TestRejectWritesStableDiagnostic(t *testing.T) {
	tokens, err := Lex(`MATCH (n) DETACH DELETE n`)
	if err != nil {
		t.Fatal(err)
	}
	if err := RejectWrites(tokens); err == nil || !strings.Contains(err.Error(), "DETACH") {
		t.Fatalf("RejectWrites = %v", err)
	}
}

func TestParseRejectsUnsupportedFunctionLoudly(t *testing.T) {
	_, err := Parse(`MATCH (f) WHERE split(f.name) = "x" RETURN f`)
	if err == nil || !strings.Contains(err.Error(), `unsupported function "split"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestParseCountStar(t *testing.T) {
	query, err := Parse(`MATCH (f:Function) RETURN COUNT(*) AS n`)
	if err != nil {
		t.Fatal(err)
	}
	call, ok := query.Return.Items[0].Expression.(CallExpr)
	if !ok || len(call.Args) != 1 || call.Args[0] != (Variable{Name: "*"}) {
		t.Fatalf("COUNT(*) = %#v", query.Return.Items[0].Expression)
	}
}
