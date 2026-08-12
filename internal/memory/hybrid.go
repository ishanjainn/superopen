package memory

import (
	"fmt"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/retrieve"
)

// HybridSearch merges memory hits with harness corpus retrieve hits.
func (s *Store) HybridSearch(q string, limit int) ([]SearchHit, error) {
	return s.HybridSearchVendor(q, limit, "")
}

// HybridSearchVendor is HybridSearch with session-vendor weighting on corpus hits.
func (s *Store) HybridSearchVendor(q string, limit int, vendor string) ([]SearchHit, error) {
	if limit <= 0 {
		limit = 20
	}
	memHits, err := s.Search(q, limit)
	if err != nil {
		return nil, err
	}
	corpus, err := retrieve.SearchWith(s.Paths, q, retrieve.SearchOptions{Limit: limit, Vendor: vendor})
	if err != nil {
		return memHits, nil
	}
	seen := map[string]bool{}
	key := func(snippet string) string {
		s := strings.ToLower(strings.TrimSpace(snippet))
		if len(s) > 80 {
			s = s[:80]
		}
		return s
	}
	var out []SearchHit
	for _, h := range memHits {
		k := key(h.Snippet)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, h)
	}
	for _, h := range corpus {
		// Skip pure memory-kind corpus docs to avoid double-counting preferences etc.
		if h.Kind == "memory" {
			continue
		}
		snippet := h.Snippet
		if snippet == "" {
			snippet = h.Path
		}
		k := key(snippet)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, SearchHit{
			Kind:    "corpus",
			ID:      h.Path,
			Snippet: fmt.Sprintf("[%s] %s", h.Kind, truncate(snippet, 220)),
			Score:   h.Score,
		})
	}
	// re-sort
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Score > out[i].Score {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// RefreshActive rebuilds context.md (Write→Store→Inject).
func (s *Store) RefreshActive(query string) (ContextPack, error) {
	return s.BuildSessionContext(12000, query, ModePersistent)
}

// NotePort records an episodic breadcrumb after a successful session port and refreshes ACTIVE.
func (s *Store) NotePort(from, to, sourceID, title string) error {
	_ = s.Ensure()
	text := fmt.Sprintf("ported %s→%s %s", from, to, sourceID)
	if title != "" {
		text += " - " + truncate(title, 120)
	}
	_ = s.AddEpisodic(EpisodicEntry{
		Text:      text,
		Tags:      []string{"port", from, to},
		CreatedAt: time.Now().UTC(),
	}, ModePersistent)
	_, err := s.RefreshActive(title)
	return err
}
