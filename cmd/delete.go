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
)

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete files from a store",
	Long: `Delete files from a Gemini store matching a regex pattern.
The pattern is matched against the local file path.`,
	RunE: runDelete,
}

func init() {
	deleteCmd.Flags().StringVarP(&deletePattern, "pattern", "P", "", "Regex pattern to match files for deletion (required)")
	deleteCmd.Flags().BoolVarP(&forceDelete, "force", "f", false, "Force deletion without confirmation")
	deleteCmd.MarkFlagRequired("pattern")
	rootCmd.AddCommand(deleteCmd)
}

func runDelete(cmd *cobra.Command, args []string) error {
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
	st, exists := storeManager.GetStore(storeName)
	if !exists {
		return fmt.Errorf("store '%s' not found", storeName)
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

	// Create client
	client := gemini.NewClient(key)
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

	// Update store info
	_ = st // Silence unused variable warning

	return nil
}
