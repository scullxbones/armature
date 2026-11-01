package adapters

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/scullxbones/armature/internal/clock"
)

const (
	// runLeaseDir is under the common git directory (rev-parse
	// --git-common-dir), not a worktree-private git dir. Linked worktrees
	// share staging refs; leases must be equally visible so one worktree's
	// sweep cannot treat another's live nonce as stale.
	runLeaseDir = "armature-run-leases"

	// armatureWorkDir is under the common git directory. Throwaway bare
	// repos for bundle create/verify live here (not os.TempDir / not a
	// worktree-private git dir) so crash leftovers are colocated with
	// leases and swept on the next run from any linked worktree.
	armatureWorkDir = "armature-tmp-work"

	leaseKindArmatureTmp = "armature-tmp"
	leaseKindOriginRun   = "origin-run"
	leaseKindWork        = "tmp-work"

	// runLeaseMaxAgeSec is an absolute ceiling for a lease even if the pid
	// still appears alive (pid reuse / Windows FindProcess always-alive /
	// missing started line). Ops-backup runs finish in seconds; age is the
	// cross-platform reclaim backstop so nothing leaks forever.
	runLeaseMaxAgeSec int64 = 3600
)

// BeginPerRunRef sweeps stale peers under ns, allocates ns/<nonce>, and holds a
// live-pid lease until release. release deletes the ref and the lease. Safe
// under concurrent BeginPerRunRef for distinct nonces.
func (c *Client) BeginPerRunRef(ns string) (ref string, release func(), err error) {
	ns = strings.TrimRight(strings.TrimSpace(ns), "/")
	if ns == "" {
		return "", nil, fmt.Errorf("begin per-run ref: namespace required")
	}
	c.sweepStaleRefNS(ns, leaseKindForRefNS(ns))
	nonce, err := randomNonce()
	if err != nil {
		return "", nil, err
	}
	ref = ns + "/" + nonce
	if err := c.writeRunLease(leaseKindForRefNS(ns), nonce); err != nil {
		return "", nil, fmt.Errorf("begin per-run ref: lease: %w", err)
	}
	released := false
	release = func() {
		if released {
			return
		}
		released = true
		_ = c.DeleteRef(ref) //nolint:errcheck // best-effort
		_ = c.removeRunLease(leaseKindForRefNS(ns), nonce)
	}
	return ref, release, nil
}

// beginArmatureTmpNS allocates refs/armature-tmp/<nonce> with a live lease after
// sweeping crash leftovers. release deletes bundle+origin refs and the lease.
func (c *Client) beginArmatureTmpNS() (prefix string, release func(), err error) {
	c.sweepStaleRefNS(armatureTmpNS, leaseKindArmatureTmp)
	nonce, err := randomNonce()
	if err != nil {
		return "", nil, err
	}
	prefix = armatureTmpNS + "/" + nonce
	if err := c.writeRunLease(leaseKindArmatureTmp, nonce); err != nil {
		return "", nil, err
	}
	released := false
	release = func() {
		if released {
			return
		}
		released = true
		_ = c.DeleteRef(prefix + "/bundle")               //nolint:errcheck // best-effort
		_ = c.DeleteRef(prefix + "/origin")               //nolint:errcheck // best-effort
		_ = c.removeRunLease(leaseKindArmatureTmp, nonce) //nolint:errcheck // best-effort
	}
	return prefix, release, nil
}

// beginArmatureWorkDir allocates <common-git-dir>/armature-tmp-work/<nonce> for
// throwaway bare repos (bundle create/verify). Sweeps stale peers first. Lease
// is written before mkdir so a concurrent sweep cannot reclaim this nonce
// mid-create.
func (c *Client) beginArmatureWorkDir() (dir string, release func(), err error) {
	c.sweepStaleWorkDirs()
	common, err := c.CommonGitDir()
	if err != nil {
		return "", nil, err
	}
	nonce, err := randomNonce()
	if err != nil {
		return "", nil, err
	}
	if err := c.writeRunLease(leaseKindWork, nonce); err != nil {
		return "", nil, err
	}
	dir = filepath.Join(common, armatureWorkDir, nonce)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		_ = c.removeRunLease(leaseKindWork, nonce) //nolint:errcheck // best-effort
		return "", nil, fmt.Errorf("armature work dir: %w", err)
	}
	released := false
	release = func() {
		if released {
			return
		}
		released = true
		_ = os.RemoveAll(dir)                      //nolint:errcheck // best-effort
		_ = c.removeRunLease(leaseKindWork, nonce) //nolint:errcheck // best-effort
	}
	return dir, release, nil
}

func leaseKindForRefNS(ns string) string {
	switch {
	case ns == armatureTmpNS || strings.HasPrefix(ns, armatureTmpNS+"/"):
		return leaseKindArmatureTmp
	default:
		return leaseKindOriginRun
	}
}

func (c *Client) leasePath(kind, nonce string) (string, error) {
	common, err := c.CommonGitDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(common, runLeaseDir, kind, nonce), nil
}

// writeRunLease atomically publishes a lease via temp file + rename so a
// concurrent reader never observes a truncated body from a live writer.
func (c *Client) writeRunLease(kind, nonce string) error {
	path, err := c.leasePath(kind, nonce)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	body := fmt.Sprintf("pid=%d\nstarted=%d\n", os.Getpid(), clock.System())
	tmp, err := os.CreateTemp(dir, "."+nonce+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName) //nolint:errcheck // best-effort
		}
	}()
	if _, err := tmp.Write([]byte(body)); err != nil {
		_ = tmp.Close() //nolint:errcheck // best-effort
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close() //nolint:errcheck // best-effort
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close() //nolint:errcheck // best-effort
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func (c *Client) removeRunLease(kind, nonce string) error {
	path, err := c.leasePath(kind, nonce)
	if err != nil {
		return err
	}
	_ = os.Remove(path) //nolint:errcheck // best-effort
	return nil
}

func (c *Client) leaseIsLive(kind, nonce string) bool {
	path, err := c.leasePath(kind, nonce)
	if err != nil {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	raw, err := os.ReadFile(path) //nolint:gosec // G304: path under common git dir from our nonce
	if err != nil {
		return false
	}
	now := clock.System()
	pid, started, ok := parseRunLease(string(raw))

	// Age backstop on every platform: prefer started= when present; otherwise
	// file mtime (covers missing started line and Windows always-alive pids).
	ageStart := info.ModTime().Unix()
	if started > 0 {
		ageStart = started
	}
	if now-ageStart > runLeaseMaxAgeSec {
		return false
	}

	// Unparseable/empty but within the age window → treat as live so a
	// truncated mid-write (or crash before rename cleanup) is never swept.
	if !ok {
		return true
	}
	return pidAlive(pid)
}

func parseRunLease(body string) (pid int, started int64, ok bool) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "pid="):
			n, err := strconv.Atoi(strings.TrimPrefix(line, "pid="))
			if err != nil || n <= 0 {
				return 0, 0, false
			}
			pid = n
		case strings.HasPrefix(line, "started="):
			n, err := strconv.ParseInt(strings.TrimPrefix(line, "started="), 10, 64)
			if err != nil {
				return 0, 0, false
			}
			started = n
		}
	}
	return pid, started, pid > 0
}

// leaseNonceName reports whether name looks like a published lease nonce
// (16 hex chars). Skips CreateTemp leftovers (.<nonce>-*.tmp) under the
// lease directory.
func leaseNonceName(name string) bool {
	if len(name) != 16 {
		return false
	}
	for _, r := range name {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func (c *Client) sweepStaleRefNS(ns, kind string) {
	ns = strings.TrimRight(strings.TrimSpace(ns), "/")
	out, err := c.cmd("for-each-ref", "--format=%(refname)", ns+"/").Output()
	if err != nil {
		// Empty / missing namespace is fine.
		out = nil
	}
	nonces := map[string]struct{}{}
	for _, line := range strings.Split(string(out), "\n") {
		ref := strings.TrimSpace(line)
		if ref == "" {
			continue
		}
		rest := strings.TrimPrefix(ref, ns+"/")
		nonce, _, _ := strings.Cut(rest, "/")
		if nonce == "" || strings.Contains(nonce, "/") {
			continue
		}
		nonces[nonce] = struct{}{}
	}
	// Also consider lease files with no remaining refs (common git dir).
	if common, gerr := c.CommonGitDir(); gerr == nil {
		leaseRoot := filepath.Join(common, runLeaseDir, kind)
		entries, readErr := os.ReadDir(leaseRoot)
		if readErr == nil {
			for _, e := range entries {
				if e.IsDir() || !leaseNonceName(e.Name()) {
					continue
				}
				nonces[e.Name()] = struct{}{}
			}
		}
	}
	for nonce := range nonces {
		if c.leaseIsLive(kind, nonce) {
			continue
		}
		prefix := ns + "/" + nonce
		// Delete every ref under this nonce (bundle/origin or the run ref itself).
		out2, listErr := c.cmd("for-each-ref", "--format=%(refname)", prefix).CombinedOutput()
		if listErr == nil {
			for _, line := range strings.Split(string(out2), "\n") {
				ref := strings.TrimSpace(line)
				if ref == "" {
					continue
				}
				_ = c.DeleteRef(ref) //nolint:errcheck // best-effort sweep
			}
		}
		// Single-level run ref: for-each-ref on exact prefix may miss; try direct.
		_ = c.DeleteRef(prefix)           //nolint:errcheck // best-effort
		_ = c.removeRunLease(kind, nonce) //nolint:errcheck // best-effort
	}
}

func (c *Client) sweepStaleWorkDirs() {
	common, err := c.CommonGitDir()
	if err != nil {
		return
	}
	root := filepath.Join(common, armatureWorkDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if !os.IsNotExist(err) {
			return
		}
		entries = nil
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		nonce := e.Name()
		if c.leaseIsLive(leaseKindWork, nonce) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(root, nonce)) //nolint:errcheck // best-effort
		_ = c.removeRunLease(leaseKindWork, nonce)   //nolint:errcheck // best-effort
	}
	// Orphan leases with no dir.
	leaseRoot := filepath.Join(common, runLeaseDir, leaseKindWork)
	leaseEntries, readErr := os.ReadDir(leaseRoot)
	if readErr != nil {
		return
	}
	for _, e := range leaseEntries {
		if e.IsDir() || !leaseNonceName(e.Name()) {
			continue
		}
		nonce := e.Name()
		if c.leaseIsLive(leaseKindWork, nonce) {
			continue
		}
		_ = c.removeRunLease(leaseKindWork, nonce) //nolint:errcheck // best-effort
	}
}

func randomNonce() (string, error) {
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(nonce[:]), nil
}
