package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takeshy/ragujuary/internal/gemini"
)

var (
	queryModel          string
	queryMetadataFilter string
	showCitations       bool
)

var queryCmd = &cobra.Command{
	Use:   "query [question]",
	Short: "Query the File Search Store using RAG",
	Long: `Query the Gemini File Search Store using natural language.
This performs semantic search over your uploaded documents and generates
an answer grounded in the retrieved content.

Example:
  ragujuary query "What are the main features of the application?"
  ragujuary query -s mystore "How does authentication work?"
  ragujuary query --model gemini-2.5-pro "Explain the architecture"`,
	Args: cobra.MinimumNArgs(1),
	RunE: runQuery,
}

func init() {
	queryCmd.Flags().StringVarP(&queryModel, "model", "m", "gemini-2.5-flash", "Model to use for generation")
	queryCmd.Flags().StringVar(&queryMetadataFilter, "filter", "", "Metadata filter (e.g., 'author=\"John\"')")
	queryCmd.Flags().BoolVar(&showCitations, "citations", false, "Show citation details")
	rootCmd.AddCommand(queryCmd)
}

func runQuery(cmd *cobra.Command, args []string) error {
	key, err := getAPIKey()
	if err != nil {
		return err
	}

	query := strings.Join(args, " ")

	// Create client
	client := gemini.NewClient(key)

	// Resolve store name (supports both API name and display name)
	resolvedName, remoteStore, err := client.ResolveStoreName(storeName)
	if err != nil {
		return fmt.Errorf("File Search Store '%s' not found: %w", storeName, err)
	}
	_ = resolvedName

	fmt.Printf("Querying File Search Store '%s'...\n\n", remoteStore.DisplayName)

	// Perform query
	resp, err := client.Query(queryModel, query, []string{remoteStore.Name}, queryMetadataFilter)
	if err != nil {
		return fmt.Errorf("query failed: %w", err)
	}

	if len(resp.Candidates) == 0 {
		fmt.Println("No response generated")
		return nil
	}

	// Print the response
	candidate := resp.Candidates[0]
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			fmt.Println(part.Text)
		}
	}

	// Print citations if requested
	if showCitations && candidate.GroundingMetadata != nil {
		fmt.Println("\n--- Citations ---")
		for i, chunk := range candidate.GroundingMetadata.GroundingChunks {
			if chunk.RetrievedContext != nil {
				fmt.Printf("\n[%d] %s\n", i+1, chunk.RetrievedContext.Title)
				if chunk.RetrievedContext.URI != "" {
					fmt.Printf("    URI: %s\n", chunk.RetrievedContext.URI)
				}
				if chunk.RetrievedContext.Text != "" {
					// Truncate long text
					text := chunk.RetrievedContext.Text
					if len(text) > 200 {
						text = text[:200] + "..."
					}
					fmt.Printf("    Text: %s\n", text)
				}
			}
		}
	}

	return nil
}
