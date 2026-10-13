// Package harnesspolicy resolves and verifies scope and verification policy for a task,
// deciding which files a worker may touch and what checks must pass before completion.
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
