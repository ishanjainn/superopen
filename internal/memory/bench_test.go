package memory

import (
	"fmt"
	"testing"

	"github.com/ishanjainn/superopen/internal/paths"
)

func benchStore(b *testing.B, n int) *Store {
	b.Helper()
	root := b.TempDir()
	if err := paths.Resolve(root).EnsureDirs(); err != nil {
		b.Fatal(err)
	}
	store, err := OpenRoot(root)
	if err != nil {
		b.Fatal(err)
	}
	types := []string{
		ObservationDecision, ObservationBugfix, ObservationFeature,
		ObservationRefactor, ObservationDiscovery, ObservationChange,
	}
	for i := 0; i < n; i++ {
		title := fmt.Sprintf("JWT expiry is 15m case %d", i)
		if i%7 == 0 {
			title = fmt.Sprintf("dashboard YAML provisioning wrap case %d", i)
		}
		_, err := store.Capture(CaptureInput{
			SessionID: fmt.Sprintf("sess-%d", i/10),
			Kind:      KindObservation,
			Title:     title,
			Text:      title + " recorded for retrieval benches.",
			Files:     []string{"internal/auth/login.go", "pkg/services/provisioning/dashboards/dashboard.go"},
			Topic:     types[i%len(types)],
			Facts:     []string{title},
		})
		if err != nil {
			b.Fatal(err)
		}
	}
	return store
}

func BenchmarkSearch(b *testing.B) {
	store := benchStore(b, 200)
	defer store.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := store.Search(SearchFilter{Query: "JWT expiry", Limit: 10}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSearchTypeFile(b *testing.B) {
	store := benchStore(b, 200)
	defer store.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.Search(SearchFilter{
			Query: "provisioning",
			Type:  ObservationDecision,
			File:  "pkg/services/provisioning/dashboards/dashboard.go",
			Limit: 10,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRecall(b *testing.B) {
	store := benchStore(b, 200)
	defer store.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := store.Recall("JWT expiry 15m", 1500); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetMany(b *testing.B) {
	store := benchStore(b, 50)
	defer store.Close()
	ids := make([]int64, 8)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := store.GetMany(ids); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTimelineAround(b *testing.B) {
	store := benchStore(b, 80)
	defer store.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := store.TimelineAround(40, 5, 5); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSessionStartIndex(b *testing.B) {
	root := b.TempDir()
	if err := paths.Resolve(root).EnsureDirs(); err != nil {
		b.Fatal(err)
	}
	store, err := OpenRoot(root)
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		_, err := store.Capture(CaptureInput{
			Kind:  KindObservation,
			Title: fmt.Sprintf("prior decision %d about auth cookies", i),
			Text:  "session start index should list titles not bodies",
			Topic: ObservationDecision,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
	store.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if SessionStartIndex(root) == "" {
			b.Fatal("expected index")
		}
	}
}

func BenchmarkApplySupersedeCap(b *testing.B) {
	hits := make([]Hit, 40)
	outgoing := map[int64][]int64{}
	stale := map[int64]bool{}
	for i := range hits {
		hits[i] = Hit{Episode: Episode{ID: int64(i + 1), Score: float64(40-i) / 40, Title: "x"}}
		if i%3 == 0 && i+1 < len(hits) {
			stale[hits[i].ID] = true
			outgoing[hits[i].ID] = []int64{hits[i+1].ID}
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clone := append([]Hit(nil), hits...)
		applySupersedeCap(clone, outgoing, stale, false, 10)
	}
}
