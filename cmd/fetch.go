package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/takeshy/ragujuary/internal/fileutil"
	"github.com/takeshy/ragujuary/internal/gemini"
	"github.com/takeshy/ragujuary/internal/store"
)

var (
	fetchForce bool
)

var fetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Fetch remote document metadata to local cache",
	Long: `Fetch document metadata from Gemini File Search Store to local cache.

This command fetches all documents from the remote store and updates the local
.ragujuary.json file with their metadata (including checksums from customMetadata).

For each document:
- If local file exists and checksum matches remote: update local cache
- If local file exists but checksum differs: show warning (use --force to override)
- If local file doesn't exist: update local cache with warning

This is useful for:
- Syncing state across multiple machines
- Importing documents uploaded via MCP into CLI's local cache
- Recovering local cache after deletion`,
	RunE: runFetch,
}

func init() {
	fetchCmd.Flags().BoolVarP(&fetchForce, "force", "f", false, "Force update even if local file checksum differs")
	rootCmd.AddCommand(fetchCmd)
}

func runFetch(cmd *cobra.Command, args []string) error {
	key, err := getAPIKey()
	if err != nil {
		return err
	}

	// Initialize store manager
	storeManager, err := store.NewManager(dataFile)
	if err != nil {
		return fmt.Errorf("failed to initialize store manager: %w", err)
	}

	// Create client
	client := gemini.NewClient(key)

	// Resolve store name
	resolvedName, remoteStore, err := client.ResolveStoreName(storeName)
	if err != nil {
		return fmt.Errorf("store '%s' not found: %w", storeName, err)
	}

	fmt.Printf("Fetching from File Search Store '%s' (%s)...\n\n", remoteStore.DisplayName, resolvedName)

	// Fetch all documents from remote
	docs, err := client.ListAllDocuments(resolvedName)
	if err != nil {
		return fmt.Errorf("failed to list documents: %w", err)
	}

	if len(docs) == 0 {
		fmt.Println("No documents found in remote store")
		return nil
	}

	// Ensure local store exists
	storeManager.GetOrCreateStore(resolvedName)

	// Get existing local files for comparison
	existingFiles := make(map[string]store.FileMetadata)
	for _, f := range storeManager.GetAllFiles(resolvedName) {
		existingFiles[f.LocalPath] = f
	}

	var added, updated, unchanged, skipped, notFound, needsUpload int

	for _, doc := range docs {
		// Extract checksum from customMetadata
		remoteChecksum := gemini.GetDocumentChecksum(&doc)

		// Parse size
		var size int64
		if doc.SizeBytes != "" {
			fmt.Sscanf(doc.SizeBytes, "%d", &size)
		}

		// Parse upload time
		uploadedAt := time.Now()
		if doc.CreateTime != "" {
			if t, err := time.Parse(time.RFC3339, doc.CreateTime); err == nil {
				uploadedAt = t
			}
		}

		// Create metadata
		meta := store.FileMetadata{
			LocalPath:  doc.DisplayName,
			RemoteID:   doc.Name,
			RemoteName: doc.DisplayName,
			Checksum:   remoteChecksum,
			Size:       size,
			UploadedAt: uploadedAt,
			MimeType:   doc.MimeType,
		}

		// Check if already in local cache with same data
		existing, existsInCache := existingFiles[doc.DisplayName]
		if existsInCache && existing.Checksum == remoteChecksum && existing.RemoteID == doc.Name {
			unchanged++
			continue
		}

		// Check if local file exists on disk
		localChecksum, err := fileutil.CalculateChecksum(doc.DisplayName)
		if err != nil {
			// File doesn't exist on disk
			fmt.Printf("⚠ %s: file not found on disk, updating cache\n", doc.DisplayName)
			storeManager.AddFile(resolvedName, meta)
			notFound++
			continue
		}

		// File exists, compare checksums
		if remoteChecksum == "" {
			// Remote has no checksum (uploaded with old version)
			meta.Checksum = localChecksum
			if existsInCache {
				if existing.Checksum == localChecksum {
					// Cache and local file match, just update RemoteID if needed
					if existing.RemoteID != doc.Name {
						fmt.Printf("↻ %s: updated remote ID\n", doc.DisplayName)
						storeManager.AddFile(resolvedName, meta)
						updated++
					} else {
						unchanged++
					}
				} else {
					// Cache and local file differ - local file was modified, needs upload
					fmt.Printf("⚠ %s: needs upload (local file changed, remote has no checksum)\n", doc.DisplayName)
					fmt.Printf("    cache: %s\n", existing.Checksum)
					fmt.Printf("    file:  %s\n", localChecksum)
					needsUpload++
				}
			} else {
				fmt.Printf("+ %s: added (remote has no checksum)\n", doc.DisplayName)
				storeManager.AddFile(resolvedName, meta)
				added++
			}
			continue
		}

		if localChecksum == remoteChecksum {
			// Checksums match, safe to update
			if existsInCache {
				fmt.Printf("↻ %s: updated\n", doc.DisplayName)
				updated++
			} else {
				fmt.Printf("+ %s: added\n", doc.DisplayName)
				added++
			}
			storeManager.AddFile(resolvedName, meta)
		} else {
			// Checksums differ
			if fetchForce {
				fmt.Printf("⚠ %s: checksum mismatch, force updating\n", doc.DisplayName)
				fmt.Printf("    local:  %s\n", localChecksum)
				fmt.Printf("    remote: %s\n", remoteChecksum)
				storeManager.AddFile(resolvedName, meta)
				updated++
			} else {
				fmt.Printf("✗ %s: checksum mismatch, skipped (use --force to override)\n", doc.DisplayName)
				fmt.Printf("    local:  %s\n", localChecksum)
				fmt.Printf("    remote: %s\n", remoteChecksum)
				skipped++
			}
		}
	}

	// Save store data
	if err := storeManager.Save(); err != nil {
		return fmt.Errorf("failed to save store data: %w", err)
	}

	// Print summary
	fmt.Printf("\nFetch complete:\n")
	fmt.Printf("  Added:     %d\n", added)
	fmt.Printf("  Updated:   %d\n", updated)
	fmt.Printf("  Unchanged: %d\n", unchanged)
	fmt.Printf("  Not found: %d\n", notFound)
	if needsUpload > 0 {
		fmt.Printf("  Needs upload: %d (local file changed)\n", needsUpload)
	}
	if skipped > 0 {
		fmt.Printf("  Skipped:   %d (checksum mismatch)\n", skipped)
	}
	fmt.Printf("  Total:     %d\n", len(docs))

	return nil
}
