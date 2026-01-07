package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takeshy/ragujuary/internal/fileutil"
	"github.com/takeshy/ragujuary/internal/gemini"
	"github.com/takeshy/ragujuary/internal/store"
)

var (
	deletePattern string
	forceDelete   bool
	deleteStore   bool
	deleteIDs     []string
)

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete files or a store",
	Long: `Delete files from a Gemini File Search Store, or delete the entire store.

Use --pattern to delete files matching a regex pattern.
Use --id to delete specific documents by their remote ID.
Use --all to delete the entire File Search Store.
Use --force to skip confirmation prompts.

Examples:
  # Delete files matching pattern
  ragujuary delete -s mystore -P '\.tmp$'

  # Delete specific documents by ID
  ragujuary delete -s mystore --id fileSearchStores/xxx/documents/yyy
  ragujuary delete -s mystore --id doc-id-1 --id doc-id-2

  # Delete entire store
  ragujuary delete -s mystore --all`,
	RunE: runDelete,
}

func init() {
	deleteCmd.Flags().StringVarP(&deletePattern, "pattern", "P", "", "Regex pattern to match files for deletion")
	deleteCmd.Flags().StringArrayVar(&deleteIDs, "id", nil, "Document ID(s) to delete (can be specified multiple times)")
	deleteCmd.Flags().BoolVarP(&forceDelete, "force", "f", false, "Force deletion without confirmation")
	deleteCmd.Flags().BoolVar(&deleteStore, "all", false, "Delete the entire File Search Store")
	rootCmd.AddCommand(deleteCmd)
}

func runDelete(cmd *cobra.Command, args []string) error {
	key, err := getAPIKey()
	if err != nil {
		return err
	}

	client := gemini.NewClient(key)

	// Delete entire store
	if deleteStore {
		return deleteEntireStore(client)
	}

	// Delete by document IDs
	if len(deleteIDs) > 0 {
		return deleteByIDs(client)
	}

	// Delete files matching pattern
	if deletePattern == "" {
		return fmt.Errorf("please specify --pattern, --id, or --all")
	}

	return deleteFilesByPattern(client)
}

func deleteByIDs(client *gemini.Client) error {
	// Initialize store manager
	storeManager, err := store.NewManager(dataFile)
	if err != nil {
		return fmt.Errorf("failed to initialize store manager: %w", err)
	}

	// Resolve store name to get the correct store key
	resolvedName, _, err := client.ResolveStoreName(storeName)
	if err != nil {
		// If store doesn't exist remotely, still try to delete by ID
		resolvedName = storeName
	}

	// Build a map of RemoteID -> LocalPath for cache lookup
	remoteIDToPath := make(map[string]string)
	for _, f := range storeManager.GetAllFiles(resolvedName) {
		remoteIDToPath[f.RemoteID] = f.LocalPath
	}

	// Expand short IDs to full document paths
	fullIDs := make([]string, len(deleteIDs))
	for i, id := range deleteIDs {
		if strings.HasPrefix(id, "fileSearchStores/") {
			// Already a full path
			fullIDs[i] = id
		} else {
			// Short ID - construct full path
			fullIDs[i] = fmt.Sprintf("%s/documents/%s", resolvedName, id)
		}
	}

	// Show documents to be deleted
	fmt.Printf("Documents to be deleted:\n")
	for _, id := range fullIDs {
		fmt.Printf("  %s\n", id)
	}
	fmt.Printf("\nTotal: %d documents\n", len(fullIDs))

	// Confirm deletion
	if !forceDelete {
		fmt.Print("\nAre you sure you want to delete these documents? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Deletion cancelled")
			return nil
		}
	}

	fmt.Println("\nDeleting documents...")

	var deleted, failed int
	for _, id := range fullIDs {
		if err := client.DeleteDocument(id); err != nil {
			fmt.Printf("  ✗ %s: %v\n", id, err)
			failed++
		} else {
			fmt.Printf("  ✓ %s\n", id)
			deleted++

			// Remove from local cache if exists
			if localPath, exists := remoteIDToPath[id]; exists {
				storeManager.RemoveFile(resolvedName, localPath)
				fmt.Printf("      (removed from local cache: %s)\n", localPath)
			}
		}
	}

	// Save store data
	if err := storeManager.Save(); err != nil {
		return fmt.Errorf("failed to save store data: %w", err)
	}

	fmt.Printf("\nDeleted: %d, Failed: %d\n", deleted, failed)

	if failed > 0 {
		return fmt.Errorf("some deletions failed")
	}

	return nil
}

func deleteEntireStore(client *gemini.Client) error {
	// Resolve store name (supports both API name and display name)
	resolvedName, remoteStore, err := client.ResolveStoreName(storeName)
	if err != nil {
		return fmt.Errorf("File Search Store '%s' not found: %w", storeName, err)
	}

	if !forceDelete {
		fmt.Printf("Are you sure you want to delete File Search Store '%s' (%s) and all its documents? [y/N]: ", remoteStore.DisplayName, resolvedName)
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Delete cancelled")
			return nil
		}
	}

	fmt.Printf("Deleting File Search Store '%s' (%s)...\n", remoteStore.DisplayName, resolvedName)
	if err := client.DeleteFileSearchStore(resolvedName, true); err != nil {
		return fmt.Errorf("failed to delete store: %w", err)
	}

	// Remove from local store
	storeManager, err := store.NewManager(dataFile)
	if err == nil {
		storeManager.DeleteStore(resolvedName)
		storeManager.Save()
	}

	fmt.Println("File Search Store deleted successfully")
	return nil
}

func deleteFilesByPattern(client *gemini.Client) error {
	// Initialize store manager
	storeManager, err := store.NewManager(dataFile)
	if err != nil {
		return fmt.Errorf("failed to initialize store manager: %w", err)
	}

	// Check if store exists
	_, exists := storeManager.GetStore(storeName)
	if !exists {
		return fmt.Errorf("store '%s' not found in local cache", storeName)
	}

	// Get files matching pattern
	files := storeManager.GetAllFiles(storeName)
	if len(files) == 0 {
		fmt.Printf("No files in store '%s'\n", storeName)
		return nil
	}

	// Convert to FileInfo for filtering
	fileInfos := make([]fileutil.FileInfo, len(files))
	for i, f := range files {
		fileInfos[i] = fileutil.FileInfo{Path: f.LocalPath}
	}

	// Filter by pattern
	matched, err := fileutil.FilterFilesByPattern(fileInfos, deletePattern)
	if err != nil {
		return fmt.Errorf("invalid pattern: %w", err)
	}

	if len(matched) == 0 {
		fmt.Printf("No files matching pattern '%s' in store '%s'\n", deletePattern, storeName)
		return nil
	}

	// Show files to be deleted
	fmt.Printf("Files to be deleted from store '%s':\n", storeName)
	for _, f := range matched {
		fmt.Printf("  %s\n", f.Path)
	}
	fmt.Printf("\nTotal: %d files\n", len(matched))

	// Confirm deletion
	if !forceDelete {
		fmt.Print("\nAre you sure you want to delete these files? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Deletion cancelled")
			return nil
		}
	}

	// Create uploader for deletion
	uploader := gemini.NewUploader(client, storeManager, storeName, parallelism)

	// Delete files
	fmt.Println("\nDeleting files...")
	deleted, errors := uploader.DeleteFilesByPattern(deletePattern)

	// Save store data
	if err := storeManager.Save(); err != nil {
		return fmt.Errorf("failed to save store data: %w", err)
	}

	// Print results
	fmt.Printf("\nDeleted %d files\n", len(deleted))
	for _, f := range deleted {
		fmt.Printf("  ✓ %s\n", f.LocalPath)
	}

	if len(errors) > 0 {
		fmt.Printf("\nErrors (%d):\n", len(errors))
		for _, err := range errors {
			fmt.Fprintf(os.Stderr, "  ✗ %v\n", err)
		}
		return fmt.Errorf("some deletions failed")
	}

	return nil
}
