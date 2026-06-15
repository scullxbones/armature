package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/sources"
	"github.com/spf13/cobra"
)

func sourcesDir(args ...*config.Context) string {
	ctx := appCtx
	if len(args) > 0 && args[0] != nil {
		ctx = args[0]
	}
	if ctx == nil {
		return filepath.Join(".", "sources")
	}
	return filepath.Join(ctx.IssuesDir, "sources")
}

func newSourcesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sources",
		Short: "Manage external knowledge sources",
	}

	cmd.AddCommand(newSourcesAddCmd())
	cmd.AddCommand(newSourcesSyncCmd())
	cmd.AddCommand(newSourcesVerifyCmd())

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
			manifest, err := sources.ReadManifest(dir)
			if err != nil {
				return fmt.Errorf("read manifest: %w", err)
			}

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

			manifest.Upsert(entry)

			if err := sources.WriteManifest(dir, manifest); err != nil {
				return fmt.Errorf("write manifest: %w", err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "added source %s (%s)\n", entry.ID, entry.URL)
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
			manifest, err := sources.ReadManifest(dir)
			if err != nil {
				return fmt.Errorf("read manifest: %w", err)
			}

			if len(manifest.Entries) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no sources in manifest")
				return nil
			}

			workerID, logPath, err := resolveWorkerAndLog(appCtx)
			if err != nil {
				return fmt.Errorf("worker not initialized: %w", err)
			}

			ctx := context.Background()
			var syncErrors []string
			syncedCount := 0
			for id, entry := range manifest.Entries {
				provider, err := providerForType(entry.ProviderType)
				if err != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "skip %s: %v\n", id, err)
					syncErrors = append(syncErrors, fmt.Sprintf("%s: %v", id, err))
					entry.SyncFailed = true
					manifest.Upsert(entry)
					continue
				}

				data, err := provider.Fetch(ctx, entry)
				if err != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "fetch %s: %v\n", id, err)
					syncErrors = append(syncErrors, fmt.Sprintf("%s: %v", id, err))
					entry.SyncFailed = true
					manifest.Upsert(entry)
					continue
				}

				entry.Fingerprint = sources.Fingerprint(data)
				entry.LastSynced = time.Now().UTC()
				entry.SyncFailed = false
				manifest.Upsert(entry)

				if err := sources.WriteCache(dir, id, data); err != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "write cache %s: %v\n", id, err)
					syncErrors = append(syncErrors, fmt.Sprintf("%s: %v", id, err))
					entry.SyncFailed = true
					manifest.Upsert(entry)
					continue
				}

				o := ops.Op{
					Type:      ops.OpSourceFingerprint,
					TargetID:  id,
					Timestamp: nowEpoch(),
					WorkerID:  workerID,
					Payload: ops.Payload{
						SHA:      entry.Fingerprint,
						Provider: entry.ProviderType,
					},
				}
				if err := appendLowStakesOp(mustState(cmd), logPath, o); err != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: emit source-fingerprint for %s: %v\n", id, err)
				}

				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "synced %s  fp=%s\n", id, entry.Fingerprint[:8])
				syncedCount++
			}

			if err := sources.WriteManifest(dir, manifest); err != nil {
				return fmt.Errorf("write manifest: %w", err)
			}

			// Return error only when no sources could be synced successfully.
			if syncedCount == 0 && len(syncErrors) > 0 {
				return fmt.Errorf("all sources failed to sync: %s", strings.Join(syncErrors, "; "))
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
			manifest, err := sources.ReadManifest(dir)
			if err != nil {
				return fmt.Errorf("read manifest: %w", err)
			}

			if len(manifest.Entries) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no sources in manifest")
				return nil
			}

			allOK := true
			for id, entry := range manifest.Entries {
				// Check if the last sync attempt failed
				if entry.SyncFailed {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-40s  STALE  (cached content exists but last sync failed)\n", id)
					allOK = false
					continue
				}

				data, err := sources.ReadCache(dir, id)
				if err != nil {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-40s  ERROR  %v\n", id, err)
					allOK = false
					continue
				}
				if data == nil {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-40s  MISSING\n", id)
					allOK = false
					continue
				}

				actual := sources.Fingerprint(data)
				if actual == entry.Fingerprint {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-40s  OK\n", id)
				} else {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-40s  CHANGED  (stored=%s actual=%s)\n",
						id, entry.Fingerprint[:8], actual[:8])
					allOK = false
				}
			}

			if !allOK {
				return fmt.Errorf("one or more sources have changed or are missing")
			}
			return nil
		},
	}
}

// providerForType returns the appropriate Provider for the given type string.
func providerForType(providerType string) (sources.Provider, error) {
	switch providerType {
	case "filesystem":
		return &sources.FilesystemProvider{}, nil
	case "confluence":
		return sources.NewConfluenceProvider("", sources.Credentials{}), nil
	case "sharepoint":
		return sources.NewSharePointProvider("", sources.Credentials{}), nil
	default:
		return nil, fmt.Errorf("unknown provider type %q", providerType)
	}
}
