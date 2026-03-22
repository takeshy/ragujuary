package embedding

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	geminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"
)

// GeminiClient is a Gemini Embedding API client
type GeminiClient struct {
	apiKey     string
	httpClient *http.Client
}

// NewGeminiClient creates a new Gemini Embedding API client
func NewGeminiClient(apiKey string) *GeminiClient {
	return &GeminiClient{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

type geminiEmbedRequest struct {
	Model                string        `json:"model"`
	Content              geminiContent `json:"content"`
	TaskType             string        `json:"taskType,omitempty"`
	OutputDimensionality int           `json:"outputDimensionality,omitempty"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *geminiInlineData `json:"inline_data,omitempty"`
}

type geminiInlineData struct {
	MIMEType string `json:"mime_type"`
	Data     string `json:"data"` // base64-encoded
}

type geminiEmbedResponse struct {
	Embedding geminiEmbeddingValues `json:"embedding"`
}

type geminiEmbeddingValues struct {
	Values []float32 `json:"values"`
}

type geminiBatchRequest struct {
	Requests []geminiEmbedRequest `json:"requests"`
}

type geminiBatchResponse struct {
	Embeddings []geminiEmbeddingValues `json:"embeddings"`
}

// doEmbedRequest sends an embedContent request and returns the embedding values
func (c *GeminiClient) doEmbedRequest(model string, reqBody geminiEmbedRequest) ([]float32, error) {
	url := fmt.Sprintf("%s/models/%s:embedContent?key=%s", geminiBaseURL, model, c.apiKey)

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to embed content: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed content failed with status %d: %s", resp.StatusCode, string(body))
	}

	var embedResp geminiEmbedResponse
	if err := json.Unmarshal(body, &embedResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return embedResp.Embedding.Values, nil
}

// EmbedContent generates an embedding for a single text
func (c *GeminiClient) EmbedContent(model, text string, taskType TaskType, dimension int) ([]float32, error) {
	reqBody := geminiEmbedRequest{
		Model: "models/" + model,
		Content: geminiContent{
			Parts: []geminiPart{{Text: text}},
		},
		TaskType: string(taskType),
	}
	if dimension > 0 {
		reqBody.OutputDimensionality = dimension
	}
	return c.doEmbedRequest(model, reqBody)
}

// EmbedMultimodalContent generates an embedding for multimodal content (image, PDF, video, audio)
func (c *GeminiClient) EmbedMultimodalContent(model string, content MultimodalContent, taskType TaskType, dimension int) ([]float32, error) {
	reqBody := geminiEmbedRequest{
		Model: "models/" + model,
		Content: geminiContent{
			Parts: []geminiPart{{
				InlineData: &geminiInlineData{
					MIMEType: content.MIMEType,
					Data:     base64.StdEncoding.EncodeToString(content.Data),
				},
			}},
		},
		TaskType: string(taskType),
	}
	if dimension > 0 {
		reqBody.OutputDimensionality = dimension
	}
	return c.doEmbedRequest(model, reqBody)
}

// BatchEmbedContents generates embeddings for multiple texts
func (c *GeminiClient) BatchEmbedContents(model string, texts []string, taskType TaskType, dimension int) ([][]float32, error) {
	url := fmt.Sprintf("%s/models/%s:batchEmbedContents?key=%s", geminiBaseURL, model, c.apiKey)

	requests := make([]geminiEmbedRequest, len(texts))
	for i, text := range texts {
		req := geminiEmbedRequest{
			Model: "models/" + model,
			Content: geminiContent{
				Parts: []geminiPart{{Text: text}},
			},
			TaskType: string(taskType),
		}
		if dimension > 0 {
			req.OutputDimensionality = dimension
		}
		requests[i] = req
	}

	reqBody := geminiBatchRequest{Requests: requests}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to batch embed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("batch embed failed with status %d: %s", resp.StatusCode, string(body))
	}

	var batchResp geminiBatchResponse
	if err := json.Unmarshal(body, &batchResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	result := make([][]float32, len(batchResp.Embeddings))
	for i, emb := range batchResp.Embeddings {
		result[i] = emb.Values
	}

	return result, nil
}
