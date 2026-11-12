package rag

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
)

// Separate from the Obsidian sidecar: tokenization is compatible with its CJK
// fallback, but cache serialization belongs to this Go implementation.
const bm25FileName = "bm25-ragujuary.json"
const bm25Tokenizer = "cjk-bigram-v1"

type bm25Document struct {
	Revision string         `json:"revision"`
	Length   int            `json:"length"`
	Terms    map[string]int `json:"terms"`
}

type bm25Snapshot struct {
	Version   int            `json:"version"`
	Tokenizer string         `json:"tokenizer"`
	Documents []bm25Document `json:"documents"`
}

type bm25Index struct {
	snapshot    bm25Snapshot
	postings    map[string]map[int]int
	totalLength int
	dirty       bool
}

func validBM25Document(doc bm25Document) bool {
	if doc.Length < 0 || doc.Terms == nil {
		return false
	}
	length := 0
	for term, tf := range doc.Terms {
		if term == "" || tf <= 0 || tf > doc.Length-length {
			return false
		}
		length += tf
	}
	return length == doc.Length
}

// Reuse token frequencies only for matching content revisions. Changed, added,
// removed, or reordered chunks are reflected without regenerating embeddings.
func buildBM25(meta []ChunkMeta, cached bm25Snapshot) *bm25Index {
	b := &bm25Index{snapshot: bm25Snapshot{Version: 1, Tokenizer: bm25Tokenizer}, postings: make(map[string]map[int]int)}
	compatible := cached.Version == 1 && cached.Tokenizer == bm25Tokenizer
	b.dirty = !compatible || len(cached.Documents) != len(meta)
	for i, chunk := range meta {
		digest := sha256.Sum256([]byte(chunk.Text))
		revision := hex.EncodeToString(digest[:])
		var doc bm25Document
		if compatible && i < len(cached.Documents) && cached.Documents[i].Revision == revision && validBM25Document(cached.Documents[i]) {
			doc = cached.Documents[i]
		} else {
			b.dirty = true
			doc = bm25Document{Revision: revision, Terms: make(map[string]int)}
			for _, token := range searchTokens(chunk.Text) {
				doc.Terms[token]++
				doc.Length++
			}
		}
		b.snapshot.Documents = append(b.snapshot.Documents, doc)
		b.totalLength += doc.Length
		for term, tf := range doc.Terms {
			if b.postings[term] == nil {
				b.postings[term] = make(map[int]int)
			}
			b.postings[term][i] = tf
		}
	}
	return b
}

func (b *bm25Index) search(query string, topK int) []scoredChunk {
	if b.totalLength == 0 {
		return nil
	}
	terms := make(map[string]struct{})
	for _, term := range searchTokens(query) {
		terms[term] = struct{}{}
	}
	queryTerms := make([]string, 0, len(terms))
	for term := range terms {
		queryTerms = append(queryTerms, term)
	}
	sort.Strings(queryTerms)
	n := float64(len(b.snapshot.Documents))
	avgLength := float64(b.totalLength) / n
	const k1, lengthWeight = 1.2, 0.75
	scores := make(map[int]float64)
	for _, term := range queryTerms {
		posting := b.postings[term]
		df := float64(len(posting))
		idf := math.Log(1 + (n-df+0.5)/(df+0.5))
		for i, frequency := range posting {
			tf := float64(frequency)
			length := float64(b.snapshot.Documents[i].Length)
			scores[i] += idf * tf * (k1 + 1) / (tf + k1*(1-lengthWeight+lengthWeight*length/avgLength))
		}
	}
	ranked := make([]scoredChunk, 0, len(scores))
	for i, score := range scores {
		ranked = append(ranked, scoredChunk{index: i, score: score})
	}
	return sortChunks(ranked, topK)
}

func loadBM25(meta []ChunkMeta, cacheDir string) *bm25Index {
	var snapshot bm25Snapshot
	if cacheDir != "" {
		if data, err := os.ReadFile(filepath.Join(cacheDir, bm25FileName)); err == nil {
			// Unusable caches are rebuilt; they never make retrieval fail.
			if json.Unmarshal(data, &snapshot) != nil {
				snapshot = bm25Snapshot{}
			}
		}
	}
	b := buildBM25(meta, snapshot)
	if cacheDir != "" && b.dirty {
		saveBM25(cacheDir, b.snapshot)
	}
	return b
}

func saveBM25(dir string, snapshot bm25Snapshot) {
	data, err := json.Marshal(snapshot)
	if err != nil {
		return
	}
	file, err := os.CreateTemp(dir, ".bm25-*.tmp")
	if err != nil {
		return
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr == nil && closeErr == nil {
		_ = os.Rename(file.Name(), filepath.Join(dir, bm25FileName))
	}
}
