package gemini

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/takeshy/ragujuary/internal/fileutil"
	"github.com/takeshy/ragujuary/internal/store"
)

// UploadResult represents the result of a file upload
type UploadResult struct {
	FileInfo fileutil.FileInfo
	Metadata *store.FileMetadata
	Error    error
	Skipped  bool
	Reason   string
}

// Uploader handles parallel file uploads to File Search Store
type Uploader struct {
	client       *Client
	storeManager *store.Manager
	storeName    string
	parallelism  int
	waitForDone  bool
}

// NewUploader creates a new uploader
func NewUploader(client *Client, storeManager *store.Manager, storeName string, parallelism int) *Uploader {
	if parallelism < 1 {
		parallelism = 5
	}
	return &Uploader{
		client:       client,
		storeManager: storeManager,
		storeName:    storeName,
		parallelism:  parallelism,
		waitForDone:  true,
	}
}

// SetWaitForDone sets whether to wait for upload operations to complete
func (u *Uploader) SetWaitForDone(wait bool) {
	u.waitForDone = wait
}

// UploadFiles uploads multiple files in parallel to a File Search Store
func (u *Uploader) UploadFiles(files []fileutil.FileInfo, progressCallback func(result UploadResult)) []UploadResult {
	results := make([]UploadResult, len(files))
	jobs := make(chan int, len(files))
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < u.parallelism; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				result := u.uploadFile(files[idx])
				results[idx] = result
				if progressCallback != nil {
					progressCallback(result)
				}
			}
		}()
	}

	// Send jobs
	for i := range files {
		jobs <- i
	}
	close(jobs)

	wg.Wait()
	return results
}

// uploadFile uploads a single file to the File Search Store
func (u *Uploader) uploadFile(file fileutil.FileInfo) UploadResult {
	result := UploadResult{
		FileInfo: file,
	}

	// Calculate checksum
	checksum, err := fileutil.CalculateChecksum(file.Path)
	if err != nil {
		result.Error = fmt.Errorf("failed to calculate checksum: %w", err)
		return result
	}
	file.Checksum = checksum

	// Check if file already exists with same checksum
	existing, found := u.storeManager.GetFileByPath(u.storeName, file.Path)
	if found && existing.Checksum == checksum {
		result.Skipped = true
		result.Reason = "file unchanged (same checksum)"
		result.Metadata = existing
		return result
	}

	// If file exists but checksum changed, delete old document first
	if found && existing.RemoteID != "" {
		if err := u.client.DeleteDocument(existing.RemoteID); err != nil {
			// Log warning but continue with upload
			fmt.Printf("Warning: failed to delete old document %s: %v\n", existing.RemoteID, err)
		}
	}

	// Create upload config with display name
	config := &UploadConfig{
		DisplayName: file.Path,
	}

	// Upload file to File Search Store
	op, err := u.client.UploadToFileSearchStore(u.storeName, file.Path, config)
	if err != nil {
		result.Error = fmt.Errorf("failed to upload: %w", err)
		return result
	}

	// Wait for operation to complete if configured
	if u.waitForDone && !op.Done {
		op, err = u.client.WaitForOperation(op.Name, 2*time.Second)
		if err != nil {
			result.Error = fmt.Errorf("upload operation failed: %w", err)
			return result
		}
	}

	// Extract actual document name from operation response
	documentName := op.Name // Fallback to operation name if parsing fails
	if op.Done && len(op.Response) > 0 {
		var doc FileSearchDocument
		if err := json.Unmarshal(op.Response, &doc); err == nil && doc.Name != "" {
			documentName = doc.Name
		}
	}

	// Create metadata
	meta := store.FileMetadata{
		LocalPath:  file.Path,
		RemoteID:   documentName,
		RemoteName: file.Path,
		Checksum:   checksum,
		Size:       file.Size,
		UploadedAt: time.Now(),
		MimeType:   file.MimeType,
	}

	// Save to store
	u.storeManager.AddFile(u.storeName, meta)

	result.Metadata = &meta
	return result
}

// DeleteFilesByPattern deletes documents matching a pattern from the File Search Store
func (u *Uploader) DeleteFilesByPattern(pattern string) ([]store.FileMetadata, []error) {
	files := u.storeManager.GetAllFiles(u.storeName)
	if len(files) == 0 {
		return nil, nil
	}

	// Convert to FileInfo for filtering
	fileInfos := make([]fileutil.FileInfo, len(files))
	for i, f := range files {
		fileInfos[i] = fileutil.FileInfo{Path: f.LocalPath}
	}

	// Filter by pattern
	matched, err := fileutil.FilterFilesByPattern(fileInfos, pattern)
	if err != nil {
		return nil, []error{err}
	}

	// Create map for quick lookup
	matchedPaths := make(map[string]bool)
	for _, f := range matched {
		matchedPaths[f.Path] = true
	}

	var deleted []store.FileMetadata
	var errors []error

	for _, f := range files {
		if !matchedPaths[f.LocalPath] {
			continue
		}

		// Delete document from File Search Store
		if err := u.client.DeleteDocument(f.RemoteID); err != nil {
			errors = append(errors, fmt.Errorf("failed to delete %s: %w", f.LocalPath, err))
			continue
		}

		// Remove from store
		u.storeManager.RemoveFile(u.storeName, f.LocalPath)
		deleted = append(deleted, f)
	}

	return deleted, errors
}
