package sources

import (
	"strings"
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

func TestPassesSubjectGate_dropsMothsForFrogClaim(t *testing.T) {
	claim := types.EnrichedClaim{
		Text:     "certain tree frog species can navigate using Earth's magnetic field at night",
		Entities: []string{"tree frog", "magnetic field"},
	}
	s := types.Source{
		Title:   "Lepidoptera",
		Snippet: "Moths lose navigational capacity when exposed to a magnetic field",
	}
	if passesSubjectGate(s, claim) {
		t.Fatal("Lepidoptera should not pass frog subject gate")
	}
}

func TestExcerptAroundKeywords_findsBuriedFact(t *testing.T) {
	full := strings.Repeat("Intro about frogs as amphibians. ", 20) +
		"The fossil record shows the earliest known frog relatives appeared roughly 190 million years ago in the Triassic. " +
		strings.Repeat("More general ecology. ", 30)
	passage := excerptAroundKeywords(full, []string{"190", "million", "dinosaur", "fossil"}, 200)
	if passage == "" {
		t.Fatal("expected keyword excerpt")
	}
	if !strings.Contains(passage, "190 million") {
		t.Fatalf("excerpt missing date fact: %s", passage)
	}
}
