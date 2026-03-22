package fileutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FileInfo represents information about a file
type FileInfo struct {
	Path     string
	Size     int64
	Checksum string
	MimeType string
}

// CalculateChecksum calculates SHA256 checksum of a file
func CalculateChecksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("failed to calculate checksum: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// DiscoverFiles discovers files in the given directories
// excluding files matching any of the exclude patterns
func DiscoverFiles(dirs []string, excludePatterns []string) ([]FileInfo, error) {
	// Compile exclude patterns
	excludeRegexps := make([]*regexp.Regexp, 0, len(excludePatterns))
	for _, pattern := range excludePatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid exclude pattern %q: %w", pattern, err)
		}
		excludeRegexps = append(excludeRegexps, re)
	}

	var files []FileInfo

	for _, dir := range dirs {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path for %q: %w", dir, err)
		}

		err = filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Skip directories
			if info.IsDir() {
				// Check if directory should be excluded
				for _, re := range excludeRegexps {
					if re.MatchString(path) {
						return filepath.SkipDir
					}
				}
				return nil
			}

			// Check if file should be excluded
			for _, re := range excludeRegexps {
				if re.MatchString(path) {
					return nil
				}
			}

			files = append(files, FileInfo{
				Path:     path,
				Size:     info.Size(),
				MimeType: detectMimeType(path),
			})

			return nil
		})

		if err != nil {
			return nil, fmt.Errorf("failed to walk directory %q: %w", dir, err)
		}
	}

	return files, nil
}

// detectMimeType detects MIME type based on file extension
func detectMimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	mimeTypes := map[string]string{
		".txt":  "text/plain",
		".md":   "text/markdown",
		".html": "text/html",
		".htm":  "text/html",
		".css":  "text/css",
		".js":   "text/javascript",
		".ts":   "text/typescript",
		".json": "application/json",
		".xml":  "application/xml",
		".yaml": "application/x-yaml",
		".yml":  "application/x-yaml",
		".go":   "text/x-go",
		".py":   "text/x-python",
		".java": "text/x-java",
		".c":    "text/x-c",
		".cpp":  "text/x-c++",
		".h":    "text/x-c",
		".hpp":  "text/x-c++",
		".rs":   "text/x-rust",
		".rb":   "text/x-ruby",
		".php":  "text/x-php",
		".sh":   "text/x-shellscript",
		".pdf":  "application/pdf",
		".doc":  "application/msword",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".csv":  "text/csv",
		// Image types
		".png":  "image/png",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".gif":  "image/gif",
		".webp": "image/webp",
		// Video types
		".mp4":  "video/mp4",
		".mpeg": "video/mpeg",
		".mpg":  "video/mpeg",
		// Audio types
		".mp3": "audio/mp3",
		".wav": "audio/wav",
		".ogg": "audio/ogg",
	}

	if mime, ok := mimeTypes[ext]; ok {
		return mime
	}
	return "application/octet-stream"
}

// ClassifyContent returns the content type category for a MIME type.
// Returns "text", "image", "pdf", "video", or "audio".
func ClassifyContent(mimeType string) string {
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return "image"
	case mimeType == "application/pdf":
		return "pdf"
	case strings.HasPrefix(mimeType, "video/"):
		return "video"
	case strings.HasPrefix(mimeType, "audio/"):
		return "audio"
	default:
		return "text"
	}
}

// IsMultimodal returns true if the content type is non-text (image, pdf, video, audio).
func IsMultimodal(contentType string) bool {
	return contentType != "text" && contentType != ""
}

// SupportedEmbeddingMIME returns true if the MIME type is supported by Gemini's
// multimodal embedding API.
func SupportedEmbeddingMIME(mimeType string) bool {
	switch mimeType {
	case "image/png", "image/jpeg",
		"application/pdf",
		"video/mp4", "video/mpeg",
		"audio/mp3", "audio/wav":
		return true
	default:
		return false
	}
}

// FilterFilesByPattern filters files by a regex pattern
func FilterFilesByPattern(files []FileInfo, pattern string) ([]FileInfo, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern %q: %w", pattern, err)
	}

	var filtered []FileInfo
	for _, f := range files {
		if re.MatchString(f.Path) {
			filtered = append(filtered, f)
		}
	}
	return filtered, nil
}
