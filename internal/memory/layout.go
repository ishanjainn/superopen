package memory

import (
	"math"

	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/zeebo/xxh3"
)

func (s *Store) Layout(maxNodes int) (api.LayoutResult, error) {
	if maxNodes <= 0 {
		maxNodes = 2000
	}
	result := api.LayoutResult{Project: "memory"}
	_ = s.db.QueryRow(`SELECT count(*) FROM memory_episodes WHERE faded=0 AND kind != 'tool'`).Scan(&result.TotalNodes)
	_ = s.db.QueryRow(`SELECT count(*) FROM memory_edges`).Scan(&result.TotalEdges)

	rows, err := s.db.Query(`SELECT `+episodeCols+` FROM memory_episodes WHERE faded=0 ORDER BY pinned DESC, created_at DESC LIMIT ?`, maxNodes)
	if err != nil {
		return result, err
	}
	episodes, err := s.scanEpisodes(rows)
	if err != nil {
		return result, err
	}
	topics, _ := s.ListTopics()
	communityOf := map[int64]string{}
	for _, t := range topics {
		for _, id := range t.EpisodeIDs {
			communityOf[id] = t.Label
		}
	}
	nodes := make([]api.LayoutNode, 0, len(episodes))
	keep := map[int64]struct{}{}
	topicIndex := map[string]int{}
	nextTopic := 0
	for _, ep := range episodes {
		if ep.Kind == KindTool {
			continue
		}
		keep[ep.ID] = struct{}{}
		comm := communityOf[ep.ID]
		if comm == "" {
			comm = ep.Kind
		}
		ti, ok := topicIndex[comm]
		if !ok {
			ti = nextTopic
			topicIndex[comm] = ti
			nextTopic++
		}
		// 3D galaxy cluster (same camera as the code graph): topic as a
		// longitude slice, hash for latitude and radius jitter.
		theta := hash01(ep.UID+"t")*2*math.Pi + float64(ti)*0.85
		phi := (hash01(ep.UID+"p") - 0.5) * math.Pi * 0.78
		radius := 70 + float64(ti)*10 + hash01(ep.UID+"r")*42
		if ep.Kind == KindSession {
			radius *= 0.55
		}
		cp := math.Cos(phi)
		node := api.LayoutNode{
			ID:            ep.ID,
			X:             cp * math.Cos(theta) * radius,
			Y:             cp * math.Sin(theta) * radius,
			Z:             math.Sin(phi) * radius * 0.88,
			Label:         layoutLabel(ep.Kind),
			Name:          firstLine(ep.Title, 48),
			QualifiedName: ep.Kind + ":" + ep.UID[:min(8, len(ep.UID))],
			FilePath:      firstFile(ep.Files),
			Degree:        1,
			Size:          sizeForKind(ep.Kind, ep.Pinned),
			Color:         colorForKind(ep.Kind),
			Community:     comm,
		}
		nodes = append(nodes, node)
	}
	edgeRows, err := s.db.Query(`SELECT source_id, target_id, type FROM memory_edges`)
	if err != nil {
		return result, err
	}
	defer edgeRows.Close()
	var edges []api.LayoutEdge
	for edgeRows.Next() {
		var e api.LayoutEdge
		if err := edgeRows.Scan(&e.Source, &e.Target, &e.Type); err != nil {
			return result, err
		}
		if _, ok := keep[e.Source]; !ok {
			continue
		}
		if _, ok := keep[e.Target]; !ok {
			continue
		}
		edges = append(edges, e)
	}
	result.Nodes = nodes
	result.Edges = edges
	return result, edgeRows.Err()
}

func layoutLabel(kind string) string {
	switch kind {
	case KindPrompt:
		return "Prompt"
	case KindTool:
		return "Tool"
	case KindSession:
		return "Session"
	case KindPin:
		return "Pin"
	case KindTeaching:
		return "Teaching"
	case KindWorking:
		return "Working"
	default:
		return "Memory"
	}
}

func sizeForKind(kind string, pinned bool) float64 {
	size := 6.5
	switch kind {
	case KindSession:
		size = 11
	case KindPin, KindTeaching:
		size = 8.5
	case KindWorking:
		size = 7.5
	}
	if pinned {
		size += 2
	}
	return size
}

func colorForKind(kind string) string {
	switch kind {
	case KindPrompt:
		return "#06b6d4"
	case KindTool:
		return "#3b82f6"
	case KindSession:
		return "#eab308"
	case KindPin:
		return "#ec4899"
	case KindTeaching:
		return "#22c55e"
	case KindWorking:
		return "#f97316"
	default:
		return "#a855f7"
	}
}

func firstFile(files []string) string {
	if len(files) == 0 {
		return ""
	}
	return files[0]
}

func hash01(s string) float64 {
	return float64(xxh3.Hash([]byte(s))%1_000_000) / 1_000_000
}
