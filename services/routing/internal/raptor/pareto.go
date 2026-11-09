package raptor

import (
	"sort"
)

// FilterPareto filters candidate journeys so that none is strictly dominated
// by another in both earlier (or equal) arrival time and fewer (or equal) transfers.
func FilterPareto(journeys []Journey) []Journey {
	if len(journeys) <= 1 {
		return journeys
	}

	// Sort primarily by arrival time ascending, then by transfers ascending
	sort.Slice(journeys, func(i, j int) bool {
		if journeys[i].ArriveAt != journeys[j].ArriveAt {
			return journeys[i].ArriveAt < journeys[j].ArriveAt
		}
		if journeys[i].Transfers != journeys[j].Transfers {
			return journeys[i].Transfers < journeys[j].Transfers
		}
		return journeys[i].WalkMeters < journeys[j].WalkMeters
	})

	var pareto []Journey
	minTransfers := int(1e9)

	for _, jn := range journeys {
		// A journey is non-dominated if it achieves this arrival time with strictly fewer transfers
		// than previously retained journeys that arrived earlier or at the same time.
		if jn.Transfers < minTransfers {
			pareto = append(pareto, jn)
			minTransfers = jn.Transfers
		}
	}

	return pareto
}
