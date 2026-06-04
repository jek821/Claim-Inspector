package corpus

import (
	"strings"
	"unicode"
)

const (
	DefaultChunkSize    = 750
	DefaultChunkOverlap = 150
)

// Chunk is a passage from a source document with citation metadata.
type Chunk struct {
	Text     string
	Title    string
	URL      string
	Provider string
	Index    int // position within source
}

// Split breaks plain text into overlapping chunks for indexing and retrieval.
func Split(text string, size, overlap int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if size <= 0 {
		size = DefaultChunkSize
	}
	if overlap < 0 || overlap >= size {
		overlap = DefaultChunkOverlap
	}
	if len(text) <= size {
		return []string{text}
	}
	var chunks []string
	start := 0
	for start < len(text) {
		end := start + size
		if end > len(text) {
			end = len(text)
		}
		piece := strings.TrimSpace(text[start:end])
		if piece != "" {
			chunks = append(chunks, piece)
		}
		if end >= len(text) {
			break
		}
		start = end - overlap
	}
	return chunks
}

// ChunkSource splits a source body into indexed chunks.
func ChunkSource(title, url, provider, body string) []Chunk {
	parts := Split(body, DefaultChunkSize, DefaultChunkOverlap)
	out := make([]Chunk, len(parts))
	for i, p := range parts {
		out[i] = Chunk{
			Text:     p,
			Title:    title,
			URL:      url,
			Provider: provider,
			Index:    i,
		}
	}
	return out
}

// Tokenize splits text into lowercase terms for lexical retrieval.
func Tokenize(s string) map[string]int {
	counts := make(map[string]int)
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		w := strings.ToLower(b.String())
		if len(w) >= 3 && !isStopword(w) {
			counts[w]++
		}
		b.Reset()
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return counts
}

func isStopword(w string) bool {
	switch w {
	case "the", "and", "for", "that", "this", "with", "from", "have", "been",
		"were", "when", "where", "which", "their", "there", "about", "into",
		"also", "than", "then", "them", "they", "these", "those", "some", "such":
		return true
	default:
		return false
	}
}
