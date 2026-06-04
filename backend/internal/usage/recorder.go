package usage

import (
	"context"
	"sync"

	"factchecker/internal/apilimits"
)

type ctxKey struct{}

// Recorder accumulates API usage for one analysis run (thread-safe).
type Recorder struct {
	mu   sync.Mutex
	data map[apilimits.Provider]Counts
}

// Counts are raw counters merged into the persistent store.
type Counts struct {
	Requests     int64   `json:"requests,omitempty"`
	InputTokens  int64   `json:"input_tokens,omitempty"`
	OutputTokens int64   `json:"output_tokens,omitempty"`
	EmbedTokens  int64   `json:"embed_tokens,omitempty"`
	EstSpendUSD  float64 `json:"est_spend_usd,omitempty"`
}

// NewRecorder creates a run-scoped usage recorder.
func NewRecorder() *Recorder {
	return &Recorder{data: make(map[apilimits.Provider]Counts)}
}

// WithRecorder attaches a recorder to ctx for downstream packages.
func WithRecorder(ctx context.Context, r *Recorder) context.Context {
	return context.WithValue(ctx, ctxKey{}, r)
}

// FromContext returns the recorder or nil.
func FromContext(ctx context.Context) *Recorder {
	r, _ := ctx.Value(ctxKey{}).(*Recorder)
	return r
}

func (r *Recorder) add(p apilimits.Provider, fn func(*Counts)) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.data[p]
	fn(&c)
	r.data[p] = c
}

// RecordAnthropic records Haiku/Batch API token usage.
func (r *Recorder) RecordAnthropic(inputTok, outputTok int) {
	r.add(apilimits.Anthropic, func(c *Counts) {
		c.Requests++
		c.InputTokens += int64(inputTok)
		c.OutputTokens += int64(outputTok)
	})
}

// RecordHTTP records outbound HTTP calls to a free/public API.
func (r *Recorder) RecordHTTP(p apilimits.Provider, n int) {
	if n <= 0 {
		return
	}
	r.add(p, func(c *Counts) {
		c.Requests += int64(n)
	})
}

// RecordOpenAlexSearch records OpenAlex work search calls and estimated spend.
func (r *Recorder) RecordOpenAlexSearch(n int) {
	if n <= 0 {
		return
	}
	r.add(apilimits.OpenAlex, func(c *Counts) {
		c.Requests += int64(n)
		c.EstSpendUSD += float64(n) * apilimits.OpenAlexSearchCostUSD
	})
}

// RecordVoyageEmbed records embedding API usage (estimated tokens from text length).
func (r *Recorder) RecordVoyageEmbed(texts []string, apiCalls int) {
	if apiCalls <= 0 && len(texts) == 0 {
		return
	}
	var tokens int64
	for _, t := range texts {
		tokens += int64(EstimateTokens(t))
	}
	r.add(apilimits.Voyage, func(c *Counts) {
		if apiCalls > 0 {
			c.Requests += int64(apiCalls)
		}
		c.EmbedTokens += tokens
	})
}

// EstimateTokens approximates embedding tokens from character count.
func EstimateTokens(s string) int {
	n := len(s) / 4
	if n < 1 && len(s) > 0 {
		return 1
	}
	return n
}

// RecordHTTPCtx records HTTP usage when a recorder is present on ctx.
func RecordHTTPCtx(ctx context.Context, p apilimits.Provider, n int) {
	if r := FromContext(ctx); r != nil {
		r.RecordHTTP(p, n)
	}
}

// RecordOpenAlexSearchCtx records OpenAlex search usage.
func RecordOpenAlexSearchCtx(ctx context.Context, n int) {
	if r := FromContext(ctx); r != nil {
		r.RecordOpenAlexSearch(n)
	}
}

// RecordVoyageEmbedCtx records Voyage embedding usage.
func RecordVoyageEmbedCtx(ctx context.Context, texts []string, apiCalls int) {
	if r := FromContext(ctx); r != nil {
		r.RecordVoyageEmbed(texts, apiCalls)
	}
}

// RecordAnthropicCtx records Anthropic token usage.
func RecordAnthropicCtx(ctx context.Context, in, out int) {
	if r := FromContext(ctx); r != nil {
		r.RecordAnthropic(in, out)
	}
}

// Snapshot returns a copy of accumulated counts for this run.
func (r *Recorder) Snapshot() map[apilimits.Provider]Counts {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[apilimits.Provider]Counts, len(r.data))
	for k, v := range r.data {
		out[k] = v
	}
	return out
}
