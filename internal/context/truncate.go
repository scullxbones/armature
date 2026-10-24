package context

const bytesPerToken = 4

func truncate(mem *workingMemory, tokenBudget int) *workingMemory {
	charBudget := tokenBudget * bytesPerToken

	layers := make([]layer, len(mem.Layers))
	copy(layers, mem.Layers)

	totalChars := func() int {
		n := 0
		for _, l := range layers {
			n += len(l.Content)
		}
		return n
	}

	for totalChars() > charBudget && len(layers) > 1 {
		drop := highestDropRankIndex(layers)
		layers = append(layers[:drop], layers[drop+1:]...)
	}

	return &workingMemory{
		IssueID: mem.IssueID,
		Layers:  layers,
	}
}

func highestDropRankIndex(layers []layer) int {
	maxIdx := 0
	for i, item := range layers[1:] {
		if item.DropRank > layers[maxIdx].DropRank {
			maxIdx = i + 1
		}
	}
	return maxIdx
}
