// Package harnesspolicy resolves and verifies scope and verification policy for a task,
// deciding which files a worker may touch and what checks must pass before completion.
package harnesspolicy

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/sources"
)

type ResolverConfig struct {
	RepoPath   string
	StateDir   string
	SourcesDir string
}

type IssuePolicy struct {
	ID         string
	Title      string
	Scope      []string
	Acceptance json.RawMessage
	Citations  []CitationCheck
}

type IssuePolicyResolver struct {
	cfg ResolverConfig
}

func NewIssuePolicyResolver(cfg ResolverConfig) *IssuePolicyResolver {
	return &IssuePolicyResolver{cfg: cfg}
}

func (r *IssuePolicyResolver) Resolve(taskID string) (IssuePolicy, error) {
	issuePath := filepath.Join(r.stateDir(), "issues", taskID+".json")
	issue, err := materialize.LoadIssue(issuePath)
	if err != nil {
		return IssuePolicy{}, fmt.Errorf("task %s not found: %w", taskID, err)
	}

	lc := sources.NewLifecycle(r.sourcesDir())
	entries, err := lc.ListAll()
	if err != nil {
		return IssuePolicy{}, fmt.Errorf("read sources: %w", err)
	}

	knownSources := make(map[string]sources.SourceEntry, len(entries))
	for _, entry := range entries {
		knownSources[entry.ID] = entry
	}

	return IssuePolicy{
		ID:         issue.ID,
		Title:      issue.Title,
		Scope:      append([]string(nil), issue.Scope...),
		Acceptance: append(json.RawMessage(nil), issue.Acceptance...),
		Citations:  resolveCitationChecks(issue, knownSources),
	}, nil
}

func (r *IssuePolicyResolver) stateDir() string {
	if r.cfg.StateDir != "" {
		return r.cfg.StateDir
	}
	return filepath.Join(r.cfg.RepoPath, ".armature", "state", "default")
}

func (r *IssuePolicyResolver) sourcesDir() string {
	if r.cfg.SourcesDir != "" {
		return r.cfg.SourcesDir
	}
	return filepath.Join(r.cfg.RepoPath, ".armature", "sources")
}

func resolveCitationChecks(issue materialize.Issue, knownSources map[string]sources.SourceEntry) []CitationCheck {
	if len(issue.SourceLinks) == 0 {
		return nil
	}

	emptySourceEntryIDAcceptsAll, accepted := citationAcceptanceIndex(issue.CitationAcceptances)

	checks := make([]CitationCheck, 0, len(issue.SourceLinks))
	for _, link := range issue.SourceLinks {
		if link.SourceEntryID == "" {
			continue
		}
		if _, ok := knownSources[link.SourceEntryID]; !ok {
			checks = append(checks, CitationCheck{
				SourceEntryID: link.SourceEntryID,
				Accepted:      emptySourceEntryIDAcceptsAll,
			})
			continue
		}
		checks = append(checks, CitationCheck{
			SourceEntryID: link.SourceEntryID,
			Accepted:      emptySourceEntryIDAcceptsAll || accepted[link.SourceEntryID],
		})
	}
	return checks
}

func citationAcceptanceIndex(acceptances []materialize.CitationAcceptance) (emptySourceEntryIDAcceptsAll bool, byID map[string]bool) {
	byID = make(map[string]bool, len(acceptances))
	for _, acceptance := range acceptances {
		if acceptance.SourceEntryID == "" {
			emptySourceEntryIDAcceptsAll = true
			continue
		}
		byID[acceptance.SourceEntryID] = true
	}
	return emptySourceEntryIDAcceptsAll, byID
}
