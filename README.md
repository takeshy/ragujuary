# ragujuary

A CLI tool for managing Gemini File Search Stores - Google's fully managed RAG (Retrieval-Augmented Generation) system.

## Features

- Create and manage Gemini File Search Stores
- Upload files from multiple directories with automatic chunking and embedding
- Query your documents using natural language (RAG)
- Parallel uploads (default 5 workers)
- Checksum-based deduplication (skip unchanged files)
- Delete files or entire stores
- List uploaded documents with filtering
- Sync local metadata with remote state
- Built-in citations for verifiable responses

## What is Gemini File Search?

Gemini File Search is a fully managed RAG system built into the Gemini API. Unlike the basic File API (which expires files after 48 hours), File Search Stores:

- Store documents indefinitely until manually deleted
- Automatically chunk and create embeddings for your documents
- Provide semantic search over your content
- Support a wide range of formats (PDF, DOCX, TXT, JSON, code files, etc.)
- Include citations in responses for verification

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

### Create a store and upload files

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

### Query your documents (RAG)

```bash
# Basic query
ragujuary query -s mystore "What are the main features?"

# Use a different model (default: gemini-3-pro-preview)
ragujuary query -s mystore -m gemini-2.5-flash "Explain the architecture"

# Show citation details
ragujuary query -s mystore --citations "How does authentication work?"
```

### List stores and files

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

### Delete files or stores

```bash
# Delete files matching pattern
ragujuary delete -s mystore -P '\.tmp$'

# Force delete without confirmation
ragujuary delete -s mystore -P '\.log$' -f

# Delete an entire store
ragujuary delete -s mystore --all

# Force delete store without confirmation
ragujuary delete -s mystore --all -f
```

### Status

Check status of files (modified, unchanged, missing):

```bash
ragujuary status -s mystore
```

### Sync

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

### Clean

Remove remote documents that no longer exist locally:

```bash
ragujuary clean -s mystore
ragujuary clean -s mystore -f  # force without confirmation
```

## Data Storage

File metadata is stored in `~/.ragujuary.json` by default. Use `--data-file` to specify a different location.

Each store tracks:
- Local file path
- Remote document ID
- SHA256 checksum
- File size
- Upload timestamp
- MIME type

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

- Max file size: 100 MB per file
- Storage: 1 GB (Free tier) to 1 TB (Tier 3)
- Max stores per project: 10

## License

MIT
