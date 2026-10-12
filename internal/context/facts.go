package context

import "github.com/scullxbones/armature/internal/dag"

const (
	statusDone   = "done"
	statusMerged = "merged"
	stateDirName = ".armature"
)

// IssueFacts is the borrowed Render Context input.
type IssueFacts struct {
	ID               string
	Title            string
	Type             string
	Status           string
	Parent           string
	Priority         string
	DefinitionOfDone string
	Outcome          string
	Scope            []string
	ContextFiles     []string
	Children         []string
	BlockedBy        []string
	Blocks           []string
	Context          []byte
	Notes            []NoteFact
	Decisions        []DecisionFact
}

type NoteFact struct {
	Timestamp int64
	Msg       string
	Deleted   bool
}

type DecisionFact struct {
	Topic     string
	Choice    string
	Rationale string
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
