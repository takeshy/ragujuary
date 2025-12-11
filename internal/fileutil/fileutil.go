package fileutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
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
	ext := filepath.Ext(path)
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
	}

	if mime, ok := mimeTypes[ext]; ok {
		return mime
	}
	return "application/octet-stream"
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
