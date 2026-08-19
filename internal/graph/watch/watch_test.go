package watch

import (
	"context"
	"testing"
	"time"
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
