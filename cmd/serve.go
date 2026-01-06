package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	mcpserver "github.com/takeshy/ragujuary/internal/mcp"
)

var (
	serveTransport string
	servePort      int
	serveAPIKey    string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start MCP server for AI assistant integration",
	Long: `Start a Model Context Protocol (MCP) server that exposes ragujuary
functionality to AI assistants like Claude Desktop, Cline, etc.

Transport options:
  stdio: Standard input/output (default, for local CLI integration)
  sse:   Server-Sent Events over HTTP (for remote connections, requires API key)
  http:  Streamable HTTP (for bidirectional HTTP communication, requires API key)

Examples:
  # Start stdio server (for Claude Desktop config)
  ragujuary serve

  # Start HTTP/SSE server on port 8080 (API key required)
  ragujuary serve --transport sse --port 8080 --serve-api-key mysecretkey

  # Or use environment variable for API key
  export RAGUJUARY_SERVE_API_KEY=mysecretkey
  ragujuary serve --transport sse --port 8080

Claude Desktop Configuration (~/.config/claude/claude_desktop_config.json):
  {
    "mcpServers": {
      "ragujuary": {
        "command": "/path/to/ragujuary",
        "args": ["serve"],
        "env": {
          "GEMINI_API_KEY": "your-gemini-api-key"
        }
      }
    }
  }`,
	RunE: runServe,
}

func init() {
	serveCmd.Flags().StringVar(&serveTransport, "transport", "stdio", "Transport type: stdio, sse, or http")
	serveCmd.Flags().IntVar(&servePort, "port", 8080, "Port for HTTP/SSE server")
	serveCmd.Flags().StringVar(&serveAPIKey, "serve-api-key", "", "API key for HTTP authentication (or RAGUJUARY_SERVE_API_KEY env var)")
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	// Get Gemini API key
	key, err := getAPIKey()
	if err != nil {
		return err
	}

	// Create MCP server config
	config := mcpserver.ServerConfig{
		APIKey: key,
	}

	// Create MCP server
	server, err := mcpserver.NewServer(config, Version)
	if err != nil {
		return fmt.Errorf("failed to create MCP server: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	switch serveTransport {
	case "stdio":
		go func() {
			<-sigChan
			cancel()
		}()
		fmt.Fprintln(os.Stderr, "Starting MCP server on stdio...")
		return server.RunStdio(ctx)

	case "sse":
		return runHTTPServerWithShutdown(server.NewHTTPHandler(), "SSE", sigChan)

	case "http":
		return runHTTPServerWithShutdown(server.NewStreamableHTTPHandler(), "HTTP", sigChan)

	default:
		return fmt.Errorf("unknown transport: %s (must be stdio, sse, or http)", serveTransport)
	}
}

func runHTTPServerWithShutdown(handler http.Handler, transportName string, sigChan chan os.Signal) error {
	// Get serve API key
	httpAPIKey := serveAPIKey
	if httpAPIKey == "" {
		httpAPIKey = os.Getenv("RAGUJUARY_SERVE_API_KEY")
	}

	// Require API key for HTTP server
	if httpAPIKey == "" {
		return fmt.Errorf("API key required for HTTP server. Use --serve-api-key or set RAGUJUARY_SERVE_API_KEY environment variable")
	}

	handler = mcpserver.APIKeyMiddleware(httpAPIKey, handler)

	addr := fmt.Sprintf(":%d", servePort)
	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	// Graceful shutdown on signal
	go func() {
		<-sigChan
		fmt.Fprintln(os.Stderr, "\nShutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()

	fmt.Fprintf(os.Stderr, "Starting MCP %s server on http://localhost%s (API key authentication enabled)\n", transportName, addr)

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}
