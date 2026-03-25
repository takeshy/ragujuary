package rag

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

const (
	indexFileName   = "index.json"
	vectorsFileName = "vectors.bin"
	formatVersion   = 2
)

// ChunkMeta holds metadata for a single chunk
type ChunkMeta struct {
	FilePath    string `json:"file_path"`
	StartOffset int    `json:"start_offset"`
	Text        string `json:"text"`
	ContentType string `json:"content_type,omitempty"` // "image", "pdf", "video", "audio" (empty = text)
	MIMEType    string `json:"mime_type,omitempty"`
	PageLabel   string `json:"page_label,omitempty"` // e.g. "pages 1-6 of 24"
}

// RagIndex holds the complete index metadata
type RagIndex struct {
	Meta           []ChunkMeta       `json:"meta"`
	Dimension      int               `json:"dimension"`
	FileChecksums  map[string]string `json:"file_checksums"`
	EmbeddingModel string            `json:"embedding_model"`
	FormatVersion  int               `json:"format_version"`
}

// storeBaseDir returns the base directory for embedding stores
func storeBaseDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".ragujuary-embed"), nil
}

// storeDir returns the directory for a specific store
func storeDir(storeName string) (string, error) {
	base, err := storeBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, storeName), nil
}

// SaveIndex saves the RAG index and vectors to disk
func SaveIndex(storeName string, index *RagIndex, vectors []float32) error {
	dir, err := storeDir(storeName)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create store directory: %w", err)
	}

	// Save metadata as JSON
	index.FormatVersion = formatVersion
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}

	indexPath := filepath.Join(dir, indexFileName)
	if err := os.WriteFile(indexPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}

	// Save vectors as binary (little-endian float32)
	vectorsPath := filepath.Join(dir, vectorsFileName)
	buf := make([]byte, len(vectors)*4)
	for i, v := range vectors {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}

	if err := os.WriteFile(vectorsPath, buf, 0644); err != nil {
		return fmt.Errorf("failed to write vectors: %w", err)
	}

	return nil
}

// LoadIndex loads the RAG index and vectors from disk
func LoadIndex(storeName string) (*RagIndex, []float32, error) {
	dir, err := storeDir(storeName)
	if err != nil {
		return nil, nil, err
	}

	// Load metadata
	indexPath := filepath.Join(dir, indexFileName)
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("failed to read index: %w", err)
	}

	var index RagIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, nil, fmt.Errorf("failed to parse index: %w", err)
	}

	if index.FormatVersion > formatVersion {
		return nil, nil, fmt.Errorf("incompatible index format version %d (max supported: %d)", index.FormatVersion, formatVersion)
	}

	// Load vectors
	vectorsPath := filepath.Join(dir, vectorsFileName)
	buf, err := os.ReadFile(vectorsPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read vectors: %w", err)
	}

	vectors := make([]float32, len(buf)/4)
	for i := range vectors {
		vectors[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}

	return &index, vectors, nil
}

// DeleteIndex removes the entire store directory
func DeleteIndex(storeName string) error {
	dir, err := storeDir(storeName)
	if err != nil {
		return err
	}

	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("store '%s' not found", storeName)
		}
		return fmt.Errorf("failed to access store: %w", err)
	}

	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("failed to delete store: %w", err)
	}

	return nil
}

// ListStores returns all available embedding store names
func ListStores() ([]string, error) {
	base, err := storeBaseDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list stores: %w", err)
	}

	var stores []string
	for _, entry := range entries {
		if entry.IsDir() {
			// Check if it has an index.json
			indexPath := filepath.Join(base, entry.Name(), indexFileName)
			if _, err := os.Stat(indexPath); err == nil {
				stores = append(stores, entry.Name())
			}
		}
	}

	return stores, nil
}
