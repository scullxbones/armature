package ready

import "github.com/scullxbones/armature/internal/dag"

type Facts struct {
	Type                       string
	Status                     string
	Parent                     string
	Title                      string
	Priority                   string
	AssignedWorker             string
	EstComplexity              string
	Confidence                 string
	ClaimedBy                  string
	ClaimedAt                  int64
	LastHeartbeat              int64
	LastClaimingWorkerActivity int64
	ClaimTTL                   int
	ClaimStale                 bool
	Scope                      []string
	Children                   []string
	BlockedBy                  []string
	Blocks                     []string
}

func graphFromFacts(facts map[string]Facts) *dag.Graph {
	nodeIndex := make(map[string]*dag.Node, len(facts))
	for id, f := range facts {
		nodeIndex[id] = &dag.Node{
			ID:        id,
			Title:     f.Title,
			Type:      f.Type,
			Parent:    f.Parent,
			Children:  append([]string(nil), f.Children...),
			BlockedBy: append([]string(nil), f.BlockedBy...),
			Blocks:    append([]string(nil), f.Blocks...),
		}
	}
	return dag.FromIndex(nodeIndex)
}
