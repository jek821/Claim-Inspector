package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"unicode"

	"factchecker/internal/types"
)

// Fetcher retrieves supporting sources for a claim using document and claim metadata.
type Fetcher struct {
	client *http.Client
}

func NewFetcher(_ string) *Fetcher {
	return &Fetcher{client: &http.Client{}}
}

// FetchAll retrieves sources from providers selected for this claim, then filters for relevance.
func (f *Fetcher) FetchAll(ctx context.Context, claim types.EnrichedClaim, doc types.DocumentContext) []types.Source {
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

	providers := providerSet(claim.Providers)
	if len(providers) == 0 {
		providers["wikipedia"] = true
	}

	if providers["wikipedia"] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			add(f.fetchWikipedia(ctx, claim, doc))
		}()
	}
	if providers["semantic_scholar"] && strings.TrimSpace(claim.ScholarQuery) != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			add(f.fetchSemanticScholar(ctx, claim.ScholarQuery))
		}()
	}
	if providers["pubmed"] && strings.TrimSpace(claim.PubMedQuery) != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			add(f.fetchPubMed(ctx, claim.PubMedQuery))
		}()
	}

	wg.Wait()
	return filterRelevant(results, claim, doc)
}

func providerSet(list []string) map[string]bool {
	m := make(map[string]bool, len(list))
	for _, p := range list {
		m[strings.ToLower(strings.TrimSpace(p))] = true
	}
	return m
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

type wikiExtractResp struct {
	Query struct {
		Pages map[string]struct {
			Title    string `json:"title"`
			Missing  string `json:"missing"`
			Extract  string `json:"extract"`
			PageID   int    `json:"pageid"`
		} `json:"pages"`
	} `json:"query"`
}

func (f *Fetcher) fetchWikipedia(ctx context.Context, claim types.EnrichedClaim, doc types.DocumentContext) []types.Source {
	if title := strings.TrimSpace(claim.WikipediaTitle); title != "" {
		if srcs := f.fetchWikipediaByTitle(ctx, title); len(srcs) > 0 {
			return srcs
		}
	}
	query := strings.TrimSpace(claim.WikiSearchQuery)
	if query == "" {
		query = claim.Text
	}
	if doc.Topic != "" {
		query = query + " " + doc.Topic
	}
	return f.fetchWikipediaSearch(ctx, query)
}

func (f *Fetcher) fetchWikipediaByTitle(ctx context.Context, title string) []types.Source {
	t := url.QueryEscape(strings.ReplaceAll(title, " ", "_"))
	endpoint := fmt.Sprintf(
		"https://en.wikipedia.org/w/api.php?action=query&format=json&prop=extracts&explaintext=1&exintro=1&exsentences=4&titles=%s",
		t,
	)
	body, ok := f.wikiGET(ctx, endpoint)
	if !ok {
		return nil
	}
	var wr wikiExtractResp
	if err := json.Unmarshal(body, &wr); err != nil {
		return nil
	}
	var sources []types.Source
	for _, page := range wr.Query.Pages {
		if page.Missing != "" || page.Extract == "" {
			continue
		}
		snippet := page.Extract
		if len(snippet) > 400 {
			snippet = snippet[:400] + "..."
		}
		sources = append(sources, types.Source{
			Title:    page.Title,
			URL:      wikiArticleURL(page.Title),
			Snippet:  snippet,
			Provider: "wikipedia",
		})
	}
	return sources
}

func (f *Fetcher) fetchWikipediaSearch(ctx context.Context, query string) []types.Source {
	q := url.QueryEscape(query)
	endpoint := fmt.Sprintf(
		"https://en.wikipedia.org/w/api.php?action=query&list=search&srsearch=%s&format=json&srlimit=3",
		q,
	)
	body, ok := f.wikiGET(ctx, endpoint)
	if !ok {
		return nil
	}
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
			URL:      wikiArticleURL(s.Title),
			Snippet:  snippet,
			Provider: "wikipedia",
		})
	}
	return sources
}

func (f *Fetcher) wikiGET(ctx context.Context, endpoint string) ([]byte, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, false
	}
	req.Header.Set("User-Agent", "ClaimInspector/1.0 (fact-checking; contact: factchecker@example.com)")
	resp, err := f.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return body, true
}

// --- Semantic Scholar ---

type scholarResp struct {
	Data []struct {
		Title    string `json:"title"`
		URL      string `json:"url"`
		Abstract string `json:"abstract"`
	} `json:"data"`
}

func (f *Fetcher) fetchSemanticScholar(ctx context.Context, query string) []types.Source {
	q := url.QueryEscape(query)
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

// --- PubMed ---

type pubmedSearchResp struct {
	ESearchResult struct {
		IDList []string `json:"idlist"`
	} `json:"esearchresult"`
}

type pubmedSummaryResp struct {
	Result map[string]struct {
		Title   string `json:"title"`
		Source  string `json:"source"`
		PubDate string `json:"pubdate"`
	} `json:"result"`
}

func (f *Fetcher) fetchPubMed(ctx context.Context, query string) []types.Source {
	q := url.QueryEscape(query)
	searchURL := fmt.Sprintf(
		"https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esearch.fcgi?db=pubmed&term=%s&retmax=2&retmode=json&tool=claiminspector&email=factchecker@example.com",
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
		"https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esummary.fcgi?db=pubmed&id=%s&retmode=json&tool=claiminspector&email=factchecker@example.com",
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

// filterRelevant drops sources whose titles/snippets share no terms with the claim or document.
func filterRelevant(sources []types.Source, claim types.EnrichedClaim, doc types.DocumentContext) []types.Source {
	terms := collectTerms(claim, doc)
	if len(terms) == 0 {
		return sources
	}
	var out []types.Source
	for _, s := range sources {
		if sourceRelevance(s, terms) >= 1 {
			out = append(out, s)
		}
	}
	// If everything was filtered out, keep unfiltered rather than scoring with zero evidence.
	if len(out) == 0 && len(sources) > 0 {
		return sources
	}
	return out
}

func collectTerms(claim types.EnrichedClaim, doc types.DocumentContext) map[string]bool {
	terms := make(map[string]bool)
	// Prefer named entities and topic — avoids stopwords like "the" causing false positives.
	for _, e := range claim.Entities {
		addTerms(terms, e)
	}
	for _, e := range doc.Entities {
		addTerms(terms, e)
	}
	addTerms(terms, doc.Topic)
	addTerms(terms, claim.WikipediaTitle)
	// Fall back to salient words from the claim when no entities were extracted.
	if len(terms) == 0 {
		for _, w := range tokenize(claim.Text) {
			if len(w) >= 5 {
				terms[w] = true
			}
		}
	}
	return terms
}

func addTerms(m map[string]bool, s string) {
	for _, w := range tokenize(s) {
		if len(w) >= 3 {
			m[w] = true
		}
	}
}

func tokenize(s string) []string {
	var words []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			words = append(words, strings.ToLower(b.String()))
			b.Reset()
		}
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return words
}

func wikiArticleURL(title string) string {
	return "https://en.wikipedia.org/wiki/" + strings.ReplaceAll(title, " ", "_")
}

func sourceRelevance(s types.Source, terms map[string]bool) int {
	hayTokens := make(map[string]bool)
	for _, w := range tokenize(s.Title + " " + s.Snippet) {
		hayTokens[w] = true
	}
	score := 0
	for term := range terms {
		if hayTokens[term] {
			score++
		}
	}
	return score
}
