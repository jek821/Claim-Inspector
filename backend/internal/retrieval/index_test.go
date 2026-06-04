package retrieval

import (
	"context"
	"strings"
	"testing"

	"factchecker/internal/types"
)

func TestRetrieve_lexicalFindsRelevantChunk(t *testing.T) {
	idx := NewIndex("", "")
	src := types.Source{
		Title:    "Frog",
		URL:      "https://en.wikipedia.org/wiki/Frog",
		Provider: "wikipedia",
		Body: stringsRepeat("General intro about amphibians. ", 30) +
			"The fossil record indicates the earliest known frog relatives appeared roughly 190 million years ago during the Triassic period. " +
			stringsRepeat("Ecology and habitat information. ", 20),
	}
	if err := idx.AddSources(context.Background(), []types.Source{src}); err != nil {
		t.Fatal(err)
	}
	claim := types.EnrichedClaim{
		Text:         "The earliest known frog appeared roughly 190 million years ago",
		LocalContext: "Frogs have shared the Earth with dinosaurs",
		Entities:     []string{"frog", "190 million years"},
	}
	passages := idx.Retrieve(context.Background(), claim, types.DocumentContext{Topic: "frogs"}, 3)
	if len(passages) == 0 {
		t.Fatal("expected retrieved passages")
	}
	found := false
	for _, p := range passages {
		if strings.Contains(p.Snippet, "190 million") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected fossil passage in results: %+v", passages)
	}
}

func stringsRepeat(s string, n int) string {
	return strings.Repeat(s, n)
}
