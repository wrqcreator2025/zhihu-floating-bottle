package matching

import (
	"sort"

	"driftbottle/internal/ai"
)

// Order selects distinct experiences; the persistence step rechecks ownership and availability.
func Order(items []ai.Match) []ai.Match {
	out := []ai.Match{}
	seen := map[string]bool{}
	for _, v := range items {
		if v.Eligible && v.Score >= 0 && v.Score <= 1 && !seen[v.ExperienceID] {
			seen[v.ExperienceID] = true
			out = append(out, v)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}
