package memory

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/leiden"
)

func (s *Store) ClusterTopics() error {
	rows, err := s.db.Query(`SELECT id FROM memory_episodes WHERE faded=0 AND kind NOT IN ('tool','working')`)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	_ = rows.Close()
	if len(ids) == 0 {
		_, err := s.db.Exec(`DELETE FROM memory_topics`)
		return err
	}

	edgeRows, err := s.db.Query(`SELECT source_id, target_id, COALESCE(weight,1) FROM memory_edges`)
	if err != nil {
		return err
	}
	var leidenEdges []leiden.Edge
	seen := map[[2]int64]float64{}
	degree := map[int64]float64{}
	for edgeRows.Next() {
		var a, b int64
		var w float64
		if edgeRows.Scan(&a, &b, &w) != nil || a == 0 || b == 0 {
			continue
		}
		if w <= 0 {
			w = 1
		}
		key := [2]int64{a, b}
		if a > b {
			key = [2]int64{b, a}
		}
		seen[key] += w
		degree[a] += w
		degree[b] += w
	}
	_ = edgeRows.Close()
	for key, w := range seen {
		leidenEdges = append(leidenEdges, leiden.Edge{Source: key[0], Target: key[1], Weight: w})
	}

	vecRows, err := s.db.Query(`
SELECT e.id, v.vector FROM memory_vectors v
JOIN memory_episodes e ON e.id=v.episode_id
WHERE v.embedder_id=? AND e.faded=0 AND e.kind NOT IN ('tool','working')
ORDER BY e.id DESC LIMIT 400`, CurrentEmbedder())
	if err == nil {
		type item struct {
			id  int64
			vec []byte
		}
		var items []item
		for vecRows.Next() {
			var it item
			if vecRows.Scan(&it.id, &it.vec) == nil {
				items = append(items, it)
			}
		}
		_ = vecRows.Close()
		limit := 80
		if len(items) < limit {
			limit = len(items)
		}
		for i := 0; i < limit; i++ {
			left, ok := vectorFromBytes(items[i].vec)
			if !ok {
				continue
			}
			bestID, best := int64(0), 0.72
			for j := i + 1; j < limit; j++ {
				c := Cosine(left, items[j].vec)
				if c > best {
					best = c
					bestID = items[j].id
				}
			}
			if bestID != 0 {
				leidenEdges = append(leidenEdges, leiden.Edge{Source: items[i].id, Target: bestID, Weight: best})
			}
		}
	}

	membership := leiden.Detect(ids, leidenEdges, 1.0)
	groups := map[int][]int64{}
	commOf := map[int64]int{}
	for _, m := range membership {
		groups[m.Community] = append(groups[m.Community], m.NodeID)
		commOf[m.NodeID] = m.Community
	}
	prev, _ := s.ListTopics()
	commOf = stabilizeCommunities(groups, commOf, prev)
	groups = map[int][]int64{}
	for id, c := range commOf {
		groups[c] = append(groups[c], id)
	}
	now := nowRFC()
	communities := make([]int, 0, len(groups))
	for c := range groups {
		communities = append(communities, c)
	}
	sort.Ints(communities)
	type topicRow struct {
		label   string
		members []int64
		id      int
	}
	rowsOut := make([]topicRow, 0, len(communities))
	for _, c := range communities {
		members := groups[c]
		sort.Slice(members, func(i, j int) bool { return members[i] < members[j] })
		rowsOut = append(rowsOut, topicRow{label: s.topicLabel(members), members: members, id: c})
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM memory_topics`); err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, row := range rowsOut {
		raw, _ := json.Marshal(row.members)
		if _, err := tx.Exec(`INSERT INTO memory_topics(label,community,episode_ids,size,updated_at) VALUES(?,?,?,?,?)`,
			row.label, row.id, string(raw), len(row.members), now); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	maxD := 1.0
	for _, d := range degree {
		if d > maxD {
			maxD = d
		}
	}
	for _, id := range ids {
		cent := degree[id] / maxD
		_, _ = s.db.Exec(`UPDATE memory_episodes SET community_id=?, centrality=? WHERE id=?`, strconv.Itoa(commOf[id]), cent, id)
	}
	return nil
}

func (s *Store) topicLabel(ids []int64) string {
	if len(ids) == 0 {
		return "topic"
	}
	files := map[string]int{}
	titles := []string{}
	for _, id := range ids {
		var title, fileBlob string
		if err := s.db.QueryRow(`SELECT title, files FROM memory_episodes WHERE id=?`, id).Scan(&title, &fileBlob); err != nil {
			continue
		}
		if title != "" {
			titles = append(titles, title)
		}
		for _, f := range splitFiles(fileBlob) {
			base := filepath.Base(filepath.Dir(f))
			if base == "" || base == "." || base == "/" {
				base = filepath.Base(f)
			}
			if base != "" {
				files[base]++
			}
		}
	}
	best, bestN := "", 0
	for k, n := range files {
		if n > bestN {
			best, bestN = k, n
		}
	}
	if best != "" && bestN >= 2 {
		return best
	}
	if len(titles) > 0 {
		words := strings.Fields(strings.ToLower(titles[0]))
		if len(words) > 4 {
			words = words[:4]
		}
		if len(words) > 0 {
			return strings.Join(words, " ")
		}
	}
	return "topic"
}

func stabilizeCommunities(groups map[int][]int64, commOf map[int64]int, prev []Topic) map[int64]int {
	if len(prev) == 0 || len(commOf) == 0 {
		return commOf
	}
	usedPrev := map[int]bool{}
	remap := map[int]int{}
	type pair struct {
		cur, old int
		overlap  int
	}
	var pairs []pair
	for cur, members := range groups {
		set := map[int64]struct{}{}
		for _, id := range members {
			set[id] = struct{}{}
		}
		for _, t := range prev {
			n := 0
			for _, id := range t.EpisodeIDs {
				if _, ok := set[id]; ok {
					n++
				}
			}
			if n > 0 {
				pairs = append(pairs, pair{cur: cur, old: t.Community, overlap: n})
			}
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].overlap > pairs[j].overlap })
	for _, p := range pairs {
		if _, ok := remap[p.cur]; ok || usedPrev[p.old] {
			continue
		}
		remap[p.cur] = p.old
		usedPrev[p.old] = true
	}
	next := 0
	for _, t := range prev {
		if t.Community >= next {
			next = t.Community + 1
		}
	}
	out := map[int64]int{}
	assigned := map[int]int{}
	for id, cur := range commOf {
		if dst, ok := remap[cur]; ok {
			out[id] = dst
			continue
		}
		if dst, ok := assigned[cur]; ok {
			out[id] = dst
			continue
		}
		assigned[cur] = next
		out[id] = next
		next++
	}
	return out
}
