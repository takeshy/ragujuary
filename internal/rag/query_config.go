package rag

import (
	"fmt"
	"math"
)

// ValidateQuery rejects invalid options before making an API request.
func (c Config) ValidateQuery() error {
	if c.SearchUntilFound && !c.Rerank {
		return fmt.Errorf("search until found requires reranking; enable rerank or disable search until found")
	}
	switch c.SearchMode {
	case "", "vector", "keyword", "hybrid":
	default:
		return fmt.Errorf("search mode must be vector, keyword, or hybrid")
	}
	if c.TopK < 0 {
		return fmt.Errorf("top K must be non-negative")
	}
	if math.IsNaN(c.MinScore) || math.IsInf(c.MinScore, 0) || c.MinScore < -1 || c.MinScore > 1 {
		return fmt.Errorf("min score must be between -1 and 1 (vector similarity)")
	}
	if c.CandidateK < 0 || c.CandidateK > 1000 {
		return fmt.Errorf("candidate K must be between 1 and 1000, or 0 for the default (30)")
	}
	if c.RerankCandidateK < 0 || c.RerankCandidateK > 1000 {
		return fmt.Errorf("rerank candidate K must be between 1 and 1000, or 0 for the default (40)")
	}
	if math.IsNaN(c.RRFK) || math.IsInf(c.RRFK, 0) || c.RRFK < 0 {
		return fmt.Errorf("RRF k must be positive, or 0 for the default (60)")
	}
	if c.Rerank && c.TopK > 1000 {
		return fmt.Errorf("top K must be at most 1000 when reranking")
	}
	return nil
}
