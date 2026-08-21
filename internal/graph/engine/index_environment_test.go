package engine

import (
	"reflect"
	"testing"
)

func TestFindEnvironmentAccesses(t *testing.T) {
	body := []byte(`package fixture
func load() {
	os.Getenv("HOME")
	os.LookupEnv("CODEX_HOME")
}`)
	gotFacts := findEnvironmentAccesses("go", body)
	got := make([]string, len(gotFacts))
	for index := range gotFacts {
		got[index] = gotFacts[index].name
		if gotFacts[index].owner != "load" {
			t.Fatalf("owner=%q, want load", gotFacts[index].owner)
		}
	}
	want := []string{"HOME", "CODEX_HOME"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("environment accesses=%v, want %v", got, want)
	}
}

func TestFindEnvironmentAccessesDoesNotParseEmbeddedOtherLanguages(t *testing.T) {
	body := []byte("const fixture = `os.getenv('TOKEN')`")
	if got := findEnvironmentAccesses("go", body); len(got) != 0 {
		t.Fatalf("embedded Python was treated as Go environment access: %v", got)
	}
}

func TestEnvironmentAccessesFromCalls(t *testing.T) {
	parsed := ParsedSyntaxFile{
		File: FileRecord{Language: "go", Path: "main.go"},
		Extraction: FileResult{Calls: []SyntaxFact{
			{Name: "os.Getenv", FirstStringArg: "HOME", StartByte: 10, Scope: "load"},
			{Name: "os.LookupEnv", FirstStringArg: "CODEX_HOME", StartByte: 40, Scope: "load"},
			{Name: "fmt.Println", FirstStringArg: "HOME", StartByte: 80, Scope: "load"},
		}},
	}
	got := environmentAccessesFromCalls(parsed)
	if len(got) != 2 || got[0].name != "HOME" || got[1].name != "CODEX_HOME" || got[0].owner != "load" {
		t.Fatalf("got=%#v", got)
	}
}
