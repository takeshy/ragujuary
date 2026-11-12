package rag

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
)

const maxSearchUntilFoundCandidates = 1000

func (c Config) warn(message string) {
	if c.OnWarning != nil {
		c.OnWarning(message)
	} else {
		fmt.Fprintln(os.Stderr, "Warning: "+message)
	}
}

// Textless media cannot be judged from their filenames or generated labels.
// PDF text extracted during indexing is usable; a PDF label on its own is not.
func rerankSkipReason(result SearchResult) string {
	kind := strings.ToLower(strings.TrimSpace(result.ContentType))
	text := strings.TrimSpace(result.Text)
	if kind == "" || kind == "text" {
		for _, media := range []string{"image", "pdf", "audio", "video"} {
			if strings.HasPrefix(strings.ToLower(text), "["+media+":") {
				kind = media
				break
			}
		}
	}
	switch kind {
	case "", "text":
	case "pdf":
		if strings.HasPrefix(strings.ToLower(text), "[pdf:") {
			// Native and external PDF placeholders occupy the first line.
			if newline := strings.IndexByte(text, '\n'); newline >= 0 {
				text = strings.TrimSpace(text[newline+1:])
			} else {
				text = ""
			}
		}
	default:
		return "media content is unavailable for scoring"
	}
	if text == "" {
		return "no indexed text is available for scoring"
	}
	return ""
}

// rerankBatch keeps unscorable candidates anchored at their retrieval positions.
// Eligible candidates fill only eligible positions after scoring. A failed or
// incomplete call returns no partial ordering, so the caller can restore it all.
func (e *Engine) rerankBatch(ctx context.Context, question string, original []SearchResult) ([]SearchResult, int, error) {
	output := append([]SearchResult(nil), original...)
	var eligible []SearchResult
	var positions []int
	skipped := 0
	for i := range output {
		if reason := rerankSkipReason(output[i]); reason != "" {
			output[i].RerankSkippedReason = reason
			output[i].RerankScore = nil
			skipped++
			continue
		}
		eligible = append(eligible, output[i])
		positions = append(positions, i)
	}
	if len(eligible) == 0 {
		return output, skipped, nil
	}
	if e.reranker == nil {
		return nil, skipped, fmt.Errorf("Gemini API key or reranker is not configured")
	}
	ranked, err := e.reranker.Rerank(ctx, question, eligible)
	if err != nil {
		return nil, skipped, err
	}
	if len(ranked) != len(eligible) {
		return nil, skipped, fmt.Errorf("Gemini returned incomplete relevance scores")
	}
	for i, result := range ranked {
		if result.RerankScore == nil || math.IsNaN(*result.RerankScore) || math.IsInf(*result.RerankScore, 0) || *result.RerankScore < 0 || *result.RerankScore > 3 {
			return nil, skipped, fmt.Errorf("Gemini returned an invalid or missing relevance score")
		}
		output[positions[i]] = result
	}
	return output, skipped, nil
}

func originalWithSkipReasons(original []SearchResult) []SearchResult {
	output := append([]SearchResult(nil), original...)
	for i := range output {
		output[i].RerankScore = nil
		output[i].RerankSkippedReason = rerankSkipReason(output[i])
	}
	return output
}

func (e *Engine) rerankRetrieved(ctx context.Context, question string, original []SearchResult, config Config) ([]SearchResult, error) {
	batchSize := len(original)
	if config.SearchUntilFound {
		batchSize = config.RerankCandidateK
		if batchSize == 0 {
			batchSize = 40
		}
		if batchSize < config.TopK {
			batchSize = config.TopK
		}
	}
	for offset := 0; offset < len(original); offset += batchSize {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := offset + batchSize
		if end > len(original) {
			end = len(original)
		}
		ranked, skipped, err := e.rerankBatch(ctx, question, original[offset:end])
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			config.warn(fmt.Sprintf("Gemini reranking failed: %v. Returning results in their original search order.", err))
			return originalWithSkipReasons(original), nil
		}
		if skipped > 0 {
			if config.SearchUntilFound {
				config.warn(fmt.Sprintf("%d candidate(s) have no scoreable content. Stopped search-until-found and kept the original search order; unscored content is not a low relevance score.", skipped))
				return originalWithSkipReasons(original), nil
			}
			config.warn(fmt.Sprintf("%d candidate(s) have no scoreable content and were kept at their original search positions.", skipped))
		}
		if !config.SearchUntilFound {
			return ranked, nil
		}
		var answers []SearchResult
		for _, result := range ranked {
			if *result.RerankScore >= 2 {
				answers = append(answers, result)
			}
		}
		if len(answers) > 0 {
			return answers, nil
		}
	}
	if config.SearchUntilFound && len(original) >= maxSearchUntilFoundCandidates {
		config.warn(fmt.Sprintf("No answer with relevance >= 2/3 was found within the %d-candidate search limit.", maxSearchUntilFoundCandidates))
	}
	return nil, nil
}
