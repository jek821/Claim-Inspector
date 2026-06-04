package api

import (
	"factchecker/internal/apilimits"
	"factchecker/internal/pricing"
	"factchecker/internal/types"
	"factchecker/internal/usage"
)

func (h *Handler) buildCostBreakdown(extractIn, extractOut, totalIn, totalOut int, batchScoring bool, snap map[apilimits.Provider]usage.Counts) types.CostBreakdown {
	scoreIn := totalIn - extractIn
	scoreOut := totalOut - extractOut
	if scoreIn < 0 {
		scoreIn = 0
	}
	if scoreOut < 0 {
		scoreOut = 0
	}
	anthropicUSD := pricing.CalcAnthropicUSD(extractIn, extractOut, scoreIn, scoreOut, batchScoring)
	aux := usage.BuildAuxCosts(snap, h.store.GetLifetimeVoyageEmbedTokens(), h.store.GetDailyOpenAlexSpend())
	return types.CostBreakdown{
		Model:            pricing.ModelName,
		Usage:            types.TokenUsage{InputTokens: totalIn, OutputTokens: totalOut},
		AnthropicCostUSD: anthropicUSD,
		ExactCostUSD:     anthropicUSD + usage.SumAuxUSD(aux),
		AuxCosts:         aux,
	}
}

func (h *Handler) estimateCost(text string) types.CostEstimate {
	return pricing.EstimateFull(
		text,
		h.voyageKey != "",
		h.store.GetLifetimeVoyageEmbedTokens(),
		h.store.GetDailyOpenAlexSpend(),
	)
}
