// Package watch implements a session-scoped git-poll graph refresher.
package watch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/graph/client"
	"github.com/ishanjainn/superopen/internal/graph/engine"
	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/projects"
)

// DefaultPollInterval is the live git-poll cadence used by so dev and MCP serve.
// Builds are local (Tree-sitter + SQLite); they do not call an LLM or the coding agent.
const DefaultPollInterval = 60 * time.Second

// stopGrace bounds how long Stop waits for an in-flight refresh. A build holds
// an OS advisory lock the kernel releases on exit, so abandoning the wait
// cannot strand the lock — whereas blocking here stalls the host's shutdown for
// the length of a full build.
const stopGrace = 2 * time.Second

// signatureFile records the git signature the current graph was built from, so
// a freshly spawned server does not rebuild a graph that is already current.
const signatureFile = "watch-signature"

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
	if r.lastSig == "" {
		r.lastSig = r.loadSignature()
	}
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
		select {
		case <-done:
		case <-time.After(stopGrace):
		}
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
	if err := r.Client.Call(ctx, api.OpBuild, api.BuildRequest{RepoRoot: r.Root, Incremental: true}, &result); err != nil {
		return
	}
	RecordSignature(r.Root)
	_ = projects.TouchGraphRefresh(r.Root)
}

func (r *Runner) loadSignature() string { return LoadSignature(r.Root) }

// LoadSignature reads the git signature the on-disk graph was last built from.
func LoadSignature(root string) string {
	path := signaturePath(root)
	if path == "" {
		return ""
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(body)
}

// RecordSignature marks the graph as current for the working tree's present
// state. Every completed build calls this, so a later `mcp serve` does not
// spend a full build proving the graph it already has is up to date.
func RecordSignature(root string) {
	path := signaturePath(root)
	if path == "" {
		return
	}
	sig := gitSignature(root)
	if sig == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, []byte(sig), 0o644)
}

func signaturePath(root string) string {
	if strings.TrimSpace(root) == "" {
		return ""
	}
	resolved := paths.Resolve(root).Root
	if resolved == "" {
		return ""
	}
	return filepath.Join(resolved, signatureFile)
}

func gitSignature(root string) string {
	head := runGit(root, "rev-parse", "HEAD")
	if head == "" {
		return ""
	}
	status := runGit(root, "status", "--porcelain")
	return head + "\n" + excludeStateDir(status)
}

// excludeStateDir drops Superopen's own directory from the signature. In a
// repository that does not gitignore it, the recorded signature would
// otherwise alter the very status it was derived from, and every poll would
// see a change and rebuild.
func excludeStateDir(status string) string {
	if status == "" {
		return ""
	}
	var kept []string
	for _, line := range strings.Split(status, "\n") {
		if strings.Contains(line, paths.DirName+"/") || strings.HasSuffix(strings.TrimSpace(line), paths.DirName) {
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
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
