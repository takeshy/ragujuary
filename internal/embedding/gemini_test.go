package embedding

import "testing"

func TestApplyGeminiEmbedding2TextConfig(t *testing.T) {
	text, taskType := applyGeminiEmbedding2TextConfig("gemini-embedding-2", "hello", TaskRetrievalQuery)
	if taskType != "" {
		t.Fatalf("taskType = %q, want empty", taskType)
	}
	if text != "task: search result | query: hello" {
		t.Fatalf("text = %q", text)
	}

	text, taskType = applyGeminiEmbedding2TextConfig("models/gemini-embedding-2", "hello", TaskRetrievalDocument)
	if taskType != "" {
		t.Fatalf("taskType with models/ prefix = %q, want empty", taskType)
	}
	if text != "title: none | text: hello" {
		t.Fatalf("document text = %q", text)
	}
}

func TestApplyGeminiEmbedding001KeepsTaskType(t *testing.T) {
	text, taskType := applyGeminiEmbedding2TextConfig("gemini-embedding-001", "hello", TaskRetrievalQuery)
	if text != "hello" {
		t.Fatalf("text = %q, want unchanged", text)
	}
	if taskType != string(TaskRetrievalQuery) {
		t.Fatalf("taskType = %q, want %q", taskType, TaskRetrievalQuery)
	}
}
