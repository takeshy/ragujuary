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
	APIKey        string
	EmbedURL      string   // Optional: OpenAI-compatible embedding URL (e.g. http://localhost:11434 for Ollama)
	EmbedAPIKey   string   // Optional: API key for OpenAI-compatible embedding APIs
	DataFile      string   // Optional: path to store data file (default: ~/.ragujuary.json)
	AllowedStores []string // Optional: restrict to specific stores
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
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "upload",
		Description: "Upload/index a file to a store. Auto-detects store type: embedding stores index content locally, FileSearch stores upload to Gemini cloud. For multimodal content (image/PDF/video/audio), set mime_type and is_base64=true.",
	}, s.handleUpload)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "query",
		Description: "Query documents in a store using natural language. Auto-detects store type: embedding stores use cosine similarity search, FileSearch stores use Gemini's grounded generation with citations.",
	}, s.handleQuery)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list",
		Description: "List files in a store. Auto-detects store type.",
	}, s.handleList)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "delete",
		Description: "Delete a file from a store by file name. Auto-detects store type.",
	}, s.handleDelete)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "create_store",
		Description: "Create a new store. Set type='embed' for embedding store or type='filesearch' (default) for Gemini FileSearch store.",
	}, s.handleCreateStore)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "delete_store",
		Description: "Delete an entire store and all its data. Auto-detects store type.",
	}, s.handleDeleteStore)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_stores",
		Description: "List all available stores (both embedding and FileSearch).",
	}, s.handleListStores)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "upload_directory",
		Description: "Upload/index files from directories to a store. Auto-detects store type: embedding stores index locally, FileSearch stores upload to Gemini cloud. Recursively discovers files and skips unchanged files.",
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

// getStoreName returns the store name from input, returns error if not specified or not allowed
func (s *Server) getStoreName(inputStoreName string) (string, error) {
	if inputStoreName == "" {
		// If only one store is allowed, use it as default
		if len(s.config.AllowedStores) == 1 {
			return s.config.AllowedStores[0], nil
		}
		return "", fmt.Errorf("store_name is required")
	}
	if !s.isStoreAllowed(inputStoreName) {
		return "", fmt.Errorf("store '%s' is not in the allowed stores list", inputStoreName)
	}
	return inputStoreName, nil
}

// getAllowedStoreNames validates and resolves one or more store names.
func (s *Server) getAllowedStoreNames(inputStoreName string, inputStoreNames []string) ([]string, error) {
	if len(inputStoreNames) == 0 {
		storeName, err := s.getStoreName(inputStoreName)
		if err != nil {
			return nil, err
		}
		return []string{storeName}, nil
	}

	names := make([]string, 0, len(inputStoreNames))
	seen := make(map[string]struct{}, len(inputStoreNames))
	for _, name := range inputStoreNames {
		if name == "" {
			return nil, fmt.Errorf("store_names must not contain empty values")
		}
		if !s.isStoreAllowed(name) {
			return nil, fmt.Errorf("store '%s' is not in the allowed stores list", name)
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("store_names must not be empty")
	}
	return names, nil
}

// isStoreAllowed checks if a store name is in the allowed list (empty list means all allowed)
func (s *Server) isStoreAllowed(name string) bool {
	if len(s.config.AllowedStores) == 0 {
		return true
	}
	for _, allowed := range s.config.AllowedStores {
		if allowed == name {
			return true
		}
	}
	return false
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
