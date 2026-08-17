package main

import (
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/session"
	"github.com/ishanjainn/superopen/internal/tracestore"
)

func TestApplyReviewCompletesAndIsNoopWhenComplete(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	if err := config.Save(paths.Config, cfg); err != nil {
		t.Fatal(err)
	}
	id := "rev-1"
	ss := session.NewStore(paths)
	if err := ss.Start(session.Meta{ID: id, Vendor: "cursor", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"exploration":0.8,"scope":0.8,"wandering":0.1,"verification":0.8,"note":"ok","findings":[],"memory":{}}`)
	res, err := applyReviewJSON(root, paths, cfg, id, "live_agent:cursor", raw, true)
	if err != nil {
		t.Fatal(err)
	}
	if !res.CompleteReview || res.Backend != "live_agent:cursor" {
		t.Fatalf("apply-review should complete, got %+v", res)
	}
	doc, err := ss.ReadDocument(id)
	if err != nil || doc.Review.Status != "complete" {
		t.Fatalf("status=%q err=%v", doc.Review.Status, err)
	}

	again, err := applyReviewJSON(root, paths, cfg, id, "live_agent:cursor", raw, true)
	if err != nil {
		t.Fatal(err)
	}
	if again.Backend != "live_agent:cursor" {
		t.Fatalf("noop should return prior result, got %+v", again)
	}
	doc, err = ss.ReadDocument(id)
	if err != nil || doc.Review.Status != "complete" {
		t.Fatalf("second apply mutated status: %+v err=%v", doc.Review, err)
	}
}

func TestApplyReviewLosesClaimRace(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	id := "rev-race"
	ss := session.NewStore(paths)
	if err := ss.Start(session.Meta{ID: id, Vendor: "cursor", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	release, ok := ss.ClaimReview(id, "cli-review")
	if !ok {
		t.Fatal("setup claim failed")
	}
	defer release()
	_, err := applyReviewJSON(root, paths, cfg, id, "live_agent:cursor", []byte(`{"findings":[],"memory":{}}`), true)
	if err == nil {
		t.Fatal("live apply must lose to an in-flight CLI claim")
	}
}

func TestFinalizeDoesNotReviewPromptOnlySession(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	if err := config.Save(paths.Config, cfg); err != nil {
		t.Fatal(err)
	}
	id := "fin-1"
	now := time.Now().UnixNano()
	store := tracestore.NewLocalJSONL(paths.TracesDir)
	if err := store.Write([]tracestore.Span{{
		TraceID: id, SpanID: "1", Name: "coding_agent.prompt", SessionID: id,
		StartTimeUnixN: now, EndTimeUnixN: now + 1e6,
		Attributes: map[string]string{"gen_ai.prompt": "do work", "coding_agent.vendor": "cursor"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := finalizeSession(root, id, finalizeOpts{SpawnCLIReview: false}); err != nil {
		t.Fatal(err)
	}
	doc, err := session.NewStore(paths).ReadDocument(id)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Review.Status != "" {
		t.Fatalf("prompt-only session must not schedule review, got %q backend=%q", doc.Review.Status, doc.Review.Backend)
	}
}
