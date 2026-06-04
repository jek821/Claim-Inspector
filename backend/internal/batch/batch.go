// Package batch implements the Anthropic Message Batches API.
// https://docs.anthropic.com/en/api/creating-message-batches
//
// Flow:
//  1. Submit() — POST /v1/messages/batches, returns a batch ID immediately
//  2. Poll()   — GET  /v1/messages/batches/:id, check processing_status
//  3. Results()— GET  /v1/messages/batches/:id/results, JSONL of results
package batch

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	baseURL          = "https://api.anthropic.com"
	anthropicVersion = "2023-06-01"
	pollInterval     = 10 * time.Second
	maxPollDuration  = 25 * time.Hour // batches expire after 24h
)

// Request is a single item in a batch submission.
type Request struct {
	CustomID string        `json:"custom_id"`
	Params   MessageParams `json:"params"`
}

// MessageParams mirrors the Messages API body fields we use.
type MessageParams struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system"`
	Messages  []Message `json:"messages"`
}

// Message is a single conversation turn.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// BatchStatus is the response from GET /v1/messages/batches/:id.
type BatchStatus struct {
	ID               string `json:"id"`
	ProcessingStatus string `json:"processing_status"` // in_progress | ended | canceling | canceled
	ResultsURL       string `json:"results_url"`
	RequestCounts    struct {
		Processing int `json:"processing"`
		Succeeded  int `json:"succeeded"`
		Errored    int `json:"errored"`
		Canceled   int `json:"canceled"`
		Expired    int `json:"expired"`
	} `json:"request_counts"`
	ExpiresAt string `json:"expires_at"`
}

// ResultItem is one line from the JSONL results stream.
type ResultItem struct {
	CustomID string `json:"custom_id"`
	Result   struct {
		Type    string `json:"type"` // succeeded | errored | canceled | expired
		Message *struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		} `json:"message"`
		Error *struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	} `json:"result"`
}

// Client wraps the Anthropic Batch API.
type Client struct {
	apiKey string
	http   *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{apiKey: apiKey, http: &http.Client{Timeout: 30 * time.Second}}
}

// Submit sends a batch of requests and returns the batch ID.
func (c *Client) Submit(ctx context.Context, requests []Request) (string, error) {
	body, err := json.Marshal(map[string]any{"requests": requests})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/v1/messages/batches", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	c.setHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("batch submit %d: %s", resp.StatusCode, string(raw))
	}

	var status BatchStatus
	if err := json.Unmarshal(raw, &status); err != nil {
		return "", fmt.Errorf("parse submit response: %w", err)
	}
	return status.ID, nil
}

// Status returns the current status of a batch.
func (c *Client) Status(ctx context.Context, batchID string) (*BatchStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		baseURL+"/v1/messages/batches/"+batchID, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("batch status %d: %s", resp.StatusCode, string(raw))
	}

	var status BatchStatus
	if err := json.Unmarshal(raw, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// Poll blocks until the batch ends or ctx is cancelled, reporting progress via progressFn.
// progressFn receives (succeeded, total int) on each poll tick.
func (c *Client) Poll(ctx context.Context, batchID string, progressFn func(succeeded, total int)) (*BatchStatus, error) {
	deadline := time.Now().Add(maxPollDuration)
	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("batch polling timed out after 25 hours")
		}

		status, err := c.Status(ctx, batchID)
		if err != nil {
			return nil, err
		}

		total := status.RequestCounts.Processing + status.RequestCounts.Succeeded +
			status.RequestCounts.Errored + status.RequestCounts.Canceled + status.RequestCounts.Expired
		if progressFn != nil {
			progressFn(status.RequestCounts.Succeeded, total)
		}

		if status.ProcessingStatus == "ended" {
			return status, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// Results fetches and parses the JSONL results for a completed batch.
// Returns a map of custom_id → ResultItem.
func (c *Client) Results(ctx context.Context, batchID string) (map[string]ResultItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		baseURL+"/v1/messages/batches/"+batchID+"/results", nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("batch results %d: %s", resp.StatusCode, string(raw))
	}

	results := make(map[string]ResultItem)
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1MB per line
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item ResultItem
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			continue // skip malformed lines
		}
		results[item.CustomID] = item
	}
	return results, scanner.Err()
}

func (c *Client) setHeaders(r *http.Request) {
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("x-api-key", c.apiKey)
	r.Header.Set("anthropic-version", anthropicVersion)
}
