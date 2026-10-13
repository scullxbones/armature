package harnesspolicy

import "encoding/json"

type IssuePolicy struct {
	ID         string
	Title      string
	Scope      []string
	Acceptance json.RawMessage
	Citations  []CitationCheck
}

type IssueFacts struct {
	ID                  string
	Title               string
	Scope               []string
	Acceptance          json.RawMessage
	SourceLinks         []SourceLinkFact
	CitationAcceptances []CitationAcceptanceFact
}

type SourceLinkFact struct {
	SourceEntryID string
}

type CitationAcceptanceFact struct {
	SourceEntryID string
}
