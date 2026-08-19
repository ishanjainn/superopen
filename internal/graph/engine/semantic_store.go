package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"sort"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func (s *Store) SemanticSearchTerms(ctx context.Context, project string, terms []string, limit int) ([]api.RankedNode, error) {
	queries := make([]semanticVector, 0, minInt(len(terms), 32))
	for _, term := range terms {
		if term == "" || len(queries) >= 32 {
			continue
		}
		vector, ok, err := s.semanticToken(ctx, project, term)
		if err != nil {
			return nil, err
		}
		if !ok {
			vector = sparseSemanticIndex(term)
			normalizeSemantic(&vector)
		}
		queries = append(queries, vector)
	}
	if len(queries) == 0 {
		return nil, nil
	}
	return s.SemanticSearchKeywords(ctx, project, queries, limit)
}

func (s *Store) semanticToken(ctx context.Context, project, token string) (semanticVector, bool, error) {
	var encoded []byte
	err := s.db.QueryRowContext(ctx, `SELECT vector FROM token_vectors WHERE project=? AND token=?
		AND dimensions=? AND quantization='int8-unit'`, project, token, semanticDimensions).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return semanticVector{}, false, nil
	}
	if err != nil {
		return semanticVector{}, false, err
	}
	if len(encoded) != semanticDimensions {
		return semanticVector{}, false, errors.New("invalid semantic token vector length")
	}
	var vector semanticVector
	for dimension, value := range encoded {
		vector[dimension] = float32(int8(value)) / 127
	}
	return vector, true, nil
}

// SemanticSearch performs the pinned normalized int8 cosine scan. RotSQ is
// intentionally reserved for semantic-edge candidate scoring; Superopen stores
// raw 768-byte signed vectors for observable vector search.
func (s *Store) SemanticSearch(ctx context.Context, project string, query semanticVector, limit int) ([]api.RankedNode, error) {
	return s.SemanticSearchKeywords(ctx, project, []semanticVector{query}, limit)
}

// SemanticSearchKeywords matches Superopen's AND-like ranking: every keyword
// is scored independently and a candidate receives the minimum cosine score.
// This prevents a strong match for one term from hiding an unrelated term.
func (s *Store) SemanticSearchKeywords(ctx context.Context, project string, queries []semanticVector, limit int) ([]api.RankedNode, error) {
	if project == "" {
		return nil, errors.New("semantic search project is required")
	}
	if len(queries) == 0 {
		return nil, errors.New("semantic search requires at least one keyword vector")
	}
	if len(queries) > 32 {
		queries = queries[:32]
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 500 {
		limit = 500
	}
	queryVectors := make([][semanticDimensions]int8, 0, len(queries))
	for _, query := range queries {
		normalizeSemantic(&query)
		if semanticCosine(query, query) == 0 {
			continue
		}
		var encoded [semanticDimensions]int8
		for dimension, value := range query {
			if value > 1 {
				value = 1
			} else if value < -1 {
				value = -1
			}
			encoded[dimension] = int8(value * 127)
		}
		queryVectors = append(queryVectors, encoded)
	}
	if len(queryVectors) == 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT n.id,n.project,n.label,n.name,n.qualified_name,n.file_path,
		n.start_line,n.start_column,n.end_line,n.end_column,n.properties,
		v.vector
		FROM node_vectors v JOIN nodes n ON n.id=v.node_id WHERE v.project=? AND n.project=?
		AND v.dimensions=? AND v.quantization='int8-unit'
		AND n.label IN ('Function','Method','Class','Struct','Interface','Enum','Type','Trait')
		ORDER BY n.id`, project, project, semanticDimensions)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []api.RankedNode{}
	for rows.Next() {
		var match api.RankedNode
		var properties string
		var packed []byte
		if err := rows.Scan(&match.ID, &match.Project, &match.Label, &match.Name, &match.QualifiedName,
			&match.Location.File, &match.Location.StartLine, &match.Location.StartColumn,
			&match.Location.EndLine, &match.Location.EndColumn, &properties, &packed); err != nil {
			return nil, err
		}
		if len(packed) != semanticDimensions {
			return nil, errors.New("invalid semantic vector code length")
		}
		_ = json.Unmarshal([]byte(properties), &match.Properties)
		score := float32(1)
		for _, queryVector := range queryVectors {
			if current := int8Cosine(queryVector, packed); current < score {
				score = current
			}
		}
		match.Score = float64(score)
		match.Signals = api.Properties{"semantic": match.Score}
		result = append(result, match)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Score != result[j].Score {
			return result[i].Score > result[j].Score
		}
		return result[i].QualifiedName < result[j].QualifiedName
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func int8Cosine(left [semanticDimensions]int8, right []byte) float32 {
	if len(right) != semanticDimensions {
		return 0
	}
	var dot, leftMagnitude, rightMagnitude float64
	for dimension, leftValue := range left {
		rightValue := int8(right[dimension])
		dot += float64(leftValue) * float64(rightValue)
		leftMagnitude += float64(leftValue) * float64(leftValue)
		rightMagnitude += float64(rightValue) * float64(rightValue)
	}
	denominator := math.Sqrt(leftMagnitude) * math.Sqrt(rightMagnitude)
	if denominator < 1e-10 {
		return 0
	}
	return float32(dot / denominator)
}

func semanticVectorForText(text string, model *pretrainedVectors, corpus *semanticCorpus) semanticVector {
	tokens := semanticTokens(text, semanticTokenLimit)
	var result semanticVector
	for _, token := range tokens {
		weight := float32(1)
		if corpus != nil {
			if idf := corpus.IDF(token); idf > 0 {
				weight = idf
			}
		}
		vector := semanticIndex(token, model)
		addScaledSemantic(&result, &vector, weight)
	}
	normalizeSemantic(&result)
	return result
}
