package main

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

func newPushOpsCmd() *cobra.Command {
	var overrideValidate bool
	var reason string

	cmd := &cobra.Command{
		Use:   "push-ops",
		Short: "Push ops logs to the remote _armature branch",
		Long: `Push the _armature branch (which contains ops logs) to the remote repository.

Before the push, this command rebases onto the current origin/_armature tip and
runs the same fail-closed graph validation as arm validate --ci / make validate-graph.
A dirty graph (errors or warnings, including W1 scope overlap) refuses the push,
prints the findings, and exits non-zero.

The escape hatch is --override-validate --reason <text>. It requires a controlling
terminal, records skipped_validate_gate, and is never a green publish. Skills must
not name this flag.`,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if overrideValidate && strings.TrimSpace(reason) == "" {
				return fmt.Errorf("--override-validate requires --reason")
			}

			state := mustState(cmd)
			ctx := state.ctx
			skipValidate := false
			if overrideValidate {
				if err := recordPublishValidateOverride(cmd, state, reason); err != nil {
					return err
				}
				skipValidate = true
			}

			gc := opsPublishGit(ctx, worktreeGit(ctx))
			if gc == nil {
				repoPath, _ := cmd.Root().PersistentFlags().GetString("repo")
				if repoPath == "" {
					repoPath = "."
				}
				gc = adapters.New(repoPath)
			}

			format, _ := cmd.Root().PersistentFlags().GetString("format")

			if err := pushOpsBranchAfter(ctx, gc, state.tracker, nil, skipValidate); err != nil {
				code := "PUSH-OPS-1"
				if strings.Contains(err.Error(), "push refused: validation") {
					code = "PUSH-OPS-2"
				}
				return wrapOpsPublishFailure(code, newOpsPublishError(fmt.Errorf("push-ops: push failed: %w", err)))
			}

			if format == "json" || format == "agent" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), `{"status":"pushed","branch":"_armature"}`+"\n")
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Pushed _armature branch to remote\n")
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&overrideValidate, "override-validate", false,
		"Human escape hatch: publish despite a dirty graph. Requires --reason and a TTY. Never green.")
	cmd.Flags().StringVar(&reason, "reason", "", "Recorded reason for --override-validate")
	return cmd
}

func recordPublishValidateOverride(cmd *cobra.Command, state *executionState, reason string) error {
	nonInteractive, err := cmd.Root().PersistentFlags().GetBool("non-interactive")
	if err != nil {
		return fmt.Errorf("non-interactive flag: %w", err)
	}
	if nonInteractive {
		return fmt.Errorf("publish validate override requires a controlling terminal")
	}
	tty, err := openControllingTTY()
	if err != nil {
		return fmt.Errorf("publish validate override requires a controlling terminal")
	}
	defer bestEffortClose(tty)

	_, _ = fmt.Fprintf(tty, "WARNING: --override-validate publishes a graph that arm validate --ci / make validate-graph would reject. This is never green.\n")
	_, _ = fmt.Fprintf(tty, "Type OVERRIDE-VALIDATE to confirm: ")
	line, err := bufio.NewReader(tty).ReadString('\n')
	if err != nil {
		return fmt.Errorf("read confirmation: %w", err)
	}
	if strings.TrimSpace(line) != "OVERRIDE-VALIDATE" {
		return fmt.Errorf("typed confirmation does not match OVERRIDE-VALIDATE")
	}

	ctx := state.ctx
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	if err != nil {
		return fmt.Errorf("worker not initialized: %w", err)
	}
	op := ops.Op{
		Type:      ops.OpNote,
		TargetID:  "push-ops",
		Timestamp: nowEpoch(),
		WorkerID:  workerID,
		Payload: ops.Payload{
			Msg:                 "publish validate override: " + reason,
			Rationale:           reason,
			SkippedValidateGate: true,
		},
	}
	if err := appendOp(ctx, logPath, op); err != nil {
		return err
	}
	return nil
}
