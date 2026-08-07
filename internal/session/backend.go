package session

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/projects"
	"github.com/ishanjainn/superopen/internal/tracestore"
)

// CommitRef is a git commit attributed to a session.
type CommitRef struct {
	SHA     string    `json:"sha"`
	Message string    `json:"message,omitempty"`
	At      time.Time `json:"at,omitempty"`
}

// PRRef is a pull request attributed to a session.
type PRRef struct {
	URL    string    `json:"url,omitempty"`
	Number int       `json:"number,omitempty"`
	Title  string    `json:"title,omitempty"`
	At     time.Time `json:"at,omitempty"`
}

// AttributionSummary is agent-vs-human line contribution at commit time.
type AttributionSummary struct {
	AgentPercent   float64 `json:"agent_percent"`
	AgentLines     int     `json:"agent_lines"`
	HumanLines     int     `json:"human_lines"`
	TotalChanged   int     `json:"total_changed"`
	Display        string  `json:"display,omitempty"` // e.g. "73% agent (146/200 lines)"
}

// Filter selects sessions across backends.
type Filter struct {
	ProjectID string // empty or "all" = all registered / current
	Query     string
	Commit    string
	PR        string // number or URL substring
	SessionID string
	Limit     int
}

// Backend reads sessions from one or more sources.
type Backend interface {
	List(ctx context.Context, filter Filter) ([]ListItem, error)
	Get(ctx context.Context, projectID, sessionID string) (Meta, error)
	StoreFor(projectID string) (*Store, harness.Paths, error)
}

// LocalMulti lists sessions from registered .so roots (and optional current paths).
type LocalMulti struct {
	// Current is the cwd project's paths (always included).
	Current     harness.Paths
	CurrentRoot string
	CurrentID   string
}

func NewLocalMulti(repoRoot string, paths harness.Paths) *LocalMulti {
	id := ""
	if p, err := projects.Register(repoRoot, paths.Root, ""); err == nil {
		id = p.ID
	}
	return &LocalMulti{Current: paths, CurrentRoot: repoRoot, CurrentID: id}
}

type projectBinding struct {
	ID       string
	Name     string
	RepoRoot string
	Paths    harness.Paths
}

func (l *LocalMulti) bindings(filter Filter) ([]projectBinding, error) {
	seen := map[string]bool{}
	var out []projectBinding
	add := func(id, name, root string, paths harness.Paths) {
		key := paths.SessionsDir
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, projectBinding{ID: id, Name: name, RepoRoot: root, Paths: paths})
	}
	if l.Current.Root != "" {
		add(l.CurrentID, "", l.CurrentRoot, l.Current)
	}
	projs, err := projects.ResolveFilter(filter.ProjectID)
	if err == nil {
		for _, p := range projs {
			add(p.ID, p.Name, p.RepoRoot, harness.Resolve(p.RepoRoot))
		}
	} else if filter.ProjectID != "" && filter.ProjectID != "all" {
		return nil, err
	}
	return out, nil
}

func (l *LocalMulti) List(ctx context.Context, filter Filter) ([]ListItem, error) {
	_ = ctx
	binds, err := l.bindings(filter)
	if err != nil {
		return nil, err
	}
	var out []ListItem
	for _, b := range binds {
		ss := NewStore(b.Paths)
		var items []ListItem
		if filter.Query != "" {
			items, err = ss.Search(filter.Query)
		} else {
			items, err = ss.ListDetailed()
		}
		if err != nil {
			continue
		}
		for i := range items {
			items[i].ProjectID = b.ID
			if items[i].ProjectID == "" {
				items[i].ProjectID = l.CurrentID
			}
			items[i].RepoRoot = b.RepoRoot
			if items[i].RepoRoot == "" {
				items[i].RepoRoot = l.CurrentRoot
			}
			if filter.Commit != "" && !metaHasCommit(items[i].Meta, filter.Commit) {
				continue
			}
			if filter.PR != "" && !metaHasPR(items[i].Meta, filter.PR) {
				continue
			}
			if filter.SessionID != "" && items[i].ID != filter.SessionID {
				continue
			}
			out = append(out, items[i])
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

func (l *LocalMulti) Get(ctx context.Context, projectID, sessionID string) (Meta, error) {
	_ = ctx
	f := Filter{ProjectID: projectID}
	if projectID == "" {
		f.ProjectID = "all"
	}
	binds, err := l.bindings(f)
	if err != nil {
		return Meta{}, err
	}
	for _, b := range binds {
		if projectID != "" && projectID != "all" && b.ID != projectID && b.RepoRoot != projectID {
			continue
		}
		ss := NewStore(b.Paths)
		m, err := ss.Get(sessionID)
		if err != nil {
			continue
		}
		m.ProjectID = b.ID
		m.RepoRoot = b.RepoRoot
		return m, nil
	}
	return Meta{}, fmt.Errorf("session not found: %s", sessionID)
}

func (l *LocalMulti) StoreFor(projectID string) (*Store, harness.Paths, error) {
	if projectID == "" || projectID == "all" {
		return NewStore(l.Current), l.Current, nil
	}
	p, err := projects.Get(projectID)
	if err != nil {
		return NewStore(l.Current), l.Current, nil
	}
	paths := harness.Resolve(p.RepoRoot)
	return NewStore(paths), paths, nil
}

func metaHasCommit(m Meta, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" {
		return true
	}
	if strings.HasPrefix(strings.ToLower(m.HeadSHA), needle) || strings.HasPrefix(strings.ToLower(m.BaseSHA), needle) {
		return true
	}
	for _, c := range m.Commits {
		if strings.HasPrefix(strings.ToLower(c.SHA), needle) {
			return true
		}
	}
	return false
}

func metaHasPR(m Meta, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" {
		return true
	}
	for _, pr := range m.PullRequests {
		if strings.Contains(strings.ToLower(pr.URL), needle) {
			return true
		}
		if fmt.Sprintf("%d", pr.Number) == needle {
			return true
		}
		if strings.Contains(strings.ToLower(pr.Title), needle) {
			return true
		}
	}
	return false
}

// Composite merges local disk with an optional remote backend (paid).
type Composite struct {
	Local  Backend
	Remote Backend // may be nil; when set, results are merged
}

func (c *Composite) List(ctx context.Context, filter Filter) ([]ListItem, error) {
	local, err := c.Local.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	if c.Remote == nil {
		return local, nil
	}
	remote, err := c.Remote.List(ctx, filter)
	if err != nil {
		return local, nil // degrade gracefully
	}
	seen := map[string]bool{}
	var out []ListItem
	for _, it := range local {
		key := it.ProjectID + "/" + it.ID
		seen[key] = true
		out = append(out, it)
	}
	for _, it := range remote {
		key := it.ProjectID + "/" + it.ID
		if seen[key] {
			continue
		}
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out, nil
}

func (c *Composite) Get(ctx context.Context, projectID, sessionID string) (Meta, error) {
	m, err := c.Local.Get(ctx, projectID, sessionID)
	if err == nil {
		return m, nil
	}
	if c.Remote == nil {
		return Meta{}, err
	}
	return c.Remote.Get(ctx, projectID, sessionID)
}

func (c *Composite) StoreFor(projectID string) (*Store, harness.Paths, error) {
	return c.Local.StoreFor(projectID)
}

// Ensure ListItem embeds enriched Meta fields - ProjectID on ListItem via Meta.
var _ Backend = (*LocalMulti)(nil)
var _ = tracestore.Span{}
