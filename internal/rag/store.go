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
	ChunkSize      int               `json:"chunk_size,omitempty"`
	ChunkOverlap   int               `json:"chunk_overlap,omitempty"`
	PDFMaxPages    int               `json:"pdf_max_pages,omitempty"`
	FormatVersion  int               `json:"format_version"`
}

func (r *RagIndex) EffectiveChunkSize() int {
	if r == nil || r.ChunkSize <= 0 {
		return 1000
	}
	return r.ChunkSize
}

func (r *RagIndex) EffectiveChunkOverlap() int {
	if r == nil || r.ChunkOverlap < 0 {
		return 200
	}
	if r.ChunkOverlap == 0 {
		return 0
	}
	return r.ChunkOverlap
}

func (r *RagIndex) EffectivePDFMaxPages() int {
	if r == nil || r.PDFMaxPages <= 0 {
		return 6
	}
	return r.PDFMaxPages
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

// externalChunkMeta handles camelCase JSON field names from external RAG tools
type externalChunkMeta struct {
	FilePath    string `json:"filePath"`
	ChunkIndex  int    `json:"chunkIndex"`
	Text        string `json:"text"`
	ContentType string `json:"contentType,omitempty"`
	PageLabel   string `json:"pageLabel,omitempty"`
}

// externalRagIndex handles camelCase JSON field names from external RAG tools
type externalRagIndex struct {
	Meta           []externalChunkMeta `json:"meta"`
	Dimension      int                 `json:"dimension"`
	FileChecksums  map[string]string   `json:"fileChecksums"`
	EmbeddingModel string              `json:"embeddingModel"`
}

// convertExternalIndex converts an external format index to ragujuary format
func convertExternalIndex(ext *externalRagIndex) *RagIndex {
	meta := make([]ChunkMeta, len(ext.Meta))
	for i, m := range ext.Meta {
		meta[i] = ChunkMeta{
			FilePath:    m.FilePath,
			StartOffset: m.ChunkIndex,
			Text:        m.Text,
			ContentType: m.ContentType,
			PageLabel:   m.PageLabel,
		}
	}
	return &RagIndex{
		Meta:           meta,
		Dimension:      ext.Dimension,
		FileChecksums:  ext.FileChecksums,
		EmbeddingModel: ext.EmbeddingModel,
	}
}

// isCamelCaseFormat detects whether the index JSON uses camelCase field names
// by checking multiple key pairs at both meta-item and top-level.
func isCamelCaseFormat(data []byte) bool {
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return false
	}

	// Check top-level keys
	if _, ok := raw["embeddingModel"]; ok {
		if _, snake := raw["embedding_model"]; !snake {
			return true
		}
	}
	if _, ok := raw["fileChecksums"]; ok {
		if _, snake := raw["file_checksums"]; !snake {
			return true
		}
	}

	// Check meta-item keys
	metaRaw, ok := raw["meta"]
	if !ok {
		return false
	}
	var meta []map[string]json.RawMessage
	if json.Unmarshal(metaRaw, &meta) != nil || len(meta) == 0 {
		return false
	}
	camelKeys := []string{"filePath", "contentType", "pageLabel", "chunkIndex"}
	for _, k := range camelKeys {
		if _, ok := meta[0][k]; ok {
			return true
		}
	}
	return false
}

// unmarshalIndex parses index JSON, auto-detecting ragujuary (snake_case) or external (camelCase) format.
// Format detection is based on JSON key names at both meta and top level.
// Falls back to snake_case (ragujuary native) when format cannot be determined.
func unmarshalIndex(data []byte) (*RagIndex, error) {
	if isCamelCaseFormat(data) {
		var ext externalRagIndex
		if err := json.Unmarshal(data, &ext); err != nil {
			return nil, fmt.Errorf("failed to parse index: %w", err)
		}
		return convertExternalIndex(&ext), nil
	}

	var index RagIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("failed to parse index: %w", err)
	}
	return &index, nil
}

// LoadIndex loads the RAG index and vectors from disk
func LoadIndex(storeName string) (*RagIndex, []float32, error) {
	dir, err := storeDir(storeName)
	if err != nil {
		return nil, nil, err
	}
	return LoadIndexFromDir(dir)
}

// LoadIndexFromDir loads the RAG index and vectors from an arbitrary directory
func LoadIndexFromDir(dir string) (*RagIndex, []float32, error) {
	// Load metadata
	indexPath := filepath.Join(dir, indexFileName)
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("failed to read index: %w", err)
	}

	index, err := unmarshalIndex(data)
	if err != nil {
		return nil, nil, err
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

	if len(buf)%4 != 0 {
		return nil, nil, fmt.Errorf("vectors file is corrupted: size %d is not a multiple of 4", len(buf))
	}

	vectors := make([]float32, len(buf)/4)
	for i := range vectors {
		vectors[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}

	expected := len(index.Meta) * index.Dimension
	if len(vectors) != expected {
		return nil, nil, fmt.Errorf("vectors/index mismatch: got %d floats, expected %d (%d chunks × %d dimensions)",
			len(vectors), expected, len(index.Meta), index.Dimension)
	}

	return index, vectors, nil
}

// CreateEmptyIndex creates a new empty embedding store
func CreateEmptyIndex(storeName string) error {
	index := &RagIndex{
		Meta:          []ChunkMeta{},
		FileChecksums: make(map[string]string),
		ChunkSize:     1000,
		ChunkOverlap:  200,
		PDFMaxPages:   6,
	}
	return SaveIndex(storeName, index, nil)
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
