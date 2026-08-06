// Package port implements hub-and-spoke coding-agent session porting.
package port

import "time"

const SchemaVersion = "1.0.0"

// HarnessID identifies a coding-agent store.
type HarnessID string

const (
	HarnessClaude   HarnessID = "claude"
	HarnessCodex    HarnessID = "codex"
	HarnessOpenCode HarnessID = "opencode"
	HarnessCursor   HarnessID = "cursor"
	HarnessPi       HarnessID = "pi"
	HarnessSOHub    HarnessID = "so"
)

// PortableTurn is a text-only turn (v1 fidelity - tools/thinking dropped).
type PortableTurn struct {
	Role      string `json:"role"` // user | assistant
	Text      string `json:"text"`
	Timestamp int64  `json:"timestamp,omitempty"` // unix ms
	Model     string `json:"model,omitempty"`
}

// PortableSession is the hub IR.
type PortableSession struct {
	SchemaVersion   string         `json:"schema_version"`
	SourceHarness   HarnessID      `json:"source_harness"`
	SourceSessionID string         `json:"source_session_id"`
	SourcePath      string         `json:"source_path"`
	CWD             string         `json:"cwd"`
	Title           string         `json:"title"`
	CreatedAt       int64          `json:"created_at,omitempty"`
	UpdatedAt       int64          `json:"updated_at,omitempty"`
	Turns           []PortableTurn `json:"turns"`
	DroppedTurns    int            `json:"dropped_turns"`
	SourceMetadata  map[string]any `json:"source_metadata,omitempty"`
}

// SessionRef is a discoverable session before full parse.
type SessionRef struct {
	Harness         HarnessID `json:"harness"`
	SourceSessionID string    `json:"source_session_id"`
	SourcePath      string    `json:"source_path"`
	Title           string    `json:"title"`
	UpdatedAt       int64     `json:"updated_at,omitempty"`
	CWD             string    `json:"cwd,omitempty"`
	Imported        bool      `json:"imported,omitempty"`
	SourceChanged   bool      `json:"source_changed,omitempty"`
}

// ExportResult is the outcome of writing one session.
type ExportResult struct {
	DestSessionID string `json:"dest_session_id"`
	Skipped       bool   `json:"skipped"`
	Reason        string `json:"reason,omitempty"`
}

func NewPortableSession(src HarnessID, id, path, cwd, title string) PortableSession {
	now := time.Now().UnixMilli()
	return PortableSession{
		SchemaVersion:   SchemaVersion,
		SourceHarness:   src,
		SourceSessionID: id,
		SourcePath:      path,
		CWD:             cwd,
		Title:           title,
		CreatedAt:       now,
		UpdatedAt:       now,
		SourceMetadata:  map[string]any{},
	}
}
