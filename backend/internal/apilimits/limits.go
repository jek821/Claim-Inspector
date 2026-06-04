// Package apilimits documents known usage caps for external APIs used by Claim Inspector.
package apilimits

// Provider identifies an external API.
type Provider string

const (
	Anthropic        Provider = "anthropic"
	Voyage           Provider = "voyage"
	OpenAlex         Provider = "openalex"
	Wikipedia        Provider = "wikipedia"
	SemanticScholar  Provider = "semantic_scholar"
	PubMed           Provider = "pubmed"
)

// Spec describes how we meter and cap a provider.
type Spec struct {
	ID          Provider `json:"id"`
	DisplayName string   `json:"display_name"`
	Unit        string   `json:"unit"`   // tokens, requests, usd
	Period      string   `json:"period"` // daily, account, none
	Limit       float64  `json:"limit"`  // 0 = no numeric cap tracked
	Note        string   `json:"note"`
}

// All returns canonical limit definitions (for UI and budgeting).
func All() []Spec {
	return []Spec{
		{
			ID: Anthropic, DisplayName: "Anthropic (Haiku)",
			Unit: "tokens", Period: "none", Limit: 0,
			Note: "Pay-as-you-go; tracked as input/output tokens. Set MAX_COST_USD to cap spend.",
		},
		{
			ID: Voyage, DisplayName: "Voyage Embeddings",
			Unit: "tokens", Period: "account", Limit: 200_000_000,
			Note: "First 200M tokens free on voyage-4-lite (see voyageai.com/docs/pricing).",
		},
		{
			ID: OpenAlex, DisplayName: "OpenAlex API",
			Unit: "usd", Period: "daily", Limit: 1.0,
			Note: "$1 free API credit per day; search ~$0.001/call (see openalex.org pricing).",
		},
		{
			ID: Wikipedia, DisplayName: "Wikipedia API",
			Unit: "requests", Period: "daily", Limit: 5000,
			Note: "No official key; polite pool. Soft daily budget for monitoring abuse.",
		},
		{
			ID: SemanticScholar, DisplayName: "Semantic Scholar",
			Unit: "requests", Period: "daily", Limit: 5000,
			Note: "Public API rate limits apply (~100 req/5 min). Soft daily budget.",
		},
		{
			ID: PubMed, DisplayName: "PubMed (NCBI)",
			Unit: "requests", Period: "daily", Limit: 10000,
			Note: "E-utilities: 3 req/s without API key. Soft daily budget.",
		},
	}
}

// OpenAlexSearchCostUSD is the documented approximate cost per search call.
const OpenAlexSearchCostUSD = 0.001
