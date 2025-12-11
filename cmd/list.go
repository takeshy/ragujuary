package cmd

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/takeshy/ragujuary/internal/gemini"
	"github.com/takeshy/ragujuary/internal/store"
)

var (
	listPattern string
	listLong    bool
	listStores  bool
	listRemote  bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List files in a store or list all stores",
	Long: `List all files in a Gemini File Search Store, or list all stores.
Optionally filter by a regex pattern.

Use --stores to list all File Search Stores.
Use --remote to fetch from Gemini API instead of local cache.`,
	RunE: runList,
}

func init() {
	listCmd.Flags().StringVarP(&listPattern, "pattern", "P", "", "Regex pattern to filter files")
	listCmd.Flags().BoolVarP(&listLong, "long", "l", false, "Show detailed information")
	listCmd.Flags().BoolVar(&listStores, "stores", false, "List all File Search Stores instead of files")
	listCmd.Flags().BoolVar(&listRemote, "remote", false, "Fetch from Gemini API instead of local cache")
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	// List stores mode
	if listStores {
		return listAllStores()
	}

	// List files in a store
	if listRemote {
		return listFilesRemote()
	}
	return listFilesLocal()
}

func listAllStores() error {
	key, err := getAPIKey()
	if err != nil {
		return err
	}

	client := gemini.NewClient(key)

	fmt.Println("Fetching File Search Stores from Gemini...")
	stores, err := client.ListAllFileSearchStores()
	if err != nil {
		return fmt.Errorf("failed to list stores: %w", err)
	}

	if len(stores) == 0 {
		fmt.Println("No File Search Stores found")
		return nil
	}

	fmt.Printf("\nFile Search Stores (%d):\n\n", len(stores))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tDISPLAY NAME\tCREATED\tUPDATED")
	fmt.Fprintln(w, "----\t------------\t-------\t-------")
	for _, s := range stores {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			s.Name,
			s.DisplayName,
			s.CreateTime,
			s.UpdateTime,
		)
	}
	w.Flush()

	return nil
}

func listFilesRemote() error {
	key, err := getAPIKey()
	if err != nil {
		return err
	}

	client := gemini.NewClient(key)

	fmt.Printf("Fetching documents from File Search Store '%s'...\n\n", storeName)
	docs, err := client.ListAllDocuments(storeName)
	if err != nil {
		return fmt.Errorf("failed to list documents: %w", err)
	}

	if len(docs) == 0 {
		fmt.Printf("No documents in File Search Store '%s'\n", storeName)
		return nil
	}

	// Filter by pattern if provided
	if listPattern != "" {
		re, err := regexp.Compile(listPattern)
		if err != nil {
			return fmt.Errorf("invalid pattern: %w", err)
		}

		var filtered []gemini.FileSearchDocument
		for _, d := range docs {
			if re.MatchString(d.DisplayName) || re.MatchString(d.Name) {
				filtered = append(filtered, d)
			}
		}
		docs = filtered
	}

	if len(docs) == 0 {
		fmt.Printf("No documents matching pattern '%s' in store '%s'\n", listPattern, storeName)
		return nil
	}

	// Sort by display name
	sort.Slice(docs, func(i, j int) bool {
		return docs[i].DisplayName < docs[j].DisplayName
	})

	fmt.Printf("Documents in File Search Store '%s' (%d total):\n\n", storeName, len(docs))

	if listLong {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "DISPLAY NAME\tSTATE\tCREATED\tNAME")
		fmt.Fprintln(w, "------------\t-----\t-------\t----")
		for _, d := range docs {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				d.DisplayName,
				d.State,
				d.CreateTime,
				d.Name,
			)
		}
		w.Flush()
	} else {
		for _, d := range docs {
			fmt.Printf("  %s\n", d.DisplayName)
		}
	}

	return nil
}

func listFilesLocal() error {
	// Initialize store manager
	storeManager, err := store.NewManager(dataFile)
	if err != nil {
		return fmt.Errorf("failed to initialize store manager: %w", err)
	}

	// Check if store exists
	st, exists := storeManager.GetStore(storeName)
	if !exists {
		return fmt.Errorf("store '%s' not found in local cache. Use --remote to fetch from Gemini API", storeName)
	}

	// Get all files
	files := storeManager.GetAllFiles(storeName)
	if len(files) == 0 {
		fmt.Printf("No files in store '%s' (local cache)\n", storeName)
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
	fmt.Printf("Files in store '%s' (%d total, from local cache):\n\n", storeName, len(files))

	if listLong {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "PATH\tSIZE\tCHECKSUM\tUPLOADED\tREMOTE ID")
		fmt.Fprintln(w, "----\t----\t--------\t--------\t---------")
		for _, f := range files {
			checksum := f.Checksum
			if len(checksum) > 12 {
				checksum = checksum[:12] + "..."
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				f.LocalPath,
				formatSize(f.Size),
				checksum,
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
