// Package watch implements a session-scoped git-poll graph refresher.
package watch

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/graph/client"
	"github.com/ishanjainn/superopen/internal/graph/engine"
	"github.com/ishanjainn/superopen/internal/projects"
)

// DefaultPollInterval is the live git-poll cadence used by so dev and MCP serve.
// Builds are local (Tree-sitter + SQLite); they do not call an LLM or the coding agent.
const DefaultPollInterval = 60 * time.Second

// Runner polls git HEAD + dirty signature and triggers incremental graph builds.
type Runner struct {
	Root   string
	Client client.Client

	mu       sync.Mutex
	lastSig  string
	cancel   context.CancelFunc
	done     chan struct{}
	interval time.Duration
}

// Start begins polling until Stop. Safe to call once.
func (r *Runner) Start(ctx context.Context) {
	r.mu.Lock()
	if r.cancel != nil {
		r.mu.Unlock()
		return
	}
	if r.interval <= 0 {
		r.interval = DefaultPollInterval
	}
	cctx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.done = make(chan struct{})
	r.mu.Unlock()

	go func() {
		defer close(r.done)
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()
		r.tick(cctx)
		for {
			select {
			case <-cctx.Done():
				return
			case <-ticker.C:
				r.tick(cctx)
			}
		}
	}()
}

// Stop cancels the poller and waits for exit.
func (r *Runner) Stop() {
	r.mu.Lock()
	cancel := r.cancel
	done := r.done
	r.cancel = nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (r *Runner) tick(ctx context.Context) {
	sig := gitSignature(r.Root)
	r.mu.Lock()
	unchanged := sig != "" && sig == r.lastSig
	if !unchanged && sig != "" {
		r.lastSig = sig
	}
	r.mu.Unlock()
	if unchanged || sig == "" {
		return
	}
	if engine.BuildBusy(r.Root) {
		return
	}
	var result api.BuildResult
	_ = r.Client.Call(ctx, api.OpBuild, api.BuildRequest{RepoRoot: r.Root, Incremental: true}, &result)
	_ = projects.TouchGraphRefresh(r.Root)
}

func gitSignature(root string) string {
	head := runGit(root, "rev-parse", "HEAD")
	if head == "" {
		return ""
	}
	status := runGit(root, "status", "--porcelain")
	return head + "\n" + status
}

func runGit(root string, args ...string) string {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
