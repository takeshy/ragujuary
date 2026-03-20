package rag

import "regexp"

// Chunk represents a text chunk with its position in the source document
type Chunk struct {
	Text        string
	StartOffset int
}

var headingPattern = regexp.MustCompile(`(?m)^(#{1,6})\s+(.+)$`)
var sentencePattern = regexp.MustCompile(`[.]\s|[。！？]`)

// ChunkText splits text into chunks with smart boundary breaking.
// Respects paragraph breaks, sentence boundaries (English and Japanese),
// and configurable overlap for context preservation.
func ChunkText(text string, chunkSize, chunkOverlap int) []Chunk {
	var chunks []Chunk
	start := 0

	for start < len(text) {
		end := start + chunkSize

		// Try to break at a natural boundary
		if end < len(text) {
			// Try paragraph break first
			region := text[start:end]
			paragraphBreak := lastIndex(region, "\n\n")
			if paragraphBreak > chunkSize/2 {
				end = start + paragraphBreak
			} else {
				// Try sentence boundary in the second half
				halfPoint := chunkSize / 2
				if halfPoint < len(region) {
					subRegion := region[halfPoint:]
					matches := sentencePattern.FindAllStringIndex(subRegion, -1)
					if len(matches) > 0 {
						last := matches[len(matches)-1]
						end = start + halfPoint + last[1]
					}
				}
			}
		} else {
			end = len(text)
		}

		chunkText := trimSpace(text[start:end])
		if chunkText != "" {
			chunks = append(chunks, Chunk{
				Text:        chunkText,
				StartOffset: start,
			})
		}

		nextStart := end - chunkOverlap
		if len(chunks) > 0 && nextStart <= chunks[len(chunks)-1].StartOffset {
			nextStart = end // Prevent infinite loop
		}
		start = nextStart
	}

	return chunks
}

// FindNearestHeading returns the nearest Markdown heading before the given offset.
func FindNearestHeading(text string, offset int) string {
	matches := headingPattern.FindAllStringSubmatchIndex(text, -1)
	var lastHeading string
	for _, match := range matches {
		if match[0] > offset {
			break
		}
		// match[4] and match[5] are the capture group for the heading text
		if match[4] >= 0 && match[5] >= 0 {
			lastHeading = text[match[4]:match[5]]
		}
	}
	return lastHeading
}

// lastIndex returns the last occurrence of substr in s, or -1 if not found.
func lastIndex(s, substr string) int {
	result := -1
	offset := 0
	for {
		idx := indexOf(s[offset:], substr)
		if idx < 0 {
			break
		}
		result = offset + idx
		offset = result + len(substr)
	}
	return result
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
