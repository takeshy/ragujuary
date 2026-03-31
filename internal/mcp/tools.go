package mcp

// UploadInput represents input for the upload tool (works for both FileSearch and Embedding stores)
type UploadInput struct {
	StoreName    string `json:"store_name" jsonschema:"name of the store"`
	FileName     string `json:"file_name" jsonschema:"file name or path for the uploaded file"`
	FileContent  string `json:"file_content" jsonschema:"file content (base64 encoded for binary files, plain text for text files)"`
	IsBase64     bool   `json:"is_base64,omitempty" jsonschema:"set to true if file_content is base64 encoded"`
	MIMEType     string `json:"mime_type,omitempty" jsonschema:"MIME type for binary content (e.g. image/png, application/pdf) - embedding stores only"`
	ChunkSize    int    `json:"chunk_size,omitempty" jsonschema:"chunk size in characters (default: 1000) - embedding stores only"`
	ChunkOverlap int    `json:"chunk_overlap,omitempty" jsonschema:"chunk overlap in characters (default: 200) - embedding stores only"`
	Dimension    int    `json:"dimension,omitempty" jsonschema:"embedding dimensionality (default: 768) - embedding stores only"`
	PDFMaxPages  int    `json:"pdf_max_pages,omitempty" jsonschema:"max pages per PDF chunk (1-6, default: 6) - embedding stores only"`
}

// UploadOutput represents output from the upload tool
type UploadOutput struct {
	Success  bool   `json:"success"`
	FileName string `json:"file_name"`
	Error    string `json:"error,omitempty"`
}

// QueryInput represents input for the query tool (works for both FileSearch and Embedding stores)
type QueryInput struct {
	Question       string   `json:"question" jsonschema:"the question to ask about your documents"`
	StoreName      string   `json:"store_name" jsonschema:"name of the store (required unless store_names is provided)"`
	StoreNames     []string `json:"store_names,omitempty" jsonschema:"names of multiple stores to query (alternative to store_name)"`
	Model          string   `json:"model,omitempty" jsonschema:"model to use (default: gemini-3-flash-preview for FileSearch)"`
	MetadataFilter string   `json:"metadata_filter,omitempty" jsonschema:"metadata filter expression - FileSearch only"`
	ShowCitations  bool     `json:"show_citations,omitempty" jsonschema:"include citation details - FileSearch only"`
	TopK           int      `json:"top_k,omitempty" jsonschema:"number of top results (default: 5) - embedding stores only"`
	MinScore       float64  `json:"min_score,omitempty" jsonschema:"minimum similarity score (default: 0.3) - embedding stores only"`
}

// QueryOutput represents output from the query tool
type QueryOutput struct {
	Answer    string         `json:"answer"`
	Citations []CitationInfo `json:"citations,omitempty"`
}

// CitationInfo represents a citation source
type CitationInfo struct {
	Title string `json:"title"`
	URI   string `json:"uri,omitempty"`
	Text  string `json:"text,omitempty"`
}

// ListInput represents input for the list tool
type ListInput struct {
	StoreName string `json:"store_name" jsonschema:"name of the store to list files from"`
	Pattern   string `json:"pattern,omitempty" jsonschema:"regex pattern to filter results"`
}

// ListOutput represents output from the list tool
type ListOutput struct {
	Items []ListItem `json:"items"`
	Total int        `json:"total"`
}

// ListItem represents a single item in list output
type ListItem struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	State       string `json:"state,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
}

// DeleteInput represents input for the delete tool
type DeleteInput struct {
	StoreName string `json:"store_name" jsonschema:"name of the store"`
	FileName  string `json:"file_name" jsonschema:"file name or path to delete"`
}

// DeleteOutput represents output from the delete tool
type DeleteOutput struct {
	Success  bool   `json:"success"`
	FileName string `json:"file_name"`
	Error    string `json:"error,omitempty"`
}

// CreateStoreInput represents input for the create_store tool
type CreateStoreInput struct {
	StoreName string `json:"store_name" jsonschema:"display name for the new store"`
	Type      string `json:"type,omitempty" jsonschema:"store type: 'embed' for embedding store, 'filesearch' for FileSearch store (default: filesearch)"`
}

// CreateStoreOutput represents output from the create_store tool
type CreateStoreOutput struct {
	Success   bool   `json:"success"`
	StoreName string `json:"store_name"`
	StoreID   string `json:"store_id"`
	Error     string `json:"error,omitempty"`
}

// DeleteStoreInput represents input for the delete_store tool
type DeleteStoreInput struct {
	StoreName string `json:"store_name" jsonschema:"name of the store to delete"`
}

// DeleteStoreOutput represents output from the delete_store tool
type DeleteStoreOutput struct {
	Success   bool   `json:"success"`
	StoreName string `json:"store_name"`
	Error     string `json:"error,omitempty"`
}

// ListStoresInput represents input for the list_stores tool
type ListStoresInput struct{}

// ListStoresOutput represents output from the list_stores tool
type ListStoresOutput struct {
	Stores []StoreInfo `json:"stores"`
	Total  int         `json:"total"`
}

// StoreInfo represents information about a store
type StoreInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	CreateTime  string `json:"create_time,omitempty"`
	UpdateTime  string `json:"update_time,omitempty"`
}

// UploadDirectoryInput represents input for the upload_directory tool
type UploadDirectoryInput struct {
	StoreName       string   `json:"store_name" jsonschema:"name of the store"`
	Directories     []string `json:"directories" jsonschema:"list of directory paths to upload/index"`
	ExcludePatterns []string `json:"exclude_patterns,omitempty" jsonschema:"regex patterns to exclude files"`
	Parallelism     int      `json:"parallelism,omitempty" jsonschema:"number of parallel uploads (default: 5) - FileSearch only"`
	ChunkSize       int      `json:"chunk_size,omitempty" jsonschema:"chunk size in characters (default: 1000) - embedding stores only"`
	ChunkOverlap    int      `json:"chunk_overlap,omitempty" jsonschema:"chunk overlap in characters (default: 200) - embedding stores only"`
	Dimension       int      `json:"dimension,omitempty" jsonschema:"embedding dimensionality (default: 768) - embedding stores only"`
	PDFMaxPages     int      `json:"pdf_max_pages,omitempty" jsonschema:"max pages per PDF chunk (1-6, default: 6) - embedding stores only"`
}

// UploadDirectoryOutput represents output from the upload_directory tool
type UploadDirectoryOutput struct {
	Success  bool   `json:"success"`
	Uploaded int    `json:"uploaded"`
	Skipped  int    `json:"skipped"`
	Failed   int    `json:"failed"`
	Error    string `json:"error,omitempty"`
}
