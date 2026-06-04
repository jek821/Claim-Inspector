package claims

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const extractSystemPrompt = `You are a claim extraction assistant. Break the user's text into individual, atomic, verifiable factual claims. Each claim should be a single checkable assertion.

Rules:
- Extract only factual claims (not opinions or vague statements)
- Keep each claim as close to the original wording as possible — it must be an exact or near-exact substring of the original
- Return ONLY a JSON array of strings, no markdown, no backticks, no explanation

Example output:
["The Eiffel Tower is 330 meters tall.", "It was completed in 1889."]`

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

// ExtractResult bundles extracted claims with exact token usage.
type ExtractResult struct {
	Claims       []string
	InputTokens  int
	OutputTokens int
}

// Extractor pulls atomic claims from a passage of text.
type Extractor struct {
	apiKey string
	client *http.Client
}

func NewExtractor(apiKey string) *Extractor {
	return &Extractor{apiKey: apiKey, client: &http.Client{}}
}

// Extract calls Claude Haiku to break text into atomic claims.
func (e *Extractor) Extract(ctx context.Context, text string) (ExtractResult, error) {
	body, _ := json.Marshal(anthropicRequest{
		Model:     "claude-haiku-4-5-20251001",
		MaxTokens: 1000,
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

	var extracted []string
	if err := json.Unmarshal([]byte(raw), &extracted); err != nil {
		return ExtractResult{}, fmt.Errorf("failed to parse claim list: %w — raw: %s", err, raw)
	}

	return ExtractResult{
		Claims:       extracted,
		InputTokens:  ar.Usage.InputTokens,
		OutputTokens: ar.Usage.OutputTokens,
	}, nil
}
