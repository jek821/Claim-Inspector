package usage

import (
	"factchecker/internal/apilimits"
	"factchecker/internal/types"
)

// BillableVoyageUSD returns USD owed for runEmbedTokens given prior lifetime usage.
func BillableVoyageUSD(runEmbedTokens, lifetimeBefore int64) float64 {
	if runEmbedTokens <= 0 {
		return 0
	}
	free := int64(apilimits.VoyageFreeEmbedTokens)
	if lifetimeBefore >= free {
		return float64(runEmbedTokens) / 1_000_000 * apilimits.VoyagePricePerMillionUSD
	}
	after := lifetimeBefore + runEmbedTokens
	if after <= free {
		return 0
	}
	return float64(after-free) / 1_000_000 * apilimits.VoyagePricePerMillionUSD
}

// BillableOpenAlexUSD returns USD owed for runSpend given prior daily spend.
func BillableOpenAlexUSD(runSpend, dailyBefore float64) float64 {
	if runSpend <= 0 {
		return 0
	}
	free := apilimits.OpenAlexDailyFreeUSD
	if dailyBefore >= free {
		return runSpend
	}
	remaining := free - dailyBefore
	if runSpend <= remaining {
		return 0
	}
	return runSpend - remaining
}

// BuildAuxCosts converts a run usage snapshot into billable aux lines (before this run is persisted).
func BuildAuxCosts(snap map[apilimits.Provider]Counts, voyageLifetimeBefore int64, openAlexDailyBefore float64) []types.AuxCostLine {
	var lines []types.AuxCostLine
	if oa, ok := snap[apilimits.OpenAlex]; ok && oa.EstSpendUSD > 0 {
		bill := BillableOpenAlexUSD(oa.EstSpendUSD, openAlexDailyBefore)
		line := types.AuxCostLine{Provider: string(apilimits.OpenAlex), Label: "OpenAlex", AmountUSD: bill}
		if bill == 0 {
			line.Note = "within $1/day OpenAlex free credit"
		}
		lines = append(lines, line)
	}
	if v, ok := snap[apilimits.Voyage]; ok && v.EmbedTokens > 0 {
		bill := BillableVoyageUSD(v.EmbedTokens, voyageLifetimeBefore)
		line := types.AuxCostLine{Provider: string(apilimits.Voyage), Label: "Voyage embeddings", AmountUSD: bill}
		if bill == 0 {
			line.Note = "within 200M token Voyage free tier"
		}
		lines = append(lines, line)
	}
	return lines
}

// SumAuxUSD totals billable aux lines.
func SumAuxUSD(lines []types.AuxCostLine) float64 {
	var sum float64
	for _, l := range lines {
		sum += l.AmountUSD
	}
	return sum
}

// EstimateAuxCosts predicts billable aux API spend for a run.
func EstimateAuxCosts(estClaims int, voyageEnabled bool, voyageLifetime int64, openAlexDailyBefore float64) []types.AuxCostLine {
	var lines []types.AuxCostLine

	estSearches := int(float64(estClaims) * 0.6)
	if estSearches < 1 && estClaims >= 2 {
		estSearches = 1
	}
	oaRaw := float64(estSearches) * apilimits.OpenAlexSearchCostUSD
	oaBill := BillableOpenAlexUSD(oaRaw, openAlexDailyBefore)
	oaLine := types.AuxCostLine{Provider: string(apilimits.OpenAlex), Label: "OpenAlex", AmountUSD: oaBill}
	if oaBill == 0 && oaRaw > 0 {
		oaLine.Note = "within $1/day OpenAlex free credit"
	}
	lines = append(lines, oaLine)

	if voyageEnabled {
		estSources := estClaims * 2
		if estSources > 15 {
			estSources = 15
		}
		if estSources < 2 {
			estSources = 2
		}
		// ~8000-char bodies → ~14 chunks/source at ~190 tokens each; 40% disk cache hit.
		docTokens := int64(estSources) * 14 * int64(EstimateTokens(estChunkSample))
		queryTokens := int64(estClaims) * int64(EstimateTokens(estQuerySample))
		runTokens := queryTokens + int64(float64(docTokens)*0.6)
		vBill := BillableVoyageUSD(runTokens, voyageLifetime)
		vLine := types.AuxCostLine{Provider: string(apilimits.Voyage), Label: "Voyage embeddings", AmountUSD: vBill}
		if vBill == 0 && runTokens > 0 {
			vLine.Note = "within 200M token Voyage free tier"
		}
		lines = append(lines, vLine)
	}
	return lines
}

// Fixed samples for token estimation (~750 and ~200 chars).
const estChunkSample = "The quick brown fox jumps over the lazy dog. The quick brown fox jumps over the lazy dog. The quick brown fox jumps over the lazy dog. The quick brown fox jumps over the lazy dog. The quick brown fox jumps over the lazy dog. The quick brown fox jumps over the lazy dog. The quick brown fox jumps over the lazy dog. The quick brown fox jumps over the lazy dog. The quick brown fox jumps over the lazy dog. The quick brown fox jumps over the lazy dog. The quick brown fox jumps over the lazy dog. The quick brown fox jumps over the lazy dog. The quick brown fox jumps over the lazy dog. The quick brown fox jumps over the lazy dog. The quick brown fox jumps over the lazy dog. The quick brown fox jumps over the lazy dog. "
const estQuerySample = "Example claim with local context and document topic for semantic retrieval. Example claim with local context and document topic for semantic retrieval. Example claim with local context and document topic."
