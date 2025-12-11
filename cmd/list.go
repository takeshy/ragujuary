package cmd

import (
	"fmt"
	"regexp"
	"sort"
	"text/tabwriter"
	"os"

	"github.com/spf13/cobra"
	"github.com/takeshy/ragujuary/internal/store"
)

var (
	listPattern string
	listLong    bool
	listStores  bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List files in a store",
	Long: `List all files in a Gemini store.
Optionally filter by a regex pattern.`,
	RunE: runList,
}

func init() {
	listCmd.Flags().StringVarP(&listPattern, "pattern", "P", "", "Regex pattern to filter files")
	listCmd.Flags().BoolVarP(&listLong, "long", "l", false, "Show detailed information")
	listCmd.Flags().BoolVar(&listStores, "stores", false, "List all stores instead of files")
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	// Initialize store manager
	storeManager, err := store.NewManager(dataFile)
	if err != nil {
		return fmt.Errorf("failed to initialize store manager: %w", err)
	}

	// List stores mode
	if listStores {
		stores := storeManager.ListStores()
		if len(stores) == 0 {
			fmt.Println("No stores found")
			return nil
		}

		fmt.Println("Stores:")
		for _, name := range stores {
			st, _ := storeManager.GetStore(name)
			fmt.Printf("  %s (%d files)\n", name, len(st.Files))
		}
		return nil
	}

	// Check if store exists
	st, exists := storeManager.GetStore(storeName)
	if !exists {
		return fmt.Errorf("store '%s' not found", storeName)
	}

	// Get all files
	files := storeManager.GetAllFiles(storeName)
	if len(files) == 0 {
		fmt.Printf("No files in store '%s'\n", storeName)
		return nil
	}

	// Filter by pattern if provided
	if listPattern != "" {
		re, err := regexp.Compile(listPattern)
		if err != nil {
			return fmt.Errorf("invalid pattern: %w", err)
		}

		var filtered []store.FileMetadata
		for _, f := range files {
			if re.MatchString(f.LocalPath) {
				filtered = append(filtered, f)
			}
		}
		files = filtered
	}

	if len(files) == 0 {
		fmt.Printf("No files matching pattern '%s' in store '%s'\n", listPattern, storeName)
		return nil
	}

	// Sort by path
	sort.Slice(files, func(i, j int) bool {
		return files[i].LocalPath < files[j].LocalPath
	})

	// Print files
	fmt.Printf("Files in store '%s' (%d total):\n\n", storeName, len(files))

	if listLong {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "PATH\tSIZE\tCHECKSUM\tUPLOADED\tREMOTE ID")
		fmt.Fprintln(w, "----\t----\t--------\t--------\t---------")
		for _, f := range files {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				f.LocalPath,
				formatSize(f.Size),
				f.Checksum[:12]+"...",
				f.UploadedAt.Format("2006-01-02 15:04:05"),
				f.RemoteID,
			)
		}
		w.Flush()
	} else {
		for _, f := range files {
			fmt.Printf("  %s\n", f.LocalPath)
		}
	}

	// Print store info
	fmt.Printf("\nStore created: %s\n", st.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Last updated: %s\n", st.UpdatedAt.Format("2006-01-02 15:04:05"))

	return nil
}

func formatSize(size int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case size >= GB:
		return fmt.Sprintf("%.2f GB", float64(size)/GB)
	case size >= MB:
		return fmt.Sprintf("%.2f MB", float64(size)/MB)
	case size >= KB:
		return fmt.Sprintf("%.2f KB", float64(size)/KB)
	default:
		return fmt.Sprintf("%d B", size)
	}
}
