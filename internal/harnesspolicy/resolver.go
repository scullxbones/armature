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

	// Build manifest from entries for compatibility.
	manifest := sources.Manifest{Entries: make(map[string]sources.SourceEntry)}
	for _, entry := range entries {
		manifest.Entries[entry.ID] = entry
	}

	return IssuePolicy{
		ID:         issue.ID,
		Title:      issue.Title,
		Scope:      append([]string(nil), issue.Scope...),
		Acceptance: append(json.RawMessage(nil), issue.Acceptance...),
		Citations:  resolveCitationChecks(issue, manifest),
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

func resolveCitationChecks(issue materialize.Issue, manifest sources.Manifest) []CitationCheck {
	if len(issue.SourceLinks) == 0 {
		return nil
	}

	// An accept-citation op with no SourceEntryID is a global acceptance that
	// covers all linked sources (matching the CLI behaviour of arm accept-citation).
	globallyAccepted := false
	accepted := make(map[string]bool, len(issue.CitationAcceptances))
	for _, acceptance := range issue.CitationAcceptances {
		if acceptance.SourceEntryID == "" {
			globallyAccepted = true
		} else {
			accepted[acceptance.SourceEntryID] = true
		}
	}

	checks := make([]CitationCheck, 0, len(issue.SourceLinks))
	for _, link := range issue.SourceLinks {
		if link.SourceEntryID == "" {
			continue
		}
		if _, ok := manifest.Get(link.SourceEntryID); !ok {
			checks = append(checks, CitationCheck{
				SourceEntryID: link.SourceEntryID,
				Accepted:      globallyAccepted,
			})
			continue
		}
		checks = append(checks, CitationCheck{
			SourceEntryID: link.SourceEntryID,
			Accepted:      globallyAccepted || accepted[link.SourceEntryID],
		})
	}
	return checks
}
