package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/takeshy/ragujuary/internal/embedding"
	"github.com/takeshy/ragujuary/internal/rag"
)

var (
	embedModel        string
	embedDimension    int
	embedChunkSize    int
	embedChunkOverlap int
	embedTopK         int
	embedMinScore     float64
	embedExclude      []string
	embedURL          string
	embedAPIKey       string
)

var embedCmd = &cobra.Command{
	Use:   "embed",
	Short: "Embedding-based RAG operations (index, query, list, delete, clear)",
	Long: `Manage a local embedding-based RAG store.

Supports Gemini Embedding API (default) and OpenAI-compatible APIs (Ollama, LM Studio, etc.).
Unlike the managed FileSearch stores, embedding mode stores vectors locally
and performs cosine similarity search for retrieval.

Examples:
  # Use Gemini Embedding API (default)
  ragujuary embed index -s mystore ./docs

  # Use Ollama (nomic-embed-text model)
  ragujuary embed index -s mystore --embed-url http://localhost:11434 --model nomic-embed-text ./docs`,
}

var embedIndexCmd = &cobra.Command{
	Use:   "index [directories...]",
	Short: "Index files using embeddings",
	Long: `Index files from directories into a local embedding store.
Files are chunked, embedded, and stored locally.
Incremental: only re-indexes files whose content has changed.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runEmbedIndex,
}

var embedQueryCmd = &cobra.Command{
	Use:   "query [question...]",
	Short: "Query the embedding store",
	Long:  `Perform a semantic search against the local embedding store using cosine similarity.`,
	Args:  cobra.MinimumNArgs(1),
	RunE:  runEmbedQuery,
}

var embedListCmd = &cobra.Command{
	Use:   "list",
	Short: "List indexed files or stores",
	RunE:  runEmbedList,
}

var embedDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete files from the embedding index",
	RunE:  runEmbedDelete,
}

var embedClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear an entire embedding store",
	RunE:  runEmbedClear,
}

var embedListStores bool
var embedDeletePattern string

func init() {
	// embed command flags
	embedCmd.PersistentFlags().StringVar(&embedModel, "model", "gemini-embedding-2-preview", "Embedding model name")
	embedCmd.PersistentFlags().IntVar(&embedDimension, "dimension", 768, "Embedding output dimensionality")
	embedCmd.PersistentFlags().StringVar(&embedURL, "embed-url", "", "OpenAI-compatible embedding API URL (e.g. http://localhost:11434 for Ollama)")
	embedCmd.PersistentFlags().StringVar(&embedAPIKey, "embed-api-key", "", "API key for OpenAI-compatible embedding APIs (or set RAGUJUARY_EMBED_API_KEY / OPENAI_API_KEY)")

	// index flags
	embedIndexCmd.Flags().StringSliceVarP(&embedExclude, "exclude", "e", nil, "Regex patterns to exclude files")
	embedIndexCmd.Flags().IntVar(&embedChunkSize, "chunk-size", 1000, "Chunk size in characters")
	embedIndexCmd.Flags().IntVar(&embedChunkOverlap, "chunk-overlap", 200, "Chunk overlap in characters")

	// query flags
	embedQueryCmd.Flags().IntVar(&embedTopK, "top-k", 5, "Number of top results to return")
	embedQueryCmd.Flags().Float64Var(&embedMinScore, "min-score", 0.3, "Minimum similarity score threshold")

	// list flags
	embedListCmd.Flags().BoolVar(&embedListStores, "stores", false, "List all embedding stores instead of files")

	// delete flags
	embedDeleteCmd.Flags().StringVarP(&embedDeletePattern, "pattern", "P", "", "Regex pattern to match files to delete")

	embedCmd.AddCommand(embedIndexCmd)
	embedCmd.AddCommand(embedQueryCmd)
	embedCmd.AddCommand(embedListCmd)
	embedCmd.AddCommand(embedDeleteCmd)
	embedCmd.AddCommand(embedClearCmd)
	rootCmd.AddCommand(embedCmd)
}

func newEmbeddingClient() (embedding.Client, error) {
	if embedURL != "" {
		return embedding.NewOpenAIClient(embedURL, getEmbeddingAPIKey()), nil
	}
	key, err := getAPIKey()
	if err != nil {
		return nil, err
	}
	return embedding.NewGeminiClient(key), nil
}

func getEmbeddingAPIKey() string {
	if embedAPIKey != "" {
		return embedAPIKey
	}
	if key := os.Getenv("RAGUJUARY_EMBED_API_KEY"); key != "" {
		return key
	}
	return os.Getenv("OPENAI_API_KEY")
}

func newEmbedConfig() rag.Config {
	config := rag.DefaultConfig()
	config.Model = embedModel
	config.Dimension = embedDimension
	config.ChunkSize = embedChunkSize
	config.ChunkOverlap = embedChunkOverlap
	config.TopK = embedTopK
	config.MinScore = embedMinScore
	return config
}

func runEmbedIndex(cmd *cobra.Command, args []string) error {
	client, err := newEmbeddingClient()
	if err != nil {
		return err
	}

	engine := rag.NewEngine(client)
	config := newEmbedConfig()

	fmt.Fprintf(os.Stderr, "Indexing files into store '%s' (model: %s, dimension: %d, chunk: %d/%d)...\n",
		storeName, config.Model, config.Dimension, config.ChunkSize, config.ChunkOverlap)

	result, err := engine.Index(args, embedExclude, storeName, config)
	if err != nil {
		return err
	}

	fmt.Printf("Indexing complete:\n")
	fmt.Printf("  Total files:   %d\n", result.IndexedFiles)
	fmt.Printf("  New files:     %d\n", result.NewFiles)
	fmt.Printf("  Updated files: %d\n", result.UpdatedFiles)
	fmt.Printf("  Skipped files: %d\n", result.SkippedFiles)
	fmt.Printf("  Total chunks:  %d\n", result.TotalChunks)

	return nil
}

func runEmbedQuery(cmd *cobra.Command, args []string) error {
	client, err := newEmbeddingClient()
	if err != nil {
		return err
	}

	question := strings.Join(args, " ")

	engine := rag.NewEngine(client)
	config := newEmbedConfig()

	results, err := engine.Query(question, storeName, config)
	if err != nil {
		return err
	}

	if len(results) == 0 {
		fmt.Println("No results found.")
		return nil
	}

	for i, r := range results {
		fmt.Printf("--- Result %d (score: %.4f, file: %s) ---\n", i+1, r.Score, r.FilePath)
		text := r.Text
		if len(text) > 500 {
			text = text[:500] + "..."
		}
		fmt.Println(text)
		fmt.Println()
	}

	return nil
}

func runEmbedList(cmd *cobra.Command, args []string) error {
	if embedListStores {
		stores, err := rag.ListStores()
		if err != nil {
			return err
		}

		if len(stores) == 0 {
			fmt.Println("No embedding stores found.")
			return nil
		}

		fmt.Printf("Embedding stores (%d):\n", len(stores))
		for _, s := range stores {
			// Load index to show stats
			index, _, _ := rag.LoadIndex(s)
			if index != nil {
				fmt.Printf("  %s  (files: %d, chunks: %d, model: %s)\n",
					s, len(index.FileChecksums), len(index.Meta), index.EmbeddingModel)
			} else {
				fmt.Printf("  %s\n", s)
			}
		}
		return nil
	}

	// List files in a specific store
	index, _, err := rag.LoadIndex(storeName)
	if err != nil {
		return err
	}
	if index == nil {
		fmt.Printf("Store '%s' not found or empty.\n", storeName)
		return nil
	}

	// Count chunks per file
	chunkCounts := make(map[string]int)
	for _, meta := range index.Meta {
		chunkCounts[meta.FilePath]++
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "FILE\tCHUNKS\n")
	for filePath, count := range chunkCounts {
		fmt.Fprintf(w, "%s\t%d\n", filePath, count)
	}
	w.Flush()

	fmt.Printf("\nTotal: %d files, %d chunks (model: %s, dimension: %d)\n",
		len(chunkCounts), len(index.Meta), index.EmbeddingModel, index.Dimension)

	return nil
}

func runEmbedDelete(cmd *cobra.Command, args []string) error {
	if embedDeletePattern == "" {
		return fmt.Errorf("--pattern is required")
	}

	client, err := newEmbeddingClient()
	if err != nil {
		return err
	}

	engine := rag.NewEngine(client)

	deleted, err := engine.DeleteFiles(storeName, embedDeletePattern)
	if err != nil {
		return err
	}

	if deleted == 0 {
		fmt.Println("No matching files found.")
	} else {
		fmt.Printf("Deleted %d files from store '%s'.\n", deleted, storeName)
	}

	return nil
}

func runEmbedClear(cmd *cobra.Command, args []string) error {
	fmt.Printf("Are you sure you want to clear store '%s'? [y/N] ", storeName)
	var confirm string
	fmt.Scanln(&confirm)

	if confirm != "y" && confirm != "Y" {
		fmt.Println("Cancelled.")
		return nil
	}

	if err := rag.DeleteIndex(storeName); err != nil {
		return err
	}

	fmt.Printf("Store '%s' cleared.\n", storeName)
	return nil
}
