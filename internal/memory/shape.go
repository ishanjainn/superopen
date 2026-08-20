package memory

import "sync"

const shapeDims = 10000

var (
	shapePOnce sync.Once
	shapeP     []int8
)

func (s *Store) writeShape(episodeID int64, vec Vector) error {
	blob := bindShape(vec)
	_, err := s.db.Exec(`INSERT INTO memory_shapes(episode_id, blob) VALUES(?,?)
ON CONFLICT(episode_id) DO UPDATE SET blob=excluded.blob`, episodeID, blob)
	return err
}

func initShapeP() {
	shapeP = make([]int8, shapeDims*embedDimensions)
	for i := 0; i < shapeDims; i++ {
		seed := uint64(i+1) * 0x9E3779B97F4A7C15
		off := i * embedDimensions
		for j := 0; j < embedDimensions; j++ {
			seed ^= seed >> 12
			seed *= 0xBF58476D1CE4E5B9
			if seed&1 == 1 {
				shapeP[off+j] = 1
			} else {
				shapeP[off+j] = -1
			}
		}
	}
}

func bindShape(vec Vector) []byte {
	shapePOnce.Do(initShapeP)
	bits := make([]byte, (shapeDims+7)/8)
	for i := 0; i < shapeDims; i++ {
		acc := 0
		off := i * embedDimensions
		for j := 0; j < embedDimensions; j++ {
			acc += int(vec[j]) * int(shapeP[off+j])
		}
		if acc >= 0 {
			bits[i/8] |= 1 << (uint(i) % 8)
		}
	}
	return bits
}

func shapeScore(a, b []byte) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	diff := 0
	total := len(a) * 8
	for i := range a {
		x := a[i] ^ b[i]
		for x != 0 {
			x &= x - 1
			diff++
		}
	}
	return 1 - 2*float64(diff)/float64(total)
}

func (s *Store) RecallShape(cue string, limit int) ([]Hit, error) {
	if limit <= 0 {
		limit = 8
	}
	q := EmbedText(cue)
	qshape := bindShape(q)
	rows, err := s.db.Query(`SELECT episode_id, blob FROM memory_shapes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type pair struct {
		id    int64
		score float64
	}
	var scored []pair
	for rows.Next() {
		var id int64
		var blob []byte
		if rows.Scan(&id, &blob) != nil {
			continue
		}
		scored = append(scored, pair{id: id, score: shapeScore(qshape, blob)})
	}
	hits := make([]Hit, 0, limit)
	for i := 0; i < len(scored) && len(hits) < limit; i++ {
		best := i
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[best].score {
				best = j
			}
		}
		scored[i], scored[best] = scored[best], scored[i]
		ep, err := s.Get(scored[i].id)
		if err != nil || ep.Faded || ep.Kind == KindTool {
			continue
		}
		ep.Score = scored[i].score
		hits = append(hits, Hit{Episode: ep, Snippet: firstLine(ep.Text, 140)})
	}
	return hits, nil
}
