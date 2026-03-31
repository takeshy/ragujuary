package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
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
