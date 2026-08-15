package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
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
		Long: `Execute the command declared for <profile> in .armature/config.json
(gates map), stream output to a log file, and append a gate-evidence op to
the invoking worker's own log.

Profile name "full" is reserved as the publish profile. A dirty working tree
still runs the command but records the result as uncommitted (not citable).
Repos with no gates map get a clear error — armature does not infer make/Go.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGateProfile(cmd, args[0])
		},
	}
}

func runGateProfile(cmd *cobra.Command, profile string) error {
	appCtx := currentCtx(cmd)
	gate, ok := appCtx.Config.Gate(profile)
	if !ok {
		if len(appCtx.Config.Gates) == 0 {
			return fmt.Errorf("no gates configured: add a gates map to .armature/config.json (arm gate run is opt-in)")
		}
		return fmt.Errorf("gate profile %q is not configured", profile)
	}
	if len(gate.Command) == 0 {
		return fmt.Errorf("gate profile %q has an empty command", profile)
	}

	git := adapters.New(appCtx.RepoPath)
	headSHA, err := git.ResolveRevision("HEAD")
	if err != nil {
		return fmt.Errorf("resolve HEAD: %w", err)
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
	_, runErr := adapters.RunProcess(cmd.Context(), appCtx.RepoPath, gate.Command, stdout, stderr)
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

	ev := ops.GateEvidence{
		Profile:     profile,
		Command:     append([]string(nil), gate.Command...),
		HeadSHA:     headSHA,
		Start:       start,
		End:         end,
		Exit:        exit,
		Uncommitted: uncommitted,
	}

	var gc ops.GitCommitter
	if appCtx.WorktreePath != "" {
		gc = adapters.New(appCtx.WorktreePath)
	}
	if err := ops.AppendGateEvidenceAndCommit(logPath, appCtx.WorktreePath, workerID, ev, gc); err != nil {
		return fmt.Errorf("record gate evidence: %w", err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "gate %s exit=%d log=%s\n", profile, exit, logFilePath)
	if uncommitted {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warning: dirty tree — gate evidence recorded as uncommitted (not citable)")
	}
	if runErr != nil {
		return fmt.Errorf("gate %s exited %d: %w", profile, exit, runErr)
	}
	return nil
}

func gateTreeUncommitted(git *adapters.Client) (bool, error) {
	entries, err := git.DirtyEntries()
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.Ignored {
			continue
		}
		if isArmatureStatePath(entry.Path) && (entry.OldPath == "" || isArmatureStatePath(entry.OldPath)) {
			continue
		}
		return true, nil
	}
	return false, nil
}

func isArmatureStatePath(path string) bool {
	return path == ".armature" || strings.HasPrefix(path, ".armature/")
}

func createGateLog(issuesDir, profile string) (*os.File, string, error) {
	dir := filepath.Join(issuesDir, "gates")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, "", fmt.Errorf("create gate log dir: %w", err)
	}
	name := fmt.Sprintf("%s-%d.log", profile, time.Now().UnixNano())
	path := filepath.Join(dir, name)
	f, err := os.Create(path) //nolint:gosec // path is under issuesDir/gates, not caller-controlled
	if err != nil {
		return nil, "", fmt.Errorf("create gate log: %w", err)
	}
	return f, path, nil
}
