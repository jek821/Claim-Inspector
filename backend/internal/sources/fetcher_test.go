package sources

import (
	"testing"

	"factchecker/internal/types"
)

func TestFilterRelevant_dropsOffTopic(t *testing.T) {
	claim := types.EnrichedClaim{
		Text:         "Mitochondria produce ATP through oxidative phosphorylation.",
		LocalContext: "In human cells, mitochondria are the main site of ATP synthesis.",
		Entities:     []string{"mitochondria", "ATP"},
	}
	doc := types.DocumentContext{
		Topic:    "cellular respiration",
		Entities: []string{"mitochondria", "ATP", "cells"},
	}
	sources := []types.Source{
		{Title: "He-Man and the Masters of the Universe", Snippet: "American animated television series", Provider: "wikipedia"},
		{Title: "Mitochondrion", Snippet: "Mitochondria generate most of the cell's ATP via oxidative phosphorylation", Provider: "wikipedia"},
	}
	out := filterRelevant(sources, claim, doc)
	if len(out) != 1 {
		t.Fatalf("expected 1 relevant source, got %d: %+v", len(out), out)
	}
	if out[0].Title != "Mitochondrion" {
		t.Fatalf("unexpected source kept: %s", out[0].Title)
	}
}
