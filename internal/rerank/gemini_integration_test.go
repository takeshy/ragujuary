package rerank_test

import (
	"context"
	"os"
	"testing"

	"github.com/takeshy/ragujuary/internal/rag"
	"github.com/takeshy/ragujuary/internal/rerank"
)

func TestIntegrationGeminiDecisionsRerank(t *testing.T) {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		t.Skip("GEMINI_API_KEY not set")
	}
	input := []rag.SearchResult{
		{FilePath: "recipes.md", Text: "卵と牛乳を混ぜてフライパンで焼くと、おいしいパンケーキが作れます。", Score: 0.9},
		{FilePath: "account.md", Text: "パスワードを変更するには、設定を開いて「セキュリティ」から「パスワードの変更」を選択します。", Score: 0.5},
	}
	results, err := rerank.NewGeminiClient(key).Rerank(context.Background(), "パスワードを変更するにはどうすればよいですか？", input)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].FilePath != "account.md" || results[0].Score != 0.5 {
		t.Fatalf("unexpected rerank order: %+v", results)
	}
	for _, result := range results {
		if result.RerankScore == nil || *result.RerankScore < 0 || *result.RerankScore > 3 {
			t.Fatalf("invalid relevance score: %+v", result)
		}
		t.Logf("%s: relevance %.3f/3, original %.3f", result.FilePath, *result.RerankScore, result.Score)
	}
	if *results[0].RerankScore <= *results[1].RerankScore {
		t.Fatal("direct answer should rank above unrelated recipe")
	}
}
