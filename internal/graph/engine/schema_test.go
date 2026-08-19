package engine

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestSchemaReportsCountsPropertiesAndPatternsDeterministically(t *testing.T) {
	ctx := context.Background()
	store, err := OpenWritable(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{Name: "fixture", RootPath: t.TempDir(), Generation: "one", EngineVersion: "test", IndexedAt: time.Now()}); err != nil {
			return err
		}
		file, err := builder.PutNode(api.Node{Project: "fixture", Label: "File", Name: "main.go", QualifiedName: "fixture.__file__", Properties: api.Properties{"language": "go"}})
		if err != nil {
			return err
		}
		function, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "Run", QualifiedName: "fixture.Run", Properties: api.Properties{"language": "go", "signature": "func Run()"}})
		if err != nil {
			return err
		}
		_, err = builder.PutEdge(api.Edge{Project: "fixture", SourceID: file, TargetID: function, Type: "DEFINES", Properties: api.Properties{"confidence": 1.0}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	got, err := store.Schema(ctx, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if got.NodeCount != 2 || got.EdgeCount != 1 || len(got.NodeLabels) != 2 || len(got.EdgeTypes) != 1 || len(got.Patterns) != 1 {
		t.Fatalf("unexpected schema: %+v", got)
	}
	if got.EdgeTypes[0].Name != "DEFINES" || len(got.EdgeTypes[0].Properties) != 1 || got.EdgeTypes[0].Properties[0] != "confidence" {
		t.Fatalf("edge properties missing: %+v", got.EdgeTypes)
	}
	if got.Patterns[0] != (api.SchemaPattern{Source: "File", Edge: "DEFINES", Target: "Function", Count: 1}) {
		t.Fatalf("unexpected pattern: %+v", got.Patterns[0])
	}
}
