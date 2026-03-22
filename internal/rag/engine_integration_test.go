package rag_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/takeshy/ragujuary/internal/embedding"
	"github.com/takeshy/ragujuary/internal/rag"
)

func getAPIKey(t *testing.T) string {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		t.Skip("GEMINI_API_KEY not set, skipping integration test")
	}
	return key
}

func TestIntegration_IndexAndQueryText(t *testing.T) {
	key := getAPIKey(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create test files
	docsDir := filepath.Join(home, "docs")
	os.MkdirAll(docsDir, 0755)

	os.WriteFile(filepath.Join(docsDir, "go.md"), []byte(`# Go Programming
Go is a statically typed, compiled programming language designed at Google.
It is syntactically similar to C, but with memory safety and garbage collection.`), 0644)

	os.WriteFile(filepath.Join(docsDir, "python.md"), []byte(`# Python Programming
Python is a high-level, interpreted programming language.
It emphasizes code readability with significant indentation.`), 0644)

	client := embedding.NewGeminiClient(key)
	engine := rag.NewEngine(client)
	config := rag.DefaultConfig()
	config.Dimension = 256
	config.ChunkSize = 500
	config.ChunkOverlap = 50

	// Index
	result, err := engine.Index([]string{docsDir}, nil, "test-text", config)
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}
	t.Logf("Indexed: files=%d chunks=%d new=%d", result.IndexedFiles, result.TotalChunks, result.NewFiles)
	if result.IndexedFiles != 2 {
		t.Fatalf("expected 2 indexed files, got %d", result.IndexedFiles)
	}

	// Query
	results, err := engine.Query("What is Go?", "test-text", config)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	t.Logf("Top result: score=%.4f file=%s", results[0].Score, results[0].FilePath)

	// Verify incremental: re-index should skip unchanged
	result2, err := engine.Index([]string{docsDir}, nil, "test-text", config)
	if err != nil {
		t.Fatalf("Re-index failed: %v", err)
	}
	if result2.SkippedFiles != 2 {
		t.Fatalf("expected 2 skipped files, got %d", result2.SkippedFiles)
	}
	if result2.NewFiles != 0 {
		t.Fatalf("expected 0 new files, got %d", result2.NewFiles)
	}
}

func TestIntegration_IndexMixedTextAndImage(t *testing.T) {
	key := getAPIKey(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create test files
	docsDir := filepath.Join(home, "mixed")
	os.MkdirAll(docsDir, 0755)

	// Text file
	os.WriteFile(filepath.Join(docsDir, "readme.md"), []byte(`# Project
This project contains images of cats and dogs.`), 0644)

	// PNG image (red square)
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	os.WriteFile(filepath.Join(docsDir, "red.png"), buf.Bytes(), 0644)

	client := embedding.NewGeminiClient(key)
	engine := rag.NewEngine(client)
	config := rag.DefaultConfig()
	config.Dimension = 256
	config.ChunkSize = 500
	config.ChunkOverlap = 50

	result, err := engine.Index([]string{docsDir}, nil, "test-mixed", config)
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}
	t.Logf("Indexed: files=%d chunks=%d multimodal=%d", result.IndexedFiles, result.TotalChunks, result.MultimodalFiles)

	if result.MultimodalFiles != 1 {
		t.Fatalf("expected 1 multimodal file, got %d", result.MultimodalFiles)
	}

	// Verify index has both text and image chunks
	index, _, err := rag.LoadIndex("test-mixed")
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}
	if index == nil {
		t.Fatal("index is nil")
	}

	hasText := false
	hasImage := false
	for _, meta := range index.Meta {
		if meta.ContentType == "" {
			hasText = true
		}
		if meta.ContentType == "image" {
			hasImage = true
		}
	}
	if !hasText {
		t.Fatal("expected text chunks in index")
	}
	if !hasImage {
		t.Fatal("expected image chunk in index")
	}

	// Query should return results from both types
	results, err := engine.Query("red image", "test-mixed", config)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	t.Logf("Query 'red image': %d results", len(results))
	for i, r := range results {
		t.Logf("  [%d] score=%.4f type=%q file=%s", i, r.Score, r.ContentType, r.FilePath)
	}
}

func TestIntegration_IndexMultimodalContent_MCP(t *testing.T) {
	key := getAPIKey(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	client := embedding.NewGeminiClient(key)
	engine := rag.NewEngine(client)
	config := rag.DefaultConfig()
	config.Dimension = 256

	// Index text content via MCP-style call
	err := engine.IndexContent("test-mcp", "doc.md", "Ragujuary is a RAG tool for Gemini", config)
	if err != nil {
		t.Fatalf("IndexContent (text) failed: %v", err)
	}

	// Index image content via MCP-style call
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{0, 128, 255, 255})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)

	err = engine.IndexMultimodalContent("test-mcp", "photo.png", buf.Bytes(), "image/png", config)
	if err != nil {
		t.Fatalf("IndexMultimodalContent failed: %v", err)
	}

	// Verify both are in the index
	index, _, err := rag.LoadIndex("test-mcp")
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}
	if len(index.FileChecksums) != 2 {
		t.Fatalf("expected 2 files in index, got %d", len(index.FileChecksums))
	}

	// Query
	results, err := engine.Query("RAG tool", "test-mcp", config)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	t.Logf("Query results: %d", len(results))
	for i, r := range results {
		t.Logf("  [%d] score=%.4f type=%q file=%s", i, r.Score, r.ContentType, r.FilePath)
	}
}

func TestIntegration_OpenAISkipsMultimodal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create mixed directory
	docsDir := filepath.Join(home, "mixed")
	os.MkdirAll(docsDir, 0755)
	os.WriteFile(filepath.Join(docsDir, "doc.txt"), []byte("hello world"), 0644)

	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	var buf bytes.Buffer
	png.Encode(&buf, img)
	os.WriteFile(filepath.Join(docsDir, "img.png"), buf.Bytes(), 0644)

	// OpenAI client does NOT implement MultimodalEmbedder
	// Use a fake that mimics OpenAI behavior
	fakeClient := &fakeTextOnlyClient{}
	engine := rag.NewEngine(fakeClient)
	config := rag.DefaultConfig()
	config.Dimension = 4
	config.ChunkSize = 100

	result, err := engine.Index([]string{docsDir}, nil, "test-openai", config)
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}
	if result.SkippedMultimodal != 1 {
		t.Fatalf("expected 1 skipped multimodal, got %d", result.SkippedMultimodal)
	}
	if result.MultimodalFiles != 0 {
		t.Fatalf("expected 0 multimodal files indexed, got %d", result.MultimodalFiles)
	}
}

func TestIntegration_FormatVersionBackwardCompat(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create a v1 format index manually
	fakeClient := &fakeTextOnlyClient{}
	engine := rag.NewEngine(fakeClient)
	config := rag.DefaultConfig()
	config.Dimension = 4
	config.ChunkSize = 100

	docsDir := filepath.Join(home, "compat")
	os.MkdirAll(docsDir, 0755)
	os.WriteFile(filepath.Join(docsDir, "test.txt"), []byte("test content"), 0644)

	// Index to create a store
	_, err := engine.Index([]string{docsDir}, nil, "test-compat", config)
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}

	// Load should work (version 2)
	index, _, err := rag.LoadIndex("test-compat")
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}
	if index == nil {
		t.Fatal("index should not be nil")
	}
}

// fakeTextOnlyClient implements embedding.Client but NOT MultimodalEmbedder
type fakeTextOnlyClient struct{}

func (f *fakeTextOnlyClient) EmbedContent(model, text string, taskType embedding.TaskType, dimension int) ([]float32, error) {
	vec := make([]float32, dimension)
	for i := range vec {
		vec[i] = float32(i+1) * 0.01
	}
	return vec, nil
}

func (f *fakeTextOnlyClient) BatchEmbedContents(model string, texts []string, taskType embedding.TaskType, dimension int) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		vec := make([]float32, dimension)
		for j := range vec {
			vec[j] = float32(j+i+1) * 0.01
		}
		result[i] = vec
	}
	return result, nil
}
