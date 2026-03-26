package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/takeshy/ragujuary/internal/gemini"
	"github.com/takeshy/ragujuary/internal/rag"
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

func TestGetAllowedStoreNamesEnforcesAllowedStores(t *testing.T) {
	t.Parallel()

	s := &Server{
		config: ServerConfig{
			AllowedStores: []string{"allowed-a", "allowed-b"},
		},
	}

	names, err := s.getAllowedStoreNames("", []string{"allowed-a", "allowed-a", "allowed-b"})
	if err != nil {
		t.Fatalf("getAllowedStoreNames() error = %v, want nil", err)
	}
	if len(names) != 2 || names[0] != "allowed-a" || names[1] != "allowed-b" {
		t.Fatalf("getAllowedStoreNames() = %v, want deduplicated allowed names", names)
	}

	_, err = s.getAllowedStoreNames("", []string{"forbidden"})
	if err == nil || !strings.Contains(err.Error(), "allowed stores list") {
		t.Fatalf("getAllowedStoreNames() error = %v, want allowed-stores error", err)
	}
}

func TestHandleCreateDeleteStoreEnforceAllowedStores(t *testing.T) {
	t.Parallel()

	s := &Server{
		config: ServerConfig{
			AllowedStores: []string{"allowed"},
		},
	}

	_, _, err := s.handleCreateStore(context.Background(), nil, CreateStoreInput{
		StoreName: "forbidden",
		Type:      "embed",
	})
	if err == nil || !strings.Contains(err.Error(), "allowed stores list") {
		t.Fatalf("handleCreateStore() error = %v, want allowed-stores error", err)
	}

	_, _, err = s.handleDeleteStore(context.Background(), nil, DeleteStoreInput{
		StoreName: "forbidden",
	})
	if err == nil || !strings.Contains(err.Error(), "allowed stores list") {
		t.Fatalf("handleDeleteStore() error = %v, want allowed-stores error", err)
	}
}

func TestHandleQueryRejectsDisallowedAndMixedEmbeddingStores(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := rag.CreateEmptyIndex("embed-a"); err != nil {
		t.Fatalf("CreateEmptyIndex(embed-a) error = %v", err)
	}
	if err := rag.CreateEmptyIndex("embed-b"); err != nil {
		t.Fatalf("CreateEmptyIndex(embed-b) error = %v", err)
	}

	s := &Server{
		config: ServerConfig{
			AllowedStores: []string{"embed-a", "embed-b"},
		},
	}

	_, _, err := s.handleQuery(context.Background(), nil, QueryInput{
		Question:   "test",
		StoreNames: []string{"forbidden"},
	})
	if err == nil || !strings.Contains(err.Error(), "allowed stores list") {
		t.Fatalf("handleQuery(disallowed) error = %v, want allowed-stores error", err)
	}

	_, _, err = s.handleQuery(context.Background(), nil, QueryInput{
		Question:   "test",
		StoreNames: []string{"embed-a", "embed-b"},
	})
	if err == nil || !strings.Contains(err.Error(), "multiple embedding stores") {
		t.Fatalf("handleQuery(multi-embed) error = %v, want multiple-embedding-stores error", err)
	}
}
