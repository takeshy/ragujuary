# ragujuary

Gemini File Search CLI tool for managing files in Google Gemini.

## Features

- Upload files from multiple directories to named stores
- Exclude files using regex patterns
- Parallel uploads (default 5 workers)
- Checksum-based deduplication (skip unchanged files)
- Delete files by regex pattern
- List uploaded files with filtering
- Sync local metadata with remote state
- Clean up files that no longer exist locally

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

## Usage

### Upload files

Upload files from directories to a store:

```bash
# Upload from a single directory
ragujuary upload ./docs

# Upload from multiple directories
ragujuary upload ./docs ./src ./config

# Upload to a named store
ragujuary upload -s mystore ./docs

# Exclude files matching patterns
ragujuary upload -e '\.git' -e 'node_modules' -e '\.test\.go$' ./project

# Set parallelism
ragujuary upload -p 10 ./large-project

# Dry run (see what would be uploaded)
ragujuary upload --dry-run ./docs
```

### List files

List files in a store:

```bash
# List all files in default store
ragujuary list

# List all stores
ragujuary list --stores

# List files in a named store
ragujuary list -s mystore

# Filter by pattern
ragujuary list -P '\.go$'

# Show detailed information
ragujuary list -l
```

### Delete files

Delete files matching a pattern:

```bash
# Delete files matching pattern
ragujuary delete -P '\.tmp$'

# Force delete without confirmation
ragujuary delete -P '\.log$' -f

# Delete from specific store
ragujuary delete -s mystore -P 'old/'
```

### Status

Check status of files (modified, unchanged, missing):

```bash
ragujuary status
ragujuary status -s mystore
```

### Sync

Sync local metadata with remote state:

```bash
ragujuary sync
```

### Clean

Remove remote files that no longer exist locally:

```bash
ragujuary clean
ragujuary clean -f  # force without confirmation
```

## Data Storage

File metadata is stored in `~/.ragujuary.json` by default. Use `--data-file` to specify a different location.

Each store tracks:
- Local file path
- Remote file ID
- SHA256 checksum
- File size
- Upload timestamp
- MIME type

## Global Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--api-key` | `-k` | Gemini API key | `$GEMINI_API_KEY` |
| `--store` | `-s` | Store name | `default` |
| `--data-file` | `-d` | Path to data file | `~/.ragujuary.json` |
| `--parallelism` | `-p` | Number of parallel uploads | `5` |

## License

MIT
