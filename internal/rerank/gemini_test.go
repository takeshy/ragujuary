package rerank

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/takeshy/ragujuary/internal/rag"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func testClient(t *testing.T, handler roundTripFunc) *GeminiClient {
	t.Helper()
	c := NewGeminiClient("test-secret")
	c.httpClient.Transport = handler
	return c
}

func TestGeminiDecisionsRequestAndRerank(t *testing.T) {
	calls := 0
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != endpoint || r.Method != http.MethodPost || r.Header.Get("x-goog-api-key") != "test-secret" || r.URL.RawQuery != "" {
			t.Fatalf("unexpected endpoint, authentication, or method: %s %s", r.Method, r.URL)
		}
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Model != "gemini-decision-1" || len(req.Questions) != 3 {
			t.Fatalf("unexpected request: %+v", req)
		}
		var candidates []candidate
		if err := json.Unmarshal([]byte(req.Input), &candidates); err != nil {
			t.Fatal(err)
		}
		if candidates[1].Text != "認証の設定" || candidates[1].PageLabel != "pages 3-4 of 8" {
			t.Fatal(candidates)
		}
		for id, q := range req.Questions {
			if q.Type != "score" || len(q.Criteria) != 4 || !strings.Contains(q.Instructions, id) || !strings.Contains(q.Instructions, "認証") {
				t.Fatalf("question lacks visible ID/query or rubric: %+v", q)
			}
		}
		return response(200, `{"answers":{"candidate_2":{"type":"score","score":2.7},"candidate_0":{"type":"score","score":0},"candidate_1":{"type":"score","score":2.7}}}`), nil
	})
	input := []rag.SearchResult{
		{Text: "unrelated", Score: 0.9},
		{Text: "認証の設定", FilePath: "auth.pdf", ContentType: "pdf", PageLabel: "pages 3-4 of 8", Score: 0.8},
		{Text: "authentication", Score: 0.7},
	}
	before := append([]rag.SearchResult(nil), input...)
	got, err := c.Rerank(context.Background(), "認証", input)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || got[0].Text != input[1].Text || got[1].Text != input[2].Text || got[2].Text != input[0].Text {
		t.Fatalf("rerank order: %+v", got)
	}
	if *got[0].RerankScore != 2.7 || got[0].Score != 0.8 || got[0].PageLabel != "pages 3-4 of 8" {
		t.Fatal(got[0])
	}
	if !reflect.DeepEqual(input, before) {
		t.Fatal("reranking mutated the input")
	}
}

func TestGeminiRejectsMissingOrMalformedScores(t *testing.T) {
	for _, body := range []string{
		`{}`, `not-json`, `{"answers":{}}`,
		`{"answers":{"candidate_0":{"type":"score"}}}`,
		`{"answers":{"candidate_0":{"type":"score","score":null}}}`,
		`{"answers":{"candidate_0":{"type":"noul","score":1}}}`,
		`{"answers":{"candidate_0":{"type":"score","score":3.1}}}`,
		`{"answers":{"candidate_0":{"type":"score","score":-0.1}}}`,
		`{"answers":{"candidate_0":{"type":"score","score":"1"}}}`,
	} {
		t.Run(body, func(t *testing.T) {
			c := testClient(t, func(r *http.Request) (*http.Response, error) { return response(200, body), nil })
			if _, err := c.Rerank(context.Background(), "q", []rag.SearchResult{{Text: "x"}}); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestGeminiSplitsLargeRequestsWithoutTruncatingCandidates(t *testing.T) {
	calls, seen := 0, make(map[string]bool)
	text := strings.Repeat("文", 20_000)
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		data, _ := io.ReadAll(r.Body)
		if len(data) > maxRequestBytes {
			t.Fatalf("request too large: %d", len(data))
		}
		var req request
		if err := json.Unmarshal(data, &req); err != nil {
			t.Fatal(err)
		}
		var candidates []candidate
		if err := json.Unmarshal([]byte(req.Input), &candidates); err != nil {
			t.Fatal(err)
		}
		answers := make(map[string]any)
		for _, candidate := range candidates {
			if seen[candidate.ID] || candidate.Text != text {
				t.Fatal("duplicate candidate or truncated text")
			}
			seen[candidate.ID] = true
			answers[candidate.ID] = map[string]any{"type": "score", "score": 2}
		}
		body, _ := json.Marshal(map[string]any{"answers": answers})
		return response(200, string(body)), nil
	})
	input := make([]rag.SearchResult, 8)
	for i := range input {
		input[i].Text = text
	}
	got, err := c.Rerank(context.Background(), "q", input)
	if err != nil || len(got) != 8 || len(seen) != 8 || calls < 2 {
		t.Fatalf("results=%d seen=%d calls=%d err=%v", len(got), len(seen), calls, err)
	}
}

func TestGeminiErrorsCancellationAndLimits(t *testing.T) {
	calls := 0
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		return response(401, "invalid test-secret"), nil
	})
	input := []rag.SearchResult{{Text: "x"}}
	if _, err := c.Rerank(context.Background(), "q", input); err == nil || strings.Contains(err.Error(), "test-secret") || !strings.Contains(err.Error(), "401") {
		t.Fatalf("unexpected API error: %v", err)
	}
	if calls != 1 {
		t.Fatal("authentication error should not retry")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Rerank(ctx, "q", input); !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("canceled request called API: %v", err)
	}
	if _, err := c.Rerank(context.Background(), "q", make([]rag.SearchResult, 1001)); err == nil || calls != 1 {
		t.Fatal("candidate limit not enforced")
	}
	if _, err := c.Rerank(context.Background(), "q", []rag.SearchResult{{Text: strings.Repeat("x", maxRequestBytes)}}); err == nil || calls != 1 {
		t.Fatal("oversized text should fail explicitly")
	}
	c.apiKey = ""
	if _, err := c.Rerank(context.Background(), "q", input); err == nil || calls != 1 {
		t.Fatal("missing key should fail before API")
	}
}

func TestGeminiRetriesTransientErrors(t *testing.T) {
	calls := 0
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return response(503, "busy"), nil
		}
		return response(200, `{"answers":{"candidate_0":{"type":"score","score":3}}}`), nil
	})
	got, err := c.Rerank(context.Background(), "q", []rag.SearchResult{{Text: "x"}})
	if err != nil || calls != 2 || *got[0].RerankScore != 3 {
		t.Fatalf("calls=%d got=%v err=%v", calls, got, err)
	}
}
