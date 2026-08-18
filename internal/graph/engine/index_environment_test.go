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
