package context

const bytesPerToken = 4

func Truncate(ctx *Context, tokenBudget int) *Context {
	charBudget := tokenBudget * bytesPerToken

	layers := make([]Layer, len(ctx.Layers))
	copy(layers, ctx.Layers)

	totalChars := func() int {
		n := 0
		for _, l := range layers {
			n += len(l.Content)
		}
		return n
	}

	for totalChars() > charBudget && len(layers) > 1 {
		maxIdx := 0
		for i, layer := range layers[1:] {
			if layer.Priority > layers[maxIdx].Priority {
				maxIdx = i + 1
			}
		}
		layers = append(layers[:maxIdx], layers[maxIdx+1:]...)
	}

	return &Context{
		IssueID: ctx.IssueID,
		Layers:  layers,
	}
}
