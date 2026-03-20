package embedding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAIClient is an OpenAI-compatible embedding client.
// Works with Ollama, LM Studio, vLLM, and any OpenAI-compatible API.
type OpenAIClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewOpenAIClient creates a new OpenAI-compatible embedding client
func NewOpenAIClient(baseURL, apiKey string) *OpenAIClient {
	// Normalize base URL
	baseURL = strings.TrimRight(baseURL, "/")
	return &OpenAIClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

type openAIEmbedRequest struct {
	Model string      `json:"model"`
	Input interface{} `json:"input"` // string or []string
}

type openAIEmbedResponse struct {
	Data []openAIEmbedData `json:"data"`
}

type openAIEmbedData struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

func (c *OpenAIClient) doRequest(model string, input interface{}) (*openAIEmbedResponse, error) {
	url := c.baseURL + "/v1/embeddings"

	reqBody := openAIEmbedRequest{
		Model: model,
		Input: input,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call embedding API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding API failed with status %d: %s", resp.StatusCode, string(body))
	}

	var embedResp openAIEmbedResponse
	if err := json.Unmarshal(body, &embedResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &embedResp, nil
}

// EmbedContent generates an embedding for a single text.
// taskType and dimension are ignored (not supported by OpenAI-compatible APIs).
func (c *OpenAIClient) EmbedContent(model, text string, _ TaskType, _ int) ([]float32, error) {
	resp, err := c.doRequest(model, text)
	if err != nil {
		return nil, err
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}

	return resp.Data[0].Embedding, nil
}

// BatchEmbedContents generates embeddings for multiple texts.
// taskType and dimension are ignored (not supported by OpenAI-compatible APIs).
func (c *OpenAIClient) BatchEmbedContents(model string, texts []string, _ TaskType, _ int) ([][]float32, error) {
	resp, err := c.doRequest(model, texts)
	if err != nil {
		return nil, err
	}

	// Sort by index to maintain order
	result := make([][]float32, len(texts))
	for _, d := range resp.Data {
		if d.Index < len(result) {
			result[d.Index] = d.Embedding
		}
	}

	return result, nil
}
