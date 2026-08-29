package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

func newGateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gate",
		Short: "Run configured quality-gate profiles and record evidence",
	}
	cmd.AddCommand(newGateRunCmd())
	return cmd
}

func newGateRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <profile>",
		Short: "Execute a configured gate profile and append evidence",
		Long: `Execute the command declared for <profile> in the tracked gates.json
at the invoking checkout root, stream output to a log file, and append a
gate-evidence op to the invoking worker's own log.

Profile name "full" is reserved as the publish profile. The command, HEAD,
and cleanliness checks use the invoking checkout (--repo or the current
directory), not the parent repo. A working tree that is dirty before the
command, or that the command dirties, still runs but records the result as
uncommitted (not citable). Repos with no gates.json (or an empty map) get
a clear error — armature does not infer make/Go.

The command is taken from the blob at HEAD:gates.json, not the worktree
file, so skip-worktree cannot substitute a different command.

A porcelain !! path is exempt only when git check-ignore names a source
that is a tracked file at HEAD (typically a committed .gitignore). Local
excludes (.git/info/exclude, core.excludesFile, XDG/global gitignore)
are not in Delivery.Diff and mark the run uncommitted.

The dirty check recurses into populated submodules with
--ignore-submodules=none --untracked-files=all. A gitlink directory that
has files but no .git is dirty. skip-worktree and assume-unchanged
(ls-files -v tags S or lowercase) are walked in the superproject and
populated submodules. The gate git client pins --work-tree/--git-dir
from the filesystem .git so GIT_WORK_TREE, GIT_DIR, and core.worktree
cannot point the check at a clean export.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGateProfile(cmd, args[0])
		},
	}
}

var validGateProfile = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func runGateProfile(cmd *cobra.Command, profile string) error {
	if !validGateProfile.MatchString(profile) {
		return fmt.Errorf("invalid gate profile %q: must match %s", profile, validGateProfile.String())
	}
	appCtx := currentCtx(cmd)
	// Context.RepoPath is the parent repo when this command runs inside a
	// linked task worktree. Context.WorktreePath is the ops worktree. Neither
	// is the checkout under test — use the invocation path for HEAD, dirtiness,
	// and the tracked gates.json so evidence matches the task head.
	checkout := invocationRepoPath(cmd)
	git := adapters.NewIsolated(checkout)
	headSHA, err := git.ResolveRevision("HEAD")
	if err != nil {
		return fmt.Errorf("resolve HEAD: %w", err)
	}
	// Execute the command recorded at HEAD:gates.json, not the worktree
	// file. skip-worktree / assume-unchanged can hide a mutated worktree
	// copy from porcelain; Delivery.Diff would also be silent.
	blob, err := git.ShowFileAtCommit(headSHA, config.GatesFileName)
	if err != nil {
		if isAbsentAtCommit(err) {
			return fmt.Errorf("no gates configured: add a gates map to gates.json (arm gate run is opt-in)")
		}
		return fmt.Errorf("load %s at HEAD: %w", config.GatesFileName, err)
	}
	gates, err := config.ParseGates(blob)
	if err != nil {
		return fmt.Errorf("load %s at HEAD: %w", config.GatesFileName, err)
	}
	gate, ok := gates[profile]
	if !ok {
		if len(gates) == 0 {
			return fmt.Errorf("no gates configured: add a gates map to gates.json (arm gate run is opt-in)")
		}
		return fmt.Errorf("gate profile %q is not configured", profile)
	}
	if len(gate.Command) == 0 {
		return fmt.Errorf("gate profile %q has an empty command", profile)
	}
	uncommitted, err := gateTreeUncommitted(git)
	if err != nil {
		return fmt.Errorf("check working tree: %w", err)
	}

	workerID, logPath, err := resolveWorkerAndLog(appCtx)
	if err != nil {
		return err
	}

	logFile, logFilePath, err := createGateLog(appCtx.IssuesDir, profile)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := logFile.Close(); closeErr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: close gate log: %v\n", closeErr)
		}
	}()

	stdout := io.MultiWriter(cmd.OutOrStdout(), logFile)
	stderr := io.MultiWriter(cmd.ErrOrStderr(), logFile)

	start := time.Now().Unix()
	_, runErr := adapters.RunProcess(cmd.Context(), checkout, gate.Command, stdout, stderr)
	end := time.Now().Unix()

	exit := 0
	if runErr != nil {
		var ee *exec.ExitError
		if errors.As(runErr, &ee) {
			exit = ee.ExitCode()
		} else {
			exit = 1
		}
	}

	afterDirty, dirtyErr := gateTreeUncommitted(git)
	if dirtyErr != nil {
		return fmt.Errorf("recheck working tree: %w", dirtyErr)
	}
	uncommitted = uncommitted || afterDirty
	// Re-read HEAD after the command. Keep HeadSHA as the pre-command
	// revision (the tree the reviewer thinks was tested) so attach cannot
	// silently accept a moved HEAD as a match for a different delivery
	// commit. A SHA change still marks the run uncommitted (I5).
	afterHEAD, headErr := git.ResolveRevision("HEAD")
	if headErr != nil {
		return fmt.Errorf("recheck HEAD: %w", headErr)
	}
	if afterHEAD != headSHA {
		uncommitted = true
	}

	if err := logFile.Sync(); err != nil {
		return fmt.Errorf("flush gate log: %w", err)
	}
	logBytes, err := os.ReadFile(logFilePath) //nolint:gosec // logFilePath is created under issuesDir/gates
	if err != nil {
		return fmt.Errorf("read gate log: %w", err)
	}
	absLog, err := filepath.Abs(logFilePath)
	if err != nil {
		return fmt.Errorf("resolve gate log path: %w", err)
	}
	outputHead, outputTail := gateOutputExcerpt(logBytes)

	ev := ops.GateEvidence{
		Profile:     profile,
		Command:     append([]string(nil), gate.Command...),
		HeadSHA:     headSHA,
		Start:       start,
		End:         end,
		Exit:        exit,
		Uncommitted: uncommitted,
		OutputHash:  hex.EncodeToString(sha256Sum(logBytes)),
		OutputHead:  outputHead,
		OutputTail:  outputTail,
		LogPath:     absLog,
	}

	var gc ops.GitCommitter
	if appCtx.WorktreePath != "" {
		gc = adapters.NewIsolated(appCtx.WorktreePath)
	}
	if err := ops.AppendGateEvidenceAndCommit(logPath, appCtx.WorktreePath, workerID, ev, gc); err != nil {
		return fmt.Errorf("record gate evidence: %w", err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "gate %s exit=%d log=%s\n", profile, exit, logFilePath)
	if uncommitted {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warning: dirty tree — gate evidence recorded as uncommitted (not citable)")
	}
	if runErr != nil {
		// The child's output and the status line above are already on stdout:
		// this is the gate's wire protocol, not a value handleRootError may
		// append to. Classify the nonzero return as a protocol exit so no
		// Command Failure object is concatenated after the streamed text
		// (ADR 0020 §7); the reason still reaches stderr.
		return skipCommandFailure(fmt.Errorf("gate %s exited %d: %w", profile, exit, runErr))
	}
	return nil
}

// gateTreeUncommitted reports whether the invoking checkout has non-ignored
// changes that are not armature state. The command itself is loaded from
// HEAD:gates.json (not the worktree file). A porcelain !! path is exempt
// only when check-ignore names a source tracked at HEAD (usually
// .gitignore); .git/info/exclude and other untracked ignore sources mark
// the run uncommitted. Status recurses into populated submodules with
// --untracked-files=all --ignore-submodules=none. A gitlink directory
// that has files but no .git is dirty. skip-worktree / assume-unchanged
// are walked in the superproject and populated submodules. The isolated
// git client pins --work-tree/--git-dir from the filesystem .git so
// GIT_WORK_TREE, GIT_DIR, and core.worktree cannot redirect the check.
func gateTreeUncommitted(git *adapters.Client) (bool, error) {
	entries, err := git.DirtyEntriesIncludingSubmodules()
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if isArmatureStatePath(entry.Path) && (entry.OldPath == "" || isArmatureStatePath(entry.OldPath)) {
			continue
		}
		if entry.Ignored {
			exempt, err := ignoreSourceTrackedAtHEAD(git, entry.Path)
			if err != nil {
				return false, err
			}
			if exempt {
				continue
			}
		}
		return true, nil
	}
	concealed, err := git.IndexConcealmentEntries()
	if err != nil {
		return false, err
	}
	for _, path := range concealed {
		if isArmatureStatePath(path) {
			continue
		}
		return true, nil
	}
	return false, nil
}

func isArmatureStatePath(path string) bool {
	return path == ".armature" || strings.HasPrefix(path, ".armature/")
}

func ignoreSourceTrackedAtHEAD(git *adapters.Client, path string) (bool, error) {
	src, ignored, err := git.CheckIgnoreSource(path)
	if err != nil {
		return false, err
	}
	if !ignored || src == "" || strings.HasPrefix(src, ".git/") || filepath.IsAbs(src) {
		return false, nil
	}
	_, err = git.ShowFileAtCommit("HEAD", src)
	if err != nil {
		if isAbsentAtCommit(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func isAbsentAtCommit(err error) bool {
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return false
	}
	msg := string(ee.Stderr)
	return strings.Contains(msg, "does not exist") || strings.Contains(msg, "exists on disk, but not in")
}

const gateOutputChunkSize = 1024

func sha256Sum(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}

func gateOutputExcerpt(output []byte) (head, tail string) {
	if len(output) <= gateOutputChunkSize*2 {
		return string(output), ""
	}
	headEnd := gateOutputChunkSize
	for headEnd > 0 && !utf8.RuneStart(output[headEnd]) {
		headEnd--
	}
	tailStart := len(output) - gateOutputChunkSize
	for tailStart < len(output) && !utf8.RuneStart(output[tailStart]) {
		tailStart++
	}
	return string(output[:headEnd]), string(output[tailStart:])
}

func createGateLog(issuesDir, profile string) (*os.File, string, error) {
	dir := filepath.Join(issuesDir, "gates")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, "", fmt.Errorf("create gate log dir: %w", err)
	}
	name := fmt.Sprintf("%s-%d.log", profile, time.Now().UnixNano())
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600) //nolint:gosec // path is under issuesDir/gates; profile is charset-validated
	if err != nil {
		return nil, "", fmt.Errorf("create gate log: %w", err)
	}
	return f, path, nil
}
