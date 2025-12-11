package store

import "time"

// FileMetadata represents metadata for an uploaded file
type FileMetadata struct {
	LocalPath   string    `json:"local_path"`
	RemoteID    string    `json:"remote_id"`
	RemoteName  string    `json:"remote_name"`
	Checksum    string    `json:"checksum"`
	Size        int64     `json:"size"`
	UploadedAt  time.Time `json:"uploaded_at"`
	MimeType    string    `json:"mime_type"`
}

// Store represents a named collection of uploaded files
type Store struct {
	Name      string                  `json:"name"`
	CreatedAt time.Time               `json:"created_at"`
	UpdatedAt time.Time               `json:"updated_at"`
	Files     map[string]FileMetadata `json:"files"` // key is local path
}

// StoreData represents all stores data
type StoreData struct {
	Stores map[string]*Store `json:"stores"` // key is store name
}

// Config represents the application configuration
type Config struct {
	TargetDirs     []string `json:"target_dirs"`
	ExcludePattern []string `json:"exclude_patterns"`
	StoreName      string   `json:"store_name"`
	Parallelism    int      `json:"parallelism"`
}
