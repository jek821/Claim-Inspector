// Package pricing holds Anthropic model pricing and cost calculation helpers.
package pricing

const (
	// Haiku 4.5 pricing (per million tokens, as of May 2026)
	HaikuInputPerM  = 1.00
	HaikuOutputPerM = 5.00
	ModelName       = "claude-haiku-4-5-20251001"

	// Tokens per word approximation (conservative)
	TokensPerWord = 1.4
	// Average words per page
	WordsPerPage = 250
	// Overhead tokens: system prompt + source snippets per claim call
	OverheadPerClaimInput = 600
	// Output tokens per claim score
	OutputPerClaim = 150
	// Average claims per 100 words of input
	ClaimsPerHundredWords = 5.0
	// Extraction call overhead (system prompt)
	ExtractionOverhead = 200
)

// CalcExact returns the exact cost in USD for known token counts.
func CalcExact(inputTokens, outputTokens int) float64 {
	input := float64(inputTokens) / 1_000_000 * HaikuInputPerM
	output := float64(outputTokens) / 1_000_000 * HaikuOutputPerM
	return input + output
}

// EstimateFromText returns a rough pre-run cost estimate.
func EstimateFromText(text string) (estClaims, estInput, estOutput int, costRegular, costBatch float64) {
	words := countWords(text)
	estClaims = int(float64(words) / 100 * ClaimsPerHundredWords)
	if estClaims < 1 {
		estClaims = 1
	}

	// Extraction call: input = text tokens + overhead
	extractionInput := int(float64(words)*TokensPerWord) + ExtractionOverhead
	extractionOutput := estClaims * 15 // ~15 tokens per claim in the JSON array

	// Scoring calls: each claim gets overhead + claim text + source snippets
	scoringInput := estClaims * OverheadPerClaimInput
	scoringOutput := estClaims * OutputPerClaim

	estInput = extractionInput + scoringInput
	estOutput = extractionOutput + scoringOutput

	costRegular = CalcExact(estInput, estOutput)
	// Batch: same token count but Anthropic batch API is 50% off
	// (sequential mode in our tool doesn't use the batch API yet, just sequential calls;
	//  true batch API savings would apply if we add that — flag as estimate)
	costBatch = costRegular * 0.5

	return
}

func countWords(s string) int {
	words := 0
	inWord := false
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\t' || r == '\r' {
			inWord = false
		} else if !inWord {
			words++
			inWord = true
		}
	}
	return words
}
