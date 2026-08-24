package harvest

import "time"

const (
	StatusProposed = "proposed"
	StatusPending  = "pending"
	StatusSkipped  = "skipped"
	StatusFailed   = "failed"

	StatusOpen     = "open"
	StatusApplied  = "applied"
	StatusDeclined = "declined"
	StatusNoop     = "noop"
	StatusStale    = "stale"

	KindImprove   = "improve"
	KindCreate    = "create"
	KindSimplify  = "simplify"
	KindPrinciple = "principle"

	MaxProposals = 3
)

type Evidence struct {
	Kind   string `json:"kind"` // session | memory | graph
	ID     string `json:"id,omitempty"`
	SpanID string `json:"span_id,omitempty"`
	QN     string `json:"qn,omitempty"`
	Path   string `json:"path,omitempty"`
	Label  string `json:"label,omitempty"`
}

type File struct {
	Path      string `json:"path"`
	Hash      string `json:"hash"`
	Bytes     int    `json:"bytes"`
	Protected bool   `json:"protected"`
}

type Run struct {
	ID        int64  `json:"id"`
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	Provider  string `json:"provider,omitempty"`
	Skipped   string `json:"skipped,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Proposal struct {
	ID         int64      `json:"id"`
	SessionID  string     `json:"session_id,omitempty"`
	Status     string     `json:"status"`
	Kind       string     `json:"kind"`
	Target     string     `json:"target"`
	Title      string     `json:"title"`
	Reason     string     `json:"reason"`
	Issue      string     `json:"issue,omitempty"`
	Suggestion string     `json:"suggestion,omitempty"`
	Diff       string     `json:"diff,omitempty"`
	BaseHash   string     `json:"base_hash,omitempty"`
	BaseMtime  string     `json:"base_mtime,omitempty"`
	Evidence   []Evidence `json:"evidence,omitempty"`
	Plus       int        `json:"plus,omitempty"`
	Minus      int        `json:"minus,omitempty"`
	CreatedAt  string     `json:"created_at"`
	UpdatedAt  string     `json:"updated_at"`
}

type ProposeInput struct {
	SessionID  string     `json:"session_id"`
	Kind       string     `json:"kind"`
	Target     string     `json:"target"`
	Title      string     `json:"title"`
	Reason     string     `json:"reason"`
	Issue      string     `json:"issue"`
	Suggestion string     `json:"suggestion"`
	Diff       string     `json:"diff"`
	Evidence   []Evidence `json:"evidence"`
	Provider   string     `json:"provider,omitempty"`
}

type GenerateResult struct {
	SessionID string `json:"session_id"`
	Provider  string `json:"provider,omitempty"`
	Inserted  int    `json:"inserted"`
	Skipped   string `json:"skipped,omitempty"`
	Pending   bool   `json:"pending,omitempty"`
}

func nowRFC() string { return time.Now().UTC().Format(time.RFC3339) }
