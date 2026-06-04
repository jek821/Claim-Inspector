package retrieval

import (
	"context"
	"math"
)

// Embedder produces dense vectors for semantic search.
type Embedder interface {
	Embed(ctx context.Context, texts []string, inputType string) ([][]float32, error)
	Available() bool
}

func cosine(a, b []float32) float32 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float32
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (float32(math.Sqrt(float64(na))) * float32(math.Sqrt(float64(nb))))
}
