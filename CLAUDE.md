# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Ragujuary is a CLI tool and MCP server for RAG (Retrieval-Augmented Generation) using Google's Gemini APIs. It supports two RAG modes:

1. **FileSearch mode** - Managed RAG using Gemini File Search Stores (server-side retrieval with citations)
2. **Embedding mode** - Self-managed RAG using Gemini Embedding API (local vector storage with cosine similarity search)

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

Version is set via LDFLAGS in Makefile (currently 0.12.0).

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
├── embed.go              # Embedding-based RAG commands (index/query/list/delete/clear)
└── ...
internal/
├── gemini/               # Gemini File Search API client
│   ├── client.go         # Store/document CRUD, RAG queries, resumable uploads
│   └── uploader.go       # Parallel upload worker pool
├── embedding/            # Gemini Embedding API client
│   └── client.go         # EmbedContent, BatchEmbedContents
├── rag/                  # Embedding-based RAG engine
│   ├── chunker.go        # Smart text chunking (paragraph/sentence boundaries, JP support)
│   ├── store.go          # Local vector storage (JSON metadata + binary vectors)
│   ├── search.go         # Cosine similarity search
│   └── engine.go         # RAG pipeline orchestration (index, query, delete)
├── store/                # Local metadata persistence (~/.ragujuary.json)
│   ├── manager.go        # Thread-safe store operations (RWMutex)
│   └── types.go          # StoreData, Store, FileMetadata types
├── fileutil/             # File discovery, checksum calculation
└── mcp/                  # MCP server implementation
    ├── server.go         # Server setup, tool registration (FileSearch + Embed)
    ├── handlers.go       # Tool handlers (FileSearch + Embed)
    ├── tools.go          # Tool input/output types
    └── middleware.go     # API key authentication
```

## Key Workflows

**Upload**: CLI → Store Manager checks metadata → File discovery with regex filtering → SHA256 checksum deduplication → Gemini resumable upload → Parallel worker pool → Persist to local JSON

**Query**: User question → Store name resolution (display name → API name) → Gemini GenerateContent with FileSearch tool → Response with citations

**Sync**: Fetch remote documents → Compare with local cache → Import missing / remove orphaned → Update remote IDs

**Pull**: Fetch all remote documents → Extract checksums from customMetadata → Create/update local cache → Enable multi-machine sync

**Embed Index**: CLI → File discovery with regex filtering → SHA256 checksum for incremental updates → Smart text chunking (paragraph/sentence boundaries) → Batch embedding via Gemini Embedding API → Merge unchanged + new vectors → Save to local binary store

**Embed Query**: User question → Embed with RETRIEVAL_QUERY task type → Load local index + vectors → Cosine similarity search → Top-K results with scores

## Configuration

Environment variables:
- `GEMINI_API_KEY` - Required for API access
- `RAGUJUARY_STORE` - Default store name
- `RAGUJUARY_SERVE_API_KEY` - MCP server authentication

Local metadata: `~/.ragujuary.json` (JSON with stores, files, checksums, timestamps)
Embedding stores: `~/.ragujuary-embed/<store-name>/` (index.json + vectors.bin)

## Dependencies

- `github.com/spf13/cobra` - CLI framework
- `github.com/modelcontextprotocol/go-sdk` - MCP protocol

## API Integration

### FileSearch API
- Base URL: `https://generativelanguage.googleapis.com/v1beta`
- Upload URL: `https://generativelanguage.googleapis.com/upload/v1beta`
- Authentication: Query parameter `?key=<GEMINI_API_KEY>`
- Uses Google's resumable upload protocol for reliability

### Embedding API
- Endpoint: `POST /v1beta/models/{model}:embedContent` (single)
- Batch: `POST /v1beta/models/{model}:batchEmbedContents`
- Default model: `gemini-embedding-2-preview` (multimodal, 8192 tokens)
- Task types: `RETRIEVAL_DOCUMENT` (indexing), `RETRIEVAL_QUERY` (searching), `SEMANTIC_SIMILARITY`, etc.
- Output dimensions: 128-3072 (default: 768)
- Embedding spaces between models are NOT compatible (must re-index when changing model)

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

## FileSearch vs Embedding Mode

| Feature | FileSearch (managed) | Embedding (local) |
|---------|---------------------|-------------------|
| Storage | Gemini Cloud | Local (~/.ragujuary-embed/) |
| Retrieval | Server-side with citations | Cosine similarity search |
| Chunking | Gemini automatic | Custom (paragraph/sentence-aware) |
| Query | GenerateContent + FileSearch tool | Embed query + vector search |
| CLI commands | upload/query/list/delete | embed index/query/list/delete/clear |
| MCP tools | upload/query/list/delete/create_store/delete_store/list_stores | embed_index/embed_query |
| Incremental | SHA256 checksum | SHA256 checksum |
| Model change | N/A | Requires full re-index |
