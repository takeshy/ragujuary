package gemini

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestQueryCurrentModelsPreserveFileSearchAndCitations(t *testing.T) {
	for _, requested := range []string{"", "gemini-4-hydrogen", "gemini-4-argon", "gemini-4-carbon", "models/gemini-4-carbon", "gemini-3.8-flash"} {
		t.Run(requested, func(t *testing.T) {
			client := NewClient("test-key")
			client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				wantModel := strings.TrimPrefix(requested, "models/")
				if wantModel == "" {
					wantModel = "gemini-4-hydrogen"
				}
				if req.URL.Path != "/v1beta/models/"+wantModel+":generateContent" {
					t.Fatalf("unexpected query path: %s", req.URL.Path)
				}
				var body GenerateContentRequest
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if len(body.Tools) != 1 || body.Tools[0].FileSearch == nil || body.Tools[0].FileSearch.FileSearchStoreNames[0] != "fileSearchStores/docs" || body.Tools[0].FileSearch.MetadataFilter != `kind="manual"` {
					t.Fatalf("File Search configuration lost: %+v", body)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"candidates":[{"content":{"parts":[{"text":"Answer"}]},"groundingMetadata":{"groundingChunks":[{"retrievedContext":{"title":"manual.txt","text":"Evidence"}}]}}]}`))}, nil
			})
			response, err := client.Query(requested, "question", []string{"docs"}, `kind="manual"`)
			if err != nil {
				t.Fatal(err)
			}
			if response.Candidates[0].Content.Parts[0].Text != "Answer" || response.Candidates[0].GroundingMetadata.GroundingChunks[0].RetrievedContext.Text != "Evidence" {
				t.Fatal("answer or citations lost")
			}
		})
	}
}

func TestQueryRejectsLegacyGenerationModelsBeforeNetworkRequest(t *testing.T) {
	for _, model := range []string{"gemini-1.5-pro", "gemini-2.0-flash", "gemini-2.5-flash", "models/gemini-2.5-pro"} {
		t.Run(model, func(t *testing.T) {
			client := NewClient("test-key")
			client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				t.Fatal("legacy model made a network request")
				return nil, nil
			})
			if _, err := client.Query(model, "question", []string{"docs"}, ""); err == nil || !strings.Contains(err.Error(), DefaultQueryModel) {
				t.Fatalf("expected migration error: %v", err)
			}
		})
	}
}
