# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Ragujuary is a CLI tool and MCP server for managing Google's Gemini File Search Stores - a fully managed RAG (Retrieval-Augmented Generation) system. It enables uploading documents, semantic search queries with citations, and exposes functionality to AI assistants via MCP protocol.

## Build Commands

```bash
# Build the binary
make build

# Clean build artifacts
make clean

# Cross-platform release builds (linux/darwin/windows, amd64/arm64)
make release

# Run directly
go run .
```

Version is set via LDFLAGS in Makefile (currently 0.11.0).

## Architecture

```
main.go                    # Entry point → cmd.Execute()
cmd/                       # CLI commands (Cobra framework)
├── root.go               # Global flags: --api-key, --store, --data-file, --parallelism
├── upload.go             # Upload files with deduplication
├── query.go              # RAG queries with citations
├── list.go               # List stores/documents
├── delete.go             # Delete files/stores
├── sync.go               # Sync local metadata with remote
├── pull.go               # Pull remote metadata to local cache
├── serve.go              # MCP server (stdio/sse/http transports)
└── ...
internal/
├── gemini/               # Gemini File Search API client
│   ├── client.go         # Store/document CRUD, RAG queries, resumable uploads
│   └── uploader.go       # Parallel upload worker pool
├── store/                # Local metadata persistence (~/.ragujuary.json)
│   ├── manager.go        # Thread-safe store operations (RWMutex)
│   └── types.go          # StoreData, Store, FileMetadata types
├── fileutil/             # File discovery, checksum calculation
└── mcp/                  # MCP server implementation
    ├── server.go         # Server setup, tool registration
    ├── handlers.go       # Tool handlers
    └── middleware.go     # API key authentication
```

## Key Workflows

**Upload**: CLI → Store Manager checks metadata → File discovery with regex filtering → SHA256 checksum deduplication → Gemini resumable upload → Parallel worker pool → Persist to local JSON

**Query**: User question → Store name resolution (display name → API name) → Gemini GenerateContent with FileSearch tool → Response with citations

**Sync**: Fetch remote documents → Compare with local cache → Import missing / remove orphaned → Update remote IDs

**Pull**: Fetch all remote documents → Extract checksums from customMetadata → Create/update local cache → Enable multi-machine sync

## Configuration

Environment variables:
- `GEMINI_API_KEY` - Required for API access
- `RAGUJUARY_STORE` - Default store name
- `RAGUJUARY_SERVE_API_KEY` - MCP server authentication

Local metadata: `~/.ragujuary.json` (JSON with stores, files, checksums, timestamps)

## Dependencies

- `github.com/spf13/cobra` - CLI framework
- `github.com/modelcontextprotocol/go-sdk` - MCP protocol

## API Integration

- Base URL: `https://generativelanguage.googleapis.com/v1beta`
- Upload URL: `https://generativelanguage.googleapis.com/upload/v1beta`
- Authentication: Query parameter `?key=<GEMINI_API_KEY>`
- Uses Google's resumable upload protocol for reliability

## Gemini File Search API Constraints

**Document Upload Behavior**:
- Same `displayName` does NOT overwrite - creates duplicates
- Must delete existing document before re-uploading to update

**API Limits**:
- `customMetadata.stringValue`: max 256 characters
- `customMetadata` entries: max 20 per document
- `ListDocuments` pageSize: max 20

**Document Deletion**:
- Requires `force=true` query parameter for documents with chunks
- Without force: returns `FAILED_PRECONDITION: Cannot delete non-empty Document`

**Response Quirks**:
- `sizeBytes` is returned as string, not number
- Upload operation response contains `documentName` in `response` field

## CLI vs MCP Differences

| Feature | CLI | MCP |
|---------|-----|-----|
| Local cache | `~/.ragujuary.json` | None (stateless) |
| Checksum storage | Local JSON | `customMetadata` |
| Duplicate prevention | Local checksum comparison | Remote customMetadata comparison |
| List speed | Fast (local) | Slower (API pagination) |

**MCP Upload Flow** (with duplicate prevention):
1. Calculate SHA256 checksum of content
2. Search for existing document by displayName (paginated)
3. If found: compare checksums via customMetadata
   - Match → Skip upload
   - Differ → Delete old document → Upload new
4. If not found: Upload with checksum in customMetadata
