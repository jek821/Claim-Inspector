package claims

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"factchecker/internal/types"
)

const extractSystemPrompt = `You prepare text for automated fact-checking against Wikipedia, Semantic Scholar, and PubMed.

Return ONLY valid JSON (no markdown, no backticks) with this shape:
{
  "document": {
    "topic": "main subject in 5-12 words",
    "domain": "one of: general, science, medicine, history, technology, law, economics, biography, geography",
    "entities": ["important proper nouns and technical terms from the whole document"],
    "summary": "1-2 sentences describing what the document is about"
  },
  "claims": [
    {
      "text": "single factual claim as exact/near-exact substring of the input",
      "local_context": "1-2 sentences from the document that disambiguate pronouns and topic",
      "entities": ["entities referenced in this claim"],
      "wikipedia_title": "exact English Wikipedia article title when confident, else empty string",
      "wiki_search_query": "short focused phrase for Wikipedia search (include subject + attribute)",
      "scholar_query": "academic search phrase or empty if not a research/science claim",
      "pubmed_query": "PubMed search phrase or empty if not a health/biology/medical claim",
      "providers": ["wikipedia"] or ["wikipedia","semantic_scholar"] or ["wikipedia","pubmed"], etc.
    }
  ]
}

Rules:
- Extract only checkable factual claims (not opinions).
- Each claim text must be traceable to the original wording.
- local_context must resolve "it", "they", "this", etc. using surrounding document text.
- wikipedia_title: only when you are confident (e.g. "World War II", "Mitochondria"). Never guess pop-culture titles unrelated to the document topic.
- providers: always include "wikipedia". Add "semantic_scholar" only for science/research/technical claims. Add "pubmed" only for medical, health, drug, disease, or clinical biology claims. Do not add scholarly/medical providers for history, geography, or general news unless clearly relevant.
- Queries must stay on the document topic — do not drift to unrelated subjects.`

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type analysisPayload struct {
	Document types.DocumentContext `json:"document"`
	Claims   []types.EnrichedClaim `json:"claims"`
}

// ExtractResult bundles document context, enriched claims, and token usage.
type ExtractResult struct {
	Document     types.DocumentContext
	Claims       []types.EnrichedClaim
	InputTokens  int
	OutputTokens int
}

// Extractor pulls atomic claims and lookup metadata from a passage of text.
type Extractor struct {
	apiKey string
	client *http.Client
}

func NewExtractor(apiKey string) *Extractor {
	return &Extractor{apiKey: apiKey, client: &http.Client{}}
}

// Extract calls Claude Haiku to analyze the document and produce enriched claims.
func (e *Extractor) Extract(ctx context.Context, text string) (ExtractResult, error) {
	body, _ := json.Marshal(anthropicRequest{
		Model:     "claude-haiku-4-5-20251001",
		MaxTokens: 4000,
		System:    extractSystemPrompt,
		Messages:  []anthropicMessage{{Role: "user", Content: text}},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return ExtractResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", e.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := e.client.Do(req)
	if err != nil {
		return ExtractResult{}, err
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return ExtractResult{}, fmt.Errorf("anthropic API error %d: %s", resp.StatusCode, string(rawBody))
	}

	var ar anthropicResponse
	if err := json.Unmarshal(rawBody, &ar); err != nil {
		return ExtractResult{}, err
	}
	if len(ar.Content) == 0 {
		return ExtractResult{}, fmt.Errorf("empty response from extractor")
	}

	raw := strings.TrimSpace(ar.Content[0].Text)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var payload analysisPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return ExtractResult{}, fmt.Errorf("failed to parse analysis: %w — raw: %s", err, raw)
	}

	claims := normalizeClaims(payload.Claims)
	if len(claims) == 0 {
		return ExtractResult{}, fmt.Errorf("no claims extracted")
	}

	return ExtractResult{
		Document:     payload.Document,
		Claims:       claims,
		InputTokens:  ar.Usage.InputTokens,
		OutputTokens: ar.Usage.OutputTokens,
	}, nil
}

func normalizeClaims(in []types.EnrichedClaim) []types.EnrichedClaim {
	var out []types.EnrichedClaim
	for _, c := range in {
		c.Text = strings.TrimSpace(c.Text)
		if c.Text == "" {
			continue
		}
		if c.WikiSearchQuery == "" {
			c.WikiSearchQuery = c.Text
		}
		if len(c.Providers) == 0 {
			c.Providers = []string{"wikipedia"}
		}
		out = append(out, c)
	}
	return out
}
