package usage

import "factchecker/internal/apilimits"

// ToStoreMap converts provider-keyed counts for JSON persistence on a Run.
func ToStoreMap(m map[apilimits.Provider]Counts) map[string]Counts {
	if m == nil {
		return nil
	}
	out := make(map[string]Counts, len(m))
	for k, v := range m {
		out[string(k)] = v
	}
	return out
}

// Merge adds b into a (mutates a).
func Merge(a, b map[apilimits.Provider]Counts) map[apilimits.Provider]Counts {
	if a == nil {
		a = make(map[apilimits.Provider]Counts)
	}
	for k, v := range b {
		c := a[k]
		c.Requests += v.Requests
		c.InputTokens += v.InputTokens
		c.OutputTokens += v.OutputTokens
		c.EmbedTokens += v.EmbedTokens
		c.EstSpendUSD += v.EstSpendUSD
		a[k] = c
	}
	return a
}
