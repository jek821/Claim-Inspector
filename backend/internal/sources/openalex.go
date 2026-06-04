package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"factchecker/internal/types"
	"factchecker/internal/usage"
)

const openAlexWorksURL = "https://api.openalex.org/works"
const openAlexMailto = "factchecker@example.com"

type openAlexWorksResp struct {
	Results []openAlexWork `json:"results"`
}

type openAlexWork struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	DOI         string `json:"doi"`
	PublicationYear int `json:"publication_year"`
	AbstractInvertedIndex map[string][]int `json:"abstract_inverted_index"`
	PrimaryLocation *struct {
		LandingPageURL string `json:"landing_page_url"`
		Source *struct {
			DisplayName string `json:"display_name"`
		} `json:"source"`
	} `json:"primary_location"`
	OpenAccess *struct {
		IsOA bool `json:"is_oa"`
	} `json:"open_access"`
}

func reconstructAbstract(index map[string][]int) string {
	if len(index) == 0 {
		return ""
	}
	maxPos := 0
	for _, positions := range index {
		for _, p := range positions {
			if p > maxPos {
				maxPos = p
			}
		}
	}
	words := make([]string, maxPos+1)
	for word, positions := range index {
		for _, p := range positions {
			if p >= 0 && p < len(words) {
				words[p] = word
			}
		}
	}
	return strings.Join(words, " ")
}

func (f *Fetcher) fetchOpenAlex(ctx context.Context, query string) []types.Source {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}

	q := url.QueryEscape(query)
	endpoint := fmt.Sprintf("%s?search=%s&per_page=2&mailto=%s", openAlexWorksURL, q, url.QueryEscape(openAlexMailto))
	if f.openAlexKey != "" {
		endpoint += "&api_key=" + url.QueryEscape(f.openAlexKey)
	}

	usage.RecordOpenAlexSearchCtx(ctx, 1)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "ClaimInspector/1.0 (mailto:"+openAlexMailto+")")

	resp, err := f.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var ar openAlexWorksResp
	if err := json.Unmarshal(body, &ar); err != nil {
		return nil
	}

	var sources []types.Source
	for _, w := range ar.Results {
		title := strings.TrimSpace(w.DisplayName)
		if title == "" {
			continue
		}
		abstract := reconstructAbstract(w.AbstractInvertedIndex)
		meta := ""
		if w.PublicationYear > 0 {
			meta = fmt.Sprintf("%d", w.PublicationYear)
		}
		if w.PrimaryLocation != nil && w.PrimaryLocation.Source != nil && w.PrimaryLocation.Source.DisplayName != "" {
			if meta != "" {
				meta += " · "
			}
			meta += w.PrimaryLocation.Source.DisplayName
		}
		if w.OpenAccess != nil && w.OpenAccess.IsOA {
			if meta != "" {
				meta += " · "
			}
			meta += "open access"
		}

		snippet := abstract
		if meta != "" {
			if snippet != "" {
				snippet = meta + "\n\n" + snippet
			} else {
				snippet = meta
			}
		}
		if len(snippet) > scorerSnippetMax {
			snippet = snippet[:scorerSnippetMax] + "…"
		}

		link := workURL(w)
		bodyText := title
		if abstract != "" {
			bodyText = title + "\n\n" + abstract
		} else if meta != "" {
			bodyText = title + "\n\n" + meta
		}

		sources = append(sources, types.Source{
			Title:    title + " [OpenAlex]",
			URL:      link,
			Snippet:  snippet,
			Body:     bodyText,
			Provider: "openalex",
		})
	}
	return sources
}

func workURL(w openAlexWork) string {
	if w.PrimaryLocation != nil && w.PrimaryLocation.LandingPageURL != "" {
		return w.PrimaryLocation.LandingPageURL
	}
	if w.DOI != "" {
		d := strings.TrimPrefix(w.DOI, "https://doi.org/")
		return "https://doi.org/" + d
	}
	if strings.HasPrefix(w.ID, "https://") {
		return w.ID
	}
	return "https://openalex.org/" + w.ID
}
