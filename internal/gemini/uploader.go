package gemini

import (
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

// Uploader handles parallel file uploads
type Uploader struct {
	client       *Client
	storeManager *store.Manager
	storeName    string
	parallelism  int
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
	}
}

// UploadFiles uploads multiple files in parallel
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

// uploadFile uploads a single file
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

	// If file exists but checksum changed, delete old file first
	if found {
		if err := u.client.DeleteFile(existing.RemoteID); err != nil {
			// Log warning but continue with upload
			fmt.Printf("Warning: failed to delete old file %s: %v\n", existing.RemoteID, err)
		}
	}

	// Create display name with store prefix
	displayName := fmt.Sprintf("[%s] %s", u.storeName, file.Path)

	// Upload file
	resp, err := u.client.UploadFile(file.Path, displayName, file.MimeType)
	if err != nil {
		result.Error = fmt.Errorf("failed to upload: %w", err)
		return result
	}

	// Create metadata
	meta := store.FileMetadata{
		LocalPath:   file.Path,
		RemoteID:    resp.Name,
		RemoteName:  resp.DisplayName,
		Checksum:    checksum,
		Size:        file.Size,
		UploadedAt:  time.Now(),
		MimeType:    file.MimeType,
	}

	// Save to store
	u.storeManager.AddFile(u.storeName, meta)

	result.Metadata = &meta
	return result
}

// DeleteFilesByPattern deletes files matching a pattern
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

		// Delete from Gemini
		if err := u.client.DeleteFile(f.RemoteID); err != nil {
			errors = append(errors, fmt.Errorf("failed to delete %s: %w", f.LocalPath, err))
			continue
		}

		// Remove from store
		u.storeManager.RemoveFile(u.storeName, f.LocalPath)
		deleted = append(deleted, f)
	}

	return deleted, errors
}
