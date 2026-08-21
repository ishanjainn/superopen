package watch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/client"
)

func TestDefaultPollInterval(t *testing.T) {
	if DefaultPollInterval != 60*time.Second {
		t.Fatalf("DefaultPollInterval = %v, want 60s", DefaultPollInterval)
	}
}

func TestRunnerUsesDefaultInterval(t *testing.T) {
	r := &Runner{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.Start(ctx)
	r.Stop()
	if r.interval != DefaultPollInterval {
		t.Fatalf("interval = %v, want %v", r.interval, DefaultPollInterval)
	}
}

func TestTickDoesNotCreateSOWhenUnmanaged(t *testing.T) {
	repo := t.TempDir()
	run := exec.Command("git", "init")
	run.Dir = repo
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	cfg := exec.Command("git", "-C", repo, "config", "user.email", "so@example.com")
	if out, err := cfg.CombinedOutput(); err != nil {
		t.Fatalf("git config email: %v %s", err, out)
	}
	cfg = exec.Command("git", "-C", repo, "config", "user.name", "so")
	if out, err := cfg.CombinedOutput(); err != nil {
		t.Fatalf("git config name: %v %s", err, out)
	}
	readme := filepath.Join(repo, "README")
	if err := os.WriteFile(readme, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	add := exec.Command("git", "-C", repo, "add", "README")
	if out, err := add.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v %s", err, out)
	}
	commit := exec.Command("git", "-C", repo, "commit", "-m", "init")
	if out, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v %s", err, out)
	}
	r := &Runner{Root: repo, Client: client.Client{}}
	r.tick(context.Background())
	if _, err := os.Stat(filepath.Join(repo, ".so")); !os.IsNotExist(err) {
		t.Fatalf("unmanaged tick must not create .so, stat=%v", err)
	}
}
