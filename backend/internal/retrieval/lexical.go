package retrieval

import "factchecker/internal/corpus"

func lexicalScore(queryTokens, docTokens map[string]int) float32 {
	if len(queryTokens) == 0 || len(docTokens) == 0 {
		return 0
	}
	var score float32
	for term, qw := range queryTokens {
		if dw, ok := docTokens[term]; ok {
			score += float32(qw * dw)
		}
	}
	return score
}

func claimQueryTokens(claimText, localContext string) map[string]int {
	q := corpus.Tokenize(claimText)
	for term, n := range corpus.Tokenize(localContext) {
		q[term] += n
	}
	return q
}
