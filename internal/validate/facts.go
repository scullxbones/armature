package validate

import "github.com/scullxbones/armature/internal/dag"

const (
	statusClaimed    = "claimed"
	statusInProgress = "in-progress"
	statusDone       = "done"
	statusMerged     = "merged"
	statusCancelled  = "cancelled"
)

// IssueFacts is the borrowed Graph Finding input.
type IssueFacts struct {
	ID                  string
	Type                string
	Status              string
	Title               string
	Parent              string
	DefinitionOfDone    string
	EstComplexity       string
	Outcome             string
	ClaimedBy           string
	ClaimStale          bool
	CitationAcceptances int
	Scope               []string
	ContextFiles        []string
	Children            []string
	BlockedBy           []string
	Blocks              []string
	Acceptance          []byte
	Context             []byte
	SourceEntryIDs      []string
	Decisions           []DecisionFact
}

type DecisionFact struct {
	Topic  string
	Choice string
}

func isTerminalStatus(status string) bool {
	switch status {
	case statusDone, statusMerged, statusCancelled:
		return true
	default:
		return false
	}
}

func graphFromFacts(facts map[string]IssueFacts) *dag.Graph {
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
