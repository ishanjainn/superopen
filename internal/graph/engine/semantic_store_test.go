package engine

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestSimilarityPublicationUsesPinnedThresholdPropertiesAndDirection(t *testing.T) {
	ctx := context.Background()
	store, err := OpenWritable(filepath.Join(t.TempDir(), "similarity.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	fingerprint := strings.Repeat("00000000", minHashSize)
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{Name: "fixture", RootPath: t.TempDir(), Generation: "one", EngineVersion: "test"}); err != nil {
			return err
		}
		ids := map[string]int64{}
		nodes := []api.Node{
			{Project: "fixture", Label: "Function", Name: "A", QualifiedName: "fixture.A", Location: api.Location{File: "a.go"}, Properties: api.Properties{"fp": fingerprint}},
			{Project: "fixture", Label: "Function", Name: "B", QualifiedName: "fixture.B", Location: api.Location{File: "b.go"}, Properties: api.Properties{"fp": fingerprint}},
		}
		for _, node := range nodes {
			id, err := builder.PutNode(node)
			if err != nil {
				return err
			}
			ids[node.QualifiedName] = id
		}
		return emitSimilarityEdges(builder, nodes, ids, "fixture")
	})
	if err != nil {
		t.Fatal(err)
	}
	var edgeType, properties string
	if err := store.db.QueryRowContext(ctx, `SELECT type,properties FROM edges`).Scan(&edgeType, &properties); err != nil {
		t.Fatal(err)
	}
	if edgeType != "SIMILAR_TO" || properties != `{"jaccard":1,"same_file":false}` {
		t.Fatalf("unexpected similarity edge %s %s", edgeType, properties)
	}
}

func TestSemanticVectorStoreAndRanking(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "graph.db")
	store, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	var first, second int64
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{Name: "fixture", RootPath: t.TempDir(), Generation: "one", EngineVersion: "test", IndexedAt: time.Now()}); err != nil {
			return err
		}
		first, err = builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "alpha", QualifiedName: "fixture.alpha"})
		if err != nil {
			return err
		}
		second, err = builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "beta", QualifiedName: "fixture.beta"})
		if err != nil {
			return err
		}
		var alpha, beta semanticVector
		alpha[0], beta[1] = 1, 1
		if err := builder.PutSemanticVector(first, "fixture", alpha); err != nil {
			return err
		}
		if err := builder.PutSemanticVector(second, "fixture", beta); err != nil {
			return err
		}
		return builder.PutSemanticToken("fixture", "alpha", alpha, 1)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var query semanticVector
	query[0] = 1
	matches, err := store.SemanticSearch(ctx, "fixture", query, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].ID != first || matches[0].Signals["semantic"] == nil {
		t.Fatalf("semantic matches = %#v", matches)
	}
	matches, err = store.SemanticSearchTerms(ctx, "fixture", []string{"alpha"}, 1)
	if err != nil || len(matches) != 1 || matches[0].ID != first {
		t.Fatalf("semantic term matches = %#v, %v", matches, err)
	}
	var secondQuery semanticVector
	secondQuery[1] = 1
	matches, err = store.SemanticSearchKeywords(ctx, "fixture", []semanticVector{query, secondQuery}, 2)
	if err != nil || len(matches) != 2 || matches[0].Score > .2 {
		t.Fatalf("multi-keyword minimum ranking = %#v, %v", matches, err)
	}
	search, err := store.Search(ctx, api.SearchRequest{Project: "fixture", SemanticQuery: []string{"alpha"}, Limit: 5})
	if err != nil || len(search.Matches) != 0 || len(search.Semantic) != 2 || search.Semantic[0].ID != first {
		t.Fatalf("semantic-only search = %#v, %v", search, err)
	}
	search, err = store.Search(ctx, api.SearchRequest{Project: "fixture", Query: "alpha", SemanticQuery: []string{"alpha"}, Limit: 5})
	if err != nil || len(search.Matches) != 1 || len(search.Semantic) != 2 {
		t.Fatalf("combined lexical/semantic sections = %#v, %v", search, err)
	}
}

func TestPutSemanticVectorRequiresMatchingNode(t *testing.T) {
	t.Parallel()
	store, err := OpenWritable(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err = store.Build(context.Background(), func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{Name: "fixture", RootPath: t.TempDir(), Generation: "one", EngineVersion: "test"}); err != nil {
			return err
		}
		return builder.PutSemanticVector(999, "fixture", semanticVector{})
	})
	if err == nil {
		t.Fatal("missing semantic node was accepted")
	}
}

func TestSemanticVectorForTextIsNormalized(t *testing.T) {
	t.Parallel()
	vector := semanticVectorForText("HTTPServer request", nil, nil)
	if cosine := semanticCosine(vector, vector); cosine < .999 || cosine > 1.001 {
		t.Fatalf("self cosine = %f", cosine)
	}
}
