// Package worker resolves writer identity, log slots, and worktree-scoped git config.
package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/filelock"
)

const gitConfigKey = "armature.worker-id"

var workerIDPattern = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)

var reservedDOSNames = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true,
	"com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true,
	"lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// Identity is the resolved writer for this invocation.
type Identity struct {
	ID      string
	LogPath string
	Source  string
}

type IdentityInput struct {
	RepoPath            string
	IssuesDir           string
	FlagWorkerID        string
	EnvWorkerID         string
	EnvLogSlot          string
	WorktreeGitConfigID string
	CloneGitConfigID    string
}

type InitWorkerInput struct {
	RepoPath  string
	IssuesDir string
	ID        string
	Reuse     bool
}

// GetWorkerID reads the worker UUID from git config (worktree, then clone).
func GetWorkerID(repoPath string) (string, error) {
	ident, err := ResolveIdentity(IdentityInput{RepoPath: repoPath})
	if err != nil {
		return "", fmt.Errorf("worker ID not configured — run 'trls worker-init': %w", err)
	}
	return ident.ID, nil
}

// CheckWorkerID returns whether a worker ID is configured, and if so, what it is.
func CheckWorkerID(repoPath string) (bool, string) {
	id, err := GetWorkerID(repoPath)
	if err != nil {
		return false, ""
	}
	return true, id
}

func validateWorkerID(id string) error {
	if !workerIDPattern.MatchString(id) || reservedDOSNames[id] {
		return claim.ErrWorkerIDInvalid
	}
	return nil
}

// ValidateLogSlot enforces the same charset as worker ids.
func ValidateLogSlot(slot string) error {
	if slot == "" {
		return nil
	}
	if !workerIDPattern.MatchString(slot) || reservedDOSNames[slot] {
		return claim.ErrLogSlotInvalid
	}
	return nil
}

// ResolveIdentity: flag > worktree config > env > clone config.
func ResolveIdentity(in IdentityInput) (Identity, error) {
	gc := adapters.New(in.RepoPath)
	worktreeEnabled := false
	if v, err := gc.ReadGitConfig("extensions.worktreeConfig"); err == nil {
		worktreeEnabled = strings.EqualFold(strings.TrimSpace(v), "true")
	}
	worktreeID := strings.TrimSpace(in.WorktreeGitConfigID)
	if worktreeID == "" && worktreeEnabled {
		if v, err := gc.ReadGitConfigWorktree(gitConfigKey); err == nil {
			worktreeID = strings.TrimSpace(v)
		}
	}
	cloneID := strings.TrimSpace(in.CloneGitConfigID)
	if cloneID == "" {
		if v, err := gc.ReadGitConfig(gitConfigKey); err == nil {
			cloneID = strings.TrimSpace(v)
		}
	}
	var id, source string
	switch {
	case strings.TrimSpace(in.FlagWorkerID) != "":
		id, source = strings.TrimSpace(in.FlagWorkerID), "flag"
	case worktreeID != "":
		id, source = worktreeID, "worktree-config"
	case strings.TrimSpace(in.EnvWorkerID) != "":
		id, source = strings.TrimSpace(in.EnvWorkerID), "env"
	case cloneID != "":
		id, source = cloneID, "clone-config"
	default:
		return Identity{}, claim.ErrWorkerIDMissing
	}
	if err := validateWorkerID(id); err != nil {
		return Identity{}, err
	}
	stem := id
	if slot := strings.TrimSpace(in.EnvLogSlot); slot != "" {
		if err := ValidateLogSlot(slot); err != nil {
			return Identity{}, err
		}
		stem = id + "~" + slot
	}
	opsDir := filepath.Join(in.IssuesDir, "ops")
	if in.IssuesDir == "" {
		opsDir = "ops"
	}
	return Identity{ID: stem, LogPath: filepath.Join(opsDir, stem+".log"), Source: source}, nil
}

// enableWorktreeConfig turns on extensions.worktreeConfig in the shared repo
// config and moves core.bare / core.worktree into the main worktree config
// as git requires.
func enableWorktreeConfig(repoPath string) error {
	gc := adapters.New(repoPath)
	if err := gc.SetGitConfig("extensions.worktreeConfig", "true"); err != nil {
		return err
	}
	for _, key := range []string{"core.bare", "core.worktree"} {
		val, err := gc.ReadGitConfig(key)
		if err != nil || strings.TrimSpace(val) == "" {
			continue
		}
		if setErr := gc.SetGitConfigWorktree(key, val); setErr != nil {
			return setErr
		}
		if unsetErr := gc.UnsetGitConfig(key); unsetErr != nil {
			return unsetErr
		}
	}
	return nil
}

// SeedWorktreeIdentity copies fromRepo's resolved worker id into
// destWorktree's worktree-scoped config when dest has none.
//
// Git 2.43+ copies the main worktree's config.worktree on `git worktree add`;
// Git 2.34 (the product floor) does not. Claim-created task worktrees must
// keep the claiming worker's identity without relying on that newer behavior.
// If dest already has a worktree-scoped id, it is left alone (W21: distinct
// per-worktree ids after an explicit worker-init in that worktree).
func SeedWorktreeIdentity(fromRepo, destWorktree string) error {
	fromRepo = strings.TrimSpace(fromRepo)
	destWorktree = strings.TrimSpace(destWorktree)
	if fromRepo == "" || destWorktree == "" {
		return fmt.Errorf("seed worktree identity: repo and destination are required")
	}
	if filepath.Clean(fromRepo) == filepath.Clean(destWorktree) {
		return nil
	}
	dest := adapters.New(destWorktree)
	if v, err := dest.ReadGitConfigWorktree(gitConfigKey); err == nil && strings.TrimSpace(v) != "" {
		return nil
	}
	ident, err := ResolveIdentity(IdentityInput{RepoPath: fromRepo})
	if err != nil {
		return fmt.Errorf("seed worktree identity: resolve source: %w", err)
	}
	// ResolveIdentity may return a slotted stem (id~slot). Worktree config
	// stores the bare worker id; strip a log-slot suffix if present.
	id := ident.ID
	if i := strings.IndexByte(id, '~'); i >= 0 {
		id = id[:i]
	}
	if err := validateWorkerID(id); err != nil {
		return fmt.Errorf("seed worktree identity: %w", err)
	}
	if err := enableWorktreeConfig(destWorktree); err != nil {
		return fmt.Errorf("seed worktree identity: enable worktreeConfig: %w", err)
	}
	if err := dest.SetGitConfigWorktree(gitConfigKey, id); err != nil {
		return fmt.Errorf("seed worktree identity: set %s: %w", gitConfigKey, err)
	}
	return nil
}

// Init writes a worktree-scoped id (never copies a baked clone id).
func Init(in InitWorkerInput) (Identity, error) {
	if in.RepoPath == "" {
		return Identity{}, fmt.Errorf("worker-init: repo path is required")
	}
	if err := enableWorktreeConfig(in.RepoPath); err != nil {
		return Identity{}, err
	}
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = uuid.New().String()
	}
	if err := validateWorkerID(id); err != nil {
		return Identity{}, err
	}
	if !in.Reuse {
		if used := workerIDInUse(in, id); used {
			return Identity{}, claim.ErrWorkerIDInUse
		}
	}
	gc := adapters.New(in.RepoPath)
	if err := gc.SetGitConfigWorktree(gitConfigKey, id); err != nil {
		if err := gc.SetGitConfig(gitConfigKey, id); err != nil {
			return Identity{}, fmt.Errorf("failed to set worker ID: %w", err)
		}
	}
	return ResolveIdentity(IdentityInput{RepoPath: in.RepoPath, IssuesDir: in.IssuesDir})
}

func workerIDInUse(in InitWorkerInput, id string) bool {
	candidates := []string{
		filepath.Join(in.RepoPath, "ops", id+".log"),
		filepath.Join(in.IssuesDir, "ops", id+".log"),
		filepath.Join(in.RepoPath, ".armature", "ops", id+".log"),
	}
	for _, p := range candidates {
		if p == "" || strings.Contains(p, string(filepath.ListSeparator)) {
			continue
		}
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return true
		}
	}
	gc := adapters.New(in.RepoPath)
	for _, path := range []string{"ops/" + id + ".log", ".armature/ops/" + id + ".log"} {
		if gc.BlobExists("origin/_armature", path) || gc.BlobExists("_armature", path) || gc.BlobExists("HEAD", path) {
			return true
		}
	}
	return false
}

// LockLogID takes a non-blocking flock on <common-dir>/armature-log-<id>.lock.
func LockLogID(repoPath, workerID string) (unlock func(), err error) {
	if repoPath == "" || workerID == "" {
		return func() {}, nil
	}
	common, err := adapters.New(repoPath).CommonGitDir()
	if err != nil {
		return func() {}, nil
	}
	lockPath := filepath.Join(common, "armature-log-"+workerID+".lock")
	f, err := openLogLockFile(lockPath)
	if err != nil {
		return nil, err
	}
	locked, lockErr := filelock.TryLock(f)
	if lockErr != nil {
		if cerr := f.Close(); cerr != nil {
			return nil, fmt.Errorf("%w: close lock: %v", lockErr, cerr)
		}
		return nil, lockErr
	}
	if !locked {
		if cerr := f.Close(); cerr != nil {
			return nil, fmt.Errorf("%w: close lock: %v", claim.ErrLogSlotCollision, cerr)
		}
		return nil, claim.ErrLogSlotCollision
	}
	return func() {
		if err := filelock.Unlock(f); err != nil {
			fmt.Fprintf(os.Stderr, "unlock log flock: %v\n", err)
		}
		if err := f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close log flock: %v\n", err)
		}
	}, nil
}

func openLogLockFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // G304: trusted lock path under git common dir
}
