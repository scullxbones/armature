package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/harnesspolicy"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/sources"
)

type issuePolicyLoader struct {
	stateDir   string
	sourcesDir string
}

func newIssuePolicyLoader(stateDir, sourcesDir string) issuePolicyLoader {
	return issuePolicyLoader{stateDir: stateDir, sourcesDir: sourcesDir}
}

func (l issuePolicyLoader) Resolve(taskID string) (harnesspolicy.IssuePolicy, error) {
	issuePath := filepath.Join(l.stateDir, "issues", taskID+".json")
	issue, err := materialize.LoadIssue(issuePath)
	if err != nil {
		return harnesspolicy.IssuePolicy{}, fmt.Errorf("task %s not found: %w", taskID, err)
	}
	entries, err := sources.NewLifecycle(l.sourcesDir).ListAll()
	if err != nil {
		return harnesspolicy.IssuePolicy{}, fmt.Errorf("read sources: %w", err)
	}
	known := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		known[entry.ID] = struct{}{}
	}
	return harnesspolicy.PolicyFromFacts(issuePolicyFacts(issue), known), nil
}

func issuePolicyFacts(issue materialize.Issue) harnesspolicy.IssueFacts {
	f := harnesspolicy.IssueFacts{
		ID:         issue.ID,
		Title:      issue.Title,
		Scope:      append([]string(nil), issue.Scope...),
		Acceptance: append(json.RawMessage(nil), issue.Acceptance...),
	}
	for _, link := range issue.SourceLinks {
		f.SourceLinks = append(f.SourceLinks, harnesspolicy.SourceLinkFact{SourceEntryID: link.SourceEntryID})
	}
	for _, acceptance := range issue.CitationAcceptances {
		f.CitationAcceptances = append(f.CitationAcceptances, harnesspolicy.CitationAcceptanceFact{
			SourceEntryID: acceptance.SourceEntryID,
		})
	}
	return f
}
