package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	Version     = "dev"
	apiKey      string
	storeName   string
	dataFile    string
	parallelism int
)

var rootCmd = &cobra.Command{
	Use:     "ragujuary",
	Short:   "Gemini File Search CLI tool",
	Version: Version,
	Long: `ragujuary is a CLI tool for managing files in Gemini File Search.
It allows you to upload, list, and delete files from named stores.`,
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	defaultStore := "default"
	if envStore := os.Getenv("RAGUJUARY_STORE"); envStore != "" {
		defaultStore = envStore
	}

	rootCmd.PersistentFlags().StringVarP(&apiKey, "api-key", "k", "", "Gemini API key (or set GEMINI_API_KEY env var)")
	rootCmd.PersistentFlags().StringVarP(&storeName, "store", "s", defaultStore, "Store name (or set RAGUJUARY_STORE env var)")
	rootCmd.PersistentFlags().StringVarP(&dataFile, "data-file", "d", "", "Path to data file (default: ~/.ragujuary.json)")
	rootCmd.PersistentFlags().IntVarP(&parallelism, "parallelism", "p", 5, "Number of parallel uploads")
}

func getAPIKey() (string, error) {
	if apiKey != "" {
		return apiKey, nil
	}
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		return "", fmt.Errorf("API key not provided. Use --api-key flag or set GEMINI_API_KEY environment variable")
	}
	return key, nil
}
