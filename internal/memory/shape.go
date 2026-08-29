package memory

// RecallShape is a compatibility alias for Search. Shape/HD vectors are no
// longer written or scored; hybrid FTS+dense ranking replaced that path.
func (s *Store) RecallShape(cue string, limit int) ([]Hit, error) {
	if limit <= 0 {
		limit = 8
	}
	return s.Search(SearchFilter{Query: cue, Limit: limit})
}
