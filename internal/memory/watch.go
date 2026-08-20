package memory

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

type WatchResult struct {
	Taught TeachReport `json:"taught"`
	Faded  []int64     `json:"faded,omitempty"`
}

func WatchOnce(root, dir string) (WatchResult, error) {
	store, err := OpenRoot(root)
	if err != nil {
		return WatchResult{}, err
	}
	defer store.Close()
	return store.watchOnce(dir)
}

func (s *Store) watchOnce(dir string) (WatchResult, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = "."
	}
	seen := map[string]bool{}
	var out WatchResult
	_ = filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return err
		}
		if !teachSuffixes[strings.ToLower(filepath.Ext(p))] {
			return nil
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil
		}
		seen[abs] = true
		rep, err := s.studyFile(abs, "")
		if err != nil {
			return nil
		}
		out.Taught.Inserted += rep.Inserted
		out.Taught.Reinforced += rep.Reinforced
		out.Taught.Edges += rep.Edges
		out.Taught.RecallTested += rep.RecallTested
		out.Taught.RecallVerified += rep.RecallVerified
		out.Taught.Episodes = append(out.Taught.Episodes, rep.Episodes...)
		return nil
	})
	rows, err := s.db.Query(`SELECT `+episodeCols+` FROM memory_episodes WHERE kind=? AND faded=0 AND source=?`, KindTeaching, SourceTeach)
	if err != nil {
		return out, err
	}
	eps, err := s.scanEpisodes(rows)
	if err != nil {
		return out, err
	}
	now := time.Now()
	_ = now
	for _, ep := range eps {
		missing := false
		for _, f := range ep.Files {
			if _, err := os.Stat(f); err != nil {
				missing = true
				break
			}
		}
		if missing && len(ep.Files) > 0 {
			if err := s.Fade(ep.ID); err == nil {
				out.Faded = append(out.Faded, ep.ID)
			}
		}
	}
	_ = s.ClusterTopics()
	return out, nil
}
