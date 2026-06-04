package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"factchecker/internal/types"
)

func decodeXML(data []byte) *xml.Decoder {
	return xml.NewDecoder(bytes.NewReader(data))
}

// Fetcher retrieves supporting sources for a claim.
type Fetcher struct {
	client *http.Client
}

func NewFetcher() *Fetcher {
	return &Fetcher{client: &http.Client{}}
}

// FetchAll retrieves sources from all available providers concurrently.
func (f *Fetcher) FetchAll(ctx context.Context, claim string) []types.Source {
	var (
		mu      sync.Mutex
		results []types.Source
		wg      sync.WaitGroup
	)

	add := func(sources []types.Source) {
		mu.Lock()
		results = append(results, sources...)
		mu.Unlock()
	}

	wg.Add(4)
	go func() { defer wg.Done(); add(f.fetchWikipedia(ctx, claim)) }()
	go func() { defer wg.Done(); add(f.fetchSemanticScholar(ctx, claim)) }()
	go func() { defer wg.Done(); add(f.fetchArXiv(ctx, claim)) }()
	go func() { defer wg.Done(); add(f.fetchPubMed(ctx, claim)) }()

	wg.Wait()
	return results
}

// --- Wikipedia ---

type wikiSearchResp struct {
	Query struct {
		Search []struct {
			Title   string `json:"title"`
			Snippet string `json:"snippet"`
		} `json:"search"`
	} `json:"query"`
}

func (f *Fetcher) fetchWikipedia(ctx context.Context, claim string) []types.Source {
	q := url.QueryEscape(claim)
	endpoint := fmt.Sprintf(
		"https://en.wikipedia.org/w/api.php?action=query&list=search&srsearch=%s&format=json&srlimit=2",
		q,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "FactChecker/1.0")

	resp, err := f.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var wr wikiSearchResp
	if err := json.Unmarshal(body, &wr); err != nil {
		return nil
	}

	var sources []types.Source
	for _, s := range wr.Query.Search {
		snippet := strings.ReplaceAll(s.Snippet, "<span class=\"searchmatch\">", "")
		snippet = strings.ReplaceAll(snippet, "</span>", "")
		sources = append(sources, types.Source{
			Title:    s.Title,
			URL:      "https://en.wikipedia.org/wiki/" + url.QueryEscape(strings.ReplaceAll(s.Title, " ", "_")),
			Snippet:  snippet,
			Provider: "wikipedia",
		})
	}
	return sources
}

// --- Semantic Scholar ---

type scholarResp struct {
	Data []struct {
		Title  string `json:"title"`
		URL    string `json:"url"`
		Abstract string `json:"abstract"`
	} `json:"data"`
}

func (f *Fetcher) fetchSemanticScholar(ctx context.Context, claim string) []types.Source {
	q := url.QueryEscape(claim)
	endpoint := fmt.Sprintf(
		"https://api.semanticscholar.org/graph/v1/paper/search?query=%s&limit=2&fields=title,url,abstract",
		q,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil
	}

	resp, err := f.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var sr scholarResp
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil
	}

	var sources []types.Source
	for _, p := range sr.Data {
		snippet := p.Abstract
		if len(snippet) > 300 {
			snippet = snippet[:300] + "..."
		}
		sources = append(sources, types.Source{
			Title:    p.Title,
			URL:      p.URL,
			Snippet:  snippet,
			Provider: "semantic_scholar",
		})
	}
	return sources
}

// --- arXiv ---

type arXivFeed struct {
	Entries []struct {
		Title   string `xml:"title"`
		Summary string `xml:"summary"`
		ID      string `xml:"id"`
	} `xml:"entry"`
}

func (f *Fetcher) fetchArXiv(ctx context.Context, claim string) []types.Source {
	q := url.QueryEscape(claim)
	endpoint := fmt.Sprintf(
		"https://export.arxiv.org/api/query?search_query=all:%s&max_results=2&sortBy=relevance",
		q,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "FactChecker/1.0")

	resp, err := f.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// Minimal Atom XML parse — strip namespaces then decode
	cleaned := strings.ReplaceAll(string(body), `xmlns="http://www.w3.org/2005/Atom"`, "")
	cleaned = strings.ReplaceAll(cleaned, `xmlns:opensearch="http://a9.com/-/spec/opensearch/1.1/"`, "")
	cleaned = strings.ReplaceAll(cleaned, `xmlns:arxiv="http://arxiv.org/schemas/atom"`, "")

	dec := decodeXML([]byte(cleaned))
	var feed arXivFeed
	if err := dec.Decode(&feed); err != nil {
		return nil
	}

	var sources []types.Source
	for _, e := range feed.Entries {
		title := strings.TrimSpace(strings.ReplaceAll(e.Title, "\n", " "))
		snippet := strings.TrimSpace(strings.ReplaceAll(e.Summary, "\n", " "))
		if len(snippet) > 300 {
			snippet = snippet[:300] + "..."
		}
		arxivURL := strings.TrimSpace(e.ID)
		// Convert API ID to abstract page URL
		arxivURL = strings.Replace(arxivURL, "http://arxiv.org/abs/", "https://arxiv.org/abs/", 1)
		if title == "" || arxivURL == "" {
			continue
		}
		sources = append(sources, types.Source{
			Title:    title + " [arXiv]",
			URL:      arxivURL,
			Snippet:  snippet,
			Provider: "arxiv",
		})
	}
	return sources
}

// --- PubMed (NCBI E-utilities) ---
// Two-step: ESearch to get IDs, then ESummary to get titles/abstracts.

type pubmedSearchResp struct {
	ESearchResult struct {
		IDList []string `json:"idlist"`
	} `json:"esearchresult"`
}

type pubmedSummaryResp struct {
	Result map[string]struct {
		Title  string `json:"title"`
		Source string `json:"source"`
		PubDate string `json:"pubdate"`
	} `json:"result"`
}

func (f *Fetcher) fetchPubMed(ctx context.Context, claim string) []types.Source {
	q := url.QueryEscape(claim)
	searchURL := fmt.Sprintf(
		"https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esearch.fcgi?db=pubmed&term=%s&retmax=2&retmode=json&tool=factchecker&email=factchecker@example.com",
		q,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil
	}
	resp, err := f.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var searchResp pubmedSearchResp
	if err := json.Unmarshal(body, &searchResp); err != nil || len(searchResp.ESearchResult.IDList) == 0 {
		return nil
	}

	ids := strings.Join(searchResp.ESearchResult.IDList, ",")
	summaryURL := fmt.Sprintf(
		"https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esummary.fcgi?db=pubmed&id=%s&retmode=json&tool=factchecker&email=factchecker@example.com",
		ids,
	)
	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, summaryURL, nil)
	if err != nil {
		return nil
	}
	resp2, err := f.client.Do(req2)
	if err != nil || resp2.StatusCode != http.StatusOK {
		return nil
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	var sumResp pubmedSummaryResp
	if err := json.Unmarshal(body2, &sumResp); err != nil {
		return nil
	}

	var sources []types.Source
	for id, article := range sumResp.Result {
		if id == "uids" || article.Title == "" {
			continue
		}
		snippet := article.Source
		if article.PubDate != "" {
			snippet += " (" + article.PubDate + ")"
		}
		sources = append(sources, types.Source{
			Title:    article.Title + " [PubMed]",
			URL:      "https://pubmed.ncbi.nlm.nih.gov/" + id + "/",
			Snippet:  snippet,
			Provider: "pubmed",
		})
	}
	return sources
}
