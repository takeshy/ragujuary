package mcp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/takeshy/ragujuary/internal/gemini"
	"github.com/takeshy/ragujuary/internal/store"
)

func TestNewServerDefersStoreManagerInitialization(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dataFile := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(dataFile, []byte("{not-json"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s, err := NewServer(ServerConfig{
		APIKey:   "test-key",
		DataFile: dataFile,
	}, "test-version")
	if err != nil {
		t.Fatalf("NewServer() error = %v, want nil", err)
	}

	if s.storeManager != nil {
		t.Fatal("storeManager initialized eagerly, want nil")
	}

	_, err = s.getStoreManager()
	if err == nil {
		t.Fatal("getStoreManager() error = nil, want error for invalid data file")
	}
}

func TestSyncRemoteDocumentsToLocalStore(t *testing.T) {
	t.Parallel()

	storeManager, err := store.NewManager(filepath.Join(t.TempDir(), "stores.json"))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	s := &Server{}
	storeManager.AddFile("store-1", store.FileMetadata{
		LocalPath: "/tmp/stale.txt",
		RemoteID:  "fileSearchStores/store-1/documents/stale",
		Checksum:  "stale",
	})
	checksum := "sha256:abc123"
	s.syncRemoteDocumentsToLocalStore(storeManager, "store-1", []gemini.FileSearchDocument{
		{
			Name:        "fileSearchStores/store-1/documents/doc-1",
			DisplayName: "/tmp/a.txt",
			MimeType:    "text/plain",
			CustomMetadata: []gemini.CustomMetadata{
				{Key: "checksum", StringValue: &checksum},
			},
		},
		{
			Name:        "fileSearchStores/store-1/documents/doc-2",
			DisplayName: "",
		},
	})

	meta, ok := storeManager.GetFileByPath("store-1", "/tmp/a.txt")
	if !ok {
		t.Fatal("GetFileByPath() found = false, want true")
	}
	if meta.RemoteID != "fileSearchStores/store-1/documents/doc-1" {
		t.Fatalf("RemoteID = %q, want doc name", meta.RemoteID)
	}
	wantChecksum := "abc123" // sha256: prefix is stripped by syncRemoteDocumentsToLocalStore
	if meta.Checksum != wantChecksum {
		t.Fatalf("Checksum = %q, want %q", meta.Checksum, wantChecksum)
	}
	if _, ok := storeManager.GetFileByPath("store-1", ""); ok {
		t.Fatal("blank display name should not be indexed")
	}
	if _, ok := storeManager.GetFileByPath("store-1", "/tmp/stale.txt"); ok {
		t.Fatal("stale local metadata should be pruned when absent from remote")
	}
}
