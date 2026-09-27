package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/output"
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

// contextWithPublishedOps returns a Context whose ops dir is the local logs
// overlaid with any origin/_armature worker logs not present locally — the
// graph CI will see after a successful integrate-and-push.
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
	if gc != nil {
		if fetchErr := gc.FetchTrackingRefWithoutMovingHEAD("_armature"); fetchErr == nil {
			if overlayErr := overlayRemoteOpsLogs(gc, tmpOps); overlayErr != nil {
				cleanup()
				return nil, nil, overlayErr
			}
		}
	}
	if src := filepath.Join(ctx.IssuesDir, "sources"); dirExists(src) {
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
		in, err := os.ReadFile(filepath.Join(src, e.Name())) //nolint:gosec
		if err != nil {
			return err
		}
		outPath := filepath.Join(dest, e.Name())
		//nolint:gosec // G703: dest is a temp overlay; names come from ReadDir of local ops/sources
		if err := os.WriteFile(outPath, in, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
