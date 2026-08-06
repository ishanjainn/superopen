package port

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LedgerEntry records a successful (or live) port.
type LedgerEntry struct {
	SourceHarness   HarnessID `json:"source_harness"`
	SourceSessionID string    `json:"source_session_id"`
	DestHarness     HarnessID `json:"dest_harness"`
	DestSessionID   string    `json:"dest_session_id"`
	SourceUpdatedAt int64     `json:"source_updated_at,omitempty"`
	PortedAt        time.Time `json:"ported_at"`
}

type ledgerFile struct {
	Entries []LedgerEntry `json:"entries"`
}

// Ledger is the single idempotency authority.
type Ledger struct {
	Path string
	mu   sync.Mutex
}

func NewLedger(path string) *Ledger {
	return &Ledger{Path: path}
}

func DefaultLedgerPath(soRoot string) string {
	return filepath.Join(soRoot, "port", "ledger.json")
}

func (l *Ledger) key(srcH HarnessID, srcID string, dstH HarnessID) string {
	return fmt.Sprintf("%s|%s|%s", srcH, srcID, dstH)
}

func (l *Ledger) load() (ledgerFile, error) {
	var f ledgerFile
	data, err := os.ReadFile(l.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return f, nil
		}
		return f, err
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return f, err
	}
	return f, nil
}

func (l *Ledger) save(f ledgerFile) error {
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(l.Path, data, 0o644)
}

func (l *Ledger) Lookup(srcH HarnessID, srcID string, dstH HarnessID) (LedgerEntry, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := l.load()
	if err != nil {
		return LedgerEntry{}, false
	}
	want := l.key(srcH, srcID, dstH)
	for _, e := range f.Entries {
		if l.key(e.SourceHarness, e.SourceSessionID, e.DestHarness) == want {
			return e, true
		}
	}
	return LedgerEntry{}, false
}

func (l *Ledger) Upsert(e LedgerEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := l.load()
	if err != nil {
		return err
	}
	want := l.key(e.SourceHarness, e.SourceSessionID, e.DestHarness)
	found := false
	for i := range f.Entries {
		if l.key(f.Entries[i].SourceHarness, f.Entries[i].SourceSessionID, f.Entries[i].DestHarness) == want {
			f.Entries[i] = e
			found = true
			break
		}
	}
	if !found {
		f.Entries = append(f.Entries, e)
	}
	return l.save(f)
}
