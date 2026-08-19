package engine

import (
	"reflect"
	"testing"
)

func TestGoModRequirements(t *testing.T) {
	body := []byte(`module example.test/root

require example.test/direct v1.2.3
require (
	example.test/first v1.0.0
	example.test/second/v2 v2.0.0 // indirect
)
`)
	want := []string{"example.test/direct", "example.test/first", "example.test/second/v2"}
	if got := goModRequirements(body); !reflect.DeepEqual(got, want) {
		t.Fatalf("requirements=%v, want %v", got, want)
	}
}
