package main

import (
	"errors"
	"fmt"

	"github.com/scullxbones/armature/internal/adapters"
	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/scullxbones/armature/internal/opsbackup"
	"github.com/spf13/cobra"
)

func newOpsBackupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ops-backup",
		Short: "Snapshot, verify, and plan restore of the published _armature tip",
		Long: `Durable tip copy for the unprotected orphan ops branch.

snapshot refreshes the published tip into a per-run tracking ref (never the
shared refs/remotes/origin/_armature tip), fail-closes on refresh failure,
never rewinds live refs/heads/_armature, and writes a git bundle that
advertises refs/heads/_armature.

verify checks that a bundle advertises that tip and that its object pack is
restorable (fetch into a throwaway bare — not header-only list-heads).

restore-plan compares the bundle tip to origin/_armature inside a throwaway
bare (never stages into the working clone) and prints an I2-safe publish
disposition (missing | fast-forward | already-contained |
diverge→_armature-recovered). Output names only durable things (recovered SHA,
bundle path, optional aside publish name). missing requires structural
ls-remote absence of origin/_armature; other ls-remote/fetch/resolve errors
stay fail-closed. A recovered tip already contained in current
origin/_armature history is already-contained (exit 0, no aside). Unrelated
tips diverge (exit non-zero). It does not apply the publish.`,
		SilenceErrors: true,
	}
	cmd.AddCommand(newOpsBackupSnapshotCmd())
	cmd.AddCommand(newOpsBackupVerifyCmd())
	cmd.AddCommand(newOpsBackupRestorePlanCmd())
	return cmd
}

func isOpsBackupCommand(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == "ops-backup" {
			return true
		}
	}
	return false
}

func opsBackupGit(cmd *cobra.Command) *adapters.Client {
	// Disaster-recovery verbs must work from a fresh clone without an ops
	// worktree / armature layout. Prefer the resolved code checkout when
	// present; otherwise use --repo / cwd.
	if state, err := stateFromCmd(cmd); err == nil && state.ctx != nil && state.ctx.RepoPath != "" {
		return adapters.New(state.ctx.RepoPath)
	}
	repoPath, _ := cmd.Root().PersistentFlags().GetString("repo")
	if repoPath == "" {
		repoPath = "."
	}
	return adapters.New(repoPath)
}

type opsBackupSnapshotRow struct {
	Status string `json:"status"`
	Tip    string `json:"tip"`
	Bundle string `json:"bundle"`
}

type opsBackupVerifyRow struct {
	Status     string `json:"status"`
	Bundle     string `json:"bundle"`
	Advertises string `json:"advertises"`
}

type opsBackupPlanRow struct {
	Disposition string `json:"disposition"`
	Recovered   string `json:"recovered"`
	Bundle      string `json:"bundle"`
	AsideRef    string `json:"aside_ref,omitempty"`
}

func newOpsBackupSnapshotCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Write a fail-closed bundle of the published _armature tip",
		RunE: func(cmd *cobra.Command, args []string) error {
			gc := opsBackupGit(cmd)
			res, err := opsbackup.Snapshot(gc, opsbackup.SnapshotRequest{OutputPath: output})
			if err != nil {
				return armerrors.Map("OPS-BACKUP-1", err.Error(), []string{"arm ops-backup snapshot --output <path>"}, err)
			}
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if isStructuredFormat(format) {
				return writeCommandEnvelope(cmd.OutOrStdout(), "snapshots", []opsBackupSnapshotRow{{
					Status: "ok",
					Tip:    res.Tip,
					Bundle: res.Bundle,
				}}, []string{"arm ops-backup verify <bundle> confirms the tip is restorable"}, nil)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "snapshot ok tip=%s bundle=%s\n", res.Tip, res.Bundle)
			return nil
		},
	}
	cmd.Flags().StringVar(&output, "output", "", "bundle output path")
	_ = cmd.MarkFlagRequired("output")
	return cmd
}

func newOpsBackupVerifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify <bundle>",
		Short: "Verify a bundle advertises a restorable _armature tip",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			gc := opsBackupGit(cmd)
			if err := opsbackup.Verify(gc, args[0]); err != nil {
				return armerrors.Map("OPS-BACKUP-1", err.Error(), []string{"arm ops-backup verify <bundle>"}, err)
			}
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if isStructuredFormat(format) {
				return writeCommandEnvelope(cmd.OutOrStdout(), "verifications", []opsBackupVerifyRow{{
					Status:     "ok",
					Bundle:     args[0],
					Advertises: opsbackup.AdvertisedRef,
				}}, []string{"arm ops-backup restore-plan --from <bundle> prints the I2 disposition"}, nil)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "verify ok advertises %s\n", opsbackup.AdvertisedRef)
			return nil
		},
	}
	return cmd
}

func newOpsBackupRestorePlanCmd() *cobra.Command {
	var from string
	cmd := &cobra.Command{
		Use:   "restore-plan",
		Short: "Print an I2-safe publish plan for a recovered tip (no --apply)",
		RunE: func(cmd *cobra.Command, args []string) error {
			gc := opsBackupGit(cmd)
			plan, err := opsbackup.PlanRestore(gc, from)
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			writePlan := func() error {
				if isStructuredFormat(format) {
					row := opsBackupPlanRow{
						Disposition: plan.Disposition.String(),
						Recovered:   plan.Recovered,
						Bundle:      plan.Bundle,
						AsideRef:    plan.AsideRef,
					}
					help := []string{"arm ops-backup restore-plan does not apply the publish"}
					if plan.Disposition == opsbackup.DispositionDivergeAside {
						help = []string{
							"push recovered tip only to refs/heads/_armature-recovered",
							"escalate; do not force-push origin/_armature",
						}
					}
					return writeCommandEnvelope(cmd.OutOrStdout(), "plans", []opsBackupPlanRow{row}, help, nil)
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", plan.Disposition.String())
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "recovered=%s bundle=%s\n", plan.Recovered, plan.Bundle)
				if plan.AsideRef != "" {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "aside_ref=%s\n", plan.AsideRef)
				}
				return nil
			}
			if err != nil {
				if errors.Is(err, opsbackup.ErrDiverge) {
					// Plan envelope (disposition + escalation help) is the
					// protocol payload; exit non-zero without a second
					// Command Failure envelope (ADR 0020 §7 / skipCommandFailure).
					if writeErr := writePlan(); writeErr != nil {
						return writeErr
					}
					return skipCommandFailure(err)
				}
				return armerrors.Map("OPS-BACKUP-1", err.Error(), []string{"arm ops-backup restore-plan --from <bundle>"}, err)
			}
			return writePlan()
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "bundle path to plan restore from")
	_ = cmd.MarkFlagRequired("from")
	return cmd
}
