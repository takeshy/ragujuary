package embedding

// TaskType represents the embedding task type (Gemini-specific, ignored by OpenAI-compatible providers)
type TaskType string

const (
	TaskRetrievalDocument  TaskType = "RETRIEVAL_DOCUMENT"
	TaskRetrievalQuery     TaskType = "RETRIEVAL_QUERY"
	TaskSemanticSimilarity TaskType = "SEMANTIC_SIMILARITY"
	TaskClassification     TaskType = "CLASSIFICATION"
	TaskClustering         TaskType = "CLUSTERING"
)

// Client is the interface for embedding providers
type Client interface {
	// EmbedContent generates an embedding for a single text
	EmbedContent(model, text string, taskType TaskType, dimension int) ([]float32, error)
	// BatchEmbedContents generates embeddings for multiple texts
	BatchEmbedContents(model string, texts []string, taskType TaskType, dimension int) ([][]float32, error)
}
