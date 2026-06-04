package sources

import (
	"context"
	"sync"
	"testing"

	"factchecker/internal/types"
)

func TestRunCache_deduplicatesConcurrentFetch(t *testing.T) {
	ctx := WithRunCache(context.Background(), NewRunCache())
	var calls int
	var mu sync.Mutex
	fetch := func() []types.Source {
		mu.Lock()
		calls++
		mu.Unlock()
		return []types.Source{{Title: "A", URL: "https://example.com/a", Body: "body"}}
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := runCacheFrom(ctx)
			_ = c.sourcesFor("openalex:test", fetch)
		}()
	}
	wg.Wait()
	if calls != 1 {
		t.Fatalf("expected 1 fetch, got %d", calls)
	}
}

func TestRunCache_wikiArticle(t *testing.T) {
	c := NewRunCache()
	c.storeWikiArticle("wiki:title:frog", types.Source{Title: "Frog", Body: "amphibian"})
	if _, ok := c.wikiArticle("wiki:title:frog"); !ok {
		t.Fatal("expected cached article")
	}
}
