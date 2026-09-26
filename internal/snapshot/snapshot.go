// Package snapshot captures point-in-time views of materialized task state.
package snapshot

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

type Snapshot struct {
	State           *materialize.State
	Index           materialize.Index
	Issues          map[string]*materialize.Issue
	Warnings        []string
	MaterializedOps []ops.Op
}

// Store is not safe for concurrent use.
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

func (s *Store) Load(ctx context.Context) (*Snapshot, error) {
	loaded, err := ops.LoadFromDirValidated(s.opsDir)
	if err != nil {
		return nil, fmt.Errorf("load ops: %w", err)
	}

	allOps := ops.ExtractOps(loaded.Items)
	if allOps == nil {
		allOps = []ops.Op{}
	}

	state, result, err := materialize.Run(s.stateDir, allOps, loaded.PhysicalEOF, materialize.Options{WriteStateFiles: true})
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
		Warnings:        append(loaded.Warnings, result.Warnings...),
		MaterializedOps: allOps,
	}
	s.current = snap
	return snap, nil
}

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

func (s *Store) ReadIndex() (materialize.Index, error) {
	return materialize.LoadIndex(s.IndexPath())
}

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
