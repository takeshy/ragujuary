package rag

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/takeshy/ragujuary/internal/embedding"
	"github.com/takeshy/ragujuary/internal/fileutil"
)

const defaultBatchSize = 32

// Config holds configuration for the RAG engine
type Config struct {
	Model        string
	Dimension    int
	ChunkSize    int
	ChunkOverlap int
	TopK         int
	MinScore     float64
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() Config {
	return Config{
		Model:        "gemini-embedding-2-preview",
		Dimension:    768,
		ChunkSize:    1000,
		ChunkOverlap: 200,
		TopK:         5,
		MinScore:     0.3,
	}
}

// IndexResult holds the result of an indexing operation
type IndexResult struct {
	TotalChunks       int
	IndexedFiles      int
	SkippedFiles      int
	NewFiles          int
	UpdatedFiles      int
	MultimodalFiles   int
	SkippedMultimodal int
}

// Engine orchestrates the RAG indexing and query pipeline
type Engine struct {
	embeddingClient embedding.Client
}

// NewEngine creates a new RAG engine
func NewEngine(embeddingClient embedding.Client) *Engine {
	return &Engine{
		embeddingClient: embeddingClient,
	}
}

// Index indexes files from directories into the local embedding store
func (e *Engine) Index(dirs []string, excludePatterns []string, storeName string, config Config) (*IndexResult, error) {
	// Discover files
	files, err := fileutil.DiscoverFiles(dirs, excludePatterns)
	if err != nil {
		return nil, fmt.Errorf("failed to discover files: %w", err)
	}

	if len(files) == 0 {
		return &IndexResult{}, nil
	}

	// Compute checksums, classify files
	newChecksums := make(map[string]string)
	fileContents := make(map[string]string) // text files only
	fileInfoMap := make(map[string]fileutil.FileInfo)
	for _, f := range files {
		checksum, err := fileutil.CalculateChecksum(f.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate checksum for %s: %w", f.Path, err)
		}
		newChecksums[f.Path] = checksum
		fileInfoMap[f.Path] = f

		ct := fileutil.ClassifyContent(f.MimeType)
		if !fileutil.IsMultimodal(ct) {
			content, err := os.ReadFile(f.Path)
			if err != nil {
				return nil, fmt.Errorf("failed to read file %s: %w", f.Path, err)
			}
			fileContents[f.Path] = string(content)
		}
	}

	// Load existing index
	existingIndex, existingVectors, _ := LoadIndex(storeName)
	oldChecksums := make(map[string]string)
	finalChecksums := make(map[string]string)
	if existingIndex != nil {
		oldChecksums = existingIndex.FileChecksums
		for path, checksum := range existingIndex.FileChecksums {
			finalChecksums[path] = checksum
		}

		// Check model compatibility
		if existingIndex.EmbeddingModel != "" && existingIndex.EmbeddingModel != config.Model {
			// Model changed, must re-index everything
			existingIndex = nil
			existingVectors = nil
			oldChecksums = make(map[string]string)
			finalChecksums = make(map[string]string)
		}
	}

	// Separate changed and unchanged files
	var changedFiles []string
	unchangedMeta := make([]ChunkMeta, 0)
	unchangedVecs := make([][]float32, 0)

	// Keep chunks from unchanged files
	if existingIndex != nil && existingVectors != nil {
		dim := existingIndex.Dimension
		for i, meta := range existingIndex.Meta {
			checksum, scanned := newChecksums[meta.FilePath]
			if !scanned || checksum == oldChecksums[meta.FilePath] {
				unchangedMeta = append(unchangedMeta, meta)
				vec := make([]float32, dim)
				copy(vec, existingVectors[i*dim:(i+1)*dim])
				unchangedVecs = append(unchangedVecs, vec)
			}
		}
	}

	// Find changed/new files
	result := &IndexResult{}
	for filePath, checksum := range newChecksums {
		if oldChecksum, exists := oldChecksums[filePath]; exists {
			if checksum == oldChecksum {
				result.SkippedFiles++
			} else {
				changedFiles = append(changedFiles, filePath)
				result.UpdatedFiles++
			}
		} else {
			changedFiles = append(changedFiles, filePath)
			result.NewFiles++
		}
	}

	// Split changed files into text and multimodal
	var textFiles []string
	var multimodalFileInfos []fileutil.FileInfo
	for _, filePath := range changedFiles {
		fi := fileInfoMap[filePath]
		ct := fileutil.ClassifyContent(fi.MimeType)
		if fileutil.IsMultimodal(ct) {
			multimodalFileInfos = append(multimodalFileInfos, fi)
		} else {
			textFiles = append(textFiles, filePath)
		}
	}

	// Chunk and embed text files
	newMeta := make([]ChunkMeta, 0)
	newVecs := make([][]float32, 0)

	if len(textFiles) > 0 {
		var allTexts []string
		var allMetas []ChunkMeta

		for _, filePath := range textFiles {
			content := fileContents[filePath]
			chunks := ChunkText(content, config.ChunkSize, config.ChunkOverlap)

			for _, chunk := range chunks {
				heading := FindNearestHeading(content, chunk.StartOffset)
				var embeddingText string
				if heading != "" {
					embeddingText = fmt.Sprintf("[%s > %s]\n%s", filePath, heading, chunk.Text)
				} else {
					embeddingText = fmt.Sprintf("[%s]\n%s", filePath, chunk.Text)
				}
				allTexts = append(allTexts, embeddingText)
				allMetas = append(allMetas, ChunkMeta{
					FilePath:    filePath,
					StartOffset: chunk.StartOffset,
					Text:        chunk.Text,
				})
			}
		}

		// Batch embed
		for i := 0; i < len(allTexts); i += defaultBatchSize {
			end := i + defaultBatchSize
			if end > len(allTexts) {
				end = len(allTexts)
			}
			batch := allTexts[i:end]

			embeddings, err := e.embeddingClient.BatchEmbedContents(config.Model, batch, embedding.TaskRetrievalDocument, config.Dimension)
			if err != nil {
				return nil, fmt.Errorf("failed to embed batch: %w", err)
			}

			newVecs = append(newVecs, embeddings...)
		}

		newMeta = append(newMeta, allMetas...)
	}

	// Embed multimodal files (one at a time, no chunking)
	mmClient, supportsMultimodal := e.embeddingClient.(embedding.MultimodalEmbedder)
	for _, fi := range multimodalFileInfos {
		if !supportsMultimodal {
			fmt.Fprintf(os.Stderr, "Warning: skipping %s (backend does not support multimodal embedding)\n", fi.Path)
			delete(finalChecksums, fi.Path)
			result.SkippedMultimodal++
			continue
		}
		if !fileutil.SupportedEmbeddingMIME(fi.MimeType) {
			fmt.Fprintf(os.Stderr, "Warning: skipping %s (unsupported MIME type %s)\n", fi.Path, fi.MimeType)
			delete(finalChecksums, fi.Path)
			result.SkippedMultimodal++
			continue
		}

		data, err := os.ReadFile(fi.Path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to read %s: %v\n", fi.Path, err)
			delete(finalChecksums, fi.Path)
			result.SkippedMultimodal++
			continue
		}

		vec, err := mmClient.EmbedMultimodalContent(config.Model, embedding.MultimodalContent{
			MIMEType: fi.MimeType,
			Data:     data,
		}, embedding.TaskRetrievalDocument, config.Dimension)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to embed %s: %v\n", fi.Path, err)
			delete(finalChecksums, fi.Path)
			result.SkippedMultimodal++
			continue
		}

		ct := fileutil.ClassifyContent(fi.MimeType)
		newMeta = append(newMeta, ChunkMeta{
			FilePath:    fi.Path,
			StartOffset: 0,
			Text:        fmt.Sprintf("[%s: %s]", ct, filepath.Base(fi.Path)),
			ContentType: ct,
			MIMEType:    fi.MimeType,
		})
		newVecs = append(newVecs, vec)
		result.MultimodalFiles++
		finalChecksums[fi.Path] = newChecksums[fi.Path]
	}

	for _, filePath := range textFiles {
		finalChecksums[filePath] = newChecksums[filePath]
	}

	// Merge unchanged + new
	allMeta := append(unchangedMeta, newMeta...)
	allVecArrays := append(unchangedVecs, newVecs...)

	// Determine dimension
	dimension := config.Dimension
	if len(allVecArrays) > 0 {
		dimension = len(allVecArrays[0])
	}

	// Build flat vector array
	flatVectors := make([]float32, len(allMeta)*dimension)
	for i, vec := range allVecArrays {
		copy(flatVectors[i*dimension:], vec)
	}

	// Save
	index := &RagIndex{
		Meta:           allMeta,
		Dimension:      dimension,
		FileChecksums:  finalChecksums,
		EmbeddingModel: config.Model,
	}

	if err := SaveIndex(storeName, index, flatVectors); err != nil {
		return nil, fmt.Errorf("failed to save index: %w", err)
	}

	result.TotalChunks = len(allMeta)
	result.IndexedFiles = len(allMetaFilePaths(allMeta))

	return result, nil
}

func allMetaFilePaths(meta []ChunkMeta) map[string]struct{} {
	files := make(map[string]struct{}, len(meta))
	for _, m := range meta {
		files[m.FilePath] = struct{}{}
	}
	return files
}

// IndexContent indexes a single piece of content (for MCP use)
func (e *Engine) IndexContent(storeName, fileName, content string, config Config) error {
	// Load existing index
	existingIndex, existingVectors, _ := LoadIndex(storeName)

	var allMeta []ChunkMeta
	var allVecs [][]float32
	dimension := config.Dimension

	// Keep existing chunks from other files
	if existingIndex != nil && existingVectors != nil {
		// Check model compatibility
		if existingIndex.EmbeddingModel != "" && existingIndex.EmbeddingModel != config.Model {
			existingIndex = nil
			existingVectors = nil
		}
	}

	if existingIndex != nil && existingVectors != nil {
		dim := existingIndex.Dimension
		dimension = dim
		for i, meta := range existingIndex.Meta {
			if meta.FilePath != fileName {
				allMeta = append(allMeta, meta)
				vec := make([]float32, dim)
				copy(vec, existingVectors[i*dim:(i+1)*dim])
				allVecs = append(allVecs, vec)
			}
		}
	}

	// Chunk new content
	chunks := ChunkText(content, config.ChunkSize, config.ChunkOverlap)

	var texts []string
	var metas []ChunkMeta
	for _, chunk := range chunks {
		heading := FindNearestHeading(content, chunk.StartOffset)
		var embeddingText string
		if heading != "" {
			embeddingText = fmt.Sprintf("[%s > %s]\n%s", fileName, heading, chunk.Text)
		} else {
			embeddingText = fmt.Sprintf("[%s]\n%s", fileName, chunk.Text)
		}
		texts = append(texts, embeddingText)
		metas = append(metas, ChunkMeta{
			FilePath:    fileName,
			StartOffset: chunk.StartOffset,
			Text:        chunk.Text,
		})
	}

	// Batch embed
	for i := 0; i < len(texts); i += defaultBatchSize {
		end := i + defaultBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]

		embeddings, err := e.embeddingClient.BatchEmbedContents(config.Model, batch, embedding.TaskRetrievalDocument, config.Dimension)
		if err != nil {
			return fmt.Errorf("failed to embed batch: %w", err)
		}

		allVecs = append(allVecs, embeddings...)
	}

	allMeta = append(allMeta, metas...)

	// Update dimension
	if len(allVecs) > 0 {
		dimension = len(allVecs[0])
	}

	// Build flat vectors
	flatVectors := make([]float32, len(allMeta)*dimension)
	for i, vec := range allVecs {
		copy(flatVectors[i*dimension:], vec)
	}

	// Build checksums from existing + new
	checksums := make(map[string]string)
	if existingIndex != nil {
		for k, v := range existingIndex.FileChecksums {
			if k != fileName {
				checksums[k] = v
			}
		}
	}
	checksums[fileName] = "content:" + fileName

	index := &RagIndex{
		Meta:           allMeta,
		Dimension:      dimension,
		FileChecksums:  checksums,
		EmbeddingModel: config.Model,
	}

	return SaveIndex(storeName, index, flatVectors)
}

// IndexMultimodalContent indexes a single multimodal file (for MCP use)
func (e *Engine) IndexMultimodalContent(storeName, fileName string, data []byte, mimeType string, config Config) error {
	mmClient, ok := e.embeddingClient.(embedding.MultimodalEmbedder)
	if !ok {
		return fmt.Errorf("current embedding backend does not support multimodal content")
	}

	// Load existing index
	existingIndex, existingVectors, _ := LoadIndex(storeName)

	var allMeta []ChunkMeta
	var allVecs [][]float32
	dimension := config.Dimension

	if existingIndex != nil && existingVectors != nil {
		if existingIndex.EmbeddingModel != "" && existingIndex.EmbeddingModel != config.Model {
			existingIndex = nil
			existingVectors = nil
		}
	}

	if existingIndex != nil && existingVectors != nil {
		dim := existingIndex.Dimension
		dimension = dim
		for i, meta := range existingIndex.Meta {
			if meta.FilePath != fileName {
				allMeta = append(allMeta, meta)
				vec := make([]float32, dim)
				copy(vec, existingVectors[i*dim:(i+1)*dim])
				allVecs = append(allVecs, vec)
			}
		}
	}

	// Embed multimodal content (single embedding, no chunking)
	vec, err := mmClient.EmbedMultimodalContent(config.Model, embedding.MultimodalContent{
		MIMEType: mimeType,
		Data:     data,
	}, embedding.TaskRetrievalDocument, config.Dimension)
	if err != nil {
		return fmt.Errorf("failed to embed multimodal content: %w", err)
	}

	ct := fileutil.ClassifyContent(mimeType)
	allMeta = append(allMeta, ChunkMeta{
		FilePath:    fileName,
		StartOffset: 0,
		Text:        fmt.Sprintf("[%s: %s]", ct, fileName),
		ContentType: ct,
		MIMEType:    mimeType,
	})
	allVecs = append(allVecs, vec)

	if len(allVecs) > 0 {
		dimension = len(allVecs[0])
	}

	flatVectors := make([]float32, len(allMeta)*dimension)
	for i, vec := range allVecs {
		copy(flatVectors[i*dimension:], vec)
	}

	checksums := make(map[string]string)
	if existingIndex != nil {
		for k, v := range existingIndex.FileChecksums {
			if k != fileName {
				checksums[k] = v
			}
		}
	}
	checksums[fileName] = "multimodal:" + fileName

	index := &RagIndex{
		Meta:           allMeta,
		Dimension:      dimension,
		FileChecksums:  checksums,
		EmbeddingModel: config.Model,
	}

	return SaveIndex(storeName, index, flatVectors)
}

// Query performs a semantic search against the local embedding store
func (e *Engine) Query(question, storeName string, config Config) ([]SearchResult, error) {
	// Load index
	index, vectors, err := LoadIndex(storeName)
	if err != nil {
		return nil, fmt.Errorf("failed to load index: %w", err)
	}
	if index == nil {
		return nil, fmt.Errorf("store '%s' not found or empty", storeName)
	}

	// Use the model from the index if available
	model := config.Model
	if index.EmbeddingModel != "" {
		model = index.EmbeddingModel
	}

	// Embed query
	queryVec, err := e.embeddingClient.EmbedContent(model, question, embedding.TaskRetrievalQuery, index.Dimension)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	// Search
	results := Search(queryVec, index, vectors, config.TopK, config.MinScore)

	return results, nil
}

// DeleteFiles removes files matching a pattern from the index
func (e *Engine) DeleteFiles(storeName, pattern string) (int, error) {
	index, vectors, err := LoadIndex(storeName)
	if err != nil {
		return 0, fmt.Errorf("failed to load index: %w", err)
	}
	if index == nil {
		return 0, fmt.Errorf("store '%s' not found", storeName)
	}

	// Find matching files
	matchedFiles := make(map[string]bool)
	for filePath := range index.FileChecksums {
		matched, _ := fileutil.FilterFilesByPattern([]fileutil.FileInfo{{Path: filePath}}, pattern)
		if len(matched) > 0 {
			matchedFiles[filePath] = true
		}
	}

	if len(matchedFiles) == 0 {
		return 0, nil
	}

	// Filter out matched chunks
	dim := index.Dimension
	var newMeta []ChunkMeta
	var newVecs [][]float32
	for i, meta := range index.Meta {
		if !matchedFiles[meta.FilePath] {
			newMeta = append(newMeta, meta)
			vec := make([]float32, dim)
			copy(vec, vectors[i*dim:(i+1)*dim])
			newVecs = append(newVecs, vec)
		}
	}

	// Update checksums
	newChecksums := make(map[string]string)
	for k, v := range index.FileChecksums {
		if !matchedFiles[k] {
			newChecksums[k] = v
		}
	}

	// Build flat vectors
	flatVectors := make([]float32, len(newMeta)*dim)
	for i, vec := range newVecs {
		copy(flatVectors[i*dim:], vec)
	}

	newIndex := &RagIndex{
		Meta:           newMeta,
		Dimension:      dim,
		FileChecksums:  newChecksums,
		EmbeddingModel: index.EmbeddingModel,
	}

	if err := SaveIndex(storeName, newIndex, flatVectors); err != nil {
		return 0, fmt.Errorf("failed to save index: %w", err)
	}

	return len(matchedFiles), nil
}
