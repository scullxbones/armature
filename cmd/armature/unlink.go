package main

import (
	"bufio"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/config"
	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/validate"
	"github.com/spf13/cobra"
)

func newUnlinkCmd() *cobra.Command {
	var sourceID, dep, reason string
	var overrideValidate bool

	cmd := &cobra.Command{
		Use:   "unlink",
		Short: "Remove a dependency link between issues",
		Long: `Remove a dependency relationship between two issues.

Unlink a source issue from a dependency (typically a blocked_by relationship).
This removes erroneous dependency links that were previously created with the link command.

The default path previews the post-edit Graph Finding set and refuses when the
edit would introduce findings (including a W1 overlap cascade). A recorded human
override is --override-validate --reason, which requires a controlling terminal
and is never green.`,
		Example: `  # Remove blocked_by dependency
  $ arm unlink --source E6-S4-T2 --dep E6-S4-T1`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGraphEdit(cmd, ops.OpUnlink, sourceID, dep, "blocked_by", overrideValidate, reason)
		},
	}

	cmd.Flags().StringVar(&sourceID, "source", "", "source issue ID")
	cmd.Flags().StringVar(&dep, "dep", "", "dependency issue ID to remove")
	cmd.Flags().BoolVar(&overrideValidate, "override-validate", false,
		"Human escape hatch: write the graph edit despite introduced findings. Requires --reason and a TTY. Never green.")
	cmd.Flags().StringVar(&reason, "reason", "", "Recorded reason for --override-validate")
	_ = cmd.MarkFlagRequired("source")
	_ = cmd.MarkFlagRequired("dep")
	return cmd
}

func runGraphEdit(cmd *cobra.Command, opType, sourceID, dep, rel string, overrideValidate bool, reason string) error {
	state := mustState(cmd)
	ctx := state.ctx
	workerID, logPath, err := resolveWorkerAndLogCmd(cmd)
	if err != nil {
		return err
	}
	op := ops.Op{Type: opType, TargetID: sourceID, Timestamp: nowEpoch(),
		WorkerID: workerID, Payload: ops.Payload{Dep: dep, Rel: rel}}

	if overrideValidate {
		if strings.TrimSpace(reason) == "" {
			return fmt.Errorf("--override-validate requires --reason")
		}
		if err := confirmGraphEditOverride(cmd); err != nil {
			return err
		}
		op.Payload.Rationale = reason
		op.Payload.SkippedValidateGate = true
		note := ops.Op{
			Type:      ops.OpNote,
			TargetID:  sourceID,
			Timestamp: nowEpoch(),
			WorkerID:  workerID,
			Payload: ops.Payload{
				Msg:                 "graph edit override: " + reason,
				Rationale:           reason,
				SkippedValidateGate: true,
			},
		}
		if err := appendHighStakesOpsExemptIntroduction(state, logPath, []ops.Op{note, op}, skipAfterIntegrate()); err != nil {
			return err
		}
	} else {
		if err := refuseGraphEditPreview(ctx, op, failureCodeForGraphEdit(opType), sourceID, dep); err != nil {
			return err
		}
		if err := appendOp(ctx, logPath, op); err != nil {
			return err
		}
	}
	if opType == ops.OpUnlink {
		writeCommandResult(cmd, map[string]string{"source": sourceID, "dep": dep},
			"Unlinked %s from %s\n", sourceID, dep)
		return nil
	}
	writeCommandResult(cmd, map[string]string{"source": sourceID, "dep": dep, "rel": rel},
		"Linked %s → %s (%s)\n", sourceID, dep, rel)
	return nil
}

func failureCodeForGraphEdit(opType string) string {
	if opType == ops.OpLink {
		return "LINK-1"
	}
	return "UNLINK-1"
}

func refuseGraphEditPreview(ctx *config.Context, op ops.Op, code, sourceID, dep string) error {
	introErr := refuseIntroduction(ctx, []ops.Op{op})
	if introErr == nil {
		return nil
	}
	cause := graphEditCascadeCause(ctx, op, sourceID, dep)
	if cause == "" {
		cause = strings.Split(introErr.Error(), "\n")[0]
	}
	return armerrors.Map(code, cause, []string{
		"arm link --source <id> --dep <id> --rel blocked_by",
		"arm amend --issue <id> --scope <narrower>",
	}, introErr)
}

func graphEditCascadeCause(ctx *config.Context, op ops.Op, sourceID, dep string) string {
	if ctx == nil {
		return ""
	}
	opsDir := filepath.Join(ctx.IssuesDir, "ops")
	allOps, err := readAllOpsFromDir(opsDir)
	if err != nil {
		return ""
	}
	state, skipped, firstErr := materialize.ReplayOpsTolerant(allOps)
	if state == nil {
		return ""
	}
	if skipped > 0 && firstErr != nil {
		return ""
	}
	after, err := projectValidateState(state, []ops.Op{op})
	if err != nil {
		return ""
	}
	result := validate.Validate(validateFacts(after.Issues, nowEpoch()), validate.Options{Strict: true, Now: nowEpoch()})
	var chosen *validate.Finding
	for i := range result.Findings {
		f := &result.Findings[i]
		if f.Rule != "W1" {
			continue
		}
		if citesGraphEditIDs(f.CitedIDs, sourceID, dep) {
			chosen = f
			break
		}
		if chosen == nil {
			chosen = f
		}
	}
	if chosen == nil {
		return ""
	}
	pair := strings.TrimPrefix(chosen.Message, "scope overlap: ")
	if pair == chosen.Message {
		ids := append([]string(nil), chosen.CitedIDs...)
		if len(ids) < 2 {
			ids = []string{sourceID, dep}
		}
		pair = ids[0] + " and " + ids[1]
	}
	return "cannot introduce Graph Finding (scope overlap W1): next pair " + pair
}

func citesGraphEditIDs(cited []string, sourceID, dep string) bool {
	var hasSource, hasDep bool
	for _, id := range cited {
		if id == sourceID {
			hasSource = true
		}
		if id == dep {
			hasDep = true
		}
	}
	return hasSource && hasDep
}

func confirmGraphEditOverride(cmd *cobra.Command) error {
	nonInteractive, err := cmd.Root().PersistentFlags().GetBool("non-interactive")
	if err != nil {
		return fmt.Errorf("non-interactive flag: %w", err)
	}
	if nonInteractive {
		return fmt.Errorf("graph edit override requires a controlling terminal")
	}
	tty, err := openControllingTTY()
	if err != nil {
		return fmt.Errorf("graph edit override requires a controlling terminal")
	}
	defer bestEffortClose(tty)

	_, _ = fmt.Fprintf(tty, "WARNING: --override-validate writes a graph edit that introduces Graph Findings. This is never green.\n")
	_, _ = fmt.Fprintf(tty, "Type OVERRIDE-VALIDATE to confirm: ")
	line, err := bufio.NewReader(tty).ReadString('\n')
	if err != nil {
		return fmt.Errorf("read confirmation: %w", err)
	}
	if strings.TrimSpace(line) != "OVERRIDE-VALIDATE" {
		return fmt.Errorf("typed confirmation does not match OVERRIDE-VALIDATE")
	}
	return nil
}
