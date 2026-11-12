package rag

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type rerankFunc func(context.Context, string, []SearchResult) ([]SearchResult, error)

func (f rerankFunc) Rerank(ctx context.Context, query string, results []SearchResult) ([]SearchResult, error) {
	return f(ctx, query, results)
}

func scoreResults(results []SearchResult, scoreFor func(SearchResult) float64) []SearchResult {
	for i := range results {
		score := scoreFor(results[i])
		results[i].RerankScore = &score
	}
	sort.SliceStable(results, func(i, j int) bool { return *results[i].RerankScore > *results[j].RerankScore })
	return results
}

func captureWarnings(config *Config) *[]string {
	warnings := new([]string)
	config.OnWarning = func(message string) { *warnings = append(*warnings, message) }
	return warnings
}

func sameVectors(count int) []float32 {
	vectors := make([]float32, 2*count)
	for i := 0; i < count; i++ {
		vectors[2*i] = 1
	}
	return vectors
}

func TestDefaultHybridRerankAndExplicitOptOut(t *testing.T) {
	c := &countingQueryClient{}
	calls := 0
	e := NewEngine(c).WithReranker(rerankFunc(func(ctx context.Context, q string, results []SearchResult) ([]SearchResult, error) {
		calls++
		return scoreResults(results, func(r SearchResult) float64 {
			if strings.Contains(r.Text, "needle") {
				return 3
			}
			return 0
		}), nil
	}))
	index := &RagIndex{Dimension: 2, Meta: []ChunkMeta{{Text: "semantic only"}, {Text: "needle answer"}}}
	config := DefaultConfig()
	config.TopK = 1
	got, err := e.queryIndex(context.Background(), "needle", index, []float32{1, 0, 0, 1}, config)
	if err != nil || len(got) != 1 || got[0].Text != "needle answer" || got[0].RetrievalSource != "bm25" || got[0].RerankScore == nil || calls != 1 || c.calls != 1 {
		t.Fatalf("default hybrid + rerank: results=%v err=%v calls=%d/%d", got, err, c.calls, calls)
	}
	config.SearchMode = "vector"
	config.Rerank = false
	got, err = e.queryIndex(context.Background(), "needle", index, []float32{1, 0, 0, 1}, config)
	if err != nil || len(got) != 1 || got[0].Text != "semantic only" || got[0].RetrievalSource != "" || got[0].RerankScore != nil || calls != 1 {
		t.Fatalf("opt out failed: %v %v calls=%d", got, err, calls)
	}
}

func TestRerankFailureRestoresOriginalResultsAndWarns(t *testing.T) {
	for _, failure := range []error{errors.New("API unavailable"), context.DeadlineExceeded} {
		e := NewEngine(&countingQueryClient{}).WithReranker(rerankFunc(func(ctx context.Context, q string, results []SearchResult) ([]SearchResult, error) {
			results[0].Text = "mutated"
			return nil, failure
		}))
		config := DefaultConfig()
		config.TopK = 1
		warnings := captureWarnings(&config)
		index := &RagIndex{Dimension: 2, Meta: []ChunkMeta{{Text: "first"}, {Text: "second"}}}
		got, err := e.queryIndex(context.Background(), "absent", index, sameVectors(2), config)
		if err != nil || len(got) != 1 || got[0].Text != "first" || got[0].RerankScore != nil || len(*warnings) != 1 || !strings.Contains((*warnings)[0], failure.Error()) {
			t.Fatalf("fallback=%v error=%v warnings=%v", got, err, *warnings)
		}
	}
}

func TestMissingRerankerWarnsAfterSuccessfulRetrieval(t *testing.T) {
	c := &countingQueryClient{}
	e := NewEngine(c)
	config := DefaultConfig()
	warnings := captureWarnings(&config)
	got, err := e.queryIndex(context.Background(), "needle", &RagIndex{Dimension: 2, Meta: []ChunkMeta{{Text: "needle"}}}, sameVectors(1), config)
	if err != nil || len(got) != 1 || c.calls != 1 || len(*warnings) != 1 {
		t.Fatalf("got=%v err=%v warnings=%v", got, err, *warnings)
	}
}

func TestRerankProtectsMediaPositionsAndScoresExtractedPDFText(t *testing.T) {
	index := &RagIndex{Dimension: 2, Meta: []ChunkMeta{
		{Text: "[image: cat.png]", ContentType: "image"},
		{Text: "low"},
		{Text: "[Pdf: scanned.pdf]", ContentType: "pdf"},
		{Text: "high"},
		{Text: "[audio: voice.wav]", ContentType: "audio"},
		{Text: "[pdf: manual.pdf]\n\nExtracted manual", ContentType: "pdf"},
		{Text: "[video: movie.mp4]", ContentType: "video"},
	}}
	calls := 0
	e := NewEngine(&countingQueryClient{}).WithReranker(rerankFunc(func(ctx context.Context, q string, results []SearchResult) ([]SearchResult, error) {
		calls++
		if len(results) != 3 {
			t.Fatalf("media sent for scoring: %v", results)
		}
		return scoreResults(results, func(r SearchResult) float64 {
			if r.Text == "high" {
				return 3
			}
			if r.ContentType == "pdf" {
				return 2
			}
			return 0
		}), nil
	}))
	config := DefaultConfig()
	config.TopK = 7
	warnings := captureWarnings(&config)
	got, err := e.queryIndex(context.Background(), "absent", index, sameVectors(7), config)
	if err != nil || len(got) != 7 || calls != 1 || len(*warnings) != 1 {
		t.Fatalf("results=%v error=%v warnings=%v", got, err, *warnings)
	}
	for _, i := range []int{0, 2, 4, 6} {
		if got[i].Text != index.Meta[i].Text || got[i].RerankScore != nil || got[i].RerankSkippedReason == "" {
			t.Fatalf("media moved or scored at position %d: %+v", i, got[i])
		}
	}
	if got[1].Text != "high" || got[3].ContentType != "pdf" || got[5].Text != "low" {
		t.Fatal("text candidates must fill only text positions", got)
	}
}

func TestAllMediaAndEmptyPDFTextNeverCallReranker(t *testing.T) {
	calls := 0
	e := NewEngine(nil).WithReranker(rerankFunc(func(ctx context.Context, q string, results []SearchResult) ([]SearchResult, error) {
		calls++
		t.Fatal("unscorable media reached API")
		return nil, nil
	}))
	original := []SearchResult{{ContentType: "image", Text: "[image: x]"}, {ContentType: "pdf", Text: "[pdf: x]\n\n"}, {Text: "[Video: x]"}}
	config := DefaultConfig()
	warnings := captureWarnings(&config)
	got, err := e.rerankRetrieved(context.Background(), "q", original, config)
	if err != nil || len(got) != 3 || calls != 0 || len(*warnings) != 1 {
		t.Fatalf("got=%v err=%v warnings=%v", got, err, *warnings)
	}
}

func TestIncompleteRerankingRestoresOriginalOrder(t *testing.T) {
	for _, result := range [][]SearchResult{nil, {{Text: "first"}}} {
		e := NewEngine(nil).WithReranker(rerankFunc(func(ctx context.Context, q string, results []SearchResult) ([]SearchResult, error) {
			return result, nil
		}))
		config := DefaultConfig()
		warnings := captureWarnings(&config)
		original := []SearchResult{{Text: "first"}, {Text: "second"}}
		got, err := e.rerankRetrieved(context.Background(), "q", original, config)
		if err != nil || !reflect.DeepEqual(got, original) || len(*warnings) != 1 {
			t.Fatalf("got=%v err=%v warnings=%v", got, err, *warnings)
		}
	}
}

func TestSearchUntilFoundWidensHybridCandidatesAndEmbedsOnce(t *testing.T) {
	c := &countingQueryClient{}
	calls := 0
	seen := make(map[string]bool)
	e := NewEngine(c).WithReranker(rerankFunc(func(ctx context.Context, q string, results []SearchResult) ([]SearchResult, error) {
		calls++
		for _, r := range results {
			if seen[r.Text] {
				t.Fatal("candidate scored twice")
			}
			seen[r.Text] = true
		}
		return scoreResults(results, func(r SearchResult) float64 {
			if r.Text == "needle c" {
				return 2
			}
			if r.Text == "needle d" {
				return 3
			}
			return 1
		}), nil
	}))
	index := &RagIndex{Dimension: 2, Meta: []ChunkMeta{{Text: "needle a"}, {Text: "needle b"}, {Text: "needle c"}, {Text: "needle d"}, {Text: "needle e"}}}
	config := DefaultConfig()
	config.TopK = 2
	config.RerankCandidateK = 2
	config.CandidateK = 1
	config.SearchUntilFound = true
	got, err := e.queryIndex(context.Background(), "needle", index, sameVectors(5), config)
	if err != nil || len(got) != 2 || got[0].Text != "needle d" || got[1].Text != "needle c" || calls != 2 || len(seen) != 4 || c.calls != 1 {
		t.Fatalf("results=%v err=%v calls=%d embed=%d seen=%v", got, err, calls, c.calls, seen)
	}
}

func TestSearchUntilFoundExhaustionAndCandidateCap(t *testing.T) {
	for _, count := range []int{5, 1005} {
		c := &countingQueryClient{}
		judged := 0
		e := NewEngine(c).WithReranker(rerankFunc(func(ctx context.Context, q string, results []SearchResult) ([]SearchResult, error) {
			judged += len(results)
			return scoreResults(results, func(SearchResult) float64 { return 1.99 }), nil
		}))
		index := &RagIndex{Dimension: 2, Meta: make([]ChunkMeta, count)}
		for i := range index.Meta {
			index.Meta[i].Text = fmt.Sprintf("needle %d", i)
		}
		config := DefaultConfig()
		config.SearchUntilFound = true
		warnings := captureWarnings(&config)
		got, err := e.queryIndex(context.Background(), "needle", index, sameVectors(count), config)
		want := count
		if want > 1000 {
			want = 1000
		}
		wantWarnings := 0
		if count >= 1000 {
			wantWarnings = 1
		}
		if err != nil || len(got) != 0 || judged != want || c.calls != 1 || len(*warnings) != wantWarnings {
			t.Fatalf("count=%d got=%v err=%v judged=%d embed=%d warnings=%v", count, got, err, judged, c.calls, *warnings)
		}
	}
}

func TestSearchUntilFoundFailureOrUnscoredBatchStopsAndRestoresFirstResults(t *testing.T) {
	for _, media := range []bool{false, true} {
		calls := 0
		e := NewEngine(&countingQueryClient{}).WithReranker(rerankFunc(func(ctx context.Context, q string, results []SearchResult) ([]SearchResult, error) {
			calls++
			if calls == 2 && !media {
				return nil, errors.New("API unavailable")
			}
			return scoreResults(results, func(SearchResult) float64 { return 0 }), nil
		}))
		index := &RagIndex{Dimension: 2, Meta: []ChunkMeta{{Text: "a"}, {Text: "b"}, {Text: "c"}, {Text: "d"}, {Text: "e"}}}
		if media {
			index.Meta[3] = ChunkMeta{Text: "[pdf: scanned]", ContentType: "pdf"}
		}
		config := DefaultConfig()
		config.TopK = 1
		config.RerankCandidateK = 2
		config.SearchUntilFound = true
		warnings := captureWarnings(&config)
		got, err := e.queryIndex(context.Background(), "absent", index, sameVectors(5), config)
		if err != nil || len(got) != 1 || got[0].Text != "a" || got[0].RerankScore != nil || calls != 2 || len(*warnings) != 1 {
			t.Fatalf("media=%v got=%v err=%v calls=%d warnings=%v", media, got, err, calls, *warnings)
		}
	}
}

func TestCanceledSearchDoesNotFallBackOrContinue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := NewEngine(nil).WithReranker(rerankFunc(func(ctx context.Context, q string, results []SearchResult) ([]SearchResult, error) {
		cancel()
		return nil, ctx.Err()
	}))
	config := DefaultConfig()
	warnings := captureWarnings(&config)
	if _, err := e.rerankRetrieved(ctx, "q", []SearchResult{{Text: "first"}}, config); !errors.Is(err, context.Canceled) || len(*warnings) != 0 {
		t.Fatalf("error=%v warnings=%v", err, *warnings)
	}
}
