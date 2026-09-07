package watch

import (
	"context"
	"sync"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/graph/client"
	"github.com/ishanjainn/superopen/internal/graph/engine"
	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/projects"
)

// DefaultPollInterval is the live git-poll cadence used by so dev.
const DefaultPollInterval = 60 * time.Second

const stopGrace = 2 * time.Second

// Runner polls the fingerprint probe and triggers incremental graph builds.
type Runner struct {
	Root   string
	Client client.Client

	mu       sync.Mutex
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
		select {
		case <-done:
		case <-time.After(stopGrace):
		}
	}
}

func (r *Runner) tick(ctx context.Context) {
	if !paths.Managed(r.Root) {
		return
	}
	dirty, err := engine.ProbeDirty(ctx, r.Root, nil)
	if err != nil || !dirty {
		return
	}
	if engine.BuildBusy(r.Root) || engine.BuildPoolFull() {
		return
	}
	var result api.BuildResult
	if err := r.Client.Call(ctx, api.OpBuild, api.BuildRequest{RepoRoot: r.Root, Incremental: true, FromProbe: true}, &result); err != nil {
		return
	}
	_ = engine.WriteFingerprint(ctx, r.Root, nil)
	_ = projects.TouchGraphRefresh(r.Root)
}
