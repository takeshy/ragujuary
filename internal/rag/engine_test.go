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
