package rag

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/takeshy/ragujuary/internal/embedding"
)

type fakeEmbeddingClient struct{}

func (f fakeEmbeddingClient) EmbedContent(model, text string, taskType embedding.TaskType, dimension int) ([]float32, error) {
	return fakeVector(text, dimension), nil
}

func (f fakeEmbeddingClient) BatchEmbedContents(model string, texts []string, taskType embedding.TaskType, dimension int) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i, text := range texts {
		result[i] = fakeVector(text, dimension)
	}
	return result, nil
}

type fakeMultimodalClient struct {
	fakeEmbeddingClient
}

func (f fakeMultimodalClient) EmbedMultimodalContent(model string, content embedding.MultimodalContent, taskType embedding.TaskType, dimension int) ([]float32, error) {
	return fakeVector(content.MIMEType, dimension), nil
}

func fakeVector(text string, dimension int) []float32 {
	if dimension <= 0 {
		dimension = 4
	}
	vec := make([]float32, dimension)
	for i := range vec {
		vec[i] = float32(len(text) + i + 1)
	}
	return vec
}

func TestIndexPreservesFilesOutsideCurrentScan(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dirA := filepath.Join(home, "dirA")
	dirB := filepath.Join(home, "dirB")
	if err := os.MkdirAll(dirA, 0755); err != nil {
		t.Fatalf("mkdir dirA: %v", err)
	}
	if err := os.MkdirAll(dirB, 0755); err != nil {
		t.Fatalf("mkdir dirB: %v", err)
	}

	fileA := filepath.Join(dirA, "a.txt")
	fileB := filepath.Join(dirB, "b.txt")
	if err := os.WriteFile(fileA, []byte("alpha document"), 0644); err != nil {
		t.Fatalf("write fileA: %v", err)
	}
	if err := os.WriteFile(fileB, []byte("beta document"), 0644); err != nil {
		t.Fatalf("write fileB: %v", err)
	}

	engine := NewEngine(fakeEmbeddingClient{})
	config := DefaultConfig()
	config.Dimension = 4
	config.ChunkSize = 32
	config.ChunkOverlap = 0

	first, err := engine.Index([]string{dirA, dirB}, nil, "test-store", config)
	if err != nil {
		t.Fatalf("first index: %v", err)
	}
	if first.IndexedFiles != 2 {
		t.Fatalf("first indexed files = %d, want 2", first.IndexedFiles)
	}

	second, err := engine.Index([]string{dirA}, nil, "test-store", config)
	if err != nil {
		t.Fatalf("second index: %v", err)
	}
	if second.IndexedFiles != 2 {
		t.Fatalf("second indexed files = %d, want 2", second.IndexedFiles)
	}

	index, _, err := LoadIndex("test-store")
	if err != nil {
		t.Fatalf("load index: %v", err)
	}
	if index == nil {
		t.Fatal("expected index to exist")
	}
	if len(index.FileChecksums) != 2 {
		t.Fatalf("file checksum count = %d, want 2", len(index.FileChecksums))
	}
	if _, ok := index.FileChecksums[fileA]; !ok {
		t.Fatalf("missing fileA from checksums")
	}
	if _, ok := index.FileChecksums[fileB]; !ok {
		t.Fatalf("missing fileB from checksums")
	}
}

func TestIndexRetriesSkippedMultimodalWhenBackendChanges(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	docsDir := filepath.Join(home, "mixed")
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		t.Fatalf("mkdir mixed: %v", err)
	}

	textPath := filepath.Join(docsDir, "doc.txt")
	imagePath := filepath.Join(docsDir, "photo.PNG")
	if err := os.WriteFile(textPath, []byte("hello world"), 0644); err != nil {
		t.Fatalf("write text: %v", err)
	}
	if err := os.WriteFile(imagePath, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, 0644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	config := DefaultConfig()
	config.Dimension = 4
	config.ChunkSize = 32
	config.ChunkOverlap = 0

	textOnlyEngine := NewEngine(fakeEmbeddingClient{})
	first, err := textOnlyEngine.Index([]string{docsDir}, nil, "retry-store", config)
	if err != nil {
		t.Fatalf("first index: %v", err)
	}
	if first.SkippedMultimodal != 1 {
		t.Fatalf("first skipped multimodal = %d, want 1", first.SkippedMultimodal)
	}
	if first.IndexedFiles != 1 {
		t.Fatalf("first indexed files = %d, want 1", first.IndexedFiles)
	}

	index, _, err := LoadIndex("retry-store")
	if err != nil {
		t.Fatalf("load first index: %v", err)
	}
	if _, ok := index.FileChecksums[imagePath]; ok {
		t.Fatalf("skipped multimodal file should not be persisted in checksums")
	}

	multimodalEngine := NewEngine(fakeMultimodalClient{})
	second, err := multimodalEngine.Index([]string{docsDir}, nil, "retry-store", config)
	if err != nil {
		t.Fatalf("second index: %v", err)
	}
	if second.MultimodalFiles != 1 {
		t.Fatalf("second multimodal files = %d, want 1", second.MultimodalFiles)
	}
	if second.IndexedFiles != 2 {
		t.Fatalf("second indexed files = %d, want 2", second.IndexedFiles)
	}

	index, _, err = LoadIndex("retry-store")
	if err != nil {
		t.Fatalf("load second index: %v", err)
	}
	if _, ok := index.FileChecksums[imagePath]; !ok {
		t.Fatalf("multimodal file should be persisted after successful indexing")
	}
	if len(index.Meta) != 2 {
		t.Fatalf("meta count = %d, want 2", len(index.Meta))
	}
}
