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
