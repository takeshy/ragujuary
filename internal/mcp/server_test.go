package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/takeshy/ragujuary/internal/embedding"
	"github.com/takeshy/ragujuary/internal/gemini"
	"github.com/takeshy/ragujuary/internal/rag"
	"github.com/takeshy/ragujuary/internal/store"
)

type testQueryReranker struct{ candidates int }

func (r *testQueryReranker) Rerank(ctx context.Context, query string, results []rag.SearchResult) ([]rag.SearchResult, error) {
	r.candidates = len(results)
	for i := range results {
		score := float64(i) / 2
		results[i].RerankScore = &score
	}
	for i, j := 0, len(results)-1; i < j; i, j = i+1, j-1 {
		results[i], results[j] = results[j], results[i]
	}
	return results, nil
}

func TestHandleQueryEmbeddingKeywordAndRerank(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	index := &rag.RagIndex{Dimension: 2, Meta: []rag.ChunkMeta{
		{FilePath: "a.md", Text: "認証エラー one"},
		{FilePath: "b.pdf", Text: "認証エラー two", ContentType: "pdf", PageLabel: "pages 2-3 of 5"},
		{FilePath: "c.md", Text: "猫の写真"},
	}}
	if err := rag.SaveIndex("search", index, []float32{1, 0, 1, 0, 1, 0}); err != nil {
		t.Fatal(err)
	}
	r := &testQueryReranker{}
	// A nil embedding client ensures keyword mode makes no embedding request.
	s := &Server{ragEngine: rag.NewEngine(nil).WithReranker(r)}
	_, output, err := s.handleQuery(context.Background(), nil, QueryInput{StoreName: "search", Question: "認証エラー", SearchMode: "keyword", TopK: 1, CandidateK: 3})
	if err != nil {
		t.Fatal(err)
	}
	if r.candidates != 2 || len(output.Results) != 1 || output.Results[0].FilePath != "b.pdf" || output.Results[0].RerankScore == nil || !strings.Contains(output.Answer, "rerank:") || !strings.Contains(output.Answer, "pages 2-3 of 5") {
		t.Fatalf("unexpected query result: %+v, candidates=%d", output, r.candidates)
	}
	_, _, err = s.handleQuery(context.Background(), nil, QueryInput{StoreName: "search", Question: "q", SearchMode: "bad-mode"})
	if err == nil || !strings.Contains(err.Error(), "search mode") {
		t.Fatalf("invalid search mode: %v", err)
	}
}

func TestQueryInputExplicitZeroMinScore(t *testing.T) {
	var input QueryInput
	if err := json.Unmarshal([]byte(`{"question":"q","store_name":"s","min_score":0}`), &input); err != nil {
		t.Fatal(err)
	}
	if input.MinScore == nil || *input.MinScore != 0 {
		t.Fatal("explicit zero threshold was lost")
	}
	if err := json.Unmarshal([]byte(`{"question":"q","store_name":"s"}`), &input); err != nil {
		t.Fatal(err)
	}
	// Unmarshal into a fresh value to represent an actual new tool call.
	var omitted QueryInput
	if err := json.Unmarshal([]byte(`{"question":"q","store_name":"s"}`), &omitted); err != nil {
		t.Fatal(err)
	}
	if omitted.MinScore != nil {
		t.Fatal("omitted threshold must use the default")
	}
}

func TestHandleQueryDefaultRerankExplicitFalseAndFallbackWarnings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	index := &rag.RagIndex{Dimension: 2, Meta: []rag.ChunkMeta{{Text: "needle a", FilePath: "a.md"}, {Text: "needle b", FilePath: "b.md"}}}
	if err := rag.SaveIndex("defaults", index, []float32{1, 0, 1, 0}); err != nil {
		t.Fatal(err)
	}
	r := &testQueryReranker{}
	s := &Server{ragEngine: rag.NewEngine(testEmbeddingClient{}).WithReranker(r)}
	// Decoding actual input verifies that omission and explicit false are distinct.
	var input QueryInput
	if err := json.Unmarshal([]byte(`{"store_name":"defaults","question":"needle","top_k":1}`), &input); err != nil {
		t.Fatal(err)
	}
	_, output, err := s.handleQuery(context.Background(), nil, input)
	if err != nil || len(output.Results) != 1 || output.Results[0].FilePath != "b.md" || output.Results[0].RetrievalSource != "bm25" || r.candidates != 2 {
		t.Fatalf("defaults: %+v err=%v", output, err)
	}
	if err := json.Unmarshal([]byte(`{"store_name":"defaults","question":"needle","top_k":1,"rerank":false}`), &input); err != nil {
		t.Fatal(err)
	}
	r.candidates = 0
	_, output, err = s.handleQuery(context.Background(), nil, input)
	if err != nil || len(output.Results) != 1 || output.Results[0].FilePath != "a.md" || output.Results[0].RerankScore != nil || len(output.Warnings) != 0 || r.candidates != 0 {
		t.Fatalf("explicit off: %+v err=%v", output, err)
	}
	s.ragEngine = rag.NewEngine(testEmbeddingClient{})
	input.Rerank = nil
	_, output, err = s.handleQuery(context.Background(), nil, input)
	if err != nil || len(output.Results) != 1 || output.Results[0].FilePath != "a.md" || len(output.Warnings) != 1 || !strings.Contains(output.Answer, "Warning:") {
		t.Fatalf("fallback warnings: %+v err=%v", output, err)
	}
}

func TestHandleQuerySearchUntilFoundAndDisabledRerankConflict(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	index := &rag.RagIndex{Dimension: 2, Meta: []rag.ChunkMeta{{Text: "needle a"}, {Text: "needle b"}}}
	if err := rag.SaveIndex("repeat", index, []float32{1, 0, 1, 0}); err != nil {
		t.Fatal(err)
	}
	s := &Server{ragEngine: rag.NewEngine(testEmbeddingClient{}).WithReranker(&testQueryReranker{})}
	input := QueryInput{StoreName: "repeat", Question: "needle", SearchUntilFound: true, TopK: 1, RerankCandidateK: 1}
	_, output, err := s.handleQuery(context.Background(), nil, input)
	if err != nil || len(output.Results) != 0 || len(output.Warnings) != 0 || output.Answer != "No results found." {
		t.Fatalf("repeat exhausted: %+v %v", output, err)
	}
	off := false
	input.Rerank = &off
	if _, _, err := s.handleQuery(context.Background(), nil, input); err == nil || !strings.Contains(err.Error(), "requires reranking") {
		t.Fatalf("conflicting options must fail: %v", err)
	}
}

type testEmbeddingClient struct{}

func (testEmbeddingClient) EmbedContent(model, text string, taskType embedding.TaskType, dimension int) ([]float32, error) {
	return make([]float32, dimension), nil
}

func (testEmbeddingClient) BatchEmbedContents(model string, texts []string, taskType embedding.TaskType, dimension int) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = make([]float32, dimension)
	}
	return out, nil
}

func (testEmbeddingClient) EmbedMultimodalContent(model string, content embedding.MultimodalContent, taskType embedding.TaskType, dimension int) ([]float32, error) {
	return make([]float32, dimension), nil
}

func makeTestPDF(t *testing.T, pages int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 0, color.RGBA{G: 255, A: 255})
	img.Set(0, 1, color.RGBA{B: 255, A: 255})
	img.Set(1, 1, color.RGBA{R: 255, G: 255, A: 255})

	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatalf("png.Encode() error = %v", err)
	}

	readers := make([]io.Reader, pages)
	for i := range readers {
		readers[i] = bytes.NewReader(pngBuf.Bytes())
	}

	var pdfBuf bytes.Buffer
	if err := api.ImportImages(nil, &pdfBuf, readers, nil, nil); err != nil {
		t.Fatalf("ImportImages() error = %v", err)
	}

	return pdfBuf.Bytes()
}

func TestNewServerDefersStoreManagerInitialization(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dataFile := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(dataFile, []byte("{not-json"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s, err := NewServer(ServerConfig{
		APIKey:   "test-key",
		DataFile: dataFile,
	}, "test-version")
	if err != nil {
		t.Fatalf("NewServer() error = %v, want nil", err)
	}

	if s.storeManager != nil {
		t.Fatal("storeManager initialized eagerly, want nil")
	}

	_, err = s.getStoreManager()
	if err == nil {
		t.Fatal("getStoreManager() error = nil, want error for invalid data file")
	}
}

func TestSyncRemoteDocumentsToLocalStore(t *testing.T) {
	t.Parallel()

	storeManager, err := store.NewManager(filepath.Join(t.TempDir(), "stores.json"))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	s := &Server{}
	storeManager.AddFile("store-1", store.FileMetadata{
		LocalPath: "/tmp/stale.txt",
		RemoteID:  "fileSearchStores/store-1/documents/stale",
		Checksum:  "stale",
	})
	checksum := "sha256:abc123"
	s.syncRemoteDocumentsToLocalStore(storeManager, "store-1", []gemini.FileSearchDocument{
		{
			Name:        "fileSearchStores/store-1/documents/doc-1",
			DisplayName: "/tmp/a.txt",
			MimeType:    "text/plain",
			CustomMetadata: []gemini.CustomMetadata{
				{Key: "checksum", StringValue: &checksum},
			},
		},
		{
			Name:        "fileSearchStores/store-1/documents/doc-2",
			DisplayName: "",
		},
	})

	meta, ok := storeManager.GetFileByPath("store-1", "/tmp/a.txt")
	if !ok {
		t.Fatal("GetFileByPath() found = false, want true")
	}
	if meta.RemoteID != "fileSearchStores/store-1/documents/doc-1" {
		t.Fatalf("RemoteID = %q, want doc name", meta.RemoteID)
	}
	wantChecksum := "abc123" // sha256: prefix is stripped by syncRemoteDocumentsToLocalStore
	if meta.Checksum != wantChecksum {
		t.Fatalf("Checksum = %q, want %q", meta.Checksum, wantChecksum)
	}
	if _, ok := storeManager.GetFileByPath("store-1", ""); ok {
		t.Fatal("blank display name should not be indexed")
	}
	if _, ok := storeManager.GetFileByPath("store-1", "/tmp/stale.txt"); ok {
		t.Fatal("stale local metadata should be pruned when absent from remote")
	}
}

func TestGetAllowedStoreNamesEnforcesAllowedStores(t *testing.T) {
	t.Parallel()

	s := &Server{
		config: ServerConfig{
			AllowedStores: []string{"allowed-a", "allowed-b"},
		},
	}

	names, err := s.getAllowedStoreNames("", []string{"allowed-a", "allowed-a", "allowed-b"})
	if err != nil {
		t.Fatalf("getAllowedStoreNames() error = %v, want nil", err)
	}
	if len(names) != 2 || names[0] != "allowed-a" || names[1] != "allowed-b" {
		t.Fatalf("getAllowedStoreNames() = %v, want deduplicated allowed names", names)
	}

	_, err = s.getAllowedStoreNames("", []string{"forbidden"})
	if err == nil || !strings.Contains(err.Error(), "allowed stores list") {
		t.Fatalf("getAllowedStoreNames() error = %v, want allowed-stores error", err)
	}
}

func TestHandleCreateDeleteStoreEnforceAllowedStores(t *testing.T) {
	t.Parallel()

	s := &Server{
		config: ServerConfig{
			AllowedStores: []string{"allowed"},
		},
	}

	_, _, err := s.handleCreateStore(context.Background(), nil, CreateStoreInput{
		StoreName: "forbidden",
		Type:      "embed",
	})
	if err == nil || !strings.Contains(err.Error(), "allowed stores list") {
		t.Fatalf("handleCreateStore() error = %v, want allowed-stores error", err)
	}

	_, _, err = s.handleDeleteStore(context.Background(), nil, DeleteStoreInput{
		StoreName: "forbidden",
	})
	if err == nil || !strings.Contains(err.Error(), "allowed stores list") {
		t.Fatalf("handleDeleteStore() error = %v, want allowed-stores error", err)
	}
}

func TestHandleQueryRejectsDisallowedAndMixedEmbeddingStores(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := rag.CreateEmptyIndex("embed-a"); err != nil {
		t.Fatalf("CreateEmptyIndex(embed-a) error = %v", err)
	}
	if err := rag.CreateEmptyIndex("embed-b"); err != nil {
		t.Fatalf("CreateEmptyIndex(embed-b) error = %v", err)
	}

	s := &Server{
		config: ServerConfig{
			AllowedStores: []string{"embed-a", "embed-b"},
		},
	}

	_, _, err := s.handleQuery(context.Background(), nil, QueryInput{
		Question:   "test",
		StoreNames: []string{"forbidden"},
	})
	if err == nil || !strings.Contains(err.Error(), "allowed stores list") {
		t.Fatalf("handleQuery(disallowed) error = %v, want allowed-stores error", err)
	}

	_, _, err = s.handleQuery(context.Background(), nil, QueryInput{
		Question:   "test",
		StoreNames: []string{"embed-a", "embed-b"},
	})
	if err == nil || !strings.Contains(err.Error(), "multiple embedding stores") {
		t.Fatalf("handleQuery(multi-embed) error = %v, want multiple-embedding-stores error", err)
	}
}

func TestHandleUploadEmbedRejectsInvalidPDFMaxPages(t *testing.T) {
	t.Parallel()

	s := &Server{
		ragEngine: rag.NewEngine(testEmbeddingClient{}),
	}

	_, _, err := s.handleUploadEmbed(context.Background(), "test-store", UploadInput{
		FileName:    "doc.pdf",
		FileContent: "ignored",
		MIMEType:    "application/pdf",
		IsBase64:    true,
		PDFMaxPages: 7,
	})
	if err == nil || !strings.Contains(err.Error(), "pdf_max_pages must be between 1 and 6") {
		t.Fatalf("handleUploadEmbed() error = %v, want pdf_max_pages validation error", err)
	}
}

func TestHandleUploadDirectoryEmbedRejectsInvalidPDFMaxPages(t *testing.T) {
	t.Parallel()

	s := &Server{
		ragEngine: rag.NewEngine(testEmbeddingClient{}),
	}

	_, _, err := s.handleUploadDirectoryEmbed(context.Background(), "test-store", UploadDirectoryInput{
		Directories: []string{"."},
		PDFMaxPages: 7,
	})
	if err == nil || !strings.Contains(err.Error(), "pdf_max_pages must be between 1 and 6") {
		t.Fatalf("handleUploadDirectoryEmbed() error = %v, want pdf_max_pages validation error", err)
	}
}

func TestHandleUploadEmbedReportsPDFChunkCountUsingConfiguredPageLimit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := rag.CreateEmptyIndex("pdf-store"); err != nil {
		t.Fatalf("CreateEmptyIndex() error = %v", err)
	}

	pdfData := makeTestPDF(t, 7)

	s := &Server{
		ragEngine: rag.NewEngine(testEmbeddingClient{}),
	}

	result, _, err := s.handleUploadEmbed(context.Background(), "pdf-store", UploadInput{
		FileName:    "doc.pdf",
		FileContent: base64.StdEncoding.EncodeToString(pdfData),
		MIMEType:    "application/pdf",
		IsBase64:    true,
		PDFMaxPages: 3,
	})
	if err != nil {
		t.Fatalf("handleUploadEmbed() error = %v", err)
	}

	if len(result.Content) != 1 {
		t.Fatalf("handleUploadEmbed() content len = %d, want 1", len(result.Content))
	}

	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("handleUploadEmbed() content type = %T, want *mcp.TextContent", result.Content[0])
	}
	if !strings.Contains(text.Text, "3 embedding(s)") {
		t.Fatalf("handleUploadEmbed() message = %q, want configured chunk count", text.Text)
	}
}
