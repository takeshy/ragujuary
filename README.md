# ragujuary

A CLI tool and MCP server for RAG (Retrieval-Augmented Generation) using Google's Gemini APIs.

## Features

### Two RAG Modes

**FileSearch Mode** (Managed RAG):
- Create and manage Gemini File Search Stores
- Upload files with automatic server-side chunking and embedding
- Query documents using natural language with built-in citations
- Parallel uploads (default 5 workers)
- Checksum-based deduplication (skip unchanged files)
- Checksum stored in customMetadata for cross-machine sync
- Sync/fetch for multi-machine workflows

**Embedding Mode** (Local RAG):
- Index files using Gemini Embedding API (`gemini-embedding-2-preview`)
- **Multimodal support**: images (PNG/JPEG), PDF, video (MP4), audio (MP3/WAV) alongside text
- Local vector storage with cosine similarity search
- Smart text chunking (paragraph/sentence-aware, Japanese supported)
- Incremental indexing (only re-embeds changed files)
- Configurable chunk size, overlap, top-K, and min-score
- OpenAI-compatible backends (Ollama, LM Studio) for text-only embedding

### Common
- Delete files or entire stores
- List uploaded/indexed documents with filtering
- **MCP Server**: Expose all features to AI assistants (Claude Desktop, Cline, etc.)

## What is Gemini File Search?

Gemini File Search is a fully managed RAG system built into the Gemini API. Unlike the basic File API (which expires files after 48 hours), File Search Stores:

- Store documents indefinitely until manually deleted
- Automatically chunk and create embeddings for your documents
- Provide semantic search over your content
- Support a wide range of formats (PDF, DOCX, TXT, JSON, code files, etc.)
- Include citations in responses for verification

## What is Gemini Embedding?

The Gemini Embedding API generates vector representations of content in a unified semantic space, enabling cross-modal search (e.g., find images with text queries):

- Model: `gemini-embedding-2-preview` (multimodal, 8192 tokens)
- **Supported modalities**: text, images (PNG/JPEG), PDF (up to 6 pages), video (up to 120s), audio (up to 80s)
- Task types optimized for retrieval: `RETRIEVAL_DOCUMENT` (indexing), `RETRIEVAL_QUERY` (searching)
- Configurable output dimensions (128-3072, default 768)
- Batch embedding for text; individual embedding for multimodal content

## Installation

```bash
go install github.com/takeshy/ragujuary@latest
```

Or build from source:

```bash
git clone https://github.com/takeshy/ragujuary.git
cd ragujuary
go build -o ragujuary .
```

## Configuration

Set your Gemini API key:

```bash
export GEMINI_API_KEY=your-api-key
```

Or use the `--api-key` flag with each command.

Optionally, set a default store name:

```bash
export RAGUJUARY_STORE=mystore
```

Or use the `--store` / `-s` flag with each command.

### Store Name Resolution

You can specify stores by **display name** (recommended) or full API name:

```bash
# Using display name (simple, recommended)
ragujuary list -s my-store --remote

# Using full API name (with fileSearchStores/ prefix)
ragujuary list -s fileSearchStores/mystore-abc123xyz --remote
```

To see available stores and their display names:

```bash
ragujuary list --stores
```

## Usage

### FileSearch Mode

#### Create a store and upload files

```bash
# Create a store and upload files
ragujuary upload --create -s mystore ./docs

# Upload from multiple directories
ragujuary upload --create -s mystore ./docs ./src ./config

# Exclude files matching patterns
ragujuary upload --create -s mystore -e '\.git' -e 'node_modules' ./project

# Set parallelism
ragujuary upload -s mystore -p 10 ./large-project

# Dry run (see what would be uploaded)
ragujuary upload -s mystore --dry-run ./docs
```

#### Query your documents (RAG)

```bash
# Basic query
ragujuary query -s mystore "What are the main features?"

# Query multiple stores
ragujuary query --stores store1,store2 "Search across all docs"

# Use a different model (default: gemini-3-flash-preview)
ragujuary query -s mystore -m gemini-2.5-flash "Explain the architecture"

# Show citation details
ragujuary query -s mystore --citations "How does authentication work?"
```

#### List stores and files

```bash
# List all File Search Stores
ragujuary list --stores

# List documents in a store (from remote API)
ragujuary list -s mystore --remote

# List documents from local cache
ragujuary list -s mystore

# Filter by pattern
ragujuary list -s mystore -P '\.go$'

# Show detailed information
ragujuary list -s mystore -l --remote
```

#### Delete files or stores

```bash
# Delete files matching pattern
ragujuary delete -s mystore -P '\.tmp$'

# Force delete without confirmation
ragujuary delete -s mystore -P '\.log$' -f

# Delete specific documents by ID (useful for duplicates)
ragujuary delete -s mystore --id hometakeshyworkjoinshubotdo-mckqpvve11hv
ragujuary delete -s mystore --id doc-id-1 --id doc-id-2

# Delete an entire store
ragujuary delete -s mystore --all

# Force delete store without confirmation
ragujuary delete -s mystore --all -f
```

#### Status

Check status of files (modified, unchanged, missing):

```bash
ragujuary status -s mystore
```

#### Sync

Sync local metadata with remote state. This imports remote documents into the local cache:

```bash
# Import remote documents to local cache
ragujuary sync -s mystore

# After sync, you can list from local cache (faster, no API call)
ragujuary list -s mystore
```

The sync command:
- Imports documents from remote that don't exist locally
- Removes orphaned local entries that no longer exist on remote
- Updates local entries with current remote document IDs

#### Fetch

Fetch remote document metadata to local cache. Useful for syncing across multiple machines or importing documents uploaded via MCP:

```bash
# Fetch remote metadata to local cache
ragujuary fetch -s mystore

# Force update even if local file checksum differs
ragujuary fetch -s mystore -f
```

The fetch command:
- Fetches metadata of all documents from remote store (not the actual files)
- Compares local file checksums with remote checksums (stored in customMetadata)
- Updates local cache if checksums match
- Shows warning and skips if checksums differ (use `--force` to override)
- Handles files not found on disk with a warning

**Important for multi-machine usage**: When uploading from a different machine, always run `fetch` first to sync the local cache with the remote store. This prevents duplicate documents from being created.

#### Clean

Remove remote documents that no longer exist locally:

```bash
ragujuary clean -s mystore
ragujuary clean -s mystore -f  # force without confirmation
```

### Embedding Mode

#### Index files

```bash
# Index files from directories (text files are chunked, images/PDF/video/audio are embedded as-is)
ragujuary embed index -s mystore ./docs

# Index from multiple directories with exclusions
ragujuary embed index -s mystore -e '\.git' -e 'node_modules' ./project ./docs

# Custom chunking parameters (applies to text files)
ragujuary embed index -s mystore --chunk-size 500 --chunk-overlap 100 ./docs

# Use a different model/dimension
ragujuary embed index -s mystore --model gemini-embedding-2-preview --dimension 1536 ./docs

# Use Ollama (text-only, multimodal files are skipped with a warning)
ragujuary embed index -s mystore --embed-url http://localhost:11434 --model nomic-embed-text ./docs
```

Indexing is incremental: only files with changed checksums are re-embedded.
Multimodal files (images, PDF, video, audio) are detected automatically by extension and embedded as single vectors without chunking.

#### Query the embedding store

Text queries search across all indexed content, including text chunks and multimodal files (cross-modal search in the same embedding space).

```bash
# Semantic search (searches text and multimodal content)
ragujuary embed query -s mystore "How does authentication work?"

# Find images by description
ragujuary embed query -s mystore "photo of a cat"

# Customize results
ragujuary embed query -s mystore --top-k 10 --min-score 0.5 "error handling patterns"
```

#### List indexed files

```bash
# List all embedding stores
ragujuary embed list --stores

# List files in a specific store
ragujuary embed list -s mystore
```

#### Delete files from index

```bash
# Delete files matching a pattern
ragujuary embed delete -s mystore -P '\.tmp$'
```

#### Clear an entire store

```bash
ragujuary embed clear -s mystore
```

### MCP Server

Start an MCP (Model Context Protocol) server to expose ragujuary functionality to AI assistants like Claude Desktop, Cline, etc.

#### Transport Options

- **http** (recommended): Streamable HTTP for bidirectional communication
- **sse**: Server-Sent Events over HTTP for remote connections
- **stdio** (default): For local CLI integration

#### Usage

```bash
# Start HTTP server on port 8080 (recommended for remote access)
ragujuary serve --transport http --port 8080 --serve-api-key mysecretkey

# Or use environment variable for API key
export RAGUJUARY_SERVE_API_KEY=mysecretkey
ragujuary serve --transport http --port 8080

# Start SSE server (alternative)
ragujuary serve --transport sse --port 8080 --serve-api-key mysecretkey

# Start stdio server (for Claude Desktop local integration)
ragujuary serve
```

#### Claude Desktop Configuration

Add to `~/.config/claude/claude_desktop_config.json`:

```json
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
}
```

#### Available MCP Tools

The MCP server exposes 9 tools: 7 for FileSearch mode and 2 for Embedding mode.

##### `upload` - Upload a file to a store

Upload a single file to a Gemini File Search Store. Provide file content directly.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `store_name` | string | Yes | Name of the File Search Store |
| `file_name` | string | Yes | File name or path for the uploaded file |
| `file_content` | string | Yes | File content (plain text or base64 encoded) |
| `is_base64` | boolean | No | Set to true if file_content is base64 encoded (for binary files like PDF, images) |

Example (text file):
```json
{
  "store_name": "my-docs",
  "file_name": "README.md",
  "file_content": "# My Document\n\nThis is the content."
}
```

Example (binary file):
```json
{
  "store_name": "my-docs",
  "file_name": "document.pdf",
  "file_content": "JVBERi0xLjQK...",
  "is_base64": true
}
```

##### `query` - Query documents (RAG)

Query documents using natural language with semantic search.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `store_name` | string | No* | Name of the File Search Store |
| `store_names` | array | No* | Names of multiple File Search Stores to query |
| `question` | string | Yes | The question to ask about your documents |
| `model` | string | No | Model to use (default: gemini-3-flash-preview) |
| `metadata_filter` | string | No | Metadata filter expression |
| `show_citations` | boolean | No | Include citation details in response |

*Either `store_name` or `store_names` must be provided.

Example (single store):
```json
{
  "store_name": "my-docs",
  "question": "How does the authentication system work?",
  "model": "gemini-2.5-flash",
  "show_citations": true
}
```

Example (multiple stores):
```json
{
  "store_names": ["docs-store", "api-store"],
  "question": "Search across all documentation"
}
```

##### `list` - List documents in a store

List all documents in a File Search Store with optional filtering.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `store_name` | string | Yes | Name of the store to list files from |
| `pattern` | string | No | Regex pattern to filter results |

Example:
```json
{
  "store_name": "my-docs",
  "pattern": "\\.go$"
}
```

##### `delete` - Delete a file from a store

Delete a single file from a File Search Store by file name.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `store_name` | string | Yes | Name of the store |
| `file_name` | string | Yes | File name to delete |

Example:
```json
{
  "store_name": "my-docs",
  "file_name": "README.md"
}
```

##### `create_store` - Create a new store

Create a new File Search Store.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `store_name` | string | Yes | Display name for the new store |

Example:
```json
{
  "store_name": "my-new-store"
}
```

##### `delete_store` - Delete a store

Delete an entire File Search Store and all its documents.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `store_name` | string | Yes | Name of the store to delete |

Example:
```json
{
  "store_name": "my-docs"
}
```

##### `list_stores` - List all stores

List all available File Search Stores.

No parameters required.

Example:
```json
{}
```

##### `embed_index` - Index content with embeddings

Index content for local semantic search. Supports text (chunked) and multimodal content (images, PDF, video, audio as single embeddings).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `store_name` | string | Yes | Name of the embedding store |
| `file_name` | string | Yes | File name or identifier |
| `file_content` | string | Yes | Text content or base64-encoded binary |
| `model` | string | No | Embedding model (default: gemini-embedding-2-preview) |
| `chunk_size` | integer | No | Chunk size in characters (default: 1000, text only) |
| `chunk_overlap` | integer | No | Chunk overlap in characters (default: 200, text only) |
| `dimension` | integer | No | Embedding dimensionality (default: 768) |
| `mime_type` | string | No | MIME type for binary content (e.g. `image/png`, `application/pdf`) |
| `is_base64` | boolean | No | Set to true if file_content is base64-encoded binary |

Example (text):
```json
{
  "store_name": "my-docs",
  "file_name": "notes.md",
  "file_content": "# Meeting Notes\n\nDiscussed the new authentication system..."
}
```

Example (image):
```json
{
  "store_name": "my-docs",
  "file_name": "diagram.png",
  "file_content": "iVBORw0KGgoAAAANSUhEUg...",
  "mime_type": "image/png",
  "is_base64": true
}
```

##### `embed_query` - Search embeddings

Query the local embedding store using semantic search.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `store_name` | string | Yes | Name of the embedding store |
| `question` | string | Yes | The question to search for |
| `top_k` | integer | No | Number of top results (default: 5) |
| `min_score` | number | No | Minimum similarity score (default: 0.3) |
| `model` | string | No | Embedding model (default: gemini-embedding-2-preview) |

Example:
```json
{
  "store_name": "my-docs",
  "question": "What was discussed about authentication?",
  "top_k": 3
}
```

#### HTTP Authentication

For HTTP/SSE transport, set authentication via:
- `--serve-api-key` flag
- `RAGUJUARY_SERVE_API_KEY` environment variable

Clients can authenticate using:
- `X-API-Key` header
- `Authorization: Bearer <key>` header
- `api_key` query parameter

## Data Storage

### FileSearch Mode
File metadata is stored in `~/.ragujuary.json` by default. Use `--data-file` to specify a different location.

Each store tracks:
- Local file path
- Remote document ID
- SHA256 checksum
- File size
- Upload timestamp
- MIME type

### Embedding Mode
Embedding stores are saved in `~/.ragujuary-embed/<store-name>/`:
- `index.json` - Chunk metadata, file checksums, embedding model, dimension
- `vectors.bin` - Float32 vector data (binary)

## Global Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--api-key` | `-k` | Gemini API key | `$GEMINI_API_KEY` |
| `--store` | `-s` | Store name | `$RAGUJUARY_STORE` or `default` |
| `--data-file` | `-d` | Path to data file | `~/.ragujuary.json` |
| `--parallelism` | `-p` | Number of parallel uploads | `5` |

## Supported File Formats

File Search supports a wide range of formats:
- Documents: PDF, DOCX, TXT, MD
- Data: JSON, CSV, XML
- Code: Go, Python, JavaScript, TypeScript, Java, C, C++, and more

## Pricing

- Embedding generation at indexing: $0.15 per 1M tokens
- Storage: Free
- Query-time embeddings: Free
- Retrieved tokens: Standard context token rates

## Limits

### FileSearch Mode
- Max file size: 100 MB per file
- Storage: 1 GB (Free tier) to 1 TB (Tier 3)
- Max stores per project: 10

### Embedding Mode
- Text: 8,192 tokens per chunk
- Images: max 6 per request (PNG, JPEG)
- PDF: max 6 pages per file
- Video: max 120 seconds (80s with audio track)
- Audio: max 80 seconds
- Output dimensions: 128-3,072
- Multimodal embedding requires Gemini backend (not available with OpenAI-compatible backends)

## License

MIT
