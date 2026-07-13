// Package snapshot captures and restores point-in-time views of materialized task state.
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
func Load(opsDir, stateDir string) (*Snapshot, error) {
	items, offsets, warnings, err := ops.LoadFromDirWithOffsetsValidated(opsDir)
	if err != nil {
		return nil, fmt.Errorf("load ops: %w", err)
	}

	allOps := ops.ExtractOps(items)

	state, result, err := materialize.MaterializeAndReturnQuiet(stateDir, allOps, offsets)
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
// Store is not safe for concurrent use. It is designed for sequential, per-command usage
// where Load/Issue/Index are called from a single goroutine.
type Store struct {
	opsDir   string
	stateDir string
	current  *Snapshot
}

// NewStore creates a new Store for loading snapshots from the given directories.
func NewStore(opsDir, stateDir string) *Store {
	return &Store{
		opsDir:   opsDir,
		stateDir: stateDir,
	}
}

// Load loads the snapshot from disk and caches it, replacing any previously
// cached snapshot. Call it both for the initial load and to refresh the
// cache after the underlying ops/state have changed.
//
// ctx is accepted for API consistency with other Store methods; the
// underlying disk reads do not yet honor cancellation.
func (s *Store) Load(ctx context.Context) (*Snapshot, error) {
	snap, err := Load(s.opsDir, s.stateDir)
	if err != nil {
		return nil, err
	}
	s.current = snap
	return snap, nil
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

// ReadIndex reads the index directly from disk without triggering materialization and
// without consulting the s.current cache. It does not call materialize.Materialize* or
// write any state files. Use this instead of Index() when you need the on-disk index
// without a full Load cycle, or instead of store.Load() when the caller only
// needs index data before appending an op. Contrast with Index(), which returns the
// cached data from the most recent Load call.
func (s *Store) ReadIndex() (materialize.Index, error) {
	return materialize.LoadIndex(s.IndexPath())
}

// ReadIssue reads a single issue directly from disk without triggering materialization and
// without consulting the s.current cache. It does not call materialize.Materialize* or
// write any state files. Use this instead of Issue() when you need a single issue from disk
// without a full Load cycle. Contrast with Issue(), which returns the cached data
// from the most recent Load call. Returns an error if the issue file does not exist.
func (s *Store) ReadIssue(id string) (*materialize.Issue, error) {
	issue, err := materialize.LoadIssue(s.IssuePath(id))
	if err != nil {
		return nil, err
	}
	return &issue, nil
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

// StatePath returns the filesystem path for a named file within the state directory.
// Use this instead of filepath.Join(ctx.StateDir, name) in handler code.
func (s *Store) StatePath(name string) string {
	return filepath.Join(s.stateDir, name)
}
