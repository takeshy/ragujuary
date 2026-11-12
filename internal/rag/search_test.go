package rag

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/takeshy/ragujuary/internal/embedding"
)

func TestHybridRecoversKeywordHitBelowVectorThreshold(t *testing.T) {
	index := &RagIndex{Dimension: 2, Meta: []ChunkMeta{
		{FilePath: "same.md", Text: "semantic match"},
		{FilePath: "same.md", Text: "ERR_AUTH_42: authentication failed", PageLabel: "page 2"},
	}}
	vectors := []float32{1, 0, 0, 1}
	if got := Search([]float32{1, 0}, index, vectors, 5, 0.5); len(got) != 1 {
		t.Fatalf("vector results = %v", got)
	}
	got := HybridSearch("ERR_AUTH_42", []float32{1, 0}, index, vectors, 5, 30, 0.5, 60)
	if len(got) != 2 || got[1].Text != index.Meta[1].Text || got[1].PageLabel != "page 2" {
		t.Fatalf("hybrid should retain both distinct chunks from the same file: %v", got)
	}
}

func TestHybridFusesByChunkIdentityAndUsesStableTies(t *testing.T) {
	index := &RagIndex{Dimension: 2, Meta: []ChunkMeta{
		{Text: "other"}, {Text: "needle"}, {Text: "needle"},
	}}
	for i := 0; i < 20; i++ {
		got := HybridSearch("needle", []float32{1, 0}, index, []float32{1, 0, 0.8, 0.6, 0, 1}, 5, 30, 0.3, 60)
		if len(got) != 3 || got[0].Text != "needle" || got[0].Score <= got[1].Score {
			t.Fatalf("shared vector and lexical candidate should rank first without duplication: %v", got)
		}
	}
	got := keywordRanks("needle", index, 0)
	if len(got) != 2 || got[0].index != 1 || got[1].index != 2 {
		t.Fatalf("identical chunks keep separate identities and index tie order: %v", got)
	}
}

func TestKeywordSearchJapaneseNormalizationAndBodyOnly(t *testing.T) {
	index := &RagIndex{Meta: []ChunkMeta{
		{FilePath: "notes/auth.md", Text: "認証エラーの解決方法とパスワードの変更"},
		{FilePath: "other.md", Text: "猫の写真"},
		{FilePath: "code.md", Text: "ＡＢＣ１２３ connection"},
	}}
	for _, tc := range []struct {
		query string
		want  int
	}{
		{"認証エラー", 0}, {"abc123", 2},
	} {
		got := keywordRanks(tc.query, index, 1)
		if len(got) != 1 || got[0].index != tc.want || got[0].score <= 0 {
			t.Errorf("keyword %q = %v", tc.query, got)
		}
	}
	for _, query := range []string{"", " \t", "?!", "nonexistent", "auth"} {
		if got := keywordRanks(query, index, 0); len(got) != 0 {
			t.Errorf("%q = %v", query, got)
		}
	}
}

func TestTokenizerPreservesIdentifiersAndCJKFallback(t *testing.T) {
	got := searchTokens("ＰＲＪ-１２３ snake_case 日本語テスト 認")
	want := []string{"prj-123", "prj", "123", "snake_case", "snake", "case", "日本", "本語", "語テ", "テス", "スト", "認"}
	// The common tokenizer emits CJK first; relative order of CJK versus Latin
	// tokens has no effect on BM25. Compare the token counts instead.
	counts := func(tokens []string) map[string]int {
		m := make(map[string]int)
		for _, token := range tokens {
			m[token]++
		}
		return m
	}
	if !reflect.DeepEqual(counts(got), counts(want)) {
		t.Fatalf("tokens=%v want=%v", got, want)
	}
}

func TestRRFSourceConstantAndVectorTiePriority(t *testing.T) {
	got := fuseRanks([]scoredChunk{{index: 2, score: 1}}, []scoredChunk{{index: 0, score: 20}}, 2, 25)
	if len(got) != 2 || got[0].index != 2 || got[0].source != "vector" || got[1].source != "bm25" || got[0].score != 1.0/26 {
		t.Fatalf("RRF must use ranks and prefer vector on ties: %+v", got)
	}
	got = fuseRanks([]scoredChunk{{index: 2}}, []scoredChunk{{index: 2}}, 1, 60)
	if len(got) != 1 || got[0].source != "both" || got[0].score != 2.0/61 {
		t.Fatal(got)
	}
}

func TestBM25RewardsRareTermsAndSaturatesRepetition(t *testing.T) {
	index := &RagIndex{Meta: []ChunkMeta{
		{Text: "common common rare"}, {Text: "common common common"}, {Text: "common common common"},
	}}
	got := keywordRanks("common rare", index, 0)
	if len(got) != 3 || got[0].index != 0 {
		t.Fatalf("rare-term match should rank first: %v", got)
	}
}

type countingQueryClient struct {
	fakeEmbeddingClient
	calls int
}

func (c *countingQueryClient) EmbedContent(model, text string, task embedding.TaskType, dim int) ([]float32, error) {
	c.calls++
	return []float32{1, 0}, nil
}

type spyReranker struct {
	count int
	err   error
}

func (r *spyReranker) Rerank(ctx context.Context, query string, results []SearchResult) ([]SearchResult, error) {
	r.count = len(results)
	if r.err != nil {
		return nil, r.err
	}
	// Promote the last retrieval candidate, which lies outside initial top K.
	results[0], results[len(results)-1] = results[len(results)-1], results[0]
	for i := range results {
		score := 3.0 - float64(i)
		results[i].RerankScore = &score
	}
	return results, nil
}

func TestQueryReranksCandidatePoolBeforeApplyingTopK(t *testing.T) {
	c := &countingQueryClient{}
	r := &spyReranker{}
	e := NewEngine(c).WithReranker(r)
	index := &RagIndex{Dimension: 2, Meta: []ChunkMeta{{Text: "a"}, {Text: "b"}, {Text: "c"}}}
	config := DefaultConfig()
	config.TopK, config.RerankCandidateK, config.Rerank = 1, 3, true
	got, err := e.queryIndex(context.Background(), "query", index, []float32{1, 0, 1, 0, 1, 0}, config)
	if err != nil || len(got) != 1 || got[0].Text != "c" || c.calls != 1 || r.count != 3 {
		t.Fatalf("results=%v err=%v embed calls=%d candidates=%d", got, err, c.calls, r.count)
	}
	r.err = errors.New("unavailable")
	var warnings []string
	config.OnWarning = func(message string) { warnings = append(warnings, message) }
	if got, err := e.queryIndex(context.Background(), "q", index, []float32{1, 0, 1, 0, 1, 0}, config); err != nil || len(got) != 1 || got[0].Text != "a" || len(warnings) != 1 {
		t.Fatalf("reranking failure must warn and retain original order: results=%v error=%v warnings=%v", got, err, warnings)
	}
}

func TestKeywordModeDoesNotEmbedAndInvalidOptionsDoNotCallAPI(t *testing.T) {
	c := &countingQueryClient{}
	e := NewEngine(c)
	index := &RagIndex{Dimension: 2, Meta: []ChunkMeta{{Text: "needle"}}}
	config := DefaultConfig()
	config.SearchMode = "keyword"
	config.Rerank = false
	got, err := e.queryIndex(context.Background(), "needle", index, nil, config)
	if err != nil || len(got) != 1 || c.calls != 0 {
		t.Fatalf("results=%v err=%v calls=%d", got, err, c.calls)
	}
	config.SearchMode = "typo"
	if _, err := e.queryIndex(context.Background(), "q", index, nil, config); err == nil || c.calls != 0 {
		t.Fatal("invalid mode must fail before embedding")
	}
	config = DefaultConfig()
	config.Rerank = false
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.queryIndex(ctx, "q", index, nil, config); !errors.Is(err, context.Canceled) || c.calls != 0 {
		t.Fatal("canceled query must not embed")
	}
}

func TestSearchHandlesMismatchedDimensions(t *testing.T) {
	index := &RagIndex{Dimension: 2, Meta: []ChunkMeta{{Text: "x"}}}
	if got := Search([]float32{1}, index, []float32{1, 0}, 5, 0); len(got) != 0 {
		t.Fatal(got)
	}
	if got := Search([]float32{1, 0}, index, []float32{1}, 5, 0); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestQueryConfigValidation(t *testing.T) {
	for _, config := range []Config{
		{SearchMode: "unknown"}, {TopK: -1}, {CandidateK: -1}, {CandidateK: 1001},
		{MinScore: math.NaN()}, {MinScore: math.Inf(1)}, {MinScore: 1.1}, {Rerank: true, TopK: 1001},
		{SearchUntilFound: true, Rerank: false},
	} {
		if err := config.ValidateQuery(); err == nil {
			t.Errorf("expected invalid config: %+v", config)
		}
	}
}
