package mcp

// UploadInput represents input for the upload tool
type UploadInput struct {
	StoreName   string `json:"store_name" jsonschema:"name of the File Search Store"`
	FileName    string `json:"file_name" jsonschema:"file name or path to use for the uploaded file"`
	FileContent string `json:"file_content" jsonschema:"file content (base64 encoded for binary files, plain text for text files)"`
	IsBase64    bool   `json:"is_base64,omitempty" jsonschema:"set to true if file_content is base64 encoded"`
}

// UploadOutput represents output from the upload tool
type UploadOutput struct {
	Success  bool   `json:"success"`
	FileName string `json:"file_name"`
	Error    string `json:"error,omitempty"`
}

// QueryInput represents input for the query tool
type QueryInput struct {
	Question       string   `json:"question" jsonschema:"the question to ask about your documents"`
	StoreName      string   `json:"store_name,omitempty" jsonschema:"name of the File Search Store (use store_name or store_names)"`
	StoreNames     []string `json:"store_names,omitempty" jsonschema:"names of multiple File Search Stores to query"`
	Model          string   `json:"model,omitempty" jsonschema:"model to use (default: gemini-3-flash-preview)"`
	MetadataFilter string   `json:"metadata_filter,omitempty" jsonschema:"metadata filter expression"`
	ShowCitations  bool     `json:"show_citations,omitempty" jsonschema:"include citation details in response"`
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
	StoreName string `json:"store_name" jsonschema:"display name for the new File Search Store"`
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

// StoreInfo represents information about a File Search Store
type StoreInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	CreateTime  string `json:"create_time,omitempty"`
	UpdateTime  string `json:"update_time,omitempty"`
}
