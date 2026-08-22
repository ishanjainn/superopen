package memory

import (
	"testing"
	"time"
)

func TestApplySupersedeCap(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-time.Hour).Format(time.RFC3339Nano)
	future := now.Add(time.Hour).Format(time.RFC3339Nano)

	hit := func(id int64, score float64, validTo string) Hit {
		return Hit{Episode: Episode{ID: id, Score: score, ValidTo: validTo, Title: "x"}}
	}

	t.Run("stale src capped below best corrector", func(t *testing.T) {
		stale := hit(1, 2.0, past)
		corrector := hit(2, 1.1, "")
		other := hit(3, 0.5, "")
		hits := []Hit{stale, corrector, other}
		outgoing := map[int64][]int64{1: {2}}
		staleMap := map[int64]bool{1: true}
		hits = applySupersedeCap(hits, outgoing, staleMap, false, 10)
		if hits[0].ID != 1 {
			// scores mutated in place by id
		}
		byID := map[int64]float64{}
		for _, h := range hits {
			byID[h.ID] = h.Score
		}
		if byID[1] >= byID[2] {
			t.Fatalf("stale %v not below corrector %v", byID[1], byID[2])
		}
		if byID[3] != 0.5 {
			t.Fatalf("other mutated: %v", byID[3])
		}
	})

	t.Run("not yet stale src untouched", func(t *testing.T) {
		src := hit(1, 2.0, future)
		corrector := hit(2, 1.0, "")
		hits := []Hit{src, corrector}
		hits = applySupersedeCap(hits, map[int64][]int64{1: {2}}, map[int64]bool{}, false, 10)
		if hitByID(hits, 1).Score != 2.0 {
			t.Fatalf("future valid_to must stay 2.0, got %+v", hits)
		}
	})

	t.Run("historical intent skips cap", func(t *testing.T) {
		stale := hit(1, 2.0, past)
		corrector := hit(2, 1.0, "")
		hits := []Hit{stale, corrector}
		hits = applySupersedeCap(hits, map[int64][]int64{1: {2}}, map[int64]bool{1: true}, true, 10)
		if hitByID(hits, 1).Score != 2.0 {
			t.Fatalf("historical cue must skip cap: %+v", hits)
		}
	})

	t.Run("corrector absent leaves src", func(t *testing.T) {
		stale := hit(1, 2.0, past)
		hits := []Hit{stale}
		hits = applySupersedeCap(hits, map[int64][]int64{1: {99}}, map[int64]bool{1: true}, false, 10)
		if hitByID(hits, 1).Score != 2.0 {
			t.Fatal("absent corrector must leave src")
		}
	})

	t.Run("chain caps transitively", func(t *testing.T) {
		a := hit(1, 3.0, past)
		b := hit(2, 2.5, past)
		c := hit(3, 1.0, "")
		hits := []Hit{a, b, c}
		outgoing := map[int64][]int64{1: {2}, 2: {3}}
		staleMap := map[int64]bool{1: true, 2: true}
		hits = applySupersedeCap(hits, outgoing, staleMap, false, 10)
		if hitByID(hits, 3).Score != 1.0 {
			t.Fatal("live successor must stay")
		}
		if hitByID(hits, 2).Score >= hitByID(hits, 3).Score {
			t.Fatalf("b must sit below c: %+v", hits)
		}
		if hitByID(hits, 1).Score >= hitByID(hits, 2).Score {
			t.Fatalf("a must sit below b: %+v", hits)
		}
	})

	t.Run("src already below corrector untouched", func(t *testing.T) {
		stale := hit(1, 0.4, past)
		corrector := hit(2, 1.0, "")
		hits := []Hit{stale, corrector}
		hits = applySupersedeCap(hits, map[int64][]int64{1: {2}}, map[int64]bool{1: true}, false, 10)
		if hitByID(hits, 1).Score != 0.4 {
			t.Fatalf("already-below src mutated: %+v", hits)
		}
	})

	t.Run("best of multiple correctors", func(t *testing.T) {
		stale := hit(1, 2.0, past)
		weak := hit(2, 0.3, "")
		strong := hit(3, 1.2, "")
		hits := []Hit{stale, weak, strong}
		hits = applySupersedeCap(hits, map[int64][]int64{1: {2, 3}}, map[int64]bool{1: true}, false, 10)
		want := 1.2 - supersedeCapEps
		got := hitByID(hits, 1).Score
		if got < want-1e-9 || got > want+1e-9 {
			t.Fatalf("got %v want %v", got, want)
		}
	})

	t.Run("corrector outside serving window leaves src", func(t *testing.T) {
		stale := hit(1, 2.0, past)
		var hits []Hit
		hits = append(hits, stale)
		for i := 0; i < 12; i++ {
			hits = append(hits, hit(int64(10+i), 1.0-0.01*float64(i), ""))
		}
		corrector := hit(99, 0.05, "")
		hits = append(hits, corrector)
		got := applySupersedeCap(hits, map[int64][]int64{1: {99}}, map[int64]bool{1: true}, false, 10)
		if hitByID(got, 1).Score != 2.0 {
			t.Fatalf("out-of-window corrector must not sink stale: %+v", hitByID(got, 1))
		}
		stale2 := hit(2, 2.0, past)
		corrector2 := hit(3, 1.5, "")
		hits2 := []Hit{stale2, corrector2}
		for i := 0; i < 12; i++ {
			hits2 = append(hits2, hit(int64(20+i), 1.0-0.01*float64(i), ""))
		}
		got2 := applySupersedeCap(hits2, map[int64][]int64{2: {3}}, map[int64]bool{2: true}, false, 10)
		if hitByID(got2, 2).Score >= hitByID(got2, 3).Score {
			t.Fatal("in-window corrector must cap stale")
		}
	})
}

func hitByID(hits []Hit, id int64) Hit {
	for _, h := range hits {
		if h.ID == id {
			return h
		}
	}
	return Hit{}
}

func TestHistoricalCue(t *testing.T) {
	if !historicalCue("what did we used to say about auth") {
		t.Fatal("expected historical cue")
	}
	if historicalCue("session auth jwt") {
		t.Fatal("current cue is not historical")
	}
}

func TestIsCompressibleDeniesPrompt(t *testing.T) {
	if isCompressible(Episode{Kind: KindPrompt, Title: "fix login"}) {
		t.Fatal("verbatim prompts must not be compressible")
	}
	if !isCompressible(Episode{Kind: KindObservation, Topic: ObservationBugfix}) {
		t.Fatal("observations may be summarized")
	}
	if isCompressible(Episode{Kind: KindSession, Pinned: true}) {
		t.Fatal("pins must not compress")
	}
}
