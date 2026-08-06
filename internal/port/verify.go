package port

import (
	"fmt"
	"os"
	"path/filepath"
)

// VerifyResult summarizes round-trip / sample integrity checks.
type VerifyResult struct {
	Sampled int      `json:"sampled"`
	OK      int      `json:"ok"`
	Failed  int      `json:"failed"`
	Events  []Event  `json:"events"`
	Details []string `json:"details,omitempty"`
}

// HubFactory builds import/export adapters rooted at repoRoot (used for temp verify).
type HubFactory func(repoRoot string) (ImportAdapter, ExportAdapter)

// Verify samples source sessions and parses IR. When hubFactory is set (or registry
// has a so hub), it round-trips into a temporary .so tree.
func (o *Orchestrator) Verify(from, to HarnessID, sample int, hubFactory HubFactory) (VerifyResult, error) {
	var res VerifyResult
	emit := func(e Event) { res.Events = append(res.Events, e) }
	if sample <= 0 {
		sample = 3
	}
	imp, ok := o.Reg.Import(from)
	if !ok {
		return res, fmt.Errorf("no import adapter for %s", from)
	}
	if okDetect, err := imp.Detect(); err != nil || !okDetect {
		return res, fmt.Errorf("source %s not detectable: %v", from, err)
	}
	refs, err := imp.Discover()
	if err != nil {
		return res, err
	}
	if len(refs) == 0 {
		return res, fmt.Errorf("no sessions discovered for %s", from)
	}

	var hubExp ExportAdapter
	var hubImp ImportAdapter
	tmp := ""
	wantHub := to == HarnessSOHub || to == ""
	if wantHub {
		var err error
		tmp, err = os.MkdirTemp("", "so-port-verify-*")
		if err != nil {
			return res, err
		}
		defer os.RemoveAll(tmp)
		_ = os.MkdirAll(filepath.Join(tmp, ".so", "sessions"), 0o755)
		if hubFactory != nil {
			hubImp, hubExp = hubFactory(tmp)
		} else {
			hubExp, _ = o.Reg.Export(HarnessSOHub)
			hubImp, _ = o.Reg.Import(HarnessSOHub)
		}
	}

	for _, ref := range refs {
		if res.OK >= sample {
			break
		}
		res.Sampled++
		sess, err := imp.Parse(ref)
		if err != nil {
			res.Failed++
			emit(Event{Type: "error", ID: ref.SourceSessionID, Error: err.Error()})
			continue
		}
		if sess.SchemaVersion != SchemaVersion {
			res.Failed++
			emit(Event{Type: "error", ID: ref.SourceSessionID, Error: "schema mismatch"})
			continue
		}
		if len(sess.Turns) == 0 {
			// Skip empty meta-only sessions without counting as hard failure for sampling.
			continue
		}
		destRoot := o.RepoRoot
		if tmp != "" {
			destRoot = tmp
		}
		if destRoot != "" {
			RemapCWD(&sess, destRoot)
		}
		if hubExp != nil && hubImp != nil {
			out, err := hubExp.Write(sess, WriteOptions{Force: true})
			if err != nil {
				res.Failed++
				emit(Event{Type: "error", ID: ref.SourceSessionID, Error: err.Error()})
				continue
			}
			sessDir := filepath.Join(tmp, ".so", "sessions", out.DestSessionID)
			back, err := hubImp.Parse(SessionRef{
				Harness: HarnessSOHub, SourceSessionID: out.DestSessionID, SourcePath: sessDir,
			})
			if err != nil || len(back.Turns) == 0 {
				res.Failed++
				emit(Event{Type: "error", ID: ref.SourceSessionID, Error: "hub round-trip failed"})
				continue
			}
			if len(back.Turns) != len(sess.Turns) {
				res.Details = append(res.Details, fmt.Sprintf("%s: turn count %d→%d", ref.SourceSessionID, len(sess.Turns), len(back.Turns)))
			}
		}
		res.OK++
		emit(Event{Type: "import", ID: ref.SourceSessionID, Detail: fmt.Sprintf("%d turns", len(sess.Turns))})
	}
	if res.OK == 0 && res.Sampled > 0 {
		res.Failed++
		return res, fmt.Errorf("no sessions with text turns in sample window")
	}
	emit(Event{Type: "done", Detail: fmt.Sprintf("ok=%d failed=%d", res.OK, res.Failed)})
	return res, nil
}
