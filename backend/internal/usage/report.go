package usage

import (
	"math"
	"time"

	"factchecker/internal/apilimits"
	"factchecker/internal/types"
)

// BuildReport combines persisted totals with limit specs for the API/UI.
func BuildReport(lifetime, daily map[apilimits.Provider]Counts, dailyDate string) types.APIUsageResponse {
	specs := apilimits.All()
	out := make([]types.APIProviderUsage, 0, len(specs))
	for _, spec := range specs {
		lt := lifetime[spec.ID]
		dy := daily[spec.ID]
		usedLife, unit := scalarUsage(lt, spec.Unit)
		usedDay, _ := scalarUsage(dy, spec.Unit)
		row := types.APIProviderUsage{
			ID:           string(spec.ID),
			DisplayName:  spec.DisplayName,
			Unit:         spec.Unit,
			Period:       spec.Period,
			Limit:        spec.Limit,
			UsedLifetime: usedLife,
			UsedDaily:    usedDay,
			Note:         spec.Note,
		}
		if spec.Period == "daily" && spec.Limit > 0 {
			row.RemainingDaily = math.Max(0, spec.Limit-usedDay)
			row.PctDaily = (usedDay / spec.Limit) * 100
			if row.PctDaily > 100 {
				row.PctDaily = 100
			}
		}
		if spec.Period == "account" && spec.Limit > 0 {
			row.RemainingDaily = math.Max(0, spec.Limit-usedLife) // reuse field for "remaining free tier"
			row.PctDaily = (usedLife / spec.Limit) * 100
		}
		_ = unit
		out = append(out, row)
	}
	if dailyDate == "" {
		dailyDate = time.Now().UTC().Format("2006-01-02")
	}
	return types.APIUsageResponse{Providers: out, DailyResetUTC: dailyDate}
}

func scalarUsage(c Counts, unit string) (float64, string) {
	switch unit {
	case "tokens":
		return float64(c.InputTokens + c.OutputTokens + c.EmbedTokens), unit
	case "usd":
		return c.EstSpendUSD, unit
	case "requests":
		return float64(c.Requests), unit
	default:
		return float64(c.Requests), unit
	}
}
