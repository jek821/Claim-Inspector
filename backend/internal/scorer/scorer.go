package scorer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"factchecker/internal/batch"
	"factchecker/internal/types"
)

const scoreSystemPrompt = `You are a fact-checking assistant. You will be given a factual claim and reference sources retrieved from Wikipedia and academic databases.

Your job:
1. Use the sources as primary evidence. If they directly address the claim, base your rating on them.
2. If the sources are off-topic or too shallow, fall back to your own training knowledge to assess the claim.
3. Only use "unverifiable" when you genuinely cannot assess the claim from either the sources or your own knowledge.
4. Assign a risk level:
   - "verified": claim is well-supported (by sources or your knowledge)
   - "low": mostly accurate with minor nuance
   - "medium": partially accurate, oversimplified, or disputed
   - "high": contradicts sources or your knowledge, or appears fabricated
   - "unverifiable": cannot be assessed even with your knowledge
5. Write a 1-2 sentence explanation. If you relied on your own knowledge instead of the sources, say so briefly.

Respond ONLY with a JSON object. No markdown, no backticks:
{"risk": "verified|low|medium|high|unverifiable", "explanation": "..."}`

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

type scoreResult struct {
	Risk        string `json:"risk"`
	Explanation string `json:"explanation"`
}

// ScoreResult bundles the scoring output with exact token usage.
type ScoreResult struct {
	Risk         string
	Explanation  string
	InputTokens  int
	OutputTokens int
}

// Scorer uses Haiku to score a claim against retrieved sources.
type Scorer struct {
	apiKey string
	client *http.Client
}

func NewScorer(apiKey string) *Scorer {
	return &Scorer{apiKey: apiKey, client: &http.Client{}}
}

// BuildParams builds the batch.MessageParams for a single claim — used by the Batch API flow.
func (s *Scorer) BuildParams(claim string, srcs []types.Source) batch.MessageParams {
	return batch.MessageParams{
		Model:     "claude-haiku-4-5-20251001",
		MaxTokens: 200,
		System:    scoreSystemPrompt,
		Messages:  []batch.Message{{Role: "user", Content: buildUserMsg(claim, srcs)}},
	}
}

// ParseScoreText parses the raw JSON text from a scoring response (used for batch results).
func (s *Scorer) ParseScoreText(text string) (risk, explanation string) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)
	var result scoreResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return "unverifiable", "Could not parse scoring result."
	}
	return result.Risk, result.Explanation
}

// Score scores a single claim synchronously and returns exact token usage.
func (s *Scorer) Score(ctx context.Context, claim string, srcs []types.Source) (ScoreResult, error) {
	body, _ := json.Marshal(anthropicRequest{
		Model:     "claude-haiku-4-5-20251001",
		MaxTokens: 200,
		System:    scoreSystemPrompt,
		Messages:  []anthropicMessage{{Role: "user", Content: buildUserMsg(claim, srcs)}},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return ScoreResult{Risk: "unverifiable"}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", s.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := s.client.Do(req)
	if err != nil {
		return ScoreResult{Risk: "unverifiable"}, err
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return ScoreResult{Risk: "unverifiable"}, fmt.Errorf("scorer API error %d: %s", resp.StatusCode, string(rawBody))
	}

	var ar anthropicResponse
	if err := json.Unmarshal(rawBody, &ar); err != nil {
		return ScoreResult{Risk: "unverifiable"}, err
	}
	if len(ar.Content) == 0 {
		return ScoreResult{Risk: "unverifiable"}, fmt.Errorf("empty scorer response")
	}

	risk, explanation := s.ParseScoreText(ar.Content[0].Text)
	return ScoreResult{
		Risk:         risk,
		Explanation:  explanation,
		InputTokens:  ar.Usage.InputTokens,
		OutputTokens: ar.Usage.OutputTokens,
	}, nil
}

func buildUserMsg(claim string, srcs []types.Source) string {
	var sb strings.Builder
	sb.WriteString("Claim: ")
	sb.WriteString(claim)
	sb.WriteString("\n\nSources:\n")
	if len(srcs) == 0 {
		sb.WriteString("(no sources found)")
	} else {
		for i, src := range srcs {
			fmt.Fprintf(&sb, "%d. %s\n   %s\n\n", i+1, src.Title, src.Snippet)
		}
	}
	return sb.String()
}
