package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const defaultDataFile = ".ragujuary.json"

// Manager handles store operations
type Manager struct {
	dataPath string
	data     *StoreData
	mu       sync.RWMutex
}

// NewManager creates a new store manager
func NewManager(dataPath string) (*Manager, error) {
	if dataPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		dataPath = filepath.Join(home, defaultDataFile)
	}

	m := &Manager{
		dataPath: dataPath,
		data: &StoreData{
			Stores: make(map[string]*Store),
		},
	}

	if err := m.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load store data: %w", err)
	}

	return m, nil
}

// load loads store data from file
func (m *Manager) load() error {
	data, err := os.ReadFile(m.dataPath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, m.data)
}

// Save saves store data to file
func (m *Manager) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, err := json.MarshalIndent(m.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal store data: %w", err)
	}

	return os.WriteFile(m.dataPath, data, 0644)
}

// GetOrCreateStore gets an existing store or creates a new one
func (m *Manager) GetOrCreateStore(name string) *Store {
	m.mu.Lock()
	defer m.mu.Unlock()

	if store, ok := m.data.Stores[name]; ok {
		return store
	}

	store := &Store{
		Name:      name,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Files:     make(map[string]FileMetadata),
	}
	m.data.Stores[name] = store
	return store
}

// GetStore gets an existing store
func (m *Manager) GetStore(name string) (*Store, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	store, ok := m.data.Stores[name]
	return store, ok
}

// ListStores returns all store names
func (m *Manager) ListStores() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.data.Stores))
	for name := range m.data.Stores {
		names = append(names, name)
	}
	return names
}

// AddFile adds a file to a store
func (m *Manager) AddFile(storeName string, meta FileMetadata) {
	m.mu.Lock()
	defer m.mu.Unlock()

	store := m.data.Stores[storeName]
	if store == nil {
		store = &Store{
			Name:      storeName,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Files:     make(map[string]FileMetadata),
		}
		m.data.Stores[storeName] = store
	}

	store.Files[meta.LocalPath] = meta
	store.UpdatedAt = time.Now()
}

// RemoveFile removes a file from a store
func (m *Manager) RemoveFile(storeName, localPath string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	store := m.data.Stores[storeName]
	if store == nil {
		return false
	}

	if _, ok := store.Files[localPath]; !ok {
		return false
	}

	delete(store.Files, localPath)
	store.UpdatedAt = time.Now()
	return true
}

// GetFileByChecksum finds a file by checksum in a store
func (m *Manager) GetFileByChecksum(storeName, checksum string) (*FileMetadata, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	store := m.data.Stores[storeName]
	if store == nil {
		return nil, false
	}

	for _, meta := range store.Files {
		if meta.Checksum == checksum {
			return &meta, true
		}
	}
	return nil, false
}

// GetFileByPath finds a file by local path in a store
func (m *Manager) GetFileByPath(storeName, localPath string) (*FileMetadata, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	store := m.data.Stores[storeName]
	if store == nil {
		return nil, false
	}

	meta, ok := store.Files[localPath]
	if !ok {
		return nil, false
	}
	return &meta, true
}

// GetAllFiles returns all files in a store
func (m *Manager) GetAllFiles(storeName string) []FileMetadata {
	m.mu.RLock()
	defer m.mu.RUnlock()

	store := m.data.Stores[storeName]
	if store == nil {
		return nil
	}

	files := make([]FileMetadata, 0, len(store.Files))
	for _, meta := range store.Files {
		files = append(files, meta)
	}
	return files
}
