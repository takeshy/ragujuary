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

// MultimodalContent represents binary content for multimodal embedding
type MultimodalContent struct {
	MIMEType string
	Data     []byte
}

// MultimodalEmbedder is an optional interface for clients that support
// multimodal embedding (images, PDF, video, audio).
// Clients that don't support this (e.g., OpenAI-compatible) don't implement it.
type MultimodalEmbedder interface {
	EmbedMultimodalContent(model string, content MultimodalContent, taskType TaskType, dimension int) ([]float32, error)
}
