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
)

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete files or a store",
	Long: `Delete files from a Gemini File Search Store, or delete the entire store.

Use --pattern to delete files matching a regex pattern.
Use --store to delete the entire File Search Store.
Use --force to skip confirmation prompts.`,
	RunE: runDelete,
}

func init() {
	deleteCmd.Flags().StringVarP(&deletePattern, "pattern", "P", "", "Regex pattern to match files for deletion")
	deleteCmd.Flags().BoolVarP(&forceDelete, "force", "f", false, "Force deletion without confirmation")
	deleteCmd.Flags().BoolVar(&deleteStore, "store", false, "Delete the entire File Search Store")
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

	// Delete files matching pattern
	if deletePattern == "" {
		return fmt.Errorf("please specify --pattern to delete files or --store to delete the entire store")
	}

	return deleteFilesByPattern(client)
}

func deleteEntireStore(client *gemini.Client) error {
	// Check if store exists
	_, err := client.GetFileSearchStore(storeName)
	if err != nil {
		return fmt.Errorf("File Search Store '%s' not found: %w", storeName, err)
	}

	if !forceDelete {
		fmt.Printf("Are you sure you want to delete File Search Store '%s' and all its documents? [y/N]: ", storeName)
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Delete cancelled")
			return nil
		}
	}

	fmt.Printf("Deleting File Search Store '%s'...\n", storeName)
	if err := client.DeleteFileSearchStore(storeName, true); err != nil {
		return fmt.Errorf("failed to delete store: %w", err)
	}

	// Remove from local store
	storeManager, err := store.NewManager(dataFile)
	if err == nil {
		storeManager.DeleteStore(storeName)
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
