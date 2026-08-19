package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestArtifactRoundTripVerifiesAndRebindsRoot(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	ctx := context.Background()
	sourceRepo := t.TempDir()
	_, err := Publish(ctx, sourceRepo, func(ctx context.Context, path string) error {
		buildFixture(t, ctx, path, "artifact-generation")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(t.TempDir(), "team.so-graph.zst")
	exported, err := ExportArtifact(ctx, sourceRepo, artifact)
	if err != nil {
		t.Fatal(err)
	}
	if exported.Generation != "artifact-generation" || exported.DatabaseSHA256 == "" {
		t.Fatalf("unexpected export manifest: %+v", exported)
	}
	verified, err := VerifyArtifact(ctx, artifact)
	if err != nil {
		t.Fatal(err)
	}
	if verified != exported {
		t.Fatalf("verification manifest changed: export=%+v verify=%+v", exported, verified)
	}
	targetRepo := t.TempDir()
	imported, live, err := ImportArtifact(ctx, targetRepo, artifact)
	if err != nil {
		t.Fatal(err)
	}
	if imported.DatabaseSHA256 != exported.DatabaseSHA256 {
		t.Fatalf("manifest changed: export=%+v import=%+v", exported, imported)
	}
	store, err := OpenReadOnly(live)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	status, err := store.Status(ctx, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	canonicalTarget, _ := CanonicalRoot(targetRepo)
	var storedRoot string
	if err := store.db.QueryRow(`SELECT root_path FROM projects WHERE name='fixture'`).Scan(&storedRoot); err != nil {
		t.Fatal(err)
	}
	if status.Generation != "artifact-generation" || storedRoot != canonicalTarget {
		t.Fatalf("import was not rebound: status=%+v root=%s", status, storedRoot)
	}
}

func TestArtifactCorruptionDoesNotReplaceLiveGraph(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	ctx := context.Background()
	repo := t.TempDir()
	live, err := Publish(ctx, repo, func(ctx context.Context, path string) error {
		buildFixture(t, ctx, path, "stable-generation")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(t.TempDir(), "team.so-graph.zst")
	if _, err := ExportArtifact(ctx, repo, artifact); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	body[len(body)/2] ^= 0xff
	corrupt := filepath.Join(t.TempDir(), "corrupt.so-graph.zst")
	if err := os.WriteFile(corrupt, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ImportArtifact(ctx, repo, corrupt); err == nil {
		t.Fatal("corrupted artifact imported")
	}
	store, err := OpenReadOnly(live)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	status, err := store.Status(ctx, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if status.Generation != "stable-generation" {
		t.Fatalf("corrupt import replaced live graph: %+v", status)
	}
}
