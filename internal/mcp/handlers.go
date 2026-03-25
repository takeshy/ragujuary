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

// handleUpload handles the upload tool
func (s *Server) handleUpload(ctx context.Context, req *mcp.CallToolRequest, input UploadInput) (*mcp.CallToolResult, UploadOutput, error) {
	output := UploadOutput{FileName: input.FileName}

	// Validate input
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

	// Resolve store name (store must exist)
	resolvedName, _, err := s.geminiClient.ResolveStoreName(storeName)
	if err != nil {
		return nil, output, fmt.Errorf("store '%s' not found", storeName)
	}

	// Decode content
	var content []byte
	if input.IsBase64 {
		content, err = base64.StdEncoding.DecodeString(input.FileContent)
		if err != nil {
			return nil, output, fmt.Errorf("failed to decode base64 content: %w", err)
		}
	} else {
		content = []byte(input.FileContent)
	}

	// Calculate checksum (raw hex, consistent with Uploader)
	hash := sha256.Sum256(content)
	checksum := hex.EncodeToString(hash[:])

	// Check for existing document with same display name
	existingDoc, err := s.geminiClient.FindDocumentByDisplayName(resolvedName, input.FileName)
	if err != nil {
		// Log error but continue with upload
		// This is not fatal - we'll just upload the file
	}

	if existingDoc != nil {
		// Check if checksum matches (normalize prefix for backward compat)
		existingChecksum := strings.TrimPrefix(gemini.GetDocumentChecksum(existingDoc), "sha256:")
		if existingChecksum == checksum {
			// Same content, skip upload
			output.Success = true
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Skipped '%s': content unchanged", input.FileName)},
				},
			}, output, nil
		}

		// Different content, delete old document first
		if err := s.geminiClient.DeleteDocument(existingDoc.Name); err != nil {
			// Log error but continue with upload
			// Old document might remain, but new one will be uploaded
		}
	}

	// Upload content with checksum metadata
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

	// Wait for operation to complete
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

// handleQuery handles the query tool
func (s *Server) handleQuery(ctx context.Context, req *mcp.CallToolRequest, input QueryInput) (*mcp.CallToolResult, QueryOutput, error) {
	output := QueryOutput{}

	if input.Question == "" {
		return nil, output, fmt.Errorf("question is required")
	}

	model := input.Model
	if model == "" {
		model = "gemini-3-flash-preview"
	}

	// Resolve store names
	var storeNames []string
	if len(input.StoreNames) > 0 {
		// Multiple stores specified
		for _, name := range input.StoreNames {
			_, remoteStore, err := s.geminiClient.ResolveStoreName(name)
			if err != nil {
				return nil, output, fmt.Errorf("store '%s' not found: %w", name, err)
			}
			storeNames = append(storeNames, remoteStore.Name)
		}
	} else {
		// Single store (backward compatible)
		storeName, err := s.getStoreName(input.StoreName)
		if err != nil {
			return nil, output, err
		}
		_, remoteStore, err := s.geminiClient.ResolveStoreName(storeName)
		if err != nil {
			return nil, output, fmt.Errorf("store '%s' not found: %w", storeName, err)
		}
		storeNames = []string{remoteStore.Name}
	}

	// Perform query
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

	// Extract answer
	candidate := resp.Candidates[0]
	var answerBuilder strings.Builder
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			answerBuilder.WriteString(part.Text)
		}
	}
	output.Answer = answerBuilder.String()

	// Extract citations if requested
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

// handleList handles the list tool
func (s *Server) handleList(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
	output := ListOutput{Items: []ListItem{}}

	storeName, err := s.getStoreName(input.StoreName)
	if err != nil {
		return nil, output, err
	}

	// Resolve store and list from remote
	resolvedName, st, err := s.geminiClient.ResolveStoreName(storeName)
	if err != nil {
		return nil, output, fmt.Errorf("store '%s' not found: %w", storeName, err)
	}

	docs, err := s.geminiClient.ListAllDocuments(resolvedName)
	if err != nil {
		return nil, output, fmt.Errorf("failed to list documents: %w", err)
	}

	// Apply pattern filter if provided
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

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Found %d documents in store '%s'", output.Total, st.DisplayName)},
		},
	}, output, nil
}

// handleDelete handles the delete tool
func (s *Server) handleDelete(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, DeleteOutput, error) {
	output := DeleteOutput{FileName: input.FileName}

	if input.FileName == "" {
		return nil, output, fmt.Errorf("file_name is required")
	}

	storeName, err := s.getStoreName(input.StoreName)
	if err != nil {
		return nil, output, err
	}

	// Resolve store
	resolvedName, _, err := s.geminiClient.ResolveStoreName(storeName)
	if err != nil {
		return nil, output, fmt.Errorf("store '%s' not found: %w", storeName, err)
	}

	// List documents to find the one matching file_name
	docs, err := s.geminiClient.ListAllDocuments(resolvedName)
	if err != nil {
		return nil, output, fmt.Errorf("failed to list documents: %w", err)
	}

	// Find document by display name
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

	store, err := s.geminiClient.CreateFileSearchStore(input.StoreName)
	if err != nil {
		output.Error = err.Error()
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Failed to create store: %v", err)},
			},
		}, output, nil
	}

	output.Success = true
	output.StoreID = store.Name
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Successfully created store '%s' (ID: %s)", input.StoreName, store.Name)},
		},
	}, output, nil
}

// handleDeleteStore handles the delete_store tool
func (s *Server) handleDeleteStore(ctx context.Context, req *mcp.CallToolRequest, input DeleteStoreInput) (*mcp.CallToolResult, DeleteStoreOutput, error) {
	output := DeleteStoreOutput{StoreName: input.StoreName}

	if input.StoreName == "" {
		return nil, output, fmt.Errorf("store_name is required")
	}

	// Resolve store
	resolvedName, remoteStore, err := s.geminiClient.ResolveStoreName(input.StoreName)
	if err != nil {
		return nil, output, fmt.Errorf("store '%s' not found: %w", input.StoreName, err)
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
			&mcp.TextContent{Text: fmt.Sprintf("Successfully deleted store '%s'", remoteStore.DisplayName)},
		},
	}, output, nil
}

// handleListStores handles the list_stores tool
func (s *Server) handleListStores(ctx context.Context, req *mcp.CallToolRequest, input ListStoresInput) (*mcp.CallToolResult, ListStoresOutput, error) {
	output := ListStoresOutput{Stores: []StoreInfo{}}

	// Get all stores with pagination
	pageToken := ""
	for {
		resp, err := s.geminiClient.ListFileSearchStores(pageToken)
		if err != nil {
			return nil, output, fmt.Errorf("failed to list stores: %w", err)
		}

		for _, st := range resp.FileSearchStores {
			output.Stores = append(output.Stores, StoreInfo{
				Name:        st.Name,
				DisplayName: st.DisplayName,
				CreateTime:  st.CreateTime,
				UpdateTime:  st.UpdateTime,
			})
		}

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	output.Total = len(output.Stores)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Found %d stores", output.Total)},
		},
	}, output, nil
}

// handleEmbedIndex handles the embed_index tool
func (s *Server) handleEmbedIndex(ctx context.Context, req *mcp.CallToolRequest, input EmbedIndexInput) (*mcp.CallToolResult, EmbedIndexOutput, error) {
	output := EmbedIndexOutput{FileName: input.FileName}

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

	config := rag.DefaultConfig()
	if input.Model != "" {
		config.Model = input.Model
	}
	if input.ChunkSize > 0 {
		config.ChunkSize = input.ChunkSize
	}
	if input.ChunkOverlap > 0 {
		config.ChunkOverlap = input.ChunkOverlap
	}
	if input.Dimension > 0 {
		config.Dimension = input.Dimension
	}

	// Multimodal path: binary content with MIME type
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

		// Count chunks from the saved index
		chunks := 1
		if input.MIMEType == "application/pdf" {
			if pageCount, err := pdfutil.PageCount(data); err == nil {
				chunks = (pageCount + pdfutil.DefaultMaxPages - 1) / pdfutil.DefaultMaxPages
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
		output.Chunks = chunks
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
	output.Chunks = len(chunks)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Successfully indexed '%s' (%d chunks)", input.FileName, output.Chunks)},
		},
	}, output, nil
}

// handleEmbedQuery handles the embed_query tool
func (s *Server) handleEmbedQuery(ctx context.Context, req *mcp.CallToolRequest, input EmbedQueryInput) (*mcp.CallToolResult, EmbedQueryOutput, error) {
	output := EmbedQueryOutput{Results: []EmbedSearchResult{}}

	if input.Question == "" {
		return nil, output, fmt.Errorf("question is required")
	}

	storeName, err := s.getStoreName(input.StoreName)
	if err != nil {
		return nil, output, err
	}

	config := rag.DefaultConfig()
	if input.Model != "" {
		config.Model = input.Model
	}
	if input.TopK > 0 {
		config.TopK = input.TopK
	}
	if input.MinScore > 0 {
		config.MinScore = input.MinScore
	}

	results, err := s.ragEngine.Query(input.Question, storeName, config)
	if err != nil {
		return nil, output, fmt.Errorf("embed query failed: %w", err)
	}

	var textBuilder strings.Builder
	for _, r := range results {
		output.Results = append(output.Results, EmbedSearchResult{
			Text:        r.Text,
			FilePath:    r.FilePath,
			Score:       r.Score,
			ContentType: r.ContentType,
			PageLabel:   r.PageLabel,
		})
		text := r.Text
		if len(text) > 300 {
			text = text[:300] + "..."
		}
		textBuilder.WriteString(fmt.Sprintf("[%.4f] %s: %s\n\n", r.Score, r.FilePath, text))
	}
	output.Total = len(output.Results)

	if output.Total == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "No results found."},
			},
		}, output, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: textBuilder.String()},
		},
	}, output, nil
}

// handleEmbedIndexDirectory handles the embed_index_directory tool
func (s *Server) handleEmbedIndexDirectory(ctx context.Context, req *mcp.CallToolRequest, input EmbedIndexDirectoryInput) (*mcp.CallToolResult, EmbedIndexDirectoryOutput, error) {
	output := EmbedIndexDirectoryOutput{}

	if len(input.Directories) == 0 {
		return nil, output, fmt.Errorf("directories is required and must not be empty")
	}

	storeName, err := s.getStoreName(input.StoreName)
	if err != nil {
		return nil, output, err
	}

	config := rag.DefaultConfig()
	if input.Model != "" {
		config.Model = input.Model
	}
	if input.ChunkSize > 0 {
		config.ChunkSize = input.ChunkSize
	}
	if input.ChunkOverlap > 0 {
		config.ChunkOverlap = input.ChunkOverlap
	}
	if input.Dimension > 0 {
		config.Dimension = input.Dimension
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
	output.TotalChunks = result.TotalChunks
	output.IndexedFiles = result.IndexedFiles
	output.SkippedFiles = result.SkippedFiles
	output.NewFiles = result.NewFiles
	output.UpdatedFiles = result.UpdatedFiles
	output.MultimodalFiles = result.MultimodalFiles
	output.SkippedMultimodal = result.SkippedMultimodal

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Indexed %d files (%d chunks). New: %d, Updated: %d, Skipped: %d, Multimodal: %d",
				output.IndexedFiles, output.TotalChunks, output.NewFiles, output.UpdatedFiles, output.SkippedFiles, output.MultimodalFiles)},
		},
	}, output, nil
}

// handleUploadDirectory handles the upload_directory tool
func (s *Server) handleUploadDirectory(ctx context.Context, req *mcp.CallToolRequest, input UploadDirectoryInput) (*mcp.CallToolResult, UploadDirectoryOutput, error) {
	output := UploadDirectoryOutput{}

	if len(input.Directories) == 0 {
		return nil, output, fmt.Errorf("directories is required and must not be empty")
	}

	storeName, err := s.getStoreName(input.StoreName)
	if err != nil {
		return nil, output, err
	}

	storeManager, err := s.getStoreManager()
	if err != nil {
		return nil, output, err
	}

	// Resolve store name (store must exist)
	_, remoteStore, err := s.geminiClient.ResolveStoreName(storeName)
	if err != nil {
		return nil, output, fmt.Errorf("store '%s' not found: %w", storeName, err)
	}
	localStoreName := strings.TrimPrefix(remoteStore.Name, "fileSearchStores/")

	// Ensure local store exists
	storeManager.GetOrCreateStore(localStoreName)

	// Discover files
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

	// Create uploader and upload files
	parallelism := input.Parallelism
	if parallelism <= 0 {
		parallelism = 5
	}
	uploader := gemini.NewUploader(s.geminiClient, storeManager, localStoreName, parallelism)

	results := uploader.UploadFiles(files, nil)

	// Tally results
	for _, r := range results {
		if r.Error != nil {
			output.Failed++
		} else if r.Skipped {
			output.Skipped++
		} else {
			output.Uploaded++
		}
	}

	// Save store data
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

func (s *Server) syncRemoteDocumentsToLocalStore(storeManager *store.Manager, storeName string, docs []gemini.FileSearchDocument) {
	remotePaths := make(map[string]struct{}, len(docs))

	for _, doc := range docs {
		if doc.DisplayName == "" {
			continue
		}
		remotePaths[doc.DisplayName] = struct{}{}

		// Normalize checksum: handleUpload stores "sha256:<hex>" but
		// Uploader/CalculateChecksum uses raw hex. Strip the prefix so
		// the local cache always matches the format Uploader expects.
		checksum := gemini.GetDocumentChecksum(&doc)
		checksum = strings.TrimPrefix(checksum, "sha256:")

		// Merge with existing local metadata to preserve Size/UploadedAt
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
