package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/scullxbones/trellis/internal/sources"
	"github.com/spf13/cobra"
)

func sourcesDir() string {
	return filepath.Join(appCtx.IssuesDir, "sources")
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
			dir := sourcesDir()
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
					"warning: relative filesystem path %q will be resolved from working directory at sync time\n", url)
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
			dir := sourcesDir()
			manifest, err := sources.ReadManifest(dir)
			if err != nil {
				return fmt.Errorf("read manifest: %w", err)
			}

			if len(manifest.Entries) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no sources in manifest")
				return nil
			}

			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return fmt.Errorf("worker not initialized: %w", err)
			}

			ctx := context.Background()
			var fetchErrors []string
			for id, entry := range manifest.Entries {
				provider, err := providerForType(entry.ProviderType)
				if err != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "skip %s: %v\n", id, err)
					continue
				}

				data, err := provider.Fetch(ctx, entry)
				if err != nil {
					// For filesystem sources, collect errors to return instead of silently skipping.
					if entry.ProviderType == "filesystem" {
						fetchErrors = append(fetchErrors, fmt.Sprintf("%s: %v", id, err))
					}
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "fetch %s: %v\n", id, err)
					continue
				}

				entry.Fingerprint = sources.Fingerprint(data)
				entry.LastSynced = time.Now().UTC()
				manifest.Upsert(entry)

				if err := sources.WriteCache(dir, id, data); err != nil {
					return fmt.Errorf("write cache %s: %w", id, err)
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
				if err := appendLowStakesOp(logPath, o); err != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: emit source-fingerprint for %s: %v\n", id, err)
				}

				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "synced %s  fp=%s\n", id, entry.Fingerprint[:8])
			}

			if err := sources.WriteManifest(dir, manifest); err != nil {
				return fmt.Errorf("write manifest: %w", err)
			}

			// Return error if any filesystem sources failed to fetch.
			if len(fetchErrors) > 0 {
				return fmt.Errorf("filesystem sources unreachable: %s", strings.Join(fetchErrors, "; "))
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
			dir := sourcesDir()
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
