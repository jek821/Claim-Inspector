package sources

import (
	"context"
	"strings"
	"sync"

	"factchecker/internal/types"
)

type runCacheKey struct{}

// RunCache deduplicates outbound source fetches within a single analysis run.
type RunCache struct {
	mu       sync.Mutex
	sources  map[string][]types.Source
	articles map[string]types.Source // wiki title key → article with Body (Snippet omitted)
	inflight map[string]chan struct{}
}

// NewRunCache creates a per-run fetch deduplication cache.
func NewRunCache() *RunCache {
	return &RunCache{
		sources:  make(map[string][]types.Source),
		articles: make(map[string]types.Source),
		inflight: make(map[string]chan struct{}),
	}
}

// WithRunCache attaches a RunCache to ctx for the duration of one analysis.
func WithRunCache(ctx context.Context, c *RunCache) context.Context {
	if c == nil {
		return ctx
	}
	return context.WithValue(ctx, runCacheKey{}, c)
}

func runCacheFrom(ctx context.Context) *RunCache {
	c, _ := ctx.Value(runCacheKey{}).(*RunCache)
	return c
}

func cacheKey(kind, value string) string {
	return kind + ":" + strings.ToLower(strings.TrimSpace(value))
}

func cloneSources(in []types.Source) []types.Source {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.Source, len(in))
	copy(out, in)
	return out
}

// sourcesFor runs fetch once per key; concurrent callers wait for the first result.
func (c *RunCache) sourcesFor(key string, fetch func() []types.Source) []types.Source {
	if c == nil {
		return fetch()
	}

	c.mu.Lock()
	if s, ok := c.sources[key]; ok {
		c.mu.Unlock()
		return cloneSources(s)
	}
	if ch, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		<-ch
		c.mu.Lock()
		s := c.sources[key]
		c.mu.Unlock()
		return cloneSources(s)
	}
	ch := make(chan struct{})
	c.inflight[key] = ch
	c.mu.Unlock()

	s := fetch()

	c.mu.Lock()
	c.sources[key] = s
	delete(c.inflight, key)
	close(ch)
	c.mu.Unlock()
	return cloneSources(s)
}

func (c *RunCache) wikiArticle(key string) (types.Source, bool) {
	if c == nil {
		return types.Source{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.articles[key]
	return s, ok
}

func (c *RunCache) storeWikiArticle(key string, s types.Source) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if prev, ok := c.articles[key]; ok && len(prev.Body) > len(s.Body) {
		return
	}
	c.articles[key] = s
}
