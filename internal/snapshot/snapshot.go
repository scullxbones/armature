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
	State           *materialize.State
	Index           materialize.Index
	Issues          map[string]*materialize.Issue
	Warnings        []string
	MaterializedOps []ops.Op
}

// Store owns ops-read→materialize→snapshot operations for a configured directory pair.
// Store is not safe for concurrent use. It is designed for sequential, per-command usage
// where Load/Issue/Index are called from a single goroutine.
type Store struct {
	opsDir   string
	stateDir string
	current  *Snapshot
}

func NewStore(opsDir, stateDir string) *Store {
	return &Store{
		opsDir:   opsDir,
		stateDir: stateDir,
	}
}

// Load materializes state from disk and caches it, replacing any previously
// cached snapshot. Call it both for the initial load and to refresh the
// cache after the underlying ops/state have changed. Returns a non-nil
// Snapshot with empty collections when opsDir is empty.
func (s *Store) Load(ctx context.Context) (*Snapshot, error) {
	items, offsets, warnings, err := ops.LoadFromDirWithOffsetsValidated(s.opsDir)
	if err != nil {
		return nil, fmt.Errorf("load ops: %w", err)
	}

	allOps := ops.ExtractOps(items)
	if allOps == nil {
		allOps = []ops.Op{}
	}

	state, result, err := materialize.Run(s.stateDir, allOps, offsets, materialize.Options{WriteStateFiles: true})
	if err != nil {
		return nil, fmt.Errorf("materialize: %w", err)
	}

	index, err := materialize.LoadIndex(filepath.Join(s.stateDir, "index.json"))
	if err != nil {
		return nil, fmt.Errorf("load index: %w", err)
	}

	issues := state.Issues
	if issues == nil {
		issues = make(map[string]*materialize.Issue)
	}

	snap := &Snapshot{
		State:           state,
		Index:           index,
		Issues:          issues,
		Warnings:        append(warnings, result.Warnings...),
		MaterializedOps: allOps,
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

func (s *Store) IssuePath(id string) string {
	issuesDir := filepath.Join(s.stateDir, "issues")
	return filepath.Join(issuesDir, id+".json")
}

func (s *Store) IndexPath() string {
	return filepath.Join(s.stateDir, "index.json")
}

func (s *Store) StatePath(name string) string {
	return filepath.Join(s.stateDir, name)
}
