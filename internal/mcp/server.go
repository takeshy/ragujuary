package mcp

import (
	"context"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/takeshy/ragujuary/internal/gemini"
)

// ServerConfig holds configuration for the MCP server
type ServerConfig struct {
	APIKey string
}

// Server wraps the MCP server with ragujuary-specific functionality
type Server struct {
	mcpServer    *mcp.Server
	geminiClient *gemini.Client
	config       ServerConfig
}

// NewServer creates a new MCP server for ragujuary
func NewServer(config ServerConfig, version string) (*Server, error) {
	// Initialize gemini client
	geminiClient := gemini.NewClient(config.APIKey)

	// Create MCP server
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    "ragujuary",
		Version: version,
	}, nil)

	s := &Server{
		mcpServer:    mcpServer,
		geminiClient: geminiClient,
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
