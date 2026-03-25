package mcp

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/takeshy/ragujuary/internal/embedding"
	"github.com/takeshy/ragujuary/internal/gemini"
	"github.com/takeshy/ragujuary/internal/rag"
	"github.com/takeshy/ragujuary/internal/store"
)

// ServerConfig holds configuration for the MCP server
type ServerConfig struct {
	APIKey      string
	EmbedURL    string // Optional: OpenAI-compatible embedding URL (e.g. http://localhost:11434 for Ollama)
	EmbedAPIKey string // Optional: API key for OpenAI-compatible embedding APIs
	DataFile    string // Optional: path to store data file (default: ~/.ragujuary.json)
}

// Server wraps the MCP server with ragujuary-specific functionality
type Server struct {
	mcpServer    *mcp.Server
	geminiClient *gemini.Client
	ragEngine    *rag.Engine
	storeManager *store.Manager
	storeMu      sync.Mutex
	config       ServerConfig
}

// NewServer creates a new MCP server for ragujuary
func NewServer(config ServerConfig, version string) (*Server, error) {
	// Initialize gemini client
	geminiClient := gemini.NewClient(config.APIKey)

	// Initialize embedding client and RAG engine
	var embeddingClient embedding.Client
	if config.EmbedURL != "" {
		embeddingClient = embedding.NewOpenAIClient(config.EmbedURL, config.EmbedAPIKey)
	} else {
		embeddingClient = embedding.NewGeminiClient(config.APIKey)
	}
	ragEngine := rag.NewEngine(embeddingClient)

	// Create MCP server
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    "ragujuary",
		Version: version,
	}, nil)

	s := &Server{
		mcpServer:    mcpServer,
		geminiClient: geminiClient,
		ragEngine:    ragEngine,
		config:       config,
	}

	// Register all tools
	s.registerTools()

	return s, nil
}

// registerTools registers all MCP tools
func (s *Server) registerTools() {
	// Upload tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "upload",
		Description: "Upload a file to a Gemini File Search Store. Provide file content directly (base64 encoded for binary files).",
	}, s.handleUpload)

	// Query tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "query",
		Description: "Query documents in a File Search Store using natural language. Performs semantic search and generates an answer grounded in retrieved content.",
	}, s.handleQuery)

	// List tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list",
		Description: "List files in a store or list all available File Search Stores.",
	}, s.handleList)

	// Delete tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "delete",
		Description: "Delete a file from a File Search Store by file name.",
	}, s.handleDelete)

	// Create store tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "create_store",
		Description: "Create a new File Search Store.",
	}, s.handleCreateStore)

	// Delete store tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "delete_store",
		Description: "Delete an entire File Search Store and all its documents.",
	}, s.handleDeleteStore)

	// List stores tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_stores",
		Description: "List all available File Search Stores.",
	}, s.handleListStores)

	// Embedding-based RAG tools
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "embed_index",
		Description: "Index content using embeddings for local semantic search. Text is chunked and batch-embedded. For multimodal (image/PDF/video/audio), set mime_type and is_base64=true to embed as a single vector.",
	}, s.handleEmbedIndex)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "embed_query",
		Description: "Query the local embedding store using semantic search. Returns the most similar text chunks with relevance scores.",
	}, s.handleEmbedQuery)

	// Directory-based tools
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "embed_index_directory",
		Description: "Index files from directories using embeddings for local semantic search. Recursively discovers files, computes checksums for incremental updates, and batch-embeds text/multimodal content. Supports exclude patterns for filtering.",
	}, s.handleEmbedIndexDirectory)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "upload_directory",
		Description: "Upload files from directories to a Gemini File Search Store. Recursively discovers files, skips unchanged files by checksum, and uploads in parallel.",
	}, s.handleUploadDirectory)
}

// RunStdio runs the server using stdio transport
func (s *Server) RunStdio(ctx context.Context) error {
	return s.mcpServer.Run(ctx, &mcp.StdioTransport{})
}

// NewHTTPHandler creates an HTTP handler for SSE transport
func (s *Server) NewHTTPHandler() http.Handler {
	return mcp.NewSSEHandler(func(req *http.Request) *mcp.Server {
		return s.mcpServer
	}, nil)
}

// NewStreamableHTTPHandler creates a streamable HTTP handler
func (s *Server) NewStreamableHTTPHandler() http.Handler {
	return mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return s.mcpServer
	}, nil)
}

// getStoreName returns the store name from input, returns error if not specified
func (s *Server) getStoreName(inputStoreName string) (string, error) {
	if inputStoreName != "" {
		return inputStoreName, nil
	}
	return "", fmt.Errorf("store_name is required")
}

func (s *Server) getStoreManager() (*store.Manager, error) {
	s.storeMu.Lock()
	defer s.storeMu.Unlock()

	if s.storeManager != nil {
		return s.storeManager, nil
	}

	storeManager, err := store.NewManager(s.config.DataFile)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize store manager: %w", err)
	}

	s.storeManager = storeManager
	return s.storeManager, nil
}
