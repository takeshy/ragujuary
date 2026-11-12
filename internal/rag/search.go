package rag

import (
	"math"
	"sort"
)

// SearchResult represents a single search result
type SearchResult struct {
	Text                string   `json:"text"`
	FilePath            string   `json:"file_path"`
	Score               float64  `json:"score"`
	ContentType         string   `json:"content_type,omitempty"`
	PageLabel           string   `json:"page_label,omitempty"`
	RerankScore         *float64 `json:"rerank_score,omitempty"`
	RetrievalSource     string   `json:"retrieval_source,omitempty"`
	RerankSkippedReason string   `json:"rerank_skipped_reason,omitempty"`
}

type scoredChunk struct {
	index  int
	score  float64
	source string
}

// Search finds the most similar chunks to the query vector
func Search(queryVec []float32, index *RagIndex, vectors []float32, topK int, minScore float64) []SearchResult {
	return chunkResults(index, vectorRanks(queryVec, index, vectors, topK, minScore))
}

func vectorRanks(queryVec []float32, index *RagIndex, vectors []float32, topK int, minScore float64) []scoredChunk {
	if index == nil || len(index.Meta) == 0 || len(vectors) == 0 {
		return nil
	}

	dim := index.Dimension

	if dim <= 0 || len(queryVec) != dim || len(vectors)/dim < len(index.Meta) {
		return nil
	}
	scores := make([]scoredChunk, 0, len(index.Meta))
	for i := range index.Meta {
		chunkVec := vectors[i*dim : (i+1)*dim]
		score := cosineSimilarity(queryVec, chunkVec)
		if score >= minScore {
			scores = append(scores, scoredChunk{index: i, score: score})
		}
	}

	return sortChunks(scores, topK)
}

func chunkResults(index *RagIndex, scores []scoredChunk) []SearchResult {
	results := make([]SearchResult, len(scores))
	for i, s := range scores {
		results[i] = SearchResult{
			Text:            index.Meta[s.index].Text,
			FilePath:        index.Meta[s.index].FilePath,
			Score:           s.score,
			ContentType:     index.Meta[s.index].ContentType,
			PageLabel:       index.Meta[s.index].PageLabel,
			RetrievalSource: s.source,
		}
	}

	return results
}

func sortChunks(scores []scoredChunk, topK int) []scoredChunk {
	sort.Slice(scores, func(a, b int) bool {
		if scores[a].score == scores[b].score {
			return scores[a].index < scores[b].index
		}
		return scores[a].score > scores[b].score
	})
	if topK > 0 && len(scores) > topK {
		return scores[:topK]
	}
	return scores
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
