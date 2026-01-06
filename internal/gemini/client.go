package gemini

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	baseURL            = "https://generativelanguage.googleapis.com/v1beta"
	uploadBaseURL      = "https://generativelanguage.googleapis.com/upload/v1beta"
	fileSearchStoreURL = "https://generativelanguage.googleapis.com/v1beta/fileSearchStores"
)

// Client is a Gemini API client
type Client struct {
	apiKey     string
	httpClient *http.Client
}

// FileSearchStore represents a File Search Store
type FileSearchStore struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	CreateTime  string `json:"createTime"`
	UpdateTime  string `json:"updateTime"`
}

// FileSearchDocument represents a document in a File Search Store
type FileSearchDocument struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	CreateTime  string `json:"createTime"`
	UpdateTime  string `json:"updateTime"`
	State       string `json:"state"`
	Error       *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// ListFileSearchStoresResponse represents the response from listing stores
type ListFileSearchStoresResponse struct {
	FileSearchStores []FileSearchStore `json:"fileSearchStores"`
	NextPageToken    string            `json:"nextPageToken"`
}

// ListDocumentsResponse represents the response from listing documents
type ListDocumentsResponse struct {
	Documents     []FileSearchDocument `json:"documents"`
	NextPageToken string               `json:"nextPageToken"`
}

// Operation represents a long-running operation
type Operation struct {
	Name     string          `json:"name"`
	Done     bool            `json:"done"`
	Error    *OperationError `json:"error,omitempty"`
	Response json.RawMessage `json:"response,omitempty"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// OperationError represents an error in an operation
type OperationError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// UploadConfig represents the configuration for uploading a file
type UploadConfig struct {
	DisplayName    string            `json:"displayName,omitempty"`
	CustomMetadata []CustomMetadata  `json:"customMetadata,omitempty"`
	ChunkingConfig *ChunkingConfig   `json:"chunkingConfig,omitempty"`
}

// CustomMetadata represents custom metadata for a document
type CustomMetadata struct {
	Key          string  `json:"key"`
	StringValue  *string `json:"stringValue,omitempty"`
	NumericValue *int    `json:"numericValue,omitempty"`
}

// ChunkingConfig represents chunking configuration
type ChunkingConfig struct {
	WhiteSpaceConfig *WhiteSpaceConfig `json:"whiteSpaceConfig,omitempty"`
}

// WhiteSpaceConfig represents white space chunking configuration
type WhiteSpaceConfig struct {
	MaxTokensPerChunk int `json:"maxTokensPerChunk,omitempty"`
	MaxOverlapTokens  int `json:"maxOverlapTokens,omitempty"`
}

// NewClient creates a new Gemini API client
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute,
		},
	}
}

// CreateFileSearchStore creates a new File Search Store
func (c *Client) CreateFileSearchStore(displayName string) (*FileSearchStore, error) {
	url := fmt.Sprintf("%s?key=%s", fileSearchStoreURL, c.apiKey)

	body := map[string]string{
		"displayName": displayName,
	}
	jsonBody, err := json.Marshal(body)
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
		return nil, fmt.Errorf("failed to create store: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("create store failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var store FileSearchStore
	if err := json.Unmarshal(respBody, &store); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &store, nil
}

// GetFileSearchStore gets a File Search Store by name
func (c *Client) GetFileSearchStore(name string) (*FileSearchStore, error) {
	// Ensure name has prefix
	if !strings.HasPrefix(name, "fileSearchStores/") {
		name = "fileSearchStores/" + name
	}

	url := fmt.Sprintf("%s/%s?key=%s", baseURL, name, c.apiKey)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get store: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get store failed with status %d: %s", resp.StatusCode, string(body))
	}

	var store FileSearchStore
	if err := json.Unmarshal(body, &store); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &store, nil
}

// ListFileSearchStores lists all File Search Stores
func (c *Client) ListFileSearchStores(pageToken string) (*ListFileSearchStoresResponse, error) {
	url := fmt.Sprintf("%s?key=%s&pageSize=20", fileSearchStoreURL, c.apiKey)
	if pageToken != "" {
		url += "&pageToken=" + pageToken
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list stores: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list stores failed with status %d: %s", resp.StatusCode, string(body))
	}

	var listResp ListFileSearchStoresResponse
	if err := json.Unmarshal(body, &listResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &listResp, nil
}

// ListAllFileSearchStores lists all File Search Stores (handles pagination)
func (c *Client) ListAllFileSearchStores() ([]FileSearchStore, error) {
	var allStores []FileSearchStore
	pageToken := ""

	for {
		resp, err := c.ListFileSearchStores(pageToken)
		if err != nil {
			return nil, err
		}

		allStores = append(allStores, resp.FileSearchStores...)

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	return allStores, nil
}

// ResolveStoreName resolves a store name
// If name contains "fileSearchStores/", treat it as API name
// Otherwise, search by display name
// Returns the API store name (without fileSearchStores/ prefix) and the store
func (c *Client) ResolveStoreName(name string) (string, *FileSearchStore, error) {
	// If contains prefix, treat as API name
	if strings.Contains(name, "fileSearchStores/") {
		apiName := strings.TrimPrefix(name, "fileSearchStores/")
		store, err := c.GetFileSearchStore(apiName)
		if err != nil {
			return "", nil, fmt.Errorf("store '%s' not found: %w", name, err)
		}
		storeName := strings.TrimPrefix(store.Name, "fileSearchStores/")
		return storeName, store, nil
	}

	// Otherwise, search by display name
	stores, err := c.ListAllFileSearchStores()
	if err != nil {
		return "", nil, fmt.Errorf("failed to list stores: %w", err)
	}

	for _, s := range stores {
		if s.DisplayName == name {
			storeName := strings.TrimPrefix(s.Name, "fileSearchStores/")
			return storeName, &s, nil
		}
	}

	return "", nil, fmt.Errorf("store with display name '%s' not found", name)
}

// DeleteFileSearchStore deletes a File Search Store
func (c *Client) DeleteFileSearchStore(name string, force bool) error {
	// Ensure name has prefix
	if !strings.HasPrefix(name, "fileSearchStores/") {
		name = "fileSearchStores/" + name
	}

	url := fmt.Sprintf("%s/%s?key=%s", baseURL, name, c.apiKey)
	if force {
		url += "&force=true"
	}

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete store: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete store failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// UploadToFileSearchStore uploads a file to a File Search Store using resumable upload
func (c *Client) UploadToFileSearchStore(storeName string, filePath string, config *UploadConfig) (*Operation, error) {
	// Ensure store name has prefix
	if !strings.HasPrefix(storeName, "fileSearchStores/") {
		storeName = "fileSearchStores/" + storeName
	}

	// Get file info
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Detect MIME type
	mimeType := detectMimeType(filePath)

	// Step 1: Initiate resumable upload
	initURL := fmt.Sprintf("%s/%s:uploadToFileSearchStore?key=%s", uploadBaseURL, storeName, c.apiKey)

	// Prepare metadata
	metadata := map[string]interface{}{}
	if config != nil && config.DisplayName != "" {
		metadata["displayName"] = config.DisplayName
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	initReq, err := http.NewRequest("POST", initURL, bytes.NewReader(metadataJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create init request: %w", err)
	}

	initReq.Header.Set("Content-Type", "application/json")
	initReq.Header.Set("X-Goog-Upload-Protocol", "resumable")
	initReq.Header.Set("X-Goog-Upload-Command", "start")
	initReq.Header.Set("X-Goog-Upload-Header-Content-Length", fmt.Sprintf("%d", fileInfo.Size()))
	initReq.Header.Set("X-Goog-Upload-Header-Content-Type", mimeType)

	initResp, err := c.httpClient.Do(initReq)
	if err != nil {
		return nil, fmt.Errorf("failed to initiate upload: %w", err)
	}
	defer initResp.Body.Close()

	if initResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(initResp.Body)
		return nil, fmt.Errorf("upload init failed with status %d: %s", initResp.StatusCode, string(body))
	}

	// Get upload URL from response header
	uploadURL := initResp.Header.Get("X-Goog-Upload-URL")
	if uploadURL == "" {
		body, _ := io.ReadAll(initResp.Body)
		return nil, fmt.Errorf("no upload URL in response, body: %s", string(body))
	}

	// Step 2: Upload file content
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	uploadReq, err := http.NewRequest("POST", uploadURL, file)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload request: %w", err)
	}

	uploadReq.Header.Set("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))
	uploadReq.Header.Set("X-Goog-Upload-Offset", "0")
	uploadReq.Header.Set("X-Goog-Upload-Command", "upload, finalize")

	uploadResp, err := c.httpClient.Do(uploadReq)
	if err != nil {
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}
	defer uploadResp.Body.Close()

	body, err := io.ReadAll(uploadResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if uploadResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upload failed with status %d: %s", uploadResp.StatusCode, string(body))
	}

	var operation Operation
	if err := json.Unmarshal(body, &operation); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// If operation name is empty but we have a response, the upload might be synchronous
	// or the response format is different
	if operation.Name == "" {
		// Try to parse as a document response directly
		var doc FileSearchDocument
		if err := json.Unmarshal(body, &doc); err == nil && doc.Name != "" {
			// Upload completed synchronously
			return &Operation{
				Name: doc.Name,
				Done: true,
			}, nil
		}
		return nil, fmt.Errorf("upload response has empty operation name, raw response: %s", string(body))
	}

	return &operation, nil
}

// UploadContentToFileSearchStore uploads content directly to a File Search Store
func (c *Client) UploadContentToFileSearchStore(storeName string, fileName string, content []byte) (*Operation, error) {
	// Ensure store name has prefix
	if !strings.HasPrefix(storeName, "fileSearchStores/") {
		storeName = "fileSearchStores/" + storeName
	}

	// Detect MIME type from file name
	mimeType := detectMimeType(fileName)

	// Step 1: Initiate resumable upload
	initURL := fmt.Sprintf("%s/%s:uploadToFileSearchStore?key=%s", uploadBaseURL, storeName, c.apiKey)

	// Prepare metadata
	metadata := map[string]interface{}{
		"displayName": fileName,
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	initReq, err := http.NewRequest("POST", initURL, bytes.NewReader(metadataJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create init request: %w", err)
	}

	initReq.Header.Set("Content-Type", "application/json")
	initReq.Header.Set("X-Goog-Upload-Protocol", "resumable")
	initReq.Header.Set("X-Goog-Upload-Command", "start")
	initReq.Header.Set("X-Goog-Upload-Header-Content-Length", fmt.Sprintf("%d", len(content)))
	initReq.Header.Set("X-Goog-Upload-Header-Content-Type", mimeType)

	initResp, err := c.httpClient.Do(initReq)
	if err != nil {
		return nil, fmt.Errorf("failed to initiate upload: %w", err)
	}
	defer initResp.Body.Close()

	if initResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(initResp.Body)
		return nil, fmt.Errorf("upload init failed with status %d: %s", initResp.StatusCode, string(body))
	}

	// Get upload URL from response header
	uploadURL := initResp.Header.Get("X-Goog-Upload-URL")
	if uploadURL == "" {
		body, _ := io.ReadAll(initResp.Body)
		return nil, fmt.Errorf("no upload URL in response, body: %s", string(body))
	}

	// Step 2: Upload content
	uploadReq, err := http.NewRequest("POST", uploadURL, bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("failed to create upload request: %w", err)
	}

	uploadReq.Header.Set("Content-Length", fmt.Sprintf("%d", len(content)))
	uploadReq.Header.Set("X-Goog-Upload-Offset", "0")
	uploadReq.Header.Set("X-Goog-Upload-Command", "upload, finalize")

	uploadResp, err := c.httpClient.Do(uploadReq)
	if err != nil {
		return nil, fmt.Errorf("failed to upload content: %w", err)
	}
	defer uploadResp.Body.Close()

	body, err := io.ReadAll(uploadResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if uploadResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upload failed with status %d: %s", uploadResp.StatusCode, string(body))
	}

	var operation Operation
	if err := json.Unmarshal(body, &operation); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if operation.Name == "" {
		var doc FileSearchDocument
		if err := json.Unmarshal(body, &doc); err == nil && doc.Name != "" {
			return &Operation{
				Name: doc.Name,
				Done: true,
			}, nil
		}
		return nil, fmt.Errorf("upload response has empty operation name, raw response: %s", string(body))
	}

	return &operation, nil
}

// detectMimeType detects the MIME type of a file based on its extension
func detectMimeType(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	mimeTypes := map[string]string{
		".txt":  "text/plain",
		".md":   "text/markdown",
		".json": "application/json",
		".pdf":  "application/pdf",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".doc":  "application/msword",
		".csv":  "text/csv",
		".xml":  "application/xml",
		".html": "text/html",
		".htm":  "text/html",
		".go":   "text/x-go",
		".py":   "text/x-python",
		".js":   "text/javascript",
		".ts":   "text/typescript",
		".java": "text/x-java",
		".c":    "text/x-c",
		".cpp":  "text/x-c++",
		".h":    "text/x-c",
		".rb":   "text/x-ruby",
		".rs":   "text/x-rust",
		".sql":  "text/x-sql",
		".sh":   "text/x-shellscript",
		".yaml": "text/yaml",
		".yml":  "text/yaml",
	}

	if mimeType, ok := mimeTypes[ext]; ok {
		return mimeType
	}
	return "application/octet-stream"
}

// GetOperation gets the status of a long-running operation
func (c *Client) GetOperation(name string) (*Operation, error) {
	// The operation name from uploadToFileSearchStore is in format:
	// fileSearchStores/{store}/upload/operations/{operation}
	// We need to use it directly
	var url string
	if strings.HasPrefix(name, "fileSearchStores/") {
		url = fmt.Sprintf("%s/%s?key=%s", baseURL, name, c.apiKey)
	} else if strings.HasPrefix(name, "operations/") {
		url = fmt.Sprintf("%s/%s?key=%s", baseURL, name, c.apiKey)
	} else {
		url = fmt.Sprintf("%s/operations/%s?key=%s", baseURL, name, c.apiKey)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get operation: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get operation failed with status %d (name=%s): %s", resp.StatusCode, name, string(body))
	}

	var operation Operation
	if err := json.Unmarshal(body, &operation); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &operation, nil
}

// WaitForOperation waits for a long-running operation to complete
func (c *Client) WaitForOperation(name string, pollInterval time.Duration) (*Operation, error) {
	for {
		op, err := c.GetOperation(name)
		if err != nil {
			return nil, err
		}

		if op.Done {
			if op.Error != nil {
				return op, fmt.Errorf("operation failed: %s", op.Error.Message)
			}
			return op, nil
		}

		time.Sleep(pollInterval)
	}
}

// ListDocuments lists all documents in a File Search Store
func (c *Client) ListDocuments(storeName string, pageToken string) (*ListDocumentsResponse, error) {
	// Ensure store name has prefix
	if !strings.HasPrefix(storeName, "fileSearchStores/") {
		storeName = "fileSearchStores/" + storeName
	}

	url := fmt.Sprintf("%s/%s/documents?key=%s&pageSize=20", baseURL, storeName, c.apiKey)
	if pageToken != "" {
		url += "&pageToken=" + pageToken
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list documents: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list documents failed with status %d: %s", resp.StatusCode, string(body))
	}

	var listResp ListDocumentsResponse
	if err := json.Unmarshal(body, &listResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &listResp, nil
}

// ListAllDocuments lists all documents in a File Search Store (handles pagination)
func (c *Client) ListAllDocuments(storeName string) ([]FileSearchDocument, error) {
	var allDocs []FileSearchDocument
	pageToken := ""

	for {
		resp, err := c.ListDocuments(storeName, pageToken)
		if err != nil {
			return nil, err
		}

		allDocs = append(allDocs, resp.Documents...)

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	return allDocs, nil
}

// DeleteDocument deletes a document from a File Search Store
func (c *Client) DeleteDocument(documentName string) error {
	url := fmt.Sprintf("%s/%s?key=%s", baseURL, documentName, c.apiKey)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete document failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GenerateContentRequest represents a request to generate content
type GenerateContentRequest struct {
	Contents []Content `json:"contents"`
	Tools    []Tool    `json:"tools,omitempty"`
}

// Content represents content in a request
type Content struct {
	Parts []Part `json:"parts"`
	Role  string `json:"role,omitempty"`
}

// Part represents a part of content
type Part struct {
	Text string `json:"text,omitempty"`
}

// Tool represents a tool configuration
type Tool struct {
	FileSearch *FileSearchTool `json:"fileSearch,omitempty"`
}

// FileSearchTool represents a File Search tool configuration
type FileSearchTool struct {
	FileSearchStoreNames []string `json:"fileSearchStoreNames"`
	MetadataFilter       string   `json:"metadataFilter,omitempty"`
}

// GenerateContentResponse represents the response from generate content
type GenerateContentResponse struct {
	Candidates []Candidate `json:"candidates"`
}

// Candidate represents a candidate response
type Candidate struct {
	Content           Content            `json:"content"`
	GroundingMetadata *GroundingMetadata `json:"groundingMetadata,omitempty"`
}

// GroundingMetadata represents grounding metadata (citations)
type GroundingMetadata struct {
	GroundingChunks []GroundingChunk `json:"groundingChunks,omitempty"`
}

// GroundingChunk represents a grounding chunk (citation source)
type GroundingChunk struct {
	RetrievedContext *RetrievedContext `json:"retrievedContext,omitempty"`
}

// RetrievedContext represents retrieved context from File Search
type RetrievedContext struct {
	URI   string `json:"uri,omitempty"`
	Title string `json:"title,omitempty"`
	Text  string `json:"text,omitempty"`
}

// Query performs a query using File Search
func (c *Client) Query(model string, query string, storeNames []string, metadataFilter string) (*GenerateContentResponse, error) {
	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", baseURL, model, c.apiKey)

	// Ensure store names have prefix
	for i, name := range storeNames {
		if !strings.HasPrefix(name, "fileSearchStores/") {
			storeNames[i] = "fileSearchStores/" + name
		}
	}

	reqBody := GenerateContentRequest{
		Contents: []Content{
			{
				Parts: []Part{
					{Text: query},
				},
			},
		},
		Tools: []Tool{
			{
				FileSearch: &FileSearchTool{
					FileSearchStoreNames: storeNames,
					MetadataFilter:       metadataFilter,
				},
			},
		},
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("query failed with status %d: %s", resp.StatusCode, string(body))
	}

	var genResp GenerateContentResponse
	if err := json.Unmarshal(body, &genResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &genResp, nil
}
