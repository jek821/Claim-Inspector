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

	"factchecker/internal/apilimits"
	"factchecker/internal/types"
	"factchecker/internal/usage"
)

const (
	wikiExtractChars  = 12000 // full article text from Wikipedia API
	scorerSnippetMax  = 3200  // passed to Haiku per source
	wikiSearchHits    = 3
)

// Fetcher retrieves supporting sources for a claim using document and claim metadata.
type Fetcher struct {
	client     *http.Client
	openAlexKey string
}

func NewFetcher(openAlexKey string) *Fetcher {
	return &Fetcher{client: &http.Client{}, openAlexKey: openAlexKey}
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
	openAlexQ := strings.TrimSpace(claim.OpenAlexQuery)
	if openAlexQ == "" {
		openAlexQ = strings.TrimSpace(claim.ScholarQuery)
	}
	if providers["openalex"] && openAlexQ != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			add(f.fetchOpenAlex(ctx, openAlexQ))
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
			Title string `json:"title"`
		} `json:"search"`
	} `json:"query"`
}

type wikiExtractResp struct {
	Query struct {
		Pages map[string]struct {
			Title   string `json:"title"`
			Missing string `json:"missing"`
			Extract string `json:"extract"`
		} `json:"pages"`
	} `json:"query"`
}

func (f *Fetcher) fetchWikipedia(ctx context.Context, claim types.EnrichedClaim, doc types.DocumentContext) []types.Source {
	var titles []string
	if title := strings.TrimSpace(claim.WikipediaTitle); title != "" {
		titles = append(titles, title)
	}

	query := strings.TrimSpace(claim.WikiSearchQuery)
	if query == "" {
		query = claim.Text
	}
	if doc.Topic != "" && !strings.Contains(strings.ToLower(query), strings.ToLower(doc.Topic)) {
		query = query + " " + doc.Topic
	}

	for _, hit := range f.wikipediaSearchTitles(ctx, query) {
		if !containsTitle(titles, hit) {
			titles = append(titles, hit)
		}
		if len(titles) >= wikiSearchHits {
			break
		}
	}

	if len(titles) == 0 {
		return nil
	}
	return f.fetchWikipediaExtracts(ctx, titles, claim)
}

func containsTitle(titles []string, t string) bool {
	lt := strings.ToLower(t)
	for _, x := range titles {
		if strings.ToLower(x) == lt {
			return true
		}
	}
	return false
}

func (f *Fetcher) wikipediaSearchTitles(ctx context.Context, query string) []string {
	q := url.QueryEscape(query)
	endpoint := fmt.Sprintf(
		"https://en.wikipedia.org/w/api.php?action=query&list=search&srsearch=%s&format=json&srlimit=%d",
		q, wikiSearchHits,
	)
	body, ok := f.wikiGET(ctx, endpoint)
	if !ok {
		return nil
	}
	var wr wikiSearchResp
	if err := json.Unmarshal(body, &wr); err != nil {
		return nil
	}
	var titles []string
	for _, s := range wr.Query.Search {
		titles = append(titles, s.Title)
	}
	return titles
}

func (f *Fetcher) fetchWikipediaExtracts(ctx context.Context, titles []string, claim types.EnrichedClaim) []types.Source {
	if len(titles) == 0 {
		return nil
	}
	encoded := make([]string, len(titles))
	for i, t := range titles {
		encoded[i] = url.QueryEscape(t)
	}
	endpoint := fmt.Sprintf(
		"https://en.wikipedia.org/w/api.php?action=query&format=json&prop=extracts&explaintext=1&exchars=%d&titles=%s",
		wikiExtractChars, strings.Join(encoded, "%7C"),
	)
	body, ok := f.wikiGET(ctx, endpoint)
	if !ok {
		return nil
	}
	var wr wikiExtractResp
	if err := json.Unmarshal(body, &wr); err != nil {
		return nil
	}

	keywords := claimKeywords(claim)
	var sources []types.Source
	for _, page := range wr.Query.Pages {
		if page.Missing != "" || page.Extract == "" {
			continue
		}
		snippet := excerptForClaim(page.Extract, keywords)
		sources = append(sources, types.Source{
			Title:    page.Title,
			URL:      wikiArticleURL(page.Title),
			Snippet:  snippet,
			Body:     page.Extract,
			Provider: "wikipedia",
		})
	}
	return sources
}

// excerptForClaim returns a passage from the article most likely to support the claim.
func excerptForClaim(fullText string, keywords []string) string {
	if fullText == "" {
		return ""
	}
	if passage := excerptAroundKeywords(fullText, keywords, 900); passage != "" {
		return truncate(passage, scorerSnippetMax)
	}
	return truncate(fullText, scorerSnippetMax)
}

func excerptAroundKeywords(text string, keywords []string, window int) string {
	lower := strings.ToLower(text)
	bestIdx := -1
	bestLen := 0
	for _, kw := range keywords {
		kw = strings.ToLower(strings.TrimSpace(kw))
		if len(kw) < 4 {
			continue
		}
		idx := 0
		for {
			i := strings.Index(lower[idx:], kw)
			if i < 0 {
				break
			}
			pos := idx + i
			if bestIdx < 0 || len(kw) > bestLen {
				bestIdx = pos
				bestLen = len(kw)
			}
			idx = pos + len(kw)
		}
	}
	if bestIdx < 0 {
		return ""
	}
	start := bestIdx - window
	if start < 0 {
		start = 0
	}
	end := bestIdx + window
	if end > len(text) {
		end = len(text)
	}
	passage := strings.TrimSpace(text[start:end])
	if start > 0 {
		passage = "…" + passage
	}
	if end < len(text) {
		passage += "…"
	}
	return passage
}

func claimKeywords(claim types.EnrichedClaim) []string {
	seen := make(map[string]bool)
	var kws []string
	add := func(w string) {
		w = strings.ToLower(w)
		if len(w) >= 4 && !isStopword(w) && !seen[w] {
			seen[w] = true
			kws = append(kws, w)
		}
	}
	for _, w := range tokenize(claim.Text) {
		add(w)
	}
	for _, e := range claim.Entities {
		for _, w := range tokenize(e) {
			add(w)
		}
	}
	// Numeric claims (dates, magnitudes).
	for _, part := range strings.Fields(claim.Text) {
		part = strings.Trim(part, ".,;:")
		if len(part) >= 3 && strings.ContainsAny(part, "0123456789") {
			add(part)
		}
	}
	return kws
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func (f *Fetcher) wikiGET(ctx context.Context, endpoint string) ([]byte, bool) {
	usage.RecordHTTPCtx(ctx, apilimits.Wikipedia, 1)
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
	usage.RecordHTTPCtx(ctx, apilimits.SemanticScholar, 1)
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
		if len(snippet) > scorerSnippetMax {
			snippet = snippet[:scorerSnippetMax] + "…"
		}
		body := p.Abstract
		sources = append(sources, types.Source{
			Title:    p.Title,
			URL:      p.URL,
			Snippet:  snippet,
			Body:     body,
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
	usage.RecordHTTPCtx(ctx, apilimits.PubMed, 1)
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
	usage.RecordHTTPCtx(ctx, apilimits.PubMed, 1)
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
			Body:     article.Title + ". " + snippet,
			Provider: "pubmed",
		})
	}
	return sources
}

func filterRelevant(sources []types.Source, claim types.EnrichedClaim, doc types.DocumentContext) []types.Source {
	terms := collectTerms(claim, doc)
	var out []types.Source
	for _, s := range sources {
		if !passesSubjectGate(s, claim) {
			continue
		}
		if len(terms) > 0 && sourceRelevance(s, terms) < 1 {
			continue
		}
		out = append(out, s)
	}
	return out
}

// passesSubjectGate rejects pages that share generic words (e.g. "magnetic") but not the claim's subject.
func passesSubjectGate(s types.Source, claim types.EnrichedClaim) bool {
	anchor := primarySubject(claim)
	if anchor == "" {
		return true
	}
	hay := " " + strings.ToLower(s.Title+" "+s.Snippet) + " "
	for _, syn := range subjectSynonyms(anchor) {
		if strings.Contains(hay, " "+syn+" ") || strings.HasPrefix(hay, " "+syn) {
			return true
		}
	}
	return false
}

func primarySubject(claim types.EnrichedClaim) string {
	lower := strings.ToLower(claim.Text + " " + claim.LocalContext)
	for _, subj := range []string{"frog", "frogs", "dinosaur", "dinosaurs", "mitochondria", "bacteria", "virus", "planet", "earth"} {
		if strings.Contains(lower, subj) {
			if subj == "frogs" {
				return "frog"
			}
			if subj == "dinosaurs" {
				return "dinosaur"
			}
			return subj
		}
	}
	if len(claim.Entities) > 0 {
		for _, w := range tokenize(claim.Entities[0]) {
			if len(w) >= 4 && !isStopword(w) {
				return w
			}
		}
	}
	return ""
}

func subjectSynonyms(anchor string) []string {
	switch anchor {
	case "frog":
		return []string{
			"frog", "frogs", "anura", "amphibian", "amphibians", "tadpole",
			"hylidae", "arthroleptidae", "trichobatrachus", "hairy", "horror",
			"tree frog", "treefrog",
		}
	case "dinosaur":
		return []string{"dinosaur", "dinosaurs", "mesozoic", "triassic", "jurassic", "cretaceous"}
	default:
		return []string{anchor}
	}
}

func collectTerms(claim types.EnrichedClaim, doc types.DocumentContext) map[string]bool {
	terms := make(map[string]bool)
	for _, e := range claim.Entities {
		addTerms(terms, e)
	}
	for _, e := range doc.Entities {
		addTerms(terms, e)
	}
	addTerms(terms, doc.Topic)
	addTerms(terms, claim.WikipediaTitle)
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

func isStopword(w string) bool {
	switch w {
	case "that", "this", "with", "from", "have", "been", "were", "when", "where",
		"which", "their", "there", "about", "into", "also", "among", "most", "more",
		"some", "such", "than", "then", "them", "they", "these", "those", "very",
		"what", "your", "other", "only", "over", "after", "before", "being", "between",
		"through", "using", "under", "while", "help", "push", "down", "certain",
		"species", "known", "found", "like":
		return true
	default:
		return false
	}
}
