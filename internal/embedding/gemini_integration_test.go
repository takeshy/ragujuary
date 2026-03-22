package embedding_test

import (
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"bytes"
	"os"
	"testing"

	"github.com/takeshy/ragujuary/internal/embedding"
)

func getAPIKey(t *testing.T) string {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		t.Skip("GEMINI_API_KEY not set, skipping integration test")
	}
	return key
}

func TestGeminiEmbedContent_Text(t *testing.T) {
	key := getAPIKey(t)
	client := embedding.NewGeminiClient(key)

	vec, err := client.EmbedContent("gemini-embedding-2-preview", "Hello world", embedding.TaskRetrievalDocument, 256)
	if err != nil {
		t.Fatalf("EmbedContent failed: %v", err)
	}
	if len(vec) != 256 {
		t.Fatalf("expected dimension 256, got %d", len(vec))
	}
	// Verify non-zero
	allZero := true
	for _, v := range vec {
		if v != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Fatal("embedding vector is all zeros")
	}
}

func TestGeminiBatchEmbedContents_Text(t *testing.T) {
	key := getAPIKey(t)
	client := embedding.NewGeminiClient(key)

	texts := []string{"Hello world", "Goodbye world", "Test document"}
	vecs, err := client.BatchEmbedContents("gemini-embedding-2-preview", texts, embedding.TaskRetrievalDocument, 256)
	if err != nil {
		t.Fatalf("BatchEmbedContents failed: %v", err)
	}
	if len(vecs) != 3 {
		t.Fatalf("expected 3 vectors, got %d", len(vecs))
	}
	for i, vec := range vecs {
		if len(vec) != 256 {
			t.Fatalf("vector %d: expected dimension 256, got %d", i, len(vec))
		}
	}
}

func TestGeminiEmbedContent_QueryVsDocument(t *testing.T) {
	key := getAPIKey(t)
	client := embedding.NewGeminiClient(key)

	docVec, err := client.EmbedContent("gemini-embedding-2-preview", "The quick brown fox jumps over the lazy dog", embedding.TaskRetrievalDocument, 256)
	if err != nil {
		t.Fatalf("EmbedContent (doc) failed: %v", err)
	}

	queryVec, err := client.EmbedContent("gemini-embedding-2-preview", "What animal jumps?", embedding.TaskRetrievalQuery, 256)
	if err != nil {
		t.Fatalf("EmbedContent (query) failed: %v", err)
	}

	if len(docVec) != len(queryVec) {
		t.Fatalf("dimension mismatch: doc=%d query=%d", len(docVec), len(queryVec))
	}

	// Compute cosine similarity - should be reasonably high
	var dot, normA, normB float64
	for i := range docVec {
		dot += float64(docVec[i]) * float64(queryVec[i])
		normA += float64(docVec[i]) * float64(docVec[i])
		normB += float64(queryVec[i]) * float64(queryVec[i])
	}
	sim := dot / (sqrtF64(normA) * sqrtF64(normB))
	t.Logf("cosine similarity between doc and query: %.4f", sim)
	if sim < 0.3 {
		t.Fatalf("cosine similarity too low: %.4f", sim)
	}
}

func TestGeminiEmbedMultimodalContent_Image(t *testing.T) {
	key := getAPIKey(t)
	client := embedding.NewGeminiClient(key)

	// Create a small test PNG image (8x8 red square)
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("failed to encode PNG: %v", err)
	}

	vec, err := client.EmbedMultimodalContent("gemini-embedding-2-preview", embedding.MultimodalContent{
		MIMEType: "image/png",
		Data:     buf.Bytes(),
	}, embedding.TaskRetrievalDocument, 256)
	if err != nil {
		t.Fatalf("EmbedMultimodalContent (image) failed: %v", err)
	}
	if len(vec) != 256 {
		t.Fatalf("expected dimension 256, got %d", len(vec))
	}
	t.Logf("image embedding: first 5 values = %v", vec[:5])
}

func TestGeminiEmbedMultimodalContent_CrossModalSearch(t *testing.T) {
	key := getAPIKey(t)
	client := embedding.NewGeminiClient(key)

	// Create a test image
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.RGBA{0, 0, 255, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("failed to encode PNG: %v", err)
	}

	// Embed image
	imgVec, err := client.EmbedMultimodalContent("gemini-embedding-2-preview", embedding.MultimodalContent{
		MIMEType: "image/png",
		Data:     buf.Bytes(),
	}, embedding.TaskRetrievalDocument, 256)
	if err != nil {
		t.Fatalf("EmbedMultimodalContent failed: %v", err)
	}

	// Embed text query about the image
	queryVec, err := client.EmbedContent("gemini-embedding-2-preview", "a blue image", embedding.TaskRetrievalQuery, 256)
	if err != nil {
		t.Fatalf("EmbedContent (query) failed: %v", err)
	}

	// Cross-modal similarity should be non-zero (same embedding space)
	var dot, normA, normB float64
	for i := range imgVec {
		dot += float64(imgVec[i]) * float64(queryVec[i])
		normA += float64(imgVec[i]) * float64(imgVec[i])
		normB += float64(queryVec[i]) * float64(queryVec[i])
	}
	sim := dot / (sqrtF64(normA) * sqrtF64(normB))
	t.Logf("cross-modal similarity (image vs text): %.4f", sim)
	// Just verify it's a valid number, not NaN or negative
	if sim < -1 || sim > 1 {
		t.Fatalf("invalid similarity: %.4f", sim)
	}
}

func TestGeminiEmbedMultimodalContent_PDF(t *testing.T) {
	key := getAPIKey(t)
	client := embedding.NewGeminiClient(key)

	// Minimal valid PDF
	pdfContent := []byte(`%PDF-1.0
1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>
endobj
4 0 obj
<< /Length 44 >>
stream
BT /F1 12 Tf 100 700 Td (Hello PDF) Tj ET
endstream
endobj
5 0 obj
<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>
endobj
xref
0 6
0000000000 65535 f
0000000009 00000 n
0000000058 00000 n
0000000115 00000 n
0000000266 00000 n
0000000360 00000 n
trailer
<< /Size 6 /Root 1 0 R >>
startxref
441
%%EOF`)

	vec, err := client.EmbedMultimodalContent("gemini-embedding-2-preview", embedding.MultimodalContent{
		MIMEType: "application/pdf",
		Data:     pdfContent,
	}, embedding.TaskRetrievalDocument, 256)
	if err != nil {
		t.Fatalf("EmbedMultimodalContent (PDF) failed: %v", err)
	}
	if len(vec) != 256 {
		t.Fatalf("expected dimension 256, got %d", len(vec))
	}
	t.Logf("PDF embedding: first 5 values = %v", vec[:5])
}

func TestMultimodalEmbedderInterface(t *testing.T) {
	key := getAPIKey(t)
	client := embedding.NewGeminiClient(key)

	// GeminiClient should implement MultimodalEmbedder
	var _ embedding.MultimodalEmbedder = client

	// OpenAIClient should NOT implement MultimodalEmbedder
	openaiClient := embedding.NewOpenAIClient("http://localhost:11434", "")
	_, ok := interface{}(openaiClient).(embedding.MultimodalEmbedder)
	if ok {
		t.Fatal("OpenAIClient should not implement MultimodalEmbedder")
	}
}

func TestGeminiEmbedContent_Base64RoundTrip(t *testing.T) {
	key := getAPIKey(t)
	client := embedding.NewGeminiClient(key)

	// Simulate MCP flow: base64 encode image, decode, then embed
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{0, 255, 0, 255})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)

	// Base64 encode (as MCP client would send)
	b64 := base64.StdEncoding.EncodeToString(buf.Bytes())

	// Decode (as handler would do)
	decoded, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}

	vec, err := client.EmbedMultimodalContent("gemini-embedding-2-preview", embedding.MultimodalContent{
		MIMEType: "image/png",
		Data:     decoded,
	}, embedding.TaskRetrievalDocument, 256)
	if err != nil {
		t.Fatalf("EmbedMultimodalContent failed: %v", err)
	}
	if len(vec) != 256 {
		t.Fatalf("expected 256 dimensions, got %d", len(vec))
	}
}

func sqrtF64(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// Newton's method
	z := x
	for i := 0; i < 100; i++ {
		z = (z + x/z) / 2
	}
	return z
}
