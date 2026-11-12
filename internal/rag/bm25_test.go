package rag

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBM25CacheReusesUnchangedAndUpdatesChangedAddedDeletedChunks(t *testing.T) {
	meta := []ChunkMeta{{Text: "alpha beta"}, {Text: "gamma"}}
	b := buildBM25(meta, bm25Snapshot{})
	if !b.dirty {
		t.Fatal("initial index must be dirty")
	}
	data, err := json.Marshal(b.snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot bm25Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	unchanged := buildBM25(meta, snapshot)
	if unchanged.dirty || !reflect.DeepEqual(b.search("alpha", 5), unchanged.search("alpha", 5)) {
		t.Fatal("unchanged index was rebuilt")
	}
	changed := buildBM25([]ChunkMeta{{Text: "updated gamma"}, {Text: "delta"}, {Text: "new"}}, snapshot)
	if !changed.dirty || len(changed.search("alpha", 5)) != 0 || len(changed.search("updated", 5)) != 1 || len(changed.search("new", 5)) != 1 {
		t.Fatal("cache did not update")
	}
	deleted := buildBM25([]ChunkMeta{{Text: "delta"}}, changed.snapshot)
	if len(deleted.search("updated", 5)) != 0 || len(deleted.search("delta", 5)) != 1 {
		t.Fatal("deleted or moved chunks left stale postings")
	}
}

func TestBM25RebuildsMalformedCaches(t *testing.T) {
	meta := []ChunkMeta{{Text: "needle"}}
	valid := buildBM25(meta, bm25Snapshot{}).snapshot
	for _, snapshot := range []bm25Snapshot{
		{Version: 99, Tokenizer: bm25Tokenizer, Documents: valid.Documents},
		{Version: 1, Tokenizer: "different", Documents: valid.Documents},
		{Version: 1, Tokenizer: bm25Tokenizer, Documents: []bm25Document{{Revision: valid.Documents[0].Revision, Length: 1, Terms: map[string]int{"needle": -1}}}},
	} {
		b := buildBM25(meta, snapshot)
		if !b.dirty || len(b.search("needle", 5)) != 1 {
			t.Fatal("malformed cache should rebuild")
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, bm25FileName), []byte("bad-json"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := loadBM25(meta, dir).search("needle", 5); len(got) != 1 {
		t.Fatal(got)
	}
}

func TestQueryCachesOnlyNativeLexicalSearchAndKeepsExternalDirectoriesUntouched(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	index := &RagIndex{Dimension: 2, Meta: []ChunkMeta{{Text: "needle"}}}
	if err := SaveIndex("store", index, []float32{1, 0}); err != nil {
		t.Fatal(err)
	}
	e := NewEngine(&countingQueryClient{})
	config := DefaultConfig()
	config.SearchMode = "vector"
	config.Rerank = false
	if _, err := e.Query("needle", "store", config); err != nil {
		t.Fatal(err)
	}
	dir, _ := storeDir("store")
	if _, err := os.Stat(filepath.Join(dir, bm25FileName)); !os.IsNotExist(err) {
		t.Fatal("vector-only search must not touch BM25 cache")
	}
	config.SearchMode = "hybrid"
	if _, err := e.Query("needle", "store", config); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, bm25FileName)); err != nil {
		t.Fatal("native hybrid cache missing", err)
	}
	// A fresh arbitrary directory is treated as external even with native JSON.
	external := t.TempDir()
	for _, name := range []string{indexFileName, vectorsFileName} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(external, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.QueryDirContext(context.Background(), "needle", external, config); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(external)
	if err != nil || len(files) != 2 {
		t.Fatalf("external directory was changed: %v, %v", files, err)
	}
}
