package snapshot

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

// Snapshot is the result of loading and materializing the full repo state.
type Snapshot struct {
	State    *materialize.State
	Index    materialize.Index
	Issues   map[string]*materialize.Issue
	Warnings []string
}

// Load materializes state from opsDir and stateDir, returning a populated Snapshot.
// Returns a non-nil Snapshot with empty collections when opsDir is empty.
func Load(opsDir, stateDir string, singleBranch bool) (*Snapshot, error) {
	items, offsets, warnings, err := ops.LoadFromDirWithOffsetsValidated(opsDir)
	if err != nil {
		return nil, fmt.Errorf("load ops: %w", err)
	}

	allOps := ops.ExtractOps(items)

	state, result, err := materialize.MaterializeAndReturnQuiet(stateDir, allOps, singleBranch, offsets)
	if err != nil {
		return nil, fmt.Errorf("materialize: %w", err)
	}

	index, err := materialize.LoadIndex(filepath.Join(stateDir, "index.json"))
	if err != nil {
		return nil, fmt.Errorf("load index: %w", err)
	}

	issues := state.Issues
	if issues == nil {
		issues = make(map[string]*materialize.Issue)
	}

	return &Snapshot{
		State:    state,
		Index:    index,
		Issues:   issues,
		Warnings: append(warnings, result.Warnings...),
	}, nil
}

// Store owns ops-read→materialize→snapshot operations for a configured directory pair.
type Store struct {
	opsDir       string
	stateDir     string
	singleBranch bool
	current      *Snapshot
}

// NewStore creates a new Store for loading snapshots from the given directories.
func NewStore(opsDir, stateDir string, singleBranch bool) *Store {
	return &Store{
		opsDir:       opsDir,
		stateDir:     stateDir,
		singleBranch: singleBranch,
	}
}

// Load loads the snapshot from disk and caches it.
func (s *Store) Load(ctx context.Context) (*Snapshot, error) {
	snap, err := Load(s.opsDir, s.stateDir, s.singleBranch)
	if err != nil {
		return nil, err
	}
	s.current = snap
	return snap, nil
}

// Refresh reloads the snapshot from disk, updating the cache.
func (s *Store) Refresh(ctx context.Context) (*Snapshot, error) {
	return s.Load(ctx)
}

// Issue returns the Issue with the given ID, or nil if not found.
func (s *Store) Issue(id string) *materialize.Issue {
	if s.current == nil {
		return nil
	}
	return s.current.Issues[id]
}

// Index returns the current snapshot's index.
func (s *Store) Index() materialize.Index {
	if s.current == nil {
		return make(materialize.Index)
	}
	return s.current.Index
}

// IssuePath returns the filesystem path where an issue with the given ID is stored.
func (s *Store) IssuePath(id string) string {
	issuesDir := filepath.Join(s.stateDir, "issues")
	return filepath.Join(issuesDir, id+".json")
}

// IndexPath returns the filesystem path to the index file.
func (s *Store) IndexPath() string {
	return filepath.Join(s.stateDir, "index.json")
}
