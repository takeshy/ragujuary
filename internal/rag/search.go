package rag

import (
	"math"
	"sort"
)

// SearchResult represents a single search result
type SearchResult struct {
	Text     string  `json:"text"`
	FilePath string  `json:"file_path"`
	Score    float64 `json:"score"`
}

// Search finds the most similar chunks to the query vector
func Search(queryVec []float32, index *RagIndex, vectors []float32, topK int, minScore float64) []SearchResult {
	if index == nil || len(index.Meta) == 0 || len(vectors) == 0 {
		return nil
	}

	dim := index.Dimension

	type scored struct {
		index int
		score float64
	}

	scores := make([]scored, 0, len(index.Meta))
	for i := range index.Meta {
		chunkVec := vectors[i*dim : (i+1)*dim]
		score := cosineSimilarity(queryVec, chunkVec)
		if score >= minScore {
			scores = append(scores, scored{index: i, score: score})
		}
	}

	sort.Slice(scores, func(a, b int) bool {
		return scores[a].score > scores[b].score
	})

	if topK > 0 && len(scores) > topK {
		scores = scores[:topK]
	}

	results := make([]SearchResult, len(scores))
	for i, s := range scores {
		results[i] = SearchResult{
			Text:     index.Meta[s.index].Text,
			FilePath: index.Meta[s.index].FilePath,
			Score:    s.score,
		}
	}

	return results
}

// cosineSimilarity computes the cosine similarity between two vectors
func cosineSimilarity(a, b []float32) float64 {
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}
