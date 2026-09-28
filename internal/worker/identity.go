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

// MaxWorkerIDLen is the filename cap (APFS case-insensitive + git).
const MaxWorkerIDLen = 64

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

// InitWorker generates a new worker UUID and stores it in git config.
func InitWorker(repoPath string) (string, error) {
	id, err := Init(InitWorkerInput{RepoPath: repoPath})
	if err != nil {
		return "", err
	}
	return id.ID, nil
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

// ValidateWorkerID enforces filename-safe ids. Never silently coerce.
func ValidateWorkerID(id string) error {
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
	if err := ValidateWorkerID(id); err != nil {
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

// EnableWorktreeConfig turns on extensions.worktreeConfig in the shared repo
// config and moves core.bare / core.worktree into the main worktree config
// as git requires.
func EnableWorktreeConfig(repoPath string) error {
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

// Init writes a worktree-scoped id (never copies a baked clone id).
func Init(in InitWorkerInput) (Identity, error) {
	if in.RepoPath == "" {
		return Identity{}, fmt.Errorf("worker-init: repo path is required")
	}
	if err := EnableWorktreeConfig(in.RepoPath); err != nil {
		return Identity{}, err
	}
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = uuid.New().String()
	}
	if err := ValidateWorkerID(id); err != nil {
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
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec
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
