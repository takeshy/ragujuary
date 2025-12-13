package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takeshy/ragujuary/internal/fileutil"
	"github.com/takeshy/ragujuary/internal/gemini"
	"github.com/takeshy/ragujuary/internal/store"
)

var (
	excludePatterns []string
	dryRun          bool
	createStore     bool
)

var uploadCmd = &cobra.Command{
	Use:   "upload [directories...]",
	Short: "Upload files to a File Search Store",
	Long: `Upload files from specified directories to a Gemini File Search Store.
Files matching exclude patterns will be skipped.
Files with unchanged checksums will not be re-uploaded.

The File Search Store must exist, or use --create to create it automatically.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runUpload,
}

func init() {
	uploadCmd.Flags().StringArrayVarP(&excludePatterns, "exclude", "e", nil, "Regex patterns to exclude files (can be specified multiple times)")
	uploadCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be uploaded without actually uploading")
	uploadCmd.Flags().BoolVar(&createStore, "create", false, "Create the File Search Store if it doesn't exist")
	rootCmd.AddCommand(uploadCmd)
}

func runUpload(cmd *cobra.Command, args []string) error {
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

	// Check if File Search Store exists on Gemini (supports both API name and display name)
	resolvedName, remoteStore, err := client.ResolveStoreName(storeName)
	if err != nil {
		if createStore {
			fmt.Printf("Creating File Search Store '%s'...\n", storeName)
			remoteStore, err = client.CreateFileSearchStore(storeName)
			if err != nil {
				return fmt.Errorf("failed to create File Search Store: %w", err)
			}
			resolvedName = strings.TrimPrefix(remoteStore.Name, "fileSearchStores/")
			fmt.Printf("Created File Search Store: %s\n", remoteStore.Name)
		} else {
			return fmt.Errorf("File Search Store '%s' not found. Use --create to create it, or create it manually first", storeName)
		}
	}
	_ = resolvedName // Used for local store name below

	// Ensure local store exists with the remote store name
	remoteStoreName := remoteStore.Name
	// Strip prefix for local storage key
	localStoreName := strings.TrimPrefix(remoteStoreName, "fileSearchStores/")
	storeManager.GetOrCreateStore(localStoreName)

	// Discover files
	fmt.Printf("Discovering files in directories: %s\n", strings.Join(args, ", "))
	if len(excludePatterns) > 0 {
		fmt.Printf("Excluding patterns: %s\n", strings.Join(excludePatterns, ", "))
	}

	files, err := fileutil.DiscoverFiles(args, excludePatterns)
	if err != nil {
		return fmt.Errorf("failed to discover files: %w", err)
	}

	fmt.Printf("Found %d files\n\n", len(files))

	if len(files) == 0 {
		fmt.Println("No files to upload")
		return nil
	}

	if dryRun {
		fmt.Println("Dry run mode - files that would be uploaded:")
		for _, f := range files {
			fmt.Printf("  %s (%d bytes)\n", f.Path, f.Size)
		}
		return nil
	}

	// Create uploader
	uploader := gemini.NewUploader(client, storeManager, localStoreName, parallelism)

	// Upload files with progress
	var uploaded, skipped, failed int
	fmt.Printf("Uploading files to File Search Store '%s' (parallelism: %d)...\n\n", localStoreName, parallelism)

	results := uploader.UploadFiles(files, func(result gemini.UploadResult) {
		if result.Error != nil {
			failed++
			fmt.Fprintf(os.Stderr, "✗ %s: %v\n", result.FileInfo.Path, result.Error)
		} else if result.Skipped {
			skipped++
			fmt.Printf("⊘ %s: %s\n", result.FileInfo.Path, result.Reason)
		} else {
			uploaded++
			fmt.Printf("✓ %s\n", result.FileInfo.Path)
		}
	})

	// Save store data
	if err := storeManager.Save(); err != nil {
		return fmt.Errorf("failed to save store data: %w", err)
	}

	// Print summary
	fmt.Printf("\nUpload complete:\n")
	fmt.Printf("  Uploaded: %d\n", uploaded)
	fmt.Printf("  Skipped:  %d\n", skipped)
	fmt.Printf("  Failed:   %d\n", failed)

	// Return error if any uploads failed
	for _, r := range results {
		if r.Error != nil {
			return fmt.Errorf("some uploads failed")
		}
	}

	return nil
}
