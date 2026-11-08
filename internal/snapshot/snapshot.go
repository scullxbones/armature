// Package snapshot captures point-in-time views of materialized task state.
package snapshot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	opsDir      string
	stateDir    string
	opsWorktree string
	current     *Snapshot
}

func NewStore(opsDir, stateDir string) *Store {
	return &Store{
		opsDir:   opsDir,
		stateDir: stateDir,
	}
}

// WithOpsWorktree returns the store configured to materialize from the ops
// git checkout's committed order (LocateOps) when the path contains .git.
// Production callers via newSnapshotStore always set this. A path without
// .git (legacy plain-.armature fixtures) still configures the store but Load
// falls back to file-concat allOps replay.
func (s *Store) WithOpsWorktree(opsWorktree string) *Store {
	s.opsWorktree = opsWorktree
	return s
}

func (s *Store) Load(ctx context.Context) (*Snapshot, error) {
	return s.load(true)
}

// LoadReadOnly materializes in memory without writing checkpoint, index, or
// issue state files. Used by dry-run classification paths and host-ref reads.
func (s *Store) LoadReadOnly(ctx context.Context) (*Snapshot, error) {
	return s.load(false)
}

func (s *Store) load(writeStateFiles bool) (*Snapshot, error) {
	if strings.TrimSpace(s.stateDir) == "" {
		return nil, fmt.Errorf("snapshot: state dir is required (refusing cwd materialization)")
	}
	if strings.TrimSpace(s.opsWorktree) == "" {
		return nil, fmt.Errorf("snapshot: ops worktree is required for committed-order load")
	}

	loaded, err := ops.LoadFromDirValidated(s.opsDir)
	if err != nil {
		return nil, fmt.Errorf("load ops: %w", err)
	}

	allOps := ops.ExtractOps(loaded.Items)
	if allOps == nil {
		allOps = []ops.Op{}
	}

	// Pass OpsWorktree only when it is a real git checkout so materialize uses
	// committed LocateOps order. A configured path without .git (legacy test
	// fixtures that store ops under a plain .armature dir) keeps file-concat
	// allOps replay — production newSnapshotStore always points at a linked
	// worktree with .git.
	opsWorktree := s.opsWorktree
	if _, err := os.Stat(filepath.Join(opsWorktree, ".git")); err != nil {
		opsWorktree = ""
	}
	state, result, err := materialize.Run(s.stateDir, allOps, loaded.PhysicalEOF, materialize.Options{
		WriteStateFiles: writeStateFiles,
		OpsWorktree:     opsWorktree,
	})
	if err != nil {
		return nil, fmt.Errorf("materialize: %w", err)
	}

	issues := state.Issues
	if issues == nil {
		issues = make(map[string]*materialize.Issue)
	}

	var index materialize.Index
	if writeStateFiles {
		index, err = materialize.LoadIndex(filepath.Join(s.stateDir, "index.json"))
		if err != nil {
			return nil, fmt.Errorf("load index: %w", err)
		}
	} else {
		index = state.BuildIndex()
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
