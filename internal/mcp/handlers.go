package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/takeshy/ragujuary/internal/fileutil"
	"github.com/takeshy/ragujuary/internal/gemini"
	"github.com/takeshy/ragujuary/internal/pdfutil"
	"github.com/takeshy/ragujuary/internal/rag"
	"github.com/takeshy/ragujuary/internal/store"
)

func validatePDFMaxPages(pdfMaxPages int) error {
	if pdfMaxPages < 1 || pdfMaxPages > 6 {
		return fmt.Errorf("pdf_max_pages must be between 1 and 6, got %d", pdfMaxPages)
	}
	return nil
}

// isEmbedStore checks if a store name corresponds to an embedding store
func isEmbedStore(name string) bool {
	index, _, err := rag.LoadIndex(name)
	return err == nil && index != nil
}

// handleUpload handles the upload tool (auto-detects Embedding or FileSearch store)
func (s *Server) handleUpload(ctx context.Context, req *mcp.CallToolRequest, input UploadInput) (*mcp.CallToolResult, UploadOutput, error) {
	output := UploadOutput{FileName: input.FileName}

	if input.FileName == "" {
		return nil, output, fmt.Errorf("file_name is required")
	}
	if input.FileContent == "" {
		return nil, output, fmt.Errorf("file_content is required")
	}

	storeName, err := s.getStoreName(input.StoreName)
	if err != nil {
		return nil, output, err
	}

	// Embedding store — delegate to embed index
	if isEmbedStore(storeName) {
		return s.handleUploadEmbed(ctx, storeName, input)
	}

	// FileSearch store
	resolvedName, _, err := s.geminiClient.ResolveStoreName(storeName)
	if err != nil {
		return nil, output, fmt.Errorf("store '%s' not found (checked both embedding and FileSearch)", storeName)
	}

	var content []byte
	if input.IsBase64 {
		content, err = base64.StdEncoding.DecodeString(input.FileContent)
		if err != nil {
			return nil, output, fmt.Errorf("failed to decode base64 content: %w", err)
		}
	} else {
		content = []byte(input.FileContent)
	}

	hash := sha256.Sum256(content)
	checksum := hex.EncodeToString(hash[:])

	existingDoc, err := s.geminiClient.FindDocumentByDisplayName(resolvedName, input.FileName)
	if err != nil {
		// Not fatal - continue with upload
	}

	if existingDoc != nil {
		existingChecksum := strings.TrimPrefix(gemini.GetDocumentChecksum(existingDoc), "sha256:")
		if existingChecksum == checksum {
			output.Success = true
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Skipped '%s': content unchanged", input.FileName)},
				},
			}, output, nil
		}
		if err := s.geminiClient.DeleteDocument(existingDoc.Name); err != nil {
			// Continue with upload
		}
	}

	checksumPtr := checksum
	customMetadata := []gemini.CustomMetadata{
		{Key: "checksum", StringValue: &checksumPtr},
	}

	op, err := s.geminiClient.UploadContentWithMetadata(resolvedName, input.FileName, content, customMetadata)
	if err != nil {
		output.Error = err.Error()
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Upload failed: %v", err)},
			},
		}, output, nil
	}

	if !op.Done {
		op, err = s.geminiClient.WaitForOperation(op.Name, 2*time.Second)
		if err != nil {
			output.Error = err.Error()
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Upload failed: %v", err)},
				},
			}, output, nil
		}
	}

	output.Success = true
	message := fmt.Sprintf("Successfully uploaded '%s'", input.FileName)
	if existingDoc != nil {
		message = fmt.Sprintf("Successfully updated '%s'", input.FileName)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: message},
		},
	}, output, nil
}

// handleUploadEmbed handles upload to an embedding store (index content)
func (s *Server) handleUploadEmbed(ctx context.Context, storeName string, input UploadInput) (*mcp.CallToolResult, UploadOutput, error) {
	output := UploadOutput{FileName: input.FileName}

	config := rag.DefaultConfig()
	if input.ChunkSize > 0 {
		config.ChunkSize = input.ChunkSize
	}
	if input.ChunkOverlap > 0 {
		config.ChunkOverlap = input.ChunkOverlap
	}
	if input.Dimension > 0 {
		config.Dimension = input.Dimension
	}
	if input.PDFMaxPages != 0 {
		if err := validatePDFMaxPages(input.PDFMaxPages); err != nil {
			return nil, output, err
		}
		config.PDFMaxPages = input.PDFMaxPages
	}

	// Multimodal path
	if input.MIMEType != "" && input.IsBase64 {
		ct := fileutil.ClassifyContent(input.MIMEType)
		if !fileutil.IsMultimodal(ct) {
			return nil, output, fmt.Errorf("mime_type %s is not a multimodal type; use plain text indexing instead", input.MIMEType)
		}

		data, err := base64.StdEncoding.DecodeString(input.FileContent)
		if err != nil {
			return nil, output, fmt.Errorf("failed to decode base64 content: %w", err)
		}

		if err := s.ragEngine.IndexMultimodalContent(storeName, input.FileName, data, input.MIMEType, config); err != nil {
			output.Error = err.Error()
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Indexing failed: %v", err)},
				},
			}, output, nil
		}

		chunks := 1
		if input.MIMEType == "application/pdf" {
			if pageCount, err := pdfutil.PageCount(data); err == nil {
				chunks = (pageCount + config.PDFMaxPages - 1) / config.PDFMaxPages
			}
		} else if idx, _, loadErr := rag.LoadIndex(storeName); loadErr == nil && idx != nil {
			count := 0
			for _, m := range idx.Meta {
				if m.FilePath == input.FileName {
					count++
				}
			}
			if count > 0 {
				chunks = count
			}
		}

		output.Success = true
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Successfully indexed multimodal '%s' (%s, %d embedding(s))", input.FileName, ct, chunks)},
			},
		}, output, nil
	}

	// Text path
	if err := s.ragEngine.IndexContent(storeName, input.FileName, input.FileContent, config); err != nil {
		output.Error = err.Error()
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Indexing failed: %v", err)},
			},
		}, output, nil
	}

	chunks := rag.ChunkText(input.FileContent, config.ChunkSize, config.ChunkOverlap)
	output.Success = true

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Successfully indexed '%s' (%d chunks)", input.FileName, len(chunks))},
		},
	}, output, nil
}

// handleQuery handles the query tool (auto-detects Embedding or FileSearch store)
func (s *Server) handleQuery(ctx context.Context, req *mcp.CallToolRequest, input QueryInput) (*mcp.CallToolResult, QueryOutput, error) {
	output := QueryOutput{}

	if input.Question == "" {
		return nil, output, fmt.Errorf("question is required")
	}

	names, err := s.getAllowedStoreNames(input.StoreName, input.StoreNames)
	if err != nil {
		return nil, output, err
	}

	embedStoreNames := make([]string, 0, len(names))
	for _, name := range names {
		if isEmbedStore(name) {
			embedStoreNames = append(embedStoreNames, name)
		}
	}
	if len(embedStoreNames) > 1 {
		return nil, output, fmt.Errorf("query across multiple embedding stores is not supported; requested: %s", strings.Join(embedStoreNames, ", "))
	}
	if len(embedStoreNames) == 1 {
		if len(names) > 1 {
			return nil, output, fmt.Errorf("cannot mix embedding and FileSearch stores in one query; use a single store")
		}
		return s.handleQueryEmbed(ctx, embedStoreNames[0], input)
	}

	// FileSearch mode
	model := input.Model
	if model == "" {
		model = "gemini-3-flash-preview"
	}

	var storeNames []string
	for _, name := range names {
		_, remoteStore, err := s.geminiClient.ResolveStoreName(name)
		if err != nil {
			return nil, output, fmt.Errorf("store '%s' not found (checked both embedding and FileSearch)", name)
		}
		storeNames = append(storeNames, remoteStore.Name)
	}

	resp, err := s.geminiClient.Query(model, input.Question, storeNames, input.MetadataFilter)
	if err != nil {
		return nil, output, fmt.Errorf("query failed: %w", err)
	}

	if len(resp.Candidates) == 0 {
		output.Answer = "No response generated"
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: output.Answer},
			},
		}, output, nil
	}

	candidate := resp.Candidates[0]
	var answerBuilder strings.Builder
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			answerBuilder.WriteString(part.Text)
		}
	}
	output.Answer = answerBuilder.String()

	if input.ShowCitations && candidate.GroundingMetadata != nil {
		for _, chunk := range candidate.GroundingMetadata.GroundingChunks {
			if chunk.RetrievedContext != nil {
				citation := CitationInfo{
					Title: chunk.RetrievedContext.Title,
					URI:   chunk.RetrievedContext.URI,
				}
				text := chunk.RetrievedContext.Text
				if len(text) > 200 {
					text = text[:200] + "..."
				}
				citation.Text = text
				output.Citations = append(output.Citations, citation)
			}
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: output.Answer},
		},
	}, output, nil
}

// handleQueryEmbed handles query for embedding stores
func (s *Server) handleQueryEmbed(ctx context.Context, storeName string, input QueryInput) (*mcp.CallToolResult, QueryOutput, error) {
	output := QueryOutput{}

	config := rag.DefaultConfig()
	if input.TopK > 0 {
		config.TopK = input.TopK
	}
	if input.MinScore > 0 {
		config.MinScore = input.MinScore
	}

	results, err := s.ragEngine.Query(input.Question, storeName, config)
	if err != nil {
		return nil, output, fmt.Errorf("query failed: %w", err)
	}

	if len(results) == 0 {
		output.Answer = "No results found."
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: output.Answer},
			},
		}, output, nil
	}

	var textBuilder strings.Builder
	for _, r := range results {
		text := r.Text
		if len(text) > 300 {
			text = text[:300] + "..."
		}
		textBuilder.WriteString(fmt.Sprintf("[%.4f] %s: %s\n\n", r.Score, r.FilePath, text))
	}
	output.Answer = textBuilder.String()

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: output.Answer},
		},
	}, output, nil
}

// handleList handles the list tool (auto-detects Embedding or FileSearch store)
func (s *Server) handleList(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
	output := ListOutput{Items: []ListItem{}}

	storeName, err := s.getStoreName(input.StoreName)
	if err != nil {
		return nil, output, err
	}

	// Embedding store first
	index, _, loadErr := rag.LoadIndex(storeName)
	if loadErr == nil && index != nil {
		return s.handleListEmbed(storeName, index, input.Pattern)
	}

	// FileSearch store
	resolvedName, st, err := s.geminiClient.ResolveStoreName(storeName)
	if err != nil {
		return nil, output, fmt.Errorf("store '%s' not found (checked both embedding and FileSearch)", storeName)
	}

	docs, err := s.geminiClient.ListAllDocuments(resolvedName)
	if err != nil {
		return nil, output, fmt.Errorf("failed to list documents: %w", err)
	}

	var re *regexp.Regexp
	if input.Pattern != "" {
		re, err = regexp.Compile(input.Pattern)
		if err != nil {
			return nil, output, fmt.Errorf("invalid pattern: %w", err)
		}
	}

	for _, d := range docs {
		if re != nil {
			if !re.MatchString(d.DisplayName) && !re.MatchString(d.Name) {
				continue
			}
		}
		output.Items = append(output.Items, ListItem{
			Name:        d.Name,
			DisplayName: d.DisplayName,
			State:       d.State,
			CreatedAt:   d.CreateTime,
		})
	}
	output.Total = len(output.Items)

	var textBuilder strings.Builder
	textBuilder.WriteString(fmt.Sprintf("[FileSearch] Found %d documents in store '%s':\n\n", output.Total, st.DisplayName))
	for _, item := range output.Items {
		textBuilder.WriteString(fmt.Sprintf("- %s (state: %s)\n", item.DisplayName, item.State))
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: textBuilder.String()},
		},
	}, output, nil
}

// handleListEmbed lists files in an embedding store
func (s *Server) handleListEmbed(storeName string, index *rag.RagIndex, pattern string) (*mcp.CallToolResult, ListOutput, error) {
	output := ListOutput{Items: []ListItem{}}

	files := make(map[string]int)
	for _, m := range index.Meta {
		files[m.FilePath]++
	}

	var re *regexp.Regexp
	if pattern != "" {
		var err error
		re, err = regexp.Compile(pattern)
		if err != nil {
			return nil, output, fmt.Errorf("invalid pattern: %w", err)
		}
	}

	for path, chunks := range files {
		if re != nil && !re.MatchString(path) {
			continue
		}
		output.Items = append(output.Items, ListItem{
			DisplayName: path,
			State:       fmt.Sprintf("%d chunks", chunks),
		})
	}
	output.Total = len(output.Items)

	var textBuilder strings.Builder
	textBuilder.WriteString(fmt.Sprintf("[Embedding] Found %d files in store '%s' (model: %s, dimension: %d):\n\n",
		output.Total, storeName, index.EmbeddingModel, index.Dimension))
	for _, item := range output.Items {
		textBuilder.WriteString(fmt.Sprintf("- %s (%s)\n", item.DisplayName, item.State))
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: textBuilder.String()},
		},
	}, output, nil
}

// handleDelete handles the delete tool (auto-detects Embedding or FileSearch store)
func (s *Server) handleDelete(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, DeleteOutput, error) {
	output := DeleteOutput{FileName: input.FileName}

	if input.FileName == "" {
		return nil, output, fmt.Errorf("file_name is required")
	}

	storeName, err := s.getStoreName(input.StoreName)
	if err != nil {
		return nil, output, err
	}

	// Embedding store
	if isEmbedStore(storeName) {
		deleted, err := s.ragEngine.DeleteFiles(storeName, regexp.QuoteMeta(input.FileName))
		if err != nil {
			return nil, output, fmt.Errorf("delete failed: %w", err)
		}
		if deleted == 0 {
			output.Error = fmt.Sprintf("file '%s' not found in store", input.FileName)
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: output.Error},
				},
			}, output, nil
		}
		output.Success = true
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Successfully deleted '%s' from embedding store", input.FileName)},
			},
		}, output, nil
	}

	// FileSearch store
	resolvedName, _, err := s.geminiClient.ResolveStoreName(storeName)
	if err != nil {
		return nil, output, fmt.Errorf("store '%s' not found (checked both embedding and FileSearch)", storeName)
	}

	docs, err := s.geminiClient.ListAllDocuments(resolvedName)
	if err != nil {
		return nil, output, fmt.Errorf("failed to list documents: %w", err)
	}

	var targetDoc *struct {
		Name        string
		DisplayName string
	}
	for _, d := range docs {
		if d.DisplayName == input.FileName {
			targetDoc = &struct {
				Name        string
				DisplayName string
			}{Name: d.Name, DisplayName: d.DisplayName}
			break
		}
	}

	if targetDoc == nil {
		output.Error = fmt.Sprintf("file '%s' not found in store", input.FileName)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: output.Error},
			},
		}, output, nil
	}

	if err := s.geminiClient.DeleteDocument(targetDoc.Name); err != nil {
		output.Error = err.Error()
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Failed to delete '%s': %v", input.FileName, err)},
			},
		}, output, nil
	}

	output.Success = true
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Successfully deleted '%s'", input.FileName)},
		},
	}, output, nil
}

// handleCreateStore handles the create_store tool
func (s *Server) handleCreateStore(ctx context.Context, req *mcp.CallToolRequest, input CreateStoreInput) (*mcp.CallToolResult, CreateStoreOutput, error) {
	output := CreateStoreOutput{StoreName: input.StoreName}

	if input.StoreName == "" {
		return nil, output, fmt.Errorf("store_name is required")
	}
	if !s.isStoreAllowed(input.StoreName) {
		return nil, output, fmt.Errorf("store '%s' is not in the allowed stores list", input.StoreName)
	}

	if input.Type == "embed" || input.Type == "embedding" {
		// Create embedding store by initializing an empty index
		if err := rag.CreateEmptyIndex(input.StoreName); err != nil {
			output.Error = err.Error()
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Failed to create embedding store: %v", err)},
				},
			}, output, nil
		}
		output.Success = true
		output.StoreID = input.StoreName
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Successfully created embedding store '%s'", input.StoreName)},
			},
		}, output, nil
	}

	// Default: FileSearch store
	st, err := s.geminiClient.CreateFileSearchStore(input.StoreName)
	if err != nil {
		output.Error = err.Error()
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Failed to create store: %v", err)},
			},
		}, output, nil
	}

	output.Success = true
	output.StoreID = st.Name
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Successfully created FileSearch store '%s' (ID: %s)", input.StoreName, st.Name)},
		},
	}, output, nil
}

// handleDeleteStore handles the delete_store tool (auto-detects Embedding or FileSearch store)
func (s *Server) handleDeleteStore(ctx context.Context, req *mcp.CallToolRequest, input DeleteStoreInput) (*mcp.CallToolResult, DeleteStoreOutput, error) {
	output := DeleteStoreOutput{StoreName: input.StoreName}

	if input.StoreName == "" {
		return nil, output, fmt.Errorf("store_name is required")
	}
	if !s.isStoreAllowed(input.StoreName) {
		return nil, output, fmt.Errorf("store '%s' is not in the allowed stores list", input.StoreName)
	}

	// Embedding store
	if isEmbedStore(input.StoreName) {
		if err := rag.DeleteIndex(input.StoreName); err != nil {
			output.Error = err.Error()
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Failed to delete embedding store: %v", err)},
				},
			}, output, nil
		}
		output.Success = true
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Successfully deleted embedding store '%s'", input.StoreName)},
			},
		}, output, nil
	}

	// FileSearch store
	resolvedName, remoteStore, err := s.geminiClient.ResolveStoreName(input.StoreName)
	if err != nil {
		return nil, output, fmt.Errorf("store '%s' not found (checked both embedding and FileSearch)", input.StoreName)
	}

	if err := s.geminiClient.DeleteFileSearchStore(resolvedName, true); err != nil {
		output.Error = err.Error()
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Failed to delete store: %v", err)},
			},
		}, output, nil
	}

	output.Success = true
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Successfully deleted FileSearch store '%s'", remoteStore.DisplayName)},
		},
	}, output, nil
}

// handleListStores handles the list_stores tool (lists both FileSearch and Embedding stores)
func (s *Server) handleListStores(ctx context.Context, req *mcp.CallToolRequest, input ListStoresInput) (*mcp.CallToolResult, ListStoresOutput, error) {
	output := ListStoresOutput{Stores: []StoreInfo{}}

	var textBuilder strings.Builder

	// Embedding stores (local)
	embedStores, _ := rag.ListStores()
	var filteredEmbedStores []string
	for _, name := range embedStores {
		if s.isStoreAllowed(name) {
			filteredEmbedStores = append(filteredEmbedStores, name)
		}
	}
	if len(filteredEmbedStores) > 0 {
		textBuilder.WriteString(fmt.Sprintf("[Embedding] %d stores:\n", len(filteredEmbedStores)))
		for _, name := range filteredEmbedStores {
			info := StoreInfo{
				Name:        name,
				DisplayName: name,
			}
			if index, _, err := rag.LoadIndex(name); err == nil && index != nil {
				textBuilder.WriteString(fmt.Sprintf("- %s (model: %s, dimension: %d, chunks: %d)\n", name, index.EmbeddingModel, index.Dimension, len(index.Meta)))
			} else {
				textBuilder.WriteString(fmt.Sprintf("- %s\n", name))
			}
			output.Stores = append(output.Stores, info)
		}
		textBuilder.WriteString("\n")
	}

	// FileSearch stores (remote)
	pageToken := ""
	var fsStores []StoreInfo
	for {
		resp, err := s.geminiClient.ListFileSearchStores(pageToken)
		if err != nil {
			return nil, output, fmt.Errorf("failed to list FileSearch stores: %w", err)
		}

		for _, st := range resp.FileSearchStores {
			if s.isStoreAllowed(st.DisplayName) {
				fsStores = append(fsStores, StoreInfo{
					Name:        st.Name,
					DisplayName: st.DisplayName,
					CreateTime:  st.CreateTime,
					UpdateTime:  st.UpdateTime,
				})
			}
		}

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	if len(fsStores) > 0 {
		textBuilder.WriteString(fmt.Sprintf("[FileSearch] %d stores:\n", len(fsStores)))
		for _, st := range fsStores {
			textBuilder.WriteString(fmt.Sprintf("- %s (ID: %s)\n", st.DisplayName, st.Name))
		}
		output.Stores = append(output.Stores, fsStores...)
	}

	output.Total = len(output.Stores)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: textBuilder.String()},
		},
	}, output, nil
}

// handleUploadDirectory handles the upload_directory tool (auto-detects Embedding or FileSearch store)
func (s *Server) handleUploadDirectory(ctx context.Context, req *mcp.CallToolRequest, input UploadDirectoryInput) (*mcp.CallToolResult, UploadDirectoryOutput, error) {
	output := UploadDirectoryOutput{}

	if len(input.Directories) == 0 {
		return nil, output, fmt.Errorf("directories is required and must not be empty")
	}

	storeName, err := s.getStoreName(input.StoreName)
	if err != nil {
		return nil, output, err
	}

	// Embedding store — delegate to embed index directory
	if isEmbedStore(storeName) {
		return s.handleUploadDirectoryEmbed(ctx, storeName, input)
	}

	// FileSearch store
	storeManager, err := s.getStoreManager()
	if err != nil {
		return nil, output, err
	}

	_, remoteStore, err := s.geminiClient.ResolveStoreName(storeName)
	if err != nil {
		return nil, output, fmt.Errorf("store '%s' not found (checked both embedding and FileSearch)", storeName)
	}
	localStoreName := strings.TrimPrefix(remoteStore.Name, "fileSearchStores/")

	storeManager.GetOrCreateStore(localStoreName)

	files, err := fileutil.DiscoverFiles(input.Directories, input.ExcludePatterns)
	if err != nil {
		output.Error = err.Error()
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Failed to discover files: %v", err)},
			},
		}, output, nil
	}

	if len(files) == 0 {
		output.Success = true
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "No files found in specified directories"},
			},
		}, output, nil
	}

	remoteDocs, err := s.geminiClient.ListAllDocuments(localStoreName)
	if err != nil {
		output.Error = err.Error()
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Failed to list remote documents: %v", err)},
			},
		}, output, nil
	}
	s.syncRemoteDocumentsToLocalStore(storeManager, localStoreName, remoteDocs)

	parallelism := input.Parallelism
	if parallelism <= 0 {
		parallelism = 5
	}
	uploader := gemini.NewUploader(s.geminiClient, storeManager, localStoreName, parallelism)

	results := uploader.UploadFiles(files, nil)

	for _, r := range results {
		if r.Error != nil {
			output.Failed++
		} else if r.Skipped {
			output.Skipped++
		} else {
			output.Uploaded++
		}
	}

	if err := storeManager.Save(); err != nil {
		output.Error = fmt.Sprintf("uploads completed but failed to save store data: %v", err)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: output.Error},
			},
		}, output, nil
	}

	output.Success = output.Failed == 0

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Upload complete. Uploaded: %d, Skipped: %d, Failed: %d",
				output.Uploaded, output.Skipped, output.Failed)},
		},
	}, output, nil
}

// handleUploadDirectoryEmbed handles directory indexing for embedding stores
func (s *Server) handleUploadDirectoryEmbed(ctx context.Context, storeName string, input UploadDirectoryInput) (*mcp.CallToolResult, UploadDirectoryOutput, error) {
	output := UploadDirectoryOutput{}

	config := rag.DefaultConfig()
	if input.ChunkSize > 0 {
		config.ChunkSize = input.ChunkSize
	}
	if input.ChunkOverlap > 0 {
		config.ChunkOverlap = input.ChunkOverlap
	}
	if input.Dimension > 0 {
		config.Dimension = input.Dimension
	}
	if input.PDFMaxPages != 0 {
		if err := validatePDFMaxPages(input.PDFMaxPages); err != nil {
			return nil, output, err
		}
		config.PDFMaxPages = input.PDFMaxPages
	}

	result, err := s.ragEngine.Index(input.Directories, input.ExcludePatterns, storeName, config)
	if err != nil {
		output.Error = err.Error()
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Indexing failed: %v", err)},
			},
		}, output, nil
	}

	output.Success = true
	output.Uploaded = result.IndexedFiles
	output.Skipped = result.SkippedFiles

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Indexed %d files (%d chunks). New: %d, Updated: %d, Skipped: %d, Multimodal: %d",
				result.IndexedFiles, result.TotalChunks, result.NewFiles, result.UpdatedFiles, result.SkippedFiles, result.MultimodalFiles)},
		},
	}, output, nil
}

func (s *Server) syncRemoteDocumentsToLocalStore(storeManager *store.Manager, storeName string, docs []gemini.FileSearchDocument) {
	remotePaths := make(map[string]struct{}, len(docs))

	for _, doc := range docs {
		if doc.DisplayName == "" {
			continue
		}
		remotePaths[doc.DisplayName] = struct{}{}

		checksum := gemini.GetDocumentChecksum(&doc)
		checksum = strings.TrimPrefix(checksum, "sha256:")

		meta := store.FileMetadata{
			LocalPath:  doc.DisplayName,
			RemoteID:   doc.Name,
			RemoteName: doc.DisplayName,
			Checksum:   checksum,
			MimeType:   doc.MimeType,
		}
		if existing, ok := storeManager.GetFileByPath(storeName, doc.DisplayName); ok {
			meta.Size = existing.Size
			meta.UploadedAt = existing.UploadedAt
		}
		storeManager.AddFile(storeName, meta)
	}

	for _, meta := range storeManager.GetAllFiles(storeName) {
		if _, ok := remotePaths[meta.LocalPath]; ok {
			continue
		}
		storeManager.RemoveFile(storeName, meta.LocalPath)
	}
}
