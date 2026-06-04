// Package pricing holds Anthropic model pricing and cost calculation helpers.
package pricing

import (
	"factchecker/internal/types"
	"factchecker/internal/usage"
)

const (
	// Haiku 4.5 pricing (per million tokens, as of May 2026)
	HaikuInputPerM  = 1.00
	HaikuOutputPerM = 5.00
	ModelName       = "claude-haiku-4-5-20251001"

	// Tokens per word approximation (conservative)
	TokensPerWord = 1.4
	// Average words per page
	WordsPerPage = 250
	// Overhead tokens: system prompt + longer Wikipedia excerpts per claim call
	OverheadPerClaimInput = 1200
	// Output tokens per claim score
	OutputPerClaim = 150
	// Average claims per 100 words of input
	ClaimsPerHundredWords = 5.0
	// Extraction call overhead (system prompt + structured metadata per claim)
	ExtractionOverhead = 400
)

// CalcExact returns the exact cost in USD for known token counts.
func CalcExact(inputTokens, outputTokens int) float64 {
	input := float64(inputTokens) / 1_000_000 * HaikuInputPerM
	output := float64(outputTokens) / 1_000_000 * HaikuOutputPerM
	return input + output
}

// CalcAnthropicUSD returns Haiku cost. Batch mode applies 50% discount to scoring tokens only.
func CalcAnthropicUSD(extractIn, extractOut, scoreIn, scoreOut int, batchScoring bool) float64 {
	cost := CalcExact(extractIn, extractOut)
	scoreCost := CalcExact(scoreIn, scoreOut)
	if batchScoring {
		scoreCost *= 0.5
	}
	return cost + scoreCost
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
	extractionOutput := estClaims * 80 // structured claim + lookup metadata per claim

	// Scoring calls: each claim gets overhead + claim text + source snippets
	scoringInput := estClaims * OverheadPerClaimInput
	scoringOutput := estClaims * OutputPerClaim

	estInput = extractionInput + scoringInput
	estOutput = extractionOutput + scoringOutput

	extractCost := CalcExact(extractionInput, extractionOutput)
	scoreCost := CalcExact(scoringInput, scoringOutput)
	costRegular = extractCost + scoreCost
	costBatch = extractCost + scoreCost*0.5

	return
}

// EstimateFull returns a pre-run cost estimate including Anthropic and aux APIs.
func EstimateFull(text string, voyageEnabled bool, voyageLifetime int64, openAlexDailyBefore float64) types.CostEstimate {
	estClaims, estInput, estOutput, anthropicUSD, anthropicBatchUSD := EstimateFromText(text)
	aux := usage.EstimateAuxCosts(estClaims, voyageEnabled, voyageLifetime, openAlexDailyBefore)
	auxUSD := usage.SumAuxUSD(aux)
	return types.CostEstimate{
		EstimatedClaims:  estClaims,
		EstInputTokens:   estInput,
		EstOutputTokens:  estOutput,
		AnthropicCostUSD: anthropicUSD,
		EstAuxCostUSD:    auxUSD,
		EstCostUSD:       anthropicUSD + auxUSD,
		EstCostBatchUSD:  anthropicBatchUSD + auxUSD,
		Model:            ModelName,
		VoyageEnabled:    voyageEnabled,
		AuxCosts:         aux,
	}
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
