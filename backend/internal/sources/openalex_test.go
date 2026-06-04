package sources

import (
	"testing"
)

func TestReconstructAbstract(t *testing.T) {
	idx := map[string][]int{
		"tree": {0},
		"frogs": {1},
		"navigate": {2},
		"magnetic": {3},
		"fields": {4},
	}
	got := reconstructAbstract(idx)
	want := "tree frogs navigate magnetic fields"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
