package api

import (
	"context"
	"sync"

	"factchecker/internal/retrieval"
	"factchecker/internal/sources"
	"factchecker/internal/types"
)

// gatherAndIndex fetches sources for all claims, deduplicates by URL, and builds a vector/lexical index.
func (h *Handler) gatherAndIndex(ctx context.Context, claims []types.EnrichedClaim, doc types.DocumentContext) (*retrieval.Index, error) {
	ctx = sources.WithRunCache(ctx, sources.NewRunCache())
	byURL := make(map[string]types.Source)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, ec := range claims {
		wg.Add(1)
		go func(claim types.EnrichedClaim) {
			defer wg.Done()
			for _, s := range h.fetcher.FetchAll(ctx, claim, doc) {
				if s.URL == "" {
					continue
				}
				mu.Lock()
				prev, ok := byURL[s.URL]
				bodyLen := len(s.Body)
				if !ok || bodyLen > len(prev.Body) {
					byURL[s.URL] = s
				}
				mu.Unlock()
			}
		}(ec)
	}
	wg.Wait()

	unique := make([]types.Source, 0, len(byURL))
	for _, s := range byURL {
		unique = append(unique, s)
	}

	idx := retrieval.NewIndex(h.voyageKey, h.corpusCacheDir)
	if err := idx.AddSources(ctx, unique); err != nil {
		return nil, err
	}
	return idx, nil
}

func (h *Handler) sourcesForClaim(ctx context.Context, idx *retrieval.Index, claim types.EnrichedClaim, doc types.DocumentContext) []types.Source {
	if idx == nil {
		return h.fetcher.FetchAll(ctx, claim, doc)
	}
	if passages := idx.Retrieve(ctx, claim, doc, retrieval.TopK()); len(passages) > 0 {
		return passages
	}
	return h.fetcher.FetchAll(ctx, claim, doc)
}
