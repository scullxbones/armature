package main

import (
	"fmt"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/sources"
	"github.com/spf13/cobra"
)

func sourcesDir(ctx *config.Context) string {
	if ctx == nil {
		return filepath.Join(".", "sources")
	}
	return filepath.Join(ctx.IssuesDir, "sources")
}

// sourcesLifecycle constructs a Lifecycle for the given sources directory,
// wiring in an auto-commit committer when a worktree path is available.
func sourcesLifecycle(appCtx *config.Context, dir string) *sources.Lifecycle {
	if appCtx.WorktreePath != "" {
		gc := adapters.New(appCtx.WorktreePath)
		return sources.NewLifecycleWithCommitter(dir, &sources.DefaultProviderRegistry{}, appCtx.WorktreePath, gc)
	}
	return sources.NewLifecycle(dir)
}

func newSourcesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sources",
		Short: "Manage external knowledge sources",
	}

	cmd.AddCommand(newSourcesAddCmd())
	cmd.AddCommand(newSourcesSyncCmd())
	cmd.AddCommand(newSourcesVerifyCmd())
	cmd.AddCommand(newSourcesLinkCmd())
	cmd.AddCommand(newSourcesAcceptCitationCmd())
	cmd.AddCommand(newSourcesStaleReviewCmd())

	return cmd
}

func newSourcesAddCmd() *cobra.Command {
	var url, providerType, title string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new source to the manifest",
		RunE: func(cmd *cobra.Command, args []string) error {
			appCtx := currentCtx(cmd)
			dir := sourcesDir(appCtx)

			// Create lifecycle with auto-commit support
			lc := sourcesLifecycle(appCtx, dir)

			entry := sources.SourceEntry{
				ID:           uuid.New().String(),
				URL:          url,
				Title:        title,
				ProviderType: providerType,
			}

			// Warn if filesystem path is relative.
			if providerType == "filesystem" && !filepath.IsAbs(url) {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
					"warning: relative filesystem path %q will be resolved from working directory at sync time; "+
						"safe when arm sync is always run from the repo root; use an absolute path to avoid this dependency\n", url)
			}

			registered, err := lc.Register(entry)
			if err != nil {
				return fmt.Errorf("register source: %w", err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "added source %s (%s)\n", registered.ID, registered.URL)
			return nil
		},
	}

	cmd.Flags().StringVar(&url, "url", "", "URL or path of the source")
	cmd.Flags().StringVar(&providerType, "type", "", "provider type: filesystem, confluence, sharepoint")
	cmd.Flags().StringVar(&title, "title", "", "optional title for the source")
	_ = cmd.MarkFlagRequired("url")
	_ = cmd.MarkFlagRequired("type")

	return cmd
}

func newSourcesSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Fetch and cache content for all sources",
		RunE: func(cmd *cobra.Command, args []string) error {
			appCtx := currentCtx(cmd)
			dir := sourcesDir(appCtx)

			// Create lifecycle with auto-commit support
			lc := sourcesLifecycle(appCtx, dir)

			entries, err := lc.ListAll()
			if err != nil {
				return fmt.Errorf("list sources: %w", err)
			}

			if len(entries) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no sources in manifest")
				return nil
			}

			workerID, logPath, err := resolveWorkerAndLog(appCtx)
			if err != nil {
				return fmt.Errorf("worker not initialized: %w", err)
			}

			results, err := lc.SyncAll(cmd.Context())

			for _, result := range results {
				if result.Error != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "sync %s: %v\n", result.ID, result.Error)
				} else {
					o := ops.Op{
						Type:      ops.OpSourceFingerprint,
						TargetID:  result.ID,
						Timestamp: nowEpoch(),
						WorkerID:  workerID,
						Payload: ops.Payload{
							SHA:      result.Fingerprint,
							Provider: result.ProviderType,
						},
					}
					if syncErr := appendLowStakesOp(mustState(cmd), logPath, o); syncErr != nil {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: emit source-fingerprint for %s: %v\n", result.ID, syncErr)
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "synced %s  fp=%s\n", result.ID, short(result.Fingerprint))
				}
			}

			// Return error only when all sources failed.
			if err != nil {
				return err
			}

			return nil
		},
	}
}

func newSourcesVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify",
		Short: "Verify cached content matches stored fingerprints",
		RunE: func(cmd *cobra.Command, args []string) error {
			appCtx := currentCtx(cmd)
			dir := sourcesDir(appCtx)

			// verify never writes, so no committer is needed.
			lc := sources.NewLifecycle(dir)

			entries, err := lc.ListAll()
			if err != nil {
				return fmt.Errorf("list sources: %w", err)
			}

			if len(entries) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no sources in manifest")
				return nil
			}

			results, err := lc.VerifyAll()

			for _, result := range results {
				if result.Error != nil {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-40s  ERROR  %v\n", result.ID, result.Error)
				} else {
					switch result.Status {
					case sources.VerifyOK:
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-40s  OK\n", result.ID)
					case sources.VerifyStale:
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-40s  STALE  (cached content exists but last sync failed)\n", result.ID)
					case sources.VerifyMissing:
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-40s  MISSING\n", result.ID)
					case sources.VerifyChanged:
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-40s  CHANGED  (stored=%s actual=%s)\n",
							result.ID, short(result.Stored), short(result.Current))
					}
				}
			}

			// The per-source lines above are the verify report and are already
			// on stdout; a non-OK result is that report's payload, not a
			// Command Failure (ADR 0020 §7). Exit non-zero on the protocol.
			if err != nil {
				return skipCommandFailure(err)
			}
			return nil
		},
	}
}

func newSourcesLinkCmd() *cobra.Command {
	cmd := newSourceLinkCmd()
	cmd.Use = "link [issue-id]"
	return cmd
}

func newSourcesAcceptCitationCmd() *cobra.Command {
	cmd := newAcceptCitationCmd()
	cmd.Use = "accept-citation [issue-id]"
	return cmd
}

func newSourcesStaleReviewCmd() *cobra.Command {
	cmd := newStaleReviewCmd()
	cmd.Use = "stale-review"
	return cmd
}
