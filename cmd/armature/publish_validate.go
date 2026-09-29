package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/sources"
)

func refusePublishedGraph(ctx *config.Context, gc *adapters.Client) error {
	if ctx == nil {
		return fmt.Errorf("refusePublishedGraph: command context unavailable")
	}
	valCtx, cleanup, err := contextWithPublishedOps(ctx, gc)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return err
	}
	result, err := runGraphValidationOnCtx(valCtx, graphValidateCIOptions())
	if err != nil {
		return err
	}
	if result.OK {
		return nil
	}
	var rendered strings.Builder
	if renderErr := output.RenderValidation(&rendered, result, false); renderErr != nil {
		return fmt.Errorf("render validation: %w", renderErr)
	}
	return skipCommandFailure(fmt.Errorf(
		"%spush refused: validation failed with %d error(s) and %d warning(s) "+
			"(same contract as arm validate --ci / make validate-graph)",
		rendered.String(), len(result.Errors), len(result.Warnings)))
}

func publishValidateAfterIntegrate(ctx *config.Context, gc *adapters.Client, skipValidate bool) func() error {
	if skipValidate {
		return nil
	}
	return func() error { return refusePublishedGraph(ctx, gc) }
}

func contextWithPublishedOps(ctx *config.Context, gc *adapters.Client) (*config.Context, func(), error) {
	localOps := filepath.Join(ctx.IssuesDir, "ops")
	tmp, err := os.MkdirTemp("", "arm-publish-validate-")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { swallowErr(os.RemoveAll(tmp)) }
	tmpOps := filepath.Join(tmp, "ops")
	if err := os.MkdirAll(tmpOps, 0o750); err != nil {
		cleanup()
		return nil, nil, err
	}
	if err := copyDirFiles(localOps, tmpOps); err != nil {
		cleanup()
		return nil, nil, err
	}
	fetchedRemote := false
	if gc != nil {
		if fetchErr := gc.FetchTrackingRefWithoutMovingHEAD("_armature"); fetchErr == nil {
			fetchedRemote = true
			if overlayErr := overlayRemoteOpsLogs(gc, tmpOps); overlayErr != nil {
				cleanup()
				return nil, nil, overlayErr
			}
		}
	}
	if src := filepath.Join(ctx.IssuesDir, "sources"); repoPathReachable(src) {
		tmpSrc := filepath.Join(tmp, "sources")
		if err := os.MkdirAll(tmpSrc, 0o750); err != nil {
			cleanup()
			return nil, nil, err
		}
		if err := copyDirFiles(src, tmpSrc); err != nil {
			cleanup()
			return nil, nil, err
		}
	}
	if fetchedRemote {
		if overlayErr := overlayRemoteSources(gc, filepath.Join(tmp, "sources")); overlayErr != nil {
			cleanup()
			return nil, nil, overlayErr
		}
	}
	cloned := *ctx
	cloned.IssuesDir = tmp
	cloned.StateDir = filepath.Join(tmp, "state")
	if err := os.MkdirAll(cloned.StateDir, 0o750); err != nil {
		cleanup()
		return nil, nil, err
	}
	return &cloned, cleanup, nil
}

func overlayRemoteOpsLogs(gc *adapters.Client, destOps string) error {
	sha, err := gc.ResolveRevision("origin/_armature")
	if err != nil {
		if remoteOpsRefMissing(err) {
			return nil
		}
		return err
	}
	files, err := gc.ListFilesAtCommit(sha)
	if err != nil {
		return err
	}
	for _, rel := range files {
		if !strings.HasPrefix(rel, "ops/") {
			continue
		}
		base := filepath.Base(rel)
		if base == "" || base == "." || base == "ops" {
			continue
		}
		dest := filepath.Join(destOps, base)
		if _, statErr := os.Stat(dest); statErr == nil {
			continue
		}
		blob, showErr := gc.ShowFileAtCommit(sha, rel)
		if showErr != nil {
			return showErr
		}
		if writeErr := os.WriteFile(dest, blob, 0o600); writeErr != nil {
			return writeErr
		}
	}
	return nil
}

func overlayRemoteSources(gc *adapters.Client, destSources string) error {
	sha, err := gc.ResolveRevision("origin/_armature")
	if err != nil {
		if remoteOpsRefMissing(err) {
			return nil
		}
		return err
	}
	files, err := gc.ListFilesAtCommit(sha)
	if err != nil {
		return err
	}
	var remoteManifest []byte
	for _, rel := range files {
		if rel == "sources/manifest.json" {
			blob, showErr := gc.ShowFileAtCommit(sha, rel)
			if showErr != nil {
				return showErr
			}
			remoteManifest = blob
			continue
		}
		if !strings.HasPrefix(rel, "sources/") {
			continue
		}
		base := filepath.Base(rel)
		if base == "" || base == "." || base == "sources" {
			continue
		}
		if err := os.MkdirAll(destSources, 0o750); err != nil {
			return err
		}
		dest := filepath.Join(destSources, base)
		if _, statErr := os.Stat(dest); statErr == nil {
			continue
		}
		blob, showErr := gc.ShowFileAtCommit(sha, rel)
		if showErr != nil {
			return showErr
		}
		if writeErr := os.WriteFile(dest, blob, 0o600); writeErr != nil {
			return writeErr
		}
	}
	if len(remoteManifest) == 0 {
		return nil
	}
	return mergeSourceManifestFile(destSources, remoteManifest)
}

func mergeSourceManifestFile(destSources string, remoteBlob []byte) error {
	if err := os.MkdirAll(destSources, 0o750); err != nil {
		return err
	}
	local, err := sources.ReadManifest(destSources)
	if err != nil {
		return err
	}
	var remote sources.Manifest
	if err := remote.Unmarshal(remoteBlob); err != nil {
		return fmt.Errorf("parse remote sources/manifest.json: %w", err)
	}
	if local.Entries == nil {
		local.Entries = make(map[string]sources.SourceEntry)
	}
	for id, entry := range remote.Entries {
		if _, exists := local.Entries[id]; exists {
			continue
		}
		local.Entries[id] = entry
	}
	return sources.WriteManifest(destSources, local)
}

func remoteOpsRefMissing(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "couldn't find remote ref") ||
		strings.Contains(s, "invalid upstream") ||
		strings.Contains(s, "unknown revision") ||
		strings.Contains(s, "needed a single revision") ||
		strings.Contains(s, "no such ref") ||
		strings.Contains(s, "does not exist") ||
		strings.Contains(s, "not a valid object")
}

func copyDirFiles(src, dest string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		in, err := readOverlayFile(src, e.Name())
		if err != nil {
			return err
		}
		if err := writeOverlayFile(dest, e.Name(), in); err != nil {
			return err
		}
	}
	return nil
}

func readOverlayFile(src, name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(src, name)) //nolint:gosec // G304
}

func writeOverlayFile(dest, name string, body []byte) error {
	return os.WriteFile(filepath.Join(dest, name), body, 0o600)
}
