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
- Index files using Gemini Embedding API (`gemini-embedding-2`)
- **Multimodal support**: images (PNG/JPEG), PDF, video (MP4), audio (MP3/WAV) alongside text
- **Automatic splitting**: PDFs over N pages (configurable, default 6, max 6), audio over 80s, and video over 80s/120s are automatically split into embeddable chunks
- Local vector storage with cosine similarity search
- Hybrid search (BM25 + vector + RRF) and Gemini Decision 1 text reranking enabled by default; each can be disabled
- Japanese/CJK and identifier search, warning-based fallback, and preservation of unscorable media positions
- Optional search-until-found: score successive candidate groups until relevant information is found
- Smart text chunking (paragraph/sentence-aware, Japanese supported)
- Incremental indexing (only re-embeds changed files)
- Configurable chunk size, overlap, top-K, and min-score
- OpenAI-compatible backends (Ollama, LM Studio) with automatic PDF text extraction
- Automatic retry on 503/429 API errors with exponential backoff

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

- Model: `gemini-embedding-2` (multimodal, 8192 tokens)
- **Supported modalities**: text, images (PNG/JPEG), PDF, video (MP4/MPEG), audio (MP3/WAV)
- **Per-request limits**: PDF up to 6 pages, video up to 120s (80s with audio), audio up to 80s — ragujuary automatically splits larger files
- Retrieval queries and documents are formatted with Gemini Embedding 2 task instructions automatically
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

### Prerequisites

- **ffmpeg** (optional): Required for audio/video file splitting. Install from [ffmpeg.org](https://ffmpeg.org/download.html) or via your package manager. If ffmpeg is not installed, indexing directories containing audio/video files will return an error.

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

### Upgrading to 1.0

- Local queries now default to hybrid search and Gemini Decision 1 reranking. Reranking sends eligible indexed text to Gemini and adds API usage. To keep vector-only retrieval without reranking, use `--search-mode vector --rerank=false`, or MCP parameters `"search_mode": "vector", "rerank": false`.
- FileSearch generation defaults to `gemini-4-hydrogen`. Gemini 1.x/2.x generation IDs return a migration error; use a current model ID.
- Existing embedding indexes remain usable without rebuilding. BM25 caches are built lazily; external index directories are not modified. Rebuild only when changing the embedding model itself.

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

# Use a different model (default: gemini-4-hydrogen)
ragujuary query -s mystore -m gemini-4-carbon "Explain the architecture"

# Use Gemini 4 Argon
ragujuary query -s mystore -m gemini-4-argon "Explain the architecture"

# Show citation details
ragujuary query -s mystore --citations "How does authentication work?"
```

The default File Search generation model is `gemini-4-hydrogen`. You can also explicitly select `gemini-4-argon`, `gemini-4-carbon`, or `gemini-3.8-flash`. Gemini 1.x/2.x generation models are no longer supported by ragujuary and return an error rather than being silently substituted. See Google's [current models](https://aistudio.google.com/docs/models) and [deprecation schedule](https://aistudio.google.com/docs/deprecations). Embedding remains `gemini-embedding-2`; reranking uses `gemini-decision-1`. Changing the generation model does not require re-indexing. Existing local indexes keep their recorded embedding model, since switching embedding spaces requires rebuilding the index.

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
# Index files from directories
# Text: chunked by paragraph/sentence boundaries
# PDF: auto-split into N-page chunks (default 6, max 6)
# Audio: auto-split into 80s segments (requires ffmpeg)
# Video: auto-split into 80s/120s segments (requires ffmpeg)
# Images: embedded as-is
ragujuary embed index -s mystore ./docs

# Index from multiple directories with exclusions
ragujuary embed index -s mystore -e '\.git' -e 'node_modules' ./project ./docs

# Custom chunking parameters (applies to text files)
ragujuary embed index -s mystore --chunk-size 500 --chunk-overlap 100 ./docs

# Custom PDF page chunk size (split into 3-page chunks instead of 6)
ragujuary embed index -s mystore --pdf-pages 3 ./docs

# Use a different model/dimension
ragujuary embed index -s mystore --model gemini-embedding-2 --dimension 1536 ./docs

# Use Ollama (PDFs are text-extracted and indexed; images/audio/video are skipped)
ragujuary embed index -s mystore --embed-url http://localhost:11434 --model nomic-embed-text ./docs
```

Indexing is incremental: only files with changed checksums are re-embedded.

**Gemini backend**: Images, PDF, video, and audio are embedded as multimodal vectors. PDFs exceeding the page limit (configurable via `--pdf-pages`, default 6, max 6) are split into page-range chunks, and audio/video files exceeding the duration limit are split into time-range segments using ffmpeg. Search results include page/time labels for split files.

**Text-only backends (Ollama, etc.)**: PDFs are automatically text-extracted and indexed as text chunks (searchable with content display). Images, audio, and video are skipped with a warning.

#### Query the embedding store

Text queries search across all indexed content, including text chunks and multimodal files (cross-modal search in the same embedding space).

```bash
# Hybrid search + Gemini reranking (default)
ragujuary embed query -s mystore "How does authentication work?"

# Find images by description
ragujuary embed query -s mystore "photo of a cat"

# Customize results
ragujuary embed query -s mystore --top-k 10 --min-score 0.5 "error handling patterns"

# Hybrid search: merge semantic and keyword rankings
ragujuary embed query -s mystore --search-mode hybrid "PRJ-123 authentication failure"

# Rerank candidates with the Gemini Decisions API, then return the best 5
ragujuary embed query -s mystore --search-mode hybrid --rerank --top-k 5 "authentication setup"

# Search successive groups until a relevant answer is found
ragujuary embed query -s mystore --search-until-found "authentication setup"

# Disable reranking while keeping hybrid search
ragujuary embed query -s mystore --rerank=false "authentication setup"

# Disable both hybrid search and reranking
ragujuary embed query -s mystore --search-mode vector --rerank=false "authentication setup"

# BM25 keyword search without calling the embedding API (no API key required)
ragujuary embed query -s mystore --search-mode keyword --rerank=false "ERR_AUTH_42"

# Query an external RAG index (created by other tools)
ragujuary embed query --dir /path/to/external/rag/store "search query"
```

The `--dir` flag allows querying RAG indexes created by external tools. It auto-detects both snake_case (ragujuary) and camelCase JSON field naming conventions. When `--dir` is specified, `--store` is not required.

`--search-mode` defaults to `hybrid`, combining vector and BM25 ranks using reciprocal rank fusion (RRF). Select `--search-mode vector` to disable hybrid retrieval or `--search-mode keyword` for BM25 alone. `--candidate-k` controls candidates **per search branch** (default 30, maximum 1000); `--rrf-k` sets the RRF constant (default 60). `--min-score` gates only vector similarity, so keyword matches below that threshold can still appear. Hybrid scores are RRF scores, keyword scores are BM25 scores, and vector scores are cosine similarities. Hybrid results include `source: vector`, `bm25`, or `both`. Final `--top-k` is applied after reranking, when enabled.

BM25 searches chunk text. It normalizes full-width characters, preserves identifiers such as `PRJ-123` and `snake_case` both whole and split, and uses overlapping CJK bigrams (the common library's fallback tokenizer). Existing embeddings remain usable. Native stores lazily build `bm25-ragujuary.json`, reusing unchanged token frequencies and updating edited, added, or removed chunks on the next lexical search. Missing or invalid caches are rebuilt; cache write failures do not prevent searching. Vector-only queries do not access the cache. External indexes build BM25 in memory and are never modified.

Reranking is **enabled by default** using [Gemini Decisions API](https://aistudio.google.com/docs/decisions-api), model `gemini-decision-1`. Disable it with `--rerank=false`. Scoring uses `GEMINI_API_KEY` or `--api-key`, also when embeddings use Ollama or another backend. `--rerank-candidate-k` sets the reranking pool (default 40, maximum 1000, raised to at least Top K). Normal hybrid retrieval uses its independent per-branch candidate limit. Candidates receive relevance scores from 0 (unrelated) to 3 (directly answers the query); fractional scores are supported, ties keep retrieval order, and the original retrieval score is preserved. The final Top K is applied **after** reranking. If credentials are missing, the API fails, scores are incomplete, or scoring times out, the original retrieval's Top K results are returned **with a warning**. No partial reranking is retained. CLI warnings go to stderr; MCP warnings appear in both `warnings` and the text answer. Cancellation of the query remains an error. Each candidate group's scoring has a 20-second deadline, including retries. Oversized requests are split without truncating text. Default reranking sends eligible indexed text to Gemini and adds API latency and input-token usage; use `--rerank=false` for local-only keyword or embedding backends.

Reranking scores text and available extracted PDF text. Images, audio, video, scanned PDFs with only a placeholder label, and empty chunks are **not scored from their filenames or labels**. These candidates keep their original retrieval positions, while scored candidates are reordered only within the remaining positions; their `rerank_skipped_reason` explains why they were skipped. A warning identifies that some candidates were protected. No binary files are read or uploaded for scoring. Vector search continues to support all indexed modalities. FileSearch mode uses managed retrieval and does not expose these local search options.

`--search-until-found` is optional and defaults to OFF. It embeds the query and retrieves candidates **once**, widening retrieval to at most **1000 fused candidates** (up to 1000 per branch before fusion) independently of the normal `--candidate-k`. It scores successive groups sized by `--rerank-candidate-k` (at least Top K), stopping at the first group with relevance **2/3 or above** and returning only those matches from that group, sorted by relevance and limited to Top K. If no group contains a match, the result is empty; reaching the 1000-candidate limit produces a warning. An API failure or an unscorable candidate in a group stops the loop and returns the **original retrieval's Top K with a warning**, since an unscored candidate is not evidence of low relevance. `--search-until-found --rerank=false` is rejected. More candidate groups mean more API calls and input-token usage.

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

# Restrict to specific stores (reduces unnecessary API calls)
ragujuary serve --stores mystore1,mystore2

# Single store mode (store_name becomes optional in all tools)
ragujuary serve --stores mystore
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

The MCP server exposes 8 unified tools. Each tool auto-detects the store type (Embedding or FileSearch) by store name.

##### `upload` - Upload/index a file

Upload a file to a store. Embedding stores index content locally; FileSearch stores upload to Gemini cloud. For multimodal content (image/PDF/video/audio), set `mime_type` and `is_base64=true`.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `store_name` | string | Yes | Name of the store |
| `file_name` | string | Yes | File name or path for the uploaded file |
| `file_content` | string | Yes | File content (plain text or base64 encoded) |
| `is_base64` | boolean | No | Set to true if file_content is base64 encoded |
| `mime_type` | string | No | MIME type for binary content (embedding stores only) |
| `chunk_size` | integer | No | Chunk size in characters (default: 1000, embedding stores only) |
| `chunk_overlap` | integer | No | Chunk overlap in characters (default: 200, embedding stores only) |
| `dimension` | integer | No | Embedding dimensionality (default: 768, embedding stores only) |
| `pdf_max_pages` | integer | No | Max pages per PDF chunk (1-6, default: 6, embedding stores only) |

##### `query` - Query documents

Query documents using natural language. Embedding stores default to hybrid search and Gemini Decision 1 reranking, with independent opt-outs and optional search-until-found. FileSearch stores use Gemini's grounded generation with citations.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `store_name` | string | No* | Name of the store |
| `store_names` | array | No* | Names of multiple stores to query |
| `question` | string | Yes | The question to ask about your documents |
| `model` | string | No | Generation model (default: gemini-4-hydrogen; alternatives: gemini-4-argon, gemini-4-carbon; FileSearch only) |
| `metadata_filter` | string | No | Metadata filter expression (FileSearch only) |
| `show_citations` | boolean | No | Include citation details (FileSearch only) |
| `top_k` | integer | No | Number of top results (default: 5, embedding stores only) |
| `min_score` | number | No | Minimum vector similarity (default: 0.3; explicit 0 supported; does not gate keyword hits) |
| `search_mode` | string | No | `hybrid` (default), `vector`, or `keyword` (embedding stores only) |
| `candidate_k` | integer | No | Candidates per hybrid branch (default: 30, maximum: 1000) |
| `rrf_k` | number | No | RRF rank constant (default: 60) |
| `rerank` | boolean | No | Gemini Decision 1 text reranking (default: true; explicit false disables it) |
| `rerank_candidate_k` | integer | No | Reranking pool (default: 40, maximum: 1000, at least Top K) |
| `search_until_found` | boolean | No | Score successive groups until relevance ≥ 2/3 is found (default: false; requires reranking; max 1000 candidates) |

*Either `store_name` or `store_names` must be provided.

For embedding stores, the output also includes structured `results` with full text, file/page metadata, the retrieval `score`, optional `retrieval_source` and `rerank_score` (0–3), and `rerank_skipped_reason` for protected unscorable candidates. `warnings` records fallback, protected media, and search-limit notices. To disable both defaults, set `"search_mode": "vector", "rerank": false`; omission of `rerank` enables it. These local search options are rejected for FileSearch stores.

##### `list` - List documents in a store

List all documents in a store with optional filtering. Auto-detects store type.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `store_name` | string | Yes | Name of the store to list files from |
| `pattern` | string | No | Regex pattern to filter results |

##### `delete` - Delete a file from a store

Delete a file from a store by file name. Auto-detects store type.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `store_name` | string | Yes | Name of the store |
| `file_name` | string | Yes | File name to delete |

##### `create_store` - Create a new store

Create a new store. Set `type` to choose between embedding and FileSearch.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `store_name` | string | Yes | Display name for the new store |
| `type` | string | No | `embed` for embedding store, `filesearch` (default) for FileSearch store |

##### `delete_store` - Delete a store

Delete an entire store and all its data. Auto-detects store type.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `store_name` | string | Yes | Name of the store to delete |

##### `list_stores` - List all stores

List all available stores (both Embedding and FileSearch).

No parameters required.

##### `upload_directory` - Upload/index files from directories

Upload/index files from directories to a store. Auto-detects store type: embedding stores index locally, FileSearch stores upload to Gemini cloud. Recursively discovers files and skips unchanged files.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `store_name` | string | Yes | Name of the store |
| `directories` | array | Yes | List of directory paths |
| `exclude_patterns` | array | No | Regex patterns to exclude files |
| `parallelism` | integer | No | Number of parallel uploads (default: 5, FileSearch only) |
| `chunk_size` | integer | No | Chunk size in characters (default: 1000, embedding stores only) |
| `chunk_overlap` | integer | No | Chunk overlap in characters (default: 200, embedding stores only) |
| `dimension` | integer | No | Embedding dimensionality (default: 768, embedding stores only) |
| `pdf_max_pages` | integer | No | Max pages per PDF chunk (1-6, default: 6, embedding stores only) |

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
- PDF: up to 6 pages per embedding request (configurable via `--pdf-pages`, larger PDFs are automatically split)
- Video: 120 seconds per request without audio, 80 seconds with audio (longer videos are automatically split using ffmpeg)
- Audio: 80 seconds per request (longer audio files are automatically split using ffmpeg)
- Output dimensions: 128-3,072
- Multimodal embedding requires Gemini backend (not available with OpenAI-compatible backends)
- API errors (503/429) are automatically retried up to 3 times with exponential backoff

## License

MIT
