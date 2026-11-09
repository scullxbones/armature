package output

import "encoding/json"

type ListFacts struct {
	Type   string
	Status string
	Title  string
}

type NoteFact struct {
	Msg     string
	Deleted bool
}

// HostRefFact is one live host name for arm show. Current marks the latest
// live name.
type HostRefFact struct {
	URL     string
	Role    string
	Current bool
}

type IssueFacts struct {
	ID                     string
	Title                  string
	Type                   string
	Status                 string
	Parent                 string
	Priority               string
	DefinitionOfDone       string
	Outcome                string
	ClaimedBy              string
	AssignedWorker         string
	Branch                 string
	Base                   string
	Tip                    string
	PR                     string
	IntegrationBranch      string
	TargetSHA              string
	CombinedPatchID        string
	MatchedCommit          string
	Derived                bool
	Scope                  []string
	BlockedBy              []string
	Blocks                 []string
	Acceptance             json.RawMessage
	Notes                  []NoteFact
	HostRefs               []HostRefFact
	AssessmentAttestations json.RawMessage
}
