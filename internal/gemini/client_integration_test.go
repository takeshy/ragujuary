package gemini

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// Opt in explicitly because this test creates and removes its own cloud store.
func TestIntegrationCurrentModelFileSearch(t *testing.T) {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" || os.Getenv("RAGUJUARY_FILESEARCH_INTEGRATION") != "1" {
		t.Skip("requires GEMINI_API_KEY and RAGUJUARY_FILESEARCH_INTEGRATION=1")
	}
	client := NewClient(key)
	client.httpClient.Timeout = 45 * time.Second
	store, err := client.CreateFileSearchStore(fmt.Sprintf("ragujuary-model-test-%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := client.DeleteFileSearchStore(store.Name, true); err != nil {
			t.Errorf("cleanup of test store %s failed: %v", store.Name, err)
		}
	}()
	operation, err := client.UploadContentToFileSearchStore(store.Name, "test-manual.txt", []byte("The recovery code for Project Lantern is RAGUJUARY-4317. This is the only recovery code in the project manual."))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(45 * time.Second)
	for !operation.Done {
		if time.Now().After(deadline) {
			t.Fatal("test document indexing timed out")
		}
		time.Sleep(time.Second)
		operation, err = client.GetOperation(operation.Name)
		if err != nil {
			t.Fatal(err)
		}
	}
	if operation.Error != nil {
		t.Fatalf("indexing failed: %+v", operation.Error)
	}
	response, err := client.Query("", "Find the recovery code for Project Lantern in the uploaded manual. Answer with the code only.", []string{store.Name}, "")
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	citations := 0
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			text.WriteString(part.Text)
		}
		if candidate.GroundingMetadata != nil {
			citations += len(candidate.GroundingMetadata.GroundingChunks)
		}
	}
	if !strings.Contains(text.String(), "RAGUJUARY-4317") || citations == 0 {
		t.Fatalf("expected grounded answer, got %q (%d citations)", text.String(), citations)
	}
	t.Logf("model=%s, answer=%s, citations=%d", DefaultQueryModel, text.String(), citations)
}
