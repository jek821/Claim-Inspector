package types

// DocumentContext is document-level metadata used to disambiguate claim lookups.
type DocumentContext struct {
	Topic    string   `json:"topic"`
	Domain   string   `json:"domain"` // general, science, medicine, history, technology, law, economics, biography, geography
	Entities []string `json:"entities"`
	Summary  string   `json:"summary"`
}

// EnrichedClaim is an atomic claim plus lookup hints from the analysis pass.
type EnrichedClaim struct {
	Text            string   `json:"text"`
	LocalContext    string   `json:"local_context"`
	Entities        []string `json:"entities"`
	WikipediaTitle  string   `json:"wikipedia_title,omitempty"`
	WikiSearchQuery string   `json:"wiki_search_query"`
	ScholarQuery    string   `json:"scholar_query,omitempty"`
	OpenAlexQuery   string   `json:"openalex_query,omitempty"`
	PubMedQuery     string   `json:"pubmed_query,omitempty"`
	Providers       []string `json:"providers"` // wikipedia, semantic_scholar, openalex, pubmed
}

// Claim is a single verifiable assertion extracted from the input text.
type Claim struct {
	Text        string   `json:"text"`
	Risk        string   `json:"risk"` // verified | low | medium | high | unverifiable
	Explanation string   `json:"explanation"`
	Sources     []Source `json:"sources"`
}

// Source is a reference document retrieved for a claim.
type Source struct {
	Title    string `json:"title"`
	URL      string `json:"url"`
	Snippet  string `json:"snippet"`
	Provider string `json:"provider"` // wikipedia | semantic_scholar | openalex | pubmed
	Body     string `json:"-"`        // full text for chunking/indexing; not sent to clients
}

// TokenUsage tracks exact token counts from the Anthropic API.
type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// CostBreakdown is the exact cost for a completed analysis.
type CostBreakdown struct {
	Model        string     `json:"model"`
	Usage        TokenUsage `json:"usage"`
	ExactCostUSD float64    `json:"exact_cost_usd"`
}

// CostEstimate is a pre-run estimate based on text length.
type CostEstimate struct {
	EstimatedClaims int     `json:"estimated_claims"`
	EstInputTokens  int     `json:"est_input_tokens"`
	EstOutputTokens int     `json:"est_output_tokens"`
	EstCostUSD      float64 `json:"est_cost_usd"`
	EstCostBatchUSD float64 `json:"est_cost_batch_usd"`
	Model           string  `json:"model"`
}

// AnalyzeRequest is the incoming request body for text paste.
type AnalyzeRequest struct {
	Text     string `json:"text"`
	Batch    bool   `json:"batch"`    // true = use Anthropic Batch API (async, 50% cheaper)
	Label    string `json:"label"`    // optional user-supplied name for this run
	FileName string `json:"filename"` // original filename if uploaded, for display in history
}

// AnalyzeResponse is the outgoing response body for synchronous runs.
type AnalyzeResponse struct {
	Claims []Claim       `json:"claims"`
	Cost   CostBreakdown `json:"cost"`
}

// BatchSubmitResponse is returned immediately when a batch job is accepted.
type BatchSubmitResponse struct {
	BatchID string `json:"batch_id"`
	Message string `json:"message"`
}

// BatchStatusResponse is returned by GET /batch/:id.
type BatchStatusResponse struct {
	BatchID   string  `json:"batch_id"`
	Status    string  `json:"status"`    // pending | processing | done | failed
	Progress  int     `json:"progress"`  // 0-100
	Succeeded int     `json:"succeeded"`
	Total     int     `json:"total"`
	Claims    []Claim `json:"claims,omitempty"` // populated when status == "done"
	Cost      *CostBreakdown `json:"cost,omitempty"`
	Error     string  `json:"error,omitempty"`
}

// EstimateRequest is the body for POST /estimate.
type EstimateRequest struct {
	Text string `json:"text"`
}

// LoginRequest is the body for POST /login.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse returns the session token on success.
type LoginResponse struct {
	Token string `json:"token"`
}

// RenameRequest is the body for PATCH /history/:id.
type RenameRequest struct {
	Label string `json:"label"`
}
type ProgressEvent struct {
	Done    int    `json:"done"`    // claims scored so far
	Total   int    `json:"total"`   // total claims extracted
	Current string `json:"current"` // claim text being scored (trimmed)
}

// APIProviderUsage is one provider's usage vs documented limits.
type APIProviderUsage struct {
	ID             string  `json:"id"`
	DisplayName    string  `json:"display_name"`
	Unit           string  `json:"unit"`
	Period         string  `json:"period"`
	Limit          float64 `json:"limit"`
	UsedLifetime   float64 `json:"used_lifetime"`
	UsedDaily      float64 `json:"used_daily"`
	RemainingDaily float64 `json:"remaining,omitempty"`
	PctDaily       float64 `json:"pct_used,omitempty"`
	Note           string  `json:"note,omitempty"`
}

// APIUsageResponse is returned alongside history totals.
type APIUsageResponse struct {
	Providers     []APIProviderUsage `json:"providers"`
	DailyResetUTC string             `json:"daily_reset_utc"`
}

// HistoryResponse is returned by GET /history.
type HistoryResponse struct {
	Runs           []HistoryRun     `json:"runs"`
	TotalInputTok  int              `json:"total_input_tokens"`
	TotalOutputTok int              `json:"total_output_tokens"`
	TotalCostUSD   float64          `json:"total_cost_usd"`
	APIUsage       APIUsageResponse `json:"api_usage"`
}

// HistoryRun is a summary of one past run for the history list.
type HistoryRun struct {
	ID        string        `json:"id"`
	CreatedAt string        `json:"created_at"`
	Title     string        `json:"title"`
	Label     string        `json:"label"`     // user-supplied name, may be empty
	FileName  string        `json:"filename"`  // original upload filename, may be empty
	Mode      string        `json:"mode"`
	Claims    []Claim       `json:"claims"`
	Cost      CostBreakdown `json:"cost"`
}
