package session

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/harness"
)

func TestClaimReviewOneWinner(t *testing.T) {
	paths := harness.Resolve(t.TempDir())
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	store := NewStore(paths)
	if err := store.Start(Meta{ID: "claim-1", Vendor: "cursor", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	release, ok := store.ClaimReview("claim-1", "cli-review")
	if !ok {
		t.Fatal("first claim should succeed")
	}
	if _, ok := store.ClaimReview("claim-1", "apply-review"); ok {
		t.Fatal("second claim must lose")
	}
	release()
	if _, ok := store.ClaimReview("claim-1", "apply-review"); !ok {
		t.Fatal("claim should succeed after release")
	}
}

func TestPreviousReviewContextIsShortAndSameVendor(t *testing.T) {
	paths := harness.Resolve(t.TempDir())
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	store := NewStore(paths)
	now := time.Now().UTC()
	if err := store.Start(Meta{ID: "claude-old", Vendor: "claude-code", StartedAt: now.Add(-2 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteDocument("claude-old", func(d *Document) { d.Review.Status = "pending" }); err != nil {
		t.Fatal(err)
	}
	if err := store.Start(Meta{ID: "cursor-old", Vendor: "cursor", StartedAt: now.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteDocument("cursor-old", func(d *Document) { d.Review.Status = "pending" }); err != nil {
		t.Fatal(err)
	}
	got := store.PreviousReviewContext("cursor", "cursor-new")
	if !strings.Contains(got, "so review-brief cursor-old") || strings.Contains(got, "claude-old") {
		t.Fatalf("same-vendor pending inject failed: %s", got)
	}
	if strings.Contains(got, "REDACTED SESSION TEXT") {
		t.Fatal("must not inject full evidence")
	}
}

func TestWriteDocumentDoesNotLoseConcurrentReviewAndMeta(t *testing.T) {
	paths := harness.Resolve(t.TempDir())
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	store := NewStore(paths)
	if err := store.Start(Meta{ID: "race-1", Vendor: "cursor", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = store.WriteDocument("race-1", func(d *Document) {
			d.Review.Status = "complete"
			d.Review.Backend = "live_agent:cursor"
		})
	}()
	go func() {
		defer wg.Done()
		_ = store.WriteDocument("race-1", func(d *Document) {
			d.EvalBadge = "good"
		})
	}()
	wg.Wait()
	doc, err := store.ReadDocument("race-1")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Review.Status != "complete" || doc.Review.Backend != "live_agent:cursor" {
		t.Fatalf("review lost: %+v", doc.Review)
	}
	if doc.EvalBadge != "good" {
		t.Fatalf("meta lost: badge=%q", doc.EvalBadge)
	}
}
