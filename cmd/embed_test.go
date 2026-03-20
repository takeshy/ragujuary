package cmd

import "testing"

func TestGetEmbeddingAPIKey(t *testing.T) {
	original := embedAPIKey
	t.Cleanup(func() {
		embedAPIKey = original
	})

	embedAPIKey = ""
	t.Setenv("RAGUJUARY_EMBED_API_KEY", "ragu-key")
	t.Setenv("OPENAI_API_KEY", "openai-key")

	if got := getEmbeddingAPIKey(); got != "ragu-key" {
		t.Fatalf("getEmbeddingAPIKey() = %q, want %q", got, "ragu-key")
	}

	embedAPIKey = "flag-key"
	if got := getEmbeddingAPIKey(); got != "flag-key" {
		t.Fatalf("getEmbeddingAPIKey() with flag = %q, want %q", got, "flag-key")
	}
}

func TestGetServeEmbeddingAPIKey(t *testing.T) {
	original := serveEmbedAPIKey
	t.Cleanup(func() {
		serveEmbedAPIKey = original
	})

	serveEmbedAPIKey = ""
	t.Setenv("RAGUJUARY_EMBED_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "openai-key")

	if got := getServeEmbeddingAPIKey(); got != "openai-key" {
		t.Fatalf("getServeEmbeddingAPIKey() = %q, want %q", got, "openai-key")
	}

	serveEmbedAPIKey = "flag-key"
	if got := getServeEmbeddingAPIKey(); got != "flag-key" {
		t.Fatalf("getServeEmbeddingAPIKey() with flag = %q, want %q", got, "flag-key")
	}
}
