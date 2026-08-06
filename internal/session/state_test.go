package session_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/superopen/so/internal/harness"
	"github.com/superopen/so/internal/session"
)

func TestStateStoreLifecycle(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	st := session.NewStateStore(paths)

	err := st.Save(session.State{
		SessionID: "s1",
		Vendor:    "cursor",
		Phase:     session.PhaseActive,
		Branch:    "main",
		WorktreeID: "wt1",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.Get("s1")
	if err != nil || got.Phase != session.PhaseActive {
		t.Fatalf("get: %+v %v", got, err)
	}
	if w := st.WarnConcurrent("s1", "wt1"); w != "" {
		t.Fatalf("unexpected warning: %s", w)
	}
	_ = st.Save(session.State{SessionID: "s2", Phase: session.PhaseActive, WorktreeID: "wt1", UpdatedAt: time.Now()})
	if w := st.WarnConcurrent("s1", "wt1"); w == "" {
		t.Fatal("expected concurrent warning")
	}
	if err := st.End("s1"); err != nil {
		t.Fatal(err)
	}
	got, _ = st.Get("s1")
	if got.Phase != session.PhaseEnded {
		t.Fatalf("want ended, got %s", got.Phase)
	}
	_ = filepath.Join(paths.Root, "session-state")
}
