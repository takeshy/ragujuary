package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/takeshy/ragujuary/internal/fileutil"
	"github.com/takeshy/ragujuary/internal/gemini"
	"github.com/takeshy/ragujuary/internal/store"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync store with remote",
	Long: `Sync the local store metadata with the remote Gemini File Search Store.
This will update local entries to match the remote state.`,
	RunE: runSync,
}

func init() {
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
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

	// Get remote store
	remoteStore, err := client.GetFileSearchStore(storeName)
	if err != nil {
		return fmt.Errorf("File Search Store '%s' not found: %w", storeName, err)
	}

	fmt.Printf("Syncing with File Search Store '%s'...\n", remoteStore.DisplayName)

	// Ensure local store exists
	storeManager.GetOrCreateStore(storeName)

	// Get all remote documents
	fmt.Println("Fetching remote documents...")
	remoteDocs, err := client.ListAllDocuments(storeName)
	if err != nil {
		return fmt.Errorf("failed to list remote documents: %w", err)
	}

	// Create map of remote documents by display name
	remoteByDisplayName := make(map[string]gemini.FileSearchDocument)
	remoteByName := make(map[string]gemini.FileSearchDocument)
	for _, d := range remoteDocs {
		remoteByDisplayName[d.DisplayName] = d
		remoteByName[d.Name] = d
	}

	// Get local files
	localFiles := storeManager.GetAllFiles(storeName)

	var removed int
	var updated int
	var orphaned int

	// Check for files that exist locally but not remotely
	for _, f := range localFiles {
		// Check by display name first
		if _, exists := remoteByDisplayName[f.LocalPath]; !exists {
			// Also check by remote ID
			if _, exists := remoteByName[f.RemoteID]; !exists {
				fmt.Printf("  Removing orphaned entry: %s\n", f.LocalPath)
				storeManager.RemoveFile(storeName, f.LocalPath)
				removed++
			}
		}
	}

	// Update local entries with remote document names
	for _, f := range localFiles {
		if doc, exists := remoteByDisplayName[f.LocalPath]; exists {
			if f.RemoteID != doc.Name {
				f.RemoteID = doc.Name
				storeManager.AddFile(storeName, f)
				updated++
			}
		}
	}

	// Check for files that exist locally but have been deleted from disk
	localFiles = storeManager.GetAllFiles(storeName) // Refresh after updates
	for _, f := range localFiles {
		if _, err := os.Stat(f.LocalPath); os.IsNotExist(err) {
			fmt.Printf("  Local file missing: %s\n", f.LocalPath)
			orphaned++
		}
	}

	// Save store data
	if err := storeManager.Save(); err != nil {
		return fmt.Errorf("failed to save store data: %w", err)
	}

	fmt.Printf("\nSync complete:\n")
	fmt.Printf("  Remote documents: %d\n", len(remoteDocs))
	fmt.Printf("  Local entries: %d\n", len(storeManager.GetAllFiles(storeName)))
	fmt.Printf("  Removed orphaned entries: %d\n", removed)
	fmt.Printf("  Updated entries: %d\n", updated)
	fmt.Printf("  Missing local files: %d\n", orphaned)

	return nil
}

// cleanCmd removes files from remote that are in the store but missing locally
var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean up files that no longer exist locally",
	Long: `Remove documents from remote File Search Store that exist in the store but are missing locally.
This is useful when you've deleted local files and want to clean up the remote.`,
	RunE: runClean,
}

func init() {
	rootCmd.AddCommand(cleanCmd)
}

func runClean(cmd *cobra.Command, args []string) error {
	key, err := getAPIKey()
	if err != nil {
		return err
	}

	// Initialize store manager
	storeManager, err := store.NewManager(dataFile)
	if err != nil {
		return fmt.Errorf("failed to initialize store manager: %w", err)
	}

	// Check if store exists
	_, exists := storeManager.GetStore(storeName)
	if !exists {
		return fmt.Errorf("store '%s' not found", storeName)
	}

	// Create client
	client := gemini.NewClient(key)

	// Get local files
	localFiles := storeManager.GetAllFiles(storeName)

	var toDelete []store.FileMetadata

	// Check for files that exist in store but are missing locally
	for _, f := range localFiles {
		if _, err := os.Stat(f.LocalPath); os.IsNotExist(err) {
			toDelete = append(toDelete, f)
		}
	}

	if len(toDelete) == 0 {
		fmt.Println("No files to clean up")
		return nil
	}

	fmt.Printf("Found %d files to clean up:\n", len(toDelete))
	for _, f := range toDelete {
		fmt.Printf("  %s\n", f.LocalPath)
	}

	if !forceDelete {
		fmt.Print("\nDelete these documents from remote? [y/N]: ")
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "yes" {
			fmt.Println("Clean cancelled")
			return nil
		}
	}

	var deleted, failed int
	for _, f := range toDelete {
		if err := client.DeleteDocument(f.RemoteID); err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ Failed to delete %s: %v\n", f.LocalPath, err)
			failed++
			continue
		}
		storeManager.RemoveFile(storeName, f.LocalPath)
		fmt.Printf("  ✓ Deleted %s\n", f.LocalPath)
		deleted++
	}

	// Save store data
	if err := storeManager.Save(); err != nil {
		return fmt.Errorf("failed to save store data: %w", err)
	}

	fmt.Printf("\nClean complete:\n")
	fmt.Printf("  Deleted: %d\n", deleted)
	fmt.Printf("  Failed: %d\n", failed)

	return nil
}

// statusCmd shows the status of files comparing local and checksum
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of files in a store",
	Long: `Show the status of files in a store, comparing local files with stored checksums.
This helps identify files that have been modified since upload.`,
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	// Initialize store manager
	storeManager, err := store.NewManager(dataFile)
	if err != nil {
		return fmt.Errorf("failed to initialize store manager: %w", err)
	}

	// Check if store exists
	_, exists := storeManager.GetStore(storeName)
	if !exists {
		return fmt.Errorf("store '%s' not found", storeName)
	}

	// Get local files
	files := storeManager.GetAllFiles(storeName)
	if len(files) == 0 {
		fmt.Printf("No files in store '%s'\n", storeName)
		return nil
	}

	var unchanged, modified, missing int

	fmt.Printf("Status of files in store '%s':\n\n", storeName)

	for _, f := range files {
		// Check if file exists
		if _, err := os.Stat(f.LocalPath); os.IsNotExist(err) {
			fmt.Printf("  ✗ [MISSING] %s\n", f.LocalPath)
			missing++
			continue
		}

		// Calculate current checksum
		currentChecksum, err := fileutil.CalculateChecksum(f.LocalPath)
		if err != nil {
			fmt.Printf("  ? [ERROR] %s: %v\n", f.LocalPath, err)
			continue
		}

		if currentChecksum != f.Checksum {
			fmt.Printf("  ~ [MODIFIED] %s\n", f.LocalPath)
			modified++
		} else {
			fmt.Printf("  ✓ [UNCHANGED] %s\n", f.LocalPath)
			unchanged++
		}
	}

	fmt.Printf("\nSummary:\n")
	fmt.Printf("  Unchanged: %d\n", unchanged)
	fmt.Printf("  Modified:  %d\n", modified)
	fmt.Printf("  Missing:   %d\n", missing)
	fmt.Printf("  Total:     %d\n", len(files))

	return nil
}
