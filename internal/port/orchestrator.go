package port

import (
	"fmt"
	"time"
)

// Event is a machine-readable progress event (--json).
type Event struct {
	Type    string `json:"type"` // detect|discover|import|export|skip|error|done
	Harness string `json:"harness,omitempty"`
	ID      string `json:"id,omitempty"`
	Detail  string `json:"detail,omitempty"`
	Error   string `json:"error,omitempty"`
}

type PortOptions struct {
	From    HarnessID
	To      HarnessID
	IDs     []string // empty + All = all
	All     bool
	Force   bool
	Preview bool // discover only
}

type PortResult struct {
	Events  []Event      `json:"events"`
	Refs    []SessionRef `json:"refs,omitempty"`
	Ported  int          `json:"ported"`
	Skipped int          `json:"skipped"`
	Failed  int          `json:"failed"`
	// DestSessionIDs are hub/resume ids written this run (last is armed for SessionStart inject).
	DestSessionIDs []string `json:"dest_session_ids,omitempty"`
	// ResumeArmed is true when a one-shot SessionStart inject was written for the destination.
	ResumeArmed bool   `json:"resume_armed,omitempty"`
	ResumeID    string `json:"resume_id,omitempty"`
	// DroppedTurns is the total non-text turns (tool calls, results, reasoning)
	// discarded across all ported sessions this run. Non-zero means the
	// destination transcript is prose-only for those turns.
	DroppedTurns int `json:"dropped_turns,omitempty"`
	// WorkingStateSessions counts ported sessions where working state (files
	// touched, commands run) was recovered from the dropped tool calls.
	WorkingStateSessions int `json:"working_state_sessions,omitempty"`
}

// Orchestrator runs detect → import → export with ledger idempotency.
type Orchestrator struct {
	Reg      *Registry
	Ledger   *Ledger
	RepoRoot string // destination worktree authority for cwd remap
}

func (o *Orchestrator) Detect() map[HarnessID]map[string]bool {
	out := map[HarnessID]map[string]bool{}
	for _, id := range o.Reg.ListImport() {
		a, _ := o.Reg.Import(id)
		ok, _ := a.Detect()
		out[id] = map[string]bool{"import": ok}
	}
	for _, id := range o.Reg.ListExport() {
		a, _ := o.Reg.Export(id)
		ok, _ := a.Detect()
		if out[id] == nil {
			out[id] = map[string]bool{}
		}
		out[id]["export"] = ok
	}
	return out
}

func (o *Orchestrator) Port(opts PortOptions) (PortResult, error) {
	var res PortResult
	emit := func(e Event) { res.Events = append(res.Events, e) }

	imp, ok := o.Reg.Import(opts.From)
	if !ok {
		return res, fmt.Errorf("no import adapter for %s", opts.From)
	}
	exp, ok := o.Reg.Export(opts.To)
	if !ok && !opts.Preview {
		return res, fmt.Errorf("no export adapter for %s", opts.To)
	}

	if okDetect, err := imp.Detect(); err != nil || !okDetect {
		return res, fmt.Errorf("source %s not detectable: %v", opts.From, err)
	}
	emit(Event{Type: "detect", Harness: string(opts.From), Detail: "ok"})

	refs, err := imp.Discover()
	if err != nil {
		return res, err
	}
	// annotate ledger
	for i := range refs {
		if e, hit := o.Ledger.Lookup(opts.From, refs[i].SourceSessionID, opts.To); hit {
			refs[i].Imported = true
			if refs[i].UpdatedAt > 0 && e.SourceUpdatedAt > 0 && refs[i].UpdatedAt > e.SourceUpdatedAt {
				refs[i].SourceChanged = true
			}
		}
	}
	res.Refs = refs
	emit(Event{Type: "discover", Harness: string(opts.From), Detail: fmt.Sprintf("%d sessions", len(refs))})

	if opts.Preview {
		emit(Event{Type: "done", Detail: "preview"})
		return res, nil
	}

	if okExp, err := exp.Detect(); err != nil || !okExp {
		return res, fmt.Errorf("destination %s not writable: %v", opts.To, err)
	}

	want := map[string]bool{}
	for _, id := range opts.IDs {
		want[id] = true
	}
	selected := []SessionRef{}
	for _, r := range refs {
		if opts.All || want[r.SourceSessionID] {
			selected = append(selected, r)
		}
	}
	if len(selected) == 0 {
		return res, fmt.Errorf("no sessions selected (use --all or --id)")
	}

	lastTitle := ""
	lastSourceID := ""
	for _, ref := range selected {
		sess, err := imp.Parse(ref)
		if err != nil {
			res.Failed++
			emit(Event{Type: "error", Harness: string(opts.From), ID: ref.SourceSessionID, Error: err.Error()})
			continue
		}
		emit(Event{Type: "import", ID: ref.SourceSessionID, Detail: fmt.Sprintf("%d turns", len(sess.Turns))})

		if o.RepoRoot != "" {
			RemapCWD(&sess, o.RepoRoot)
		}

		writeOpts := WriteOptions{Force: opts.Force}
		if e, hit := o.Ledger.Lookup(opts.From, ref.SourceSessionID, opts.To); hit && !opts.Force {
			if !ref.SourceChanged {
				res.Skipped++
				emit(Event{Type: "skip", ID: ref.SourceSessionID, Detail: "ledger hit"})
				continue
			}
			writeOpts.ExistingDestID = e.DestSessionID
		}

		out, err := exp.Write(sess, writeOpts)
		if err != nil {
			res.Failed++
			emit(Event{Type: "error", Harness: string(opts.To), ID: ref.SourceSessionID, Error: err.Error()})
			continue
		}
		if out.Skipped {
			res.Skipped++
			emit(Event{Type: "skip", ID: ref.SourceSessionID, Detail: out.Reason})
			continue
		}
		_ = o.Ledger.Upsert(LedgerEntry{
			SourceHarness:   opts.From,
			SourceSessionID: ref.SourceSessionID,
			DestHarness:     opts.To,
			DestSessionID:   out.DestSessionID,
			SourceUpdatedAt: ref.UpdatedAt,
			PortedAt:        time.Now().UTC(),
		})
		res.Ported++
		res.DroppedTurns += sess.DroppedTurns
		if !sess.WorkingState.Empty() {
			res.WorkingStateSessions++
		}
		res.DestSessionIDs = append(res.DestSessionIDs, out.DestSessionID)
		emit(Event{Type: "export", Harness: string(opts.To), ID: out.DestSessionID, Detail: "ok"})
		lastTitle = sess.Title
		lastSourceID = ref.SourceSessionID
		// Arm SessionStart inject for every destination harness (not just Cursor).
		if o.RepoRoot != "" {
			if err := ArmResume(o.RepoRoot, opts.To, out.DestSessionID, sess); err == nil {
				res.ResumeArmed = true
				res.ResumeID = out.DestSessionID
			}
		}
	}
	base := fmt.Sprintf("ported=%d skipped=%d failed=%d", res.Ported, res.Skipped, res.Failed)
	if res.DroppedTurns > 0 {
		base += fmt.Sprintf(" dropped_turns=%d", res.DroppedTurns)
	}
	if res.Ported > 0 && o.RepoRoot != "" {
		RefreshMemoryAfterPort(o.RepoRoot, string(opts.From), string(opts.To), lastSourceID, lastTitle)
		detail := base + " memory_refreshed=1"
		if res.ResumeArmed {
			detail += fmt.Sprintf(" resume_armed=%s→%s", opts.To, res.ResumeID)
		}
		emit(Event{Type: "done", Detail: detail})
	} else {
		emit(Event{Type: "done", Detail: base})
	}
	return res, nil
}
