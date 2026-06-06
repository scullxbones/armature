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

type TaskPolicy struct {
	ID         string
	Title      string
	Scope      []string
	Acceptance json.RawMessage
	Citations  []CitationCheck
}

type TaskPolicyResolver struct {
	cfg ResolverConfig
}

func NewTaskPolicyResolver(cfg ResolverConfig) *TaskPolicyResolver {
	return &TaskPolicyResolver{cfg: cfg}
}

func (r *TaskPolicyResolver) Resolve(taskID string) (TaskPolicy, error) {
	issuePath := filepath.Join(r.stateDir(), "issues", taskID+".json")
	issue, err := materialize.LoadIssue(issuePath)
	if err != nil {
		return TaskPolicy{}, fmt.Errorf("task %s not found: %w", taskID, err)
	}

	manifest, err := sources.ReadManifest(r.sourcesDir())
	if err != nil {
		return TaskPolicy{}, fmt.Errorf("read sources manifest: %w", err)
	}

	return TaskPolicy{
		ID:         issue.ID,
		Title:      issue.Title,
		Scope:      append([]string(nil), issue.Scope...),
		Acceptance: append(json.RawMessage(nil), issue.Acceptance...),
		Citations:  resolveCitationChecks(issue, manifest),
	}, nil
}

func (r *TaskPolicyResolver) stateDir() string {
	if r.cfg.StateDir != "" {
		return r.cfg.StateDir
	}
	return filepath.Join(r.cfg.RepoPath, ".armature", "state", "default")
}

func (r *TaskPolicyResolver) sourcesDir() string {
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
