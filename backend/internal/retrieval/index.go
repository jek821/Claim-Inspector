package retrieval

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"factchecker/internal/corpus"
	"factchecker/internal/types"
	"factchecker/internal/usage"
)

const defaultTopK = 4

// TopK is the default number of passages retrieved per claim.
func TopK() int { return defaultTopK }

type indexedChunk struct {
	corpus.Chunk
	vec        []float32
	termCounts map[string]int
}

// Index holds chunked, embedded source passages for a single analysis run.
type Index struct {
	embed Embedder
	cache *DiskCache
	items []indexedChunk
}

// NewIndex builds a run-scoped retrieval index. Uses Voyage when apiKey is set; always uses lexical scoring.
func NewIndex(voyageAPIKey string, cacheDir string) *Index {
	var embed Embedder = noopEmbedder{}
	if v := NewVoyageEmbedder(voyageAPIKey); v != nil {
		embed = v
	}
	return &Index{
		embed: embed,
		cache: NewDiskCache(cacheDir),
	}
}

type noopEmbedder struct{}

func (noopEmbedder) Available() bool { return false }
func (noopEmbedder) Embed(context.Context, []string, string) ([][]float32, error) {
	return nil, nil
}

// AddSources chunks and indexes unique sources (by URL). Uses disk cache when embeddings are available.
func (idx *Index) AddSources(ctx context.Context, sources []types.Source) error {
	byURL := make(map[string]types.Source)
	for _, s := range sources {
		if s.URL == "" {
			continue
		}
		body := s.Body
		if body == "" {
			body = s.Snippet
		}
		if len(body) < 80 {
			continue
		}
		prev, ok := byURL[s.URL]
		if !ok || len(body) > len(prev.Body) {
			s.Body = body
			byURL[s.URL] = s
		}
	}

	for _, src := range byURL {
		if err := idx.indexOne(ctx, src); err != nil {
			return err
		}
	}
	return nil
}

func (idx *Index) indexOne(ctx context.Context, src types.Source) error {
	chunks := corpus.ChunkSource(src.Title, src.URL, src.Provider, src.Body)
	if len(chunks) == 0 {
		return nil
	}

	var vecs [][]float32
	if idx.embed.Available() {
		if cached, ok := idx.cache.Load(src.URL); ok && len(cached.Chunks) == len(chunks) {
			match := true
			for i, c := range chunks {
				if cached.Chunks[i].Text != c.Text {
					match = false
					break
				}
			}
			if match && len(cached.Chunks[0].Embedding) > 0 {
				vecs = make([][]float32, len(cached.Chunks))
				for i, cc := range cached.Chunks {
					vecs[i] = cc.Embedding
				}
			}
		}
		if vecs == nil {
			texts := make([]string, len(chunks))
			for i, c := range chunks {
				texts[i] = c.Text
			}
			embedded, err := idx.embed.Embed(ctx, texts, "document")
			if err != nil {
				return fmt.Errorf("embed %s: %w", src.Title, err)
			}
			usage.RecordVoyageEmbedCtx(ctx, texts, 1)
			vecs = embedded
			if idx.cache != nil {
				cs := &cachedSource{URL: src.URL, Chunks: make([]cachedChunk, len(chunks))}
				for i, c := range chunks {
					cs.Chunks[i] = cachedChunk{Text: c.Text, Embedding: vecs[i]}
				}
				_ = idx.cache.Save(cs)
			}
		}
	}

	for i, c := range chunks {
		idx.items = append(idx.items, indexedChunk{
			Chunk:      c,
			vec:        vecAt(vecs, i),
			termCounts: corpus.Tokenize(c.Text),
		})
	}
	return nil
}

func vecAt(vecs [][]float32, i int) []float32 {
	if i < len(vecs) {
		return vecs[i]
	}
	return nil
}

type scored struct {
	item  indexedChunk
	score float32
}

// Retrieve returns the top-k passages most relevant to a claim.
func (idx *Index) Retrieve(ctx context.Context, claim types.EnrichedClaim, doc types.DocumentContext, k int) []types.Source {
	if k <= 0 {
		k = defaultTopK
	}
	if len(idx.items) == 0 {
		return nil
	}

	query := strings.TrimSpace(claim.Text + " " + claim.LocalContext + " " + doc.Topic)
	qTokens := claimQueryTokens(claim.Text, claim.LocalContext)
	for _, e := range claim.Entities {
		for term, n := range corpus.Tokenize(e) {
			qTokens[term] += n
		}
	}

	var queryVec []float32
	if idx.embed.Available() {
		vecs, err := idx.embed.Embed(ctx, []string{query}, "query")
		if err == nil && len(vecs) > 0 {
			usage.RecordVoyageEmbedCtx(ctx, []string{query}, 1)
			queryVec = vecs[0]
		}
	}

	scores := make([]scored, len(idx.items))
	for i, item := range idx.items {
		lex := lexicalScore(qTokens, item.termCounts)
		var sem float32
		if len(queryVec) > 0 && len(item.vec) > 0 {
			sem = cosine(queryVec, item.vec)
		}
		var combined float32
		switch {
		case sem > 0 && lex > 0:
			combined = 0.55*sem + 0.45*lex
		case sem > 0:
			combined = sem
		default:
			combined = lex
		}
		scores[i] = scored{item: item, score: combined}
	}

	sort.Slice(scores, func(i, j int) bool { return scores[i].score > scores[j].score })

	var out []types.Source
	for i := 0; i < len(scores) && len(out) < k; i++ {
		if scores[i].score <= 0 {
			break
		}
		c := scores[i].item
		label := c.Title
		if c.Index > 0 {
			label = fmt.Sprintf("%s (excerpt %d)", c.Title, c.Index+1)
		}
		out = append(out, types.Source{
			Title:    label,
			URL:      c.URL,
			Snippet:  c.Text,
			Provider: c.Provider,
		})
	}
	return out
}
