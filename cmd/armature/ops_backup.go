package main

import (
	"encoding/json"
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

snapshot refreshes refs/remotes/origin/_armature with the same +refspec as
FetchTrackingRefWithoutMovingHEAD, fail-closes on refresh failure, never
rewinds live refs/heads/_armature, and writes a git bundle that advertises
refs/heads/_armature.

verify checks that a bundle is readable and advertises that tip name.

restore-plan loads the bundle into a scratch tracking ref and prints an
I2-safe publish disposition (missing | fast-forward | diverge→_armature-recovered).
It does not apply the publish. Diverge exits non-zero.`,
		SilenceErrors: true,
	}
	cmd.AddCommand(newOpsBackupSnapshotCmd())
	cmd.AddCommand(newOpsBackupVerifyCmd())
	cmd.AddCommand(newOpsBackupRestorePlanCmd())
	return cmd
}

func opsBackupGit(cmd *cobra.Command) *adapters.Client {
	// Use the code checkout (common git dir), not the ops worktree path: tip
	// refresh updates shared refs/remotes/origin/_armature and must not depend
	// on a linked worktree being present for disaster-recovery verbs.
	state := mustState(cmd)
	if state.ctx != nil && state.ctx.RepoPath != "" {
		return adapters.New(state.ctx.RepoPath)
	}
	repoPath, _ := cmd.Root().PersistentFlags().GetString("repo")
	if repoPath == "" {
		repoPath = "."
	}
	return adapters.New(repoPath)
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
				payload := map[string]string{
					"status": "ok",
					"tip":    res.Tip,
					"bundle": res.Bundle,
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetEscapeHTML(false)
				return enc.Encode(payload)
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
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), `{"status":"ok","bundle":%q,"advertises":%q}`+"\n", args[0], opsbackup.AdvertisedRef)
				return nil
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
			writePlan := func() {
				if isStructuredFormat(format) {
					payload := map[string]any{
						"disposition": plan.Disposition.String(),
						"recovered":   plan.Recovered,
						"scratch":     plan.Scratch,
					}
					if plan.AsideRef != "" {
						payload["aside_ref"] = plan.AsideRef
					}
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetEscapeHTML(false)
					if encErr := enc.Encode(payload); encErr != nil {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "ops-backup: encode plan: %v\n", encErr)
					}
					return
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", plan.Disposition.String())
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "recovered=%s scratch=%s\n", plan.Recovered, plan.Scratch)
				if plan.AsideRef != "" {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "aside_ref=%s\n", plan.AsideRef)
				}
			}
			if err != nil {
				if errors.Is(err, opsbackup.ErrDiverge) {
					writePlan()
					return armerrors.Map("OPS-BACKUP-2", err.Error(), []string{
						"push recovered tip only to refs/heads/_armature-recovered",
						"escalate; do not force-push origin/_armature",
					}, err)
				}
				return armerrors.Map("OPS-BACKUP-1", err.Error(), []string{"arm ops-backup restore-plan --from <bundle>"}, err)
			}
			writePlan()
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "bundle path to plan restore from")
	_ = cmd.MarkFlagRequired("from")
	return cmd
}
