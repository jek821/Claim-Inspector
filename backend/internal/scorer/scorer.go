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
	"factchecker/internal/usage"
)

const scoreSystemPrompt = `You are a fact-checking assistant. You will be given document context, a factual claim (with local context), and reference sources from Wikipedia and/or academic databases.

Sources are the top retrieved passages from chunked articles (vector + keyword search over full text). Read each passage carefully for dates, mechanisms, and specific facts.

Your job:
1. Use ONLY the provided sources as evidence. Do NOT use your own training knowledge.
2. Use the document topic and local context to judge whether sources are on-topic. Ignore sources about unrelated subjects (e.g. moths when the claim is about frogs).
3. If no on-topic excerpt contains information about the specific assertion, rate "unverifiable".
4. Assign a risk level:
   - "verified": on-topic sources directly and clearly support the claim
   - "low": sources mostly support the claim with minor nuance
   - "medium": sources partially support or the claim oversimplifies
   - "high": on-topic sources contradict the claim or show a major factual error
   - "unverifiable": no on-topic source addresses the claim
5. Write 1-2 sentences citing specific sources (or why they are off-topic/unhelpful).

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
func (s *Scorer) BuildParams(claim types.EnrichedClaim, doc types.DocumentContext, srcs []types.Source) batch.MessageParams {
	return batch.MessageParams{
		Model:     "claude-haiku-4-5-20251001",
		MaxTokens: 280,
		System:    scoreSystemPrompt,
		Messages:  []batch.Message{{Role: "user", Content: buildUserMsg(claim, doc, srcs)}},
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
func (s *Scorer) Score(ctx context.Context, claim types.EnrichedClaim, doc types.DocumentContext, srcs []types.Source) (ScoreResult, error) {
	body, _ := json.Marshal(anthropicRequest{
		Model:     "claude-haiku-4-5-20251001",
		MaxTokens: 280,
		System:    scoreSystemPrompt,
		Messages:  []anthropicMessage{{Role: "user", Content: buildUserMsg(claim, doc, srcs)}},
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
	usage.RecordAnthropicCtx(ctx, ar.Usage.InputTokens, ar.Usage.OutputTokens)
	return ScoreResult{
		Risk:         risk,
		Explanation:  explanation,
		InputTokens:  ar.Usage.InputTokens,
		OutputTokens: ar.Usage.OutputTokens,
	}, nil
}

func buildUserMsg(claim types.EnrichedClaim, doc types.DocumentContext, srcs []types.Source) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Document topic: %s\n", doc.Topic)
	fmt.Fprintf(&sb, "Document domain: %s\n", doc.Domain)
	if doc.Summary != "" {
		fmt.Fprintf(&sb, "Document summary: %s\n", doc.Summary)
	}
	sb.WriteString("\nClaim: ")
	sb.WriteString(claim.Text)
	if claim.LocalContext != "" {
		sb.WriteString("\nLocal context: ")
		sb.WriteString(claim.LocalContext)
	}
	sb.WriteString("\n\nSources:\n")
	if len(srcs) == 0 {
		sb.WriteString("(no on-topic sources found)")
	} else {
		for i, src := range srcs {
			fmt.Fprintf(&sb, "%d. [%s] %s\n   %s\n\n", i+1, src.Provider, src.Title, src.Snippet)
		}
	}
	return sb.String()
}
