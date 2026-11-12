package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/takeshy/ragujuary/internal/embedding"
	"github.com/takeshy/ragujuary/internal/rag"
	"github.com/takeshy/ragujuary/internal/rerank"
)

var (
	embedModel            string
	embedDimension        int
	embedChunkSize        int
	embedChunkOverlap     int
	embedPDFMaxPages      int
	embedTopK             int
	embedMinScore         float64
	embedExclude          []string
	embedURL              string
	embedAPIKey           string
	embedDir              string
	embedSearchMode       string
	embedRerank           bool
	embedSearchUntilFound bool
	embedCandidateK       int
	embedRerankCandidateK int
	embedRRFK             float64
)

var embedCmd = &cobra.Command{
	Use:   "embed",
	Short: "Embedding-based RAG operations (index, query, list, delete, clear)",
	Long: `Manage a local embedding-based RAG store.

Supports Gemini Embedding API (default) and OpenAI-compatible APIs (Ollama, LM Studio, etc.).
Unlike the managed FileSearch stores, embedding mode stores vectors locally
and uses hybrid vector/BM25 search with Gemini reranking by default.

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
	Long:  `Search the local embedding store using hybrid vector/BM25 retrieval and Gemini reranking by default. Use --search-mode vector to disable hybrid retrieval, --rerank=false to disable reranking, or --search-until-found to score successive candidate groups until an answer is found.`,
	Args:  cobra.MinimumNArgs(1),
	RunE:  runEmbedQuery,
}

var embedListCmd = &cobra.Command{
	Use:   "list",
	Short: "List indexed files or stores",
	Args:  cobra.NoArgs,
	RunE:  runEmbedList,
}

var embedDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete files from the embedding index",
	Args:  cobra.NoArgs,
	RunE:  runEmbedDelete,
}

var embedClearCmd = &cobra.Command{
	Use:   "clear [store-name]",
	Short: "Clear an entire embedding store",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runEmbedClear,
}

var embedListStores bool
var embedDeletePattern string

func init() {
	// embed command flags
	embedCmd.PersistentFlags().StringVar(&embedModel, "model", "gemini-embedding-2", "Embedding model name")
	embedCmd.PersistentFlags().IntVar(&embedDimension, "dimension", 768, "Embedding output dimensionality")
	embedCmd.PersistentFlags().StringVar(&embedURL, "embed-url", "", "OpenAI-compatible embedding API URL (e.g. http://localhost:11434 for Ollama)")
	embedCmd.PersistentFlags().StringVar(&embedAPIKey, "embed-api-key", "", "API key for OpenAI-compatible embedding APIs (or set RAGUJUARY_EMBED_API_KEY / OPENAI_API_KEY)")

	// index flags
	embedIndexCmd.Flags().StringSliceVarP(&embedExclude, "exclude", "e", nil, "Regex patterns to exclude files")
	embedIndexCmd.Flags().IntVar(&embedChunkSize, "chunk-size", 1000, "Chunk size in characters")
	embedIndexCmd.Flags().IntVar(&embedChunkOverlap, "chunk-overlap", 200, "Chunk overlap in characters")
	embedIndexCmd.Flags().IntVar(&embedPDFMaxPages, "pdf-pages", 6, "Max pages per PDF chunk (1-6)")

	// query flags
	embedQueryCmd.Flags().IntVar(&embedTopK, "top-k", 5, "Number of top results to return")
	embedQueryCmd.Flags().Float64Var(&embedMinScore, "min-score", 0.3, "Minimum similarity score threshold")
	embedQueryCmd.Flags().StringVar(&embedDir, "dir", "", "Path to external RAG index directory (overrides --store)")
	embedQueryCmd.Flags().StringVar(&embedSearchMode, "search-mode", "hybrid", "Search mode: hybrid (vector + BM25 with RRF), vector, or keyword (BM25)")
	embedQueryCmd.Flags().BoolVar(&embedRerank, "rerank", true, "Rerank indexed text with Gemini Decision 1; use --rerank=false to disable")
	embedQueryCmd.Flags().BoolVar(&embedSearchUntilFound, "search-until-found", false, "Score successive candidate groups until relevance >= 2/3 is found (requires reranking; max 1000 candidates)")
	embedQueryCmd.Flags().IntVar(&embedCandidateK, "candidate-k", 30, "Candidates per hybrid search branch (1-1000)")
	embedQueryCmd.Flags().IntVar(&embedRerankCandidateK, "rerank-candidate-k", 40, "Candidates to rerank (1-1000; raised to top-k if needed)")
	embedQueryCmd.Flags().Float64Var(&embedRRFK, "rrf-k", 60, "RRF rank constant for hybrid search")

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
	config.PDFMaxPages = embedPDFMaxPages
	config.TopK = embedTopK
	config.MinScore = embedMinScore
	config.SearchMode = embedSearchMode
	config.Rerank = embedRerank
	config.SearchUntilFound = embedSearchUntilFound
	config.CandidateK = embedCandidateK
	config.RerankCandidateK = embedRerankCandidateK
	config.RRFK = embedRRFK
	return config
}

func runEmbedIndex(cmd *cobra.Command, args []string) error {
	if embedPDFMaxPages < 1 || embedPDFMaxPages > 6 {
		return fmt.Errorf("--pdf-pages must be between 1 and 6, got %d", embedPDFMaxPages)
	}

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
	if result.MultimodalFiles > 0 {
		fmt.Printf("  Multimodal:    %d\n", result.MultimodalFiles)
	}
	if result.SkippedMultimodal > 0 {
		fmt.Printf("  Skipped (multimodal): %d\n", result.SkippedMultimodal)
	}
	fmt.Printf("  Total chunks:  %d\n", result.TotalChunks)

	return nil
}

func runEmbedQuery(cmd *cobra.Command, args []string) error {
	config := newEmbedConfig()
	config.OnWarning = func(message string) { fmt.Fprintln(cmd.ErrOrStderr(), "Warning: "+message) }
	if err := config.ValidateQuery(); err != nil {
		return err
	}
	var client embedding.Client
	var err error
	if config.SearchMode != "keyword" {
		client, err = newEmbeddingClient()
		if err != nil {
			return err
		}
	}

	question := strings.Join(args, " ")

	engine := rag.NewEngine(client)
	if config.Rerank {
		key, err := getAPIKey()
		if err == nil {
			engine.WithReranker(rerank.NewGeminiClient(key))
		}
	}

	var results []rag.SearchResult
	if embedDir != "" {
		results, err = engine.QueryDirContext(cmd.Context(), question, embedDir, config)
	} else {
		results, err = engine.QueryContext(cmd.Context(), question, storeName, config)
	}
	if err != nil {
		return err
	}

	if len(results) == 0 {
		fmt.Println("No results found.")
		return nil
	}

	for i, r := range results {
		typeLabel := ""
		if r.ContentType != "" {
			typeLabel = fmt.Sprintf(" [%s]", r.ContentType)
		}
		pageInfo := ""
		if r.PageLabel != "" {
			pageInfo = fmt.Sprintf(", %s", r.PageLabel)
		}
		rerankInfo := ""
		if r.RerankScore != nil {
			rerankInfo = fmt.Sprintf(", rerank: %.4f/3", *r.RerankScore)
		}
		if r.RerankSkippedReason != "" {
			rerankInfo += ", rerank skipped: " + r.RerankSkippedReason + " (original rank kept)"
		}
		if r.RetrievalSource != "" {
			rerankInfo += ", source: " + r.RetrievalSource
		}
		fmt.Printf("--- Result %d (score: %.4f%s, file: %s%s%s) ---\n", i+1, r.Score, rerankInfo, r.FilePath, typeLabel, pageInfo)
		fmt.Println(r.Text)
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

	// Count chunks per file and track content types
	chunkCounts := make(map[string]int)
	fileTypes := make(map[string]string)
	for _, meta := range index.Meta {
		chunkCounts[meta.FilePath]++
		if meta.ContentType != "" {
			fileTypes[meta.FilePath] = meta.ContentType
		}
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "FILE\tTYPE\tCHUNKS\n")
	for filePath, count := range chunkCounts {
		ct := fileTypes[filePath]
		if ct == "" {
			ct = "text"
		}
		fmt.Fprintf(w, "%s\t%s\t%d\n", filePath, ct, count)
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
	name := storeName
	if len(args) > 0 {
		name = args[0]
	}
	fmt.Printf("Are you sure you want to clear store '%s'? [y/N] ", name)
	var confirm string
	fmt.Scanln(&confirm)

	if confirm != "y" && confirm != "Y" {
		fmt.Println("Cancelled.")
		return nil
	}

	if err := rag.DeleteIndex(name); err != nil {
		return err
	}

	fmt.Printf("Store '%s' cleared.\n", name)
	return nil
}
