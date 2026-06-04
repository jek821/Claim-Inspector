package retrieval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const voyageEmbedURL = "https://api.voyageai.com/v1/embeddings"

// VoyageEmbedder calls the Voyage embeddings API (optional; set VOYAGE_API_KEY).
type VoyageEmbedder struct {
	client *http.Client
	apiKey string
	model  string
}

func NewVoyageEmbedder(apiKey string) *VoyageEmbedder {
	if apiKey == "" {
		return nil
	}
	return &VoyageEmbedder{
		client: &http.Client{},
		apiKey: apiKey,
		model:  "voyage-4-lite",
	}
}

func (v *VoyageEmbedder) Available() bool { return v != nil && v.apiKey != "" }

type voyageEmbedReq struct {
	Input     []string `json:"input"`
	Model     string   `json:"model"`
	InputType string   `json:"input_type,omitempty"`
}

type voyageEmbedResp struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (v *VoyageEmbedder) Embed(ctx context.Context, texts []string, inputType string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	body, _ := json.Marshal(voyageEmbedReq{
		Input:     texts,
		Model:     v.model,
		InputType: inputType,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, voyageEmbedURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+v.apiKey)

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("voyage embed %d: %s", resp.StatusCode, string(raw))
	}
	var er voyageEmbedResp
	if err := json.Unmarshal(raw, &er); err != nil {
		return nil, err
	}
	if er.Error != nil {
		return nil, fmt.Errorf("voyage: %s", er.Error.Message)
	}
	out := make([][]float32, len(er.Data))
	for i, d := range er.Data {
		vec := make([]float32, len(d.Embedding))
		for j, x := range d.Embedding {
			vec[j] = float32(x)
		}
		out[i] = vec
	}
	return out, nil
}
