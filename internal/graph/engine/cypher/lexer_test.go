package cypher

import (
	"strings"
	"testing"
)

func TestLexObservableDialect(t *testing.T) {
	tokens, err := Lex(`MATCH (f:Function)-[r:CALLS|HTTP_CALLS*1..3]->(g {name: "x\\n"}) WHERE f.name <> 'bad' AND g.depth >= 2 RETURN DISTINCT f.name ORDER BY f.name DESC SKIP 1 LIMIT 5`)
	if err != nil {
		t.Fatal(err)
	}
	want := []Kind{Match, LParen, Identifier, Colon, Identifier, RParen, Dash, LBracket,
		Identifier, Colon, Identifier, Pipe, Identifier, Star, Number, DotDot, Number,
		RBracket, Dash, GT, LParen, Identifier, LBrace, Identifier, Colon, String, RBrace,
		RParen, Where, Identifier, Dot, Identifier, NotEqual, String, And, Identifier, Dot,
		Identifier, GTE, Number, Return, Distinct, Identifier, Dot, Identifier, Order, By,
		Identifier, Dot, Identifier, Desc, Skip, Number, Limit, Number, EOF}
	if len(tokens) != len(want) {
		t.Fatalf("got %d tokens, want %d: %#v", len(tokens), len(want), tokens)
	}
	for i := range want {
		if tokens[i].Kind != want[i] {
			t.Fatalf("token %d = %v (%q), want %v", i, tokens[i].Kind, tokens[i].Text, want[i])
		}
	}
}

func TestLexCaseInsensitiveKeywordsCommentsAndParameters(t *testing.T) {
	tokens, err := Lex("match (n) // comment\n WHERE n.name = $name /* ok */ RETURN toLower(n.name)")
	if err != nil {
		t.Fatal(err)
	}
	if tokens[0].Kind != Match || tokens[4].Kind != Where {
		t.Fatalf("keywords not recognized: %#v", tokens)
	}
	var parameter, lower bool
	for _, token := range tokens {
		parameter = parameter || token.Kind == Parameter && token.Text == "name"
		lower = lower || token.Kind == ToLower
	}
	if !parameter || !lower {
		t.Fatalf("missing parameter or function: %#v", tokens)
	}
}

func TestLexRejectsMalformedAndOversizedInput(t *testing.T) {
	for _, query := range []string{"MATCH (n) /*", `RETURN "unterminated`, "MATCH (n)\x00 RETURN n", "RETURN $"} {
		if _, err := Lex(query); err == nil {
			t.Errorf("Lex(%q) unexpectedly succeeded", query)
		}
	}
	tokens, err := Lex(`RETURN "` + strings.Repeat("x", maxTokenBytes+100) + `"`)
	if err != nil || len(tokens[1].Text) != maxTokenBytes {
		t.Fatalf("oversized string did not truncate safely: len=%d err=%v", len(tokens[1].Text), err)
	}
}

func TestLexRecognizesWriteKeywordsForLoudRejection(t *testing.T) {
	for query, want := range map[string]Kind{"CREATE": Create, "DELETE": Delete, "DETACH": Detach, "SET": Set, "MERGE": Merge, "CALL": Call} {
		tokens, err := Lex(query)
		if err != nil {
			t.Fatal(err)
		}
		if tokens[0].Kind != want {
			t.Errorf("%s = %v, want %v", query, tokens[0].Kind, want)
		}
	}
}
