package gemini

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	baseURL       = "https://generativelanguage.googleapis.com"
	uploadURL     = "https://generativelanguage.googleapis.com/upload/v1beta/files"
	filesURL      = "https://generativelanguage.googleapis.com/v1beta/files"
)

// Client is a Gemini API client
type Client struct {
	apiKey     string
	httpClient *http.Client
}

// FileResponse represents a file in Gemini
type FileResponse struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	MimeType    string `json:"mimeType"`
	SizeBytes   string `json:"sizeBytes"`
	CreateTime  string `json:"createTime"`
	UpdateTime  string `json:"updateTime"`
	ExpirationTime string `json:"expirationTime"`
	SHA256Hash  string `json:"sha256Hash"`
	URI         string `json:"uri"`
	State       string `json:"state"`
}

// UploadResponse represents the response from file upload
type UploadResponse struct {
	File FileResponse `json:"file"`
}

// ListFilesResponse represents the response from listing files
type ListFilesResponse struct {
	Files         []FileResponse `json:"files"`
	NextPageToken string         `json:"nextPageToken"`
}

// NewClient creates a new Gemini API client
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

// UploadFile uploads a file to Gemini
func (c *Client) UploadFile(filePath string, displayName string, mimeType string) (*FileResponse, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// If displayName is empty, use the file name
	if displayName == "" {
		displayName = filepath.Base(filePath)
	}

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add metadata
	metadataField, err := writer.CreateFormField("metadata")
	if err != nil {
		return nil, fmt.Errorf("failed to create metadata field: %w", err)
	}

	metadata := map[string]interface{}{
		"file": map[string]string{
			"display_name": displayName,
		},
	}
	if err := json.NewEncoder(metadataField).Encode(metadata); err != nil {
		return nil, fmt.Errorf("failed to encode metadata: %w", err)
	}

	// Add file
	fileField, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("failed to create file field: %w", err)
	}

	if _, err := io.Copy(fileField, file); err != nil {
		return nil, fmt.Errorf("failed to copy file content: %w", err)
	}

	writer.Close()

	// Create request
	url := fmt.Sprintf("%s?key=%s", uploadURL, c.apiKey)
	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Goog-Upload-Protocol", "multipart")
	req.Header.Set("X-Goog-Upload-Header-Content-Length", fmt.Sprintf("%d", fileInfo.Size()))
	if mimeType != "" {
		req.Header.Set("X-Goog-Upload-Header-Content-Type", mimeType)
	}

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	var uploadResp UploadResponse
	if err := json.Unmarshal(body, &uploadResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &uploadResp.File, nil
}

// ListFiles lists all files
func (c *Client) ListFiles(pageToken string) (*ListFilesResponse, error) {
	url := fmt.Sprintf("%s?key=%s&pageSize=100", filesURL, c.apiKey)
	if pageToken != "" {
		url += "&pageToken=" + pageToken
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list failed with status %d: %s", resp.StatusCode, string(body))
	}

	var listResp ListFilesResponse
	if err := json.Unmarshal(body, &listResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &listResp, nil
}

// ListAllFiles lists all files (handles pagination)
func (c *Client) ListAllFiles() ([]FileResponse, error) {
	var allFiles []FileResponse
	pageToken := ""

	for {
		resp, err := c.ListFiles(pageToken)
		if err != nil {
			return nil, err
		}

		allFiles = append(allFiles, resp.Files...)

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	return allFiles, nil
}

// GetFile gets a file by name
func (c *Client) GetFile(name string) (*FileResponse, error) {
	url := fmt.Sprintf("%s/%s?key=%s", filesURL, name, c.apiKey)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get file: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get failed with status %d: %s", resp.StatusCode, string(body))
	}

	var fileResp FileResponse
	if err := json.Unmarshal(body, &fileResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &fileResp, nil
}

// DeleteFile deletes a file by name
func (c *Client) DeleteFile(name string) error {
	url := fmt.Sprintf("%s/%s?key=%s", filesURL, name, c.apiKey)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
