package review_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// activityLogLineJSON builds a single JSONL activity log line (the format
// written by internal/harnesshook.AppendActivity / read by
// review.parseActivityLogFile) for use in tests. Fields omitted from opts
// default to their zero value.
func activityLogLineJSON(t *testing.T, opts map[string]any) string {
	t.Helper()
	line := map[string]any{
		"timestamp":       "2026-01-15T10:30:45Z",
		"command":         "make build",
		"exit_code":       0,
		"exit_code_known": true,
		"head_sha":        "abc123",
		"output_hash":     "def456",
	}
	for k, v := range opts {
		line[k] = v
	}
	data, err := json.Marshal(line)
	require.NoError(t, err)
	return string(data)
}

// mockGitAdapter is a mock implementation of GitAdapter for testing.
type mockGitAdapter struct {
	resolveRevisionFn   func(rev string) (string, error)
	diffRangeFn         func(base, head string) (string, error)
	diffNameOnlyRangeFn func(base, head string) ([]string, error)
}

func (m *mockGitAdapter) ResolveRevision(rev string) (string, error) {
	return m.resolveRevisionFn(rev)
}

func (m *mockGitAdapter) DiffRange(base, head string) (string, error) {
	return m.diffRangeFn(base, head)
}

func (m *mockGitAdapter) DiffNameOnlyRange(base, head string) ([]string, error) {
	return m.diffNameOnlyRangeFn(base, head)
}

func TestPrepare_Success(t *testing.T) {
	t.Parallel()

	baseSHA := "abc123abc123abc123abc123abc123abc123abc1"
	headSHA := "def456def456def456def456def456def456def4"

	git := &mockGitAdapter{
		resolveRevisionFn: func(rev string) (string, error) {
			if rev == "main" {
				return baseSHA, nil
			}
			if rev == "HEAD" {
				return headSHA, nil
			}
			return "", nil
		},
		diffRangeFn: func(base, head string) (string, error) {
			return "diff --git a/file.go b/file.go\n--- a/file.go\n+++ b/file.go\n@@ -1,3 +1,4 @@\n", nil
		},
		diffNameOnlyRangeFn: func(base, head string) ([]string, error) {
			return []string{"file.go", "test.go"}, nil
		},
	}

	bundle, err := review.Prepare(
		git, "SMTC-S1-T2", "Add git range helpers", "dod", "task", "outcome",
		[]string{"file1", "file2"}, []string{"criteria1", "criteria2"}, "main", "HEAD", "",
	)

	require.NoError(t, err)
	require.NotNil(t, bundle)

	// Verify bundle properties
	assert.Equal(t, review.SchemaVersion, bundle.SchemaVersion)
	assert.NotEmpty(t, bundle.BundleID)
	assert.Equal(t, "SMTC-S1-T2", bundle.Issue.ID)
	assert.Equal(t, "Add git range helpers", bundle.Issue.Title)
	assert.Equal(t, baseSHA, bundle.Delivery.BaseSHA)
	assert.Equal(t, headSHA, bundle.Delivery.HeadSHA)
	assert.Equal(t, []string{"file.go", "test.go"}, bundle.Delivery.ChangedFiles)
	assert.NotEmpty(t, bundle.Delivery.Diff)
	assert.NotEmpty(t, bundle.Fingerprints.Contract)
	assert.NotEmpty(t, bundle.Fingerprints.Delivery)
}

func TestPrepare_ResolveRevisionError(t *testing.T) {
	t.Parallel()

	git := &mockGitAdapter{
		resolveRevisionFn: func(rev string) (string, error) {
			return "", assert.AnError
		},
	}

	_, err := review.Prepare(git, "SMTC-S1-T2", "Test", "", "task", "", []string{}, []string{}, "main", "HEAD", "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve")
}

func TestPrepare_DiffRangeError(t *testing.T) {
	t.Parallel()

	git := &mockGitAdapter{
		resolveRevisionFn: func(rev string) (string, error) {
			return "someSHA", nil
		},
		diffRangeFn: func(base, head string) (string, error) {
			return "", assert.AnError
		},
	}

	_, err := review.Prepare(git, "SMTC-S1-T2", "Test", "", "task", "", []string{}, []string{}, "main", "HEAD", "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to compute diff")
}

func TestPrepare_DiffNameOnlyRangeError(t *testing.T) {
	t.Parallel()

	git := &mockGitAdapter{
		resolveRevisionFn: func(rev string) (string, error) {
			return "someSHA", nil
		},
		diffRangeFn: func(base, head string) (string, error) {
			return "diff content", nil
		},
		diffNameOnlyRangeFn: func(base, head string) ([]string, error) {
			return nil, assert.AnError
		},
	}

	_, err := review.Prepare(git, "SMTC-S1-T2", "Test", "", "task", "", []string{}, []string{}, "main", "HEAD", "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get changed files")
}

func TestPrepare_BundleIDDeterministic(t *testing.T) {
	t.Parallel()

	baseSHA := "abc123abc123abc123abc123abc123abc123abc1"
	headSHA := "def456def456def456def456def456def456def4"

	git := &mockGitAdapter{
		resolveRevisionFn: func(rev string) (string, error) {
			if rev == "main" {
				return baseSHA, nil
			}
			return headSHA, nil
		},
		diffRangeFn: func(base, head string) (string, error) {
			return "diff content", nil
		},
		diffNameOnlyRangeFn: func(base, head string) ([]string, error) {
			return []string{"file.go"}, nil
		},
	}

	// Call prepare twice with the same inputs
	bundle1, err1 := review.Prepare(git, "SMTC-S1-T2", "Test", "", "task", "", []string{}, []string{}, "main", "HEAD", "")
	require.NoError(t, err1)

	bundle2, err2 := review.Prepare(git, "SMTC-S1-T2", "Test", "", "task", "", []string{}, []string{}, "main", "HEAD", "")
	require.NoError(t, err2)

	// The bundle IDs should be identical
	assert.Equal(t, bundle1.BundleID, bundle2.BundleID)
}

func TestPrepare_WithCriteria(t *testing.T) {
	t.Parallel()

	baseSHA := "abc123abc123abc123abc123abc123abc123abc1"
	headSHA := "def456def456def456def456def456def456def4"

	criteria := []string{
		"Functionality meets requirements",
		"Tests cover edge cases",
		"Documentation is complete",
	}

	git := &mockGitAdapter{
		resolveRevisionFn: func(rev string) (string, error) {
			if rev == "main" {
				return baseSHA, nil
			}
			return headSHA, nil
		},
		diffRangeFn: func(base, head string) (string, error) {
			return "diff content", nil
		},
		diffNameOnlyRangeFn: func(base, head string) ([]string, error) {
			return []string{"feature.go"}, nil
		},
	}

	bundle, err := review.Prepare(git, "SMTC-S1-T2", "Add feature", "", "task", "", []string{}, criteria, "main", "HEAD", "")

	require.NoError(t, err)
	require.NotNil(t, bundle)

	// Verify criteria are stored in the contract
	assert.Equal(t, criteria, bundle.Contract.Acceptance)
}

func TestPrepare_EmptyChangedFiles(t *testing.T) {
	t.Parallel()

	baseSHA := "abc123abc123abc123abc123abc123abc123abc1"
	headSHA := "def456def456def456def456def456def456def4"

	git := &mockGitAdapter{
		resolveRevisionFn: func(rev string) (string, error) {
			if rev == "main" {
				return baseSHA, nil
			}
			return headSHA, nil
		},
		diffRangeFn: func(base, head string) (string, error) {
			return "", nil
		},
		diffNameOnlyRangeFn: func(base, head string) ([]string, error) {
			return []string{}, nil
		},
	}

	_, err := review.Prepare(git, "SMTC-S1-T2", "Test", "", "task", "", []string{}, []string{}, "main", "HEAD", "")

	require.Error(t, err, "empty delivery should return an error")
	assert.Contains(t, err.Error(), "no changed files")
}

func TestPrepare_IssueTitlePreserved(t *testing.T) {
	t.Parallel()

	baseSHA := "abc123abc123abc123abc123abc123abc123abc1"
	headSHA := "def456def456def456def456def456def456def4"

	issueTitle := "Complex feature implementation with tests"

	git := &mockGitAdapter{
		resolveRevisionFn: func(rev string) (string, error) {
			if rev == "main" {
				return baseSHA, nil
			}
			return headSHA, nil
		},
		diffRangeFn: func(base, head string) (string, error) {
			return "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n+code\n", nil
		},
		diffNameOnlyRangeFn: func(base, head string) ([]string, error) {
			return []string{"main.go"}, nil
		},
	}

	bundle, err := review.Prepare(git, "SMTC-S1-T2", issueTitle, "", "task", "", []string{}, []string{}, "main", "HEAD", "")

	require.NoError(t, err)
	assert.Equal(t, issueTitle, bundle.Issue.Title)
}

func TestPrepare_BundleValidation(t *testing.T) {
	t.Parallel()

	baseSHA := "abc123abc123abc123abc123abc123abc123abc1"
	headSHA := "def456def456def456def456def456def456def4"

	git := &mockGitAdapter{
		resolveRevisionFn: func(rev string) (string, error) {
			if rev == "main" {
				return baseSHA, nil
			}
			return headSHA, nil
		},
		diffRangeFn: func(base, head string) (string, error) {
			return "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n+code\n", nil
		},
		diffNameOnlyRangeFn: func(base, head string) ([]string, error) {
			return []string{"main.go"}, nil
		},
	}

	bundle, err := review.Prepare(git, "SMTC-S1-T2", "Test", "", "task", "", []string{}, []string{}, "main", "HEAD", "")

	require.NoError(t, err)
	// Verify the bundle is valid
	assert.NoError(t, bundle.Valid())
}

func TestPrepare_ExcludesArmatureArtifacts(t *testing.T) {
	t.Parallel()

	baseSHA := "abc123abc123abc123abc123abc123abc123abc1"
	headSHA := "def456def456def456def456def456def456def4"

	git := &mockGitAdapter{
		resolveRevisionFn: func(rev string) (string, error) {
			if rev == "main" {
				return baseSHA, nil
			}
			return headSHA, nil
		},
		diffRangeFn: func(base, head string) (string, error) {
			diff := "diff --git a/.armature/ops/worker.log b/.armature/ops/worker.log\n"
			diff += "--- a/.armature/ops/worker.log\n"
			diff += "+++ b/.armature/ops/worker.log\n"
			diff += "@@ -1,1 +1,2 @@\n"
			return diff, nil
		},
		diffNameOnlyRangeFn: func(base, head string) ([]string, error) {
			return []string{".armature/ops/worker.log", "internal/foo.go"}, nil
		},
	}

	bundle, err := review.Prepare(git, "SMTC-S1-T2", "Add feature", "", "task", "", []string{}, []string{}, "main", "HEAD", "")

	require.NoError(t, err)
	require.NotNil(t, bundle)
	// Only internal/foo.go should be in changed files, not .armature paths
	assert.Equal(t, []string{"internal/foo.go"}, bundle.Delivery.ChangedFiles)
}

func TestPrepare_ExcludesArmDir(t *testing.T) {
	t.Parallel()

	baseSHA := "abc123abc123abc123abc123abc123abc123abc1"
	headSHA := "def456def456def456def456def456def456def4"

	git := &mockGitAdapter{
		resolveRevisionFn: func(rev string) (string, error) {
			if rev == "main" {
				return baseSHA, nil
			}
			return headSHA, nil
		},
		diffRangeFn: func(base, head string) (string, error) {
			return "diff", nil
		},
		diffNameOnlyRangeFn: func(base, head string) ([]string, error) {
			return []string{".arm/state.json", "cmd/main.go"}, nil
		},
	}

	bundle, err := review.Prepare(git, "SMTC-S1-T2", "Add feature", "", "task", "", []string{}, []string{}, "main", "HEAD", "")

	require.NoError(t, err)
	require.NotNil(t, bundle)
	assert.Equal(t, []string{"cmd/main.go"}, bundle.Delivery.ChangedFiles)
}

func TestPrepare_ErrorWhenDeliveryEmpty(t *testing.T) {
	t.Parallel()

	baseSHA := "abc123abc123abc123abc123abc123abc123abc1"
	headSHA := "def456def456def456def456def456def456def4"

	git := &mockGitAdapter{
		resolveRevisionFn: func(rev string) (string, error) {
			if rev == "main" {
				return baseSHA, nil
			}
			return headSHA, nil
		},
		diffRangeFn: func(base, head string) (string, error) {
			return "", nil
		},
		diffNameOnlyRangeFn: func(base, head string) ([]string, error) {
			// Only armature coordination paths
			return []string{".armature/ops/worker.log", ".arm/state.json"}, nil
		},
	}

	_, err := review.Prepare(git, "SMTC-S1-T2", "Task", "", "task", "", []string{}, []string{}, "main", "HEAD", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no changed files")
}

func TestFilterDiff_OnlyExcludedSections_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	diff := "diff --git a/.armature/ops/worker.log b/.armature/ops/worker.log\n" +
		"--- a/.armature/ops/worker.log\n" +
		"+++ b/.armature/ops/worker.log\n" +
		"@@ -1 +1,2 @@\n" +
		"+new op\n" +
		" old op\n"

	result := review.FilterDiff(diff, []string{".armature/", ".arm/"})
	assert.Empty(t, result, "diff with only excluded sections should return empty string")
}

func TestFilterDiff_MixedSections_KeepsProductOnly(t *testing.T) {
	t.Parallel()

	armaturePart := "diff --git a/.armature/ops/worker.log b/.armature/ops/worker.log\n" +
		"--- a/.armature/ops/worker.log\n" +
		"+++ b/.armature/ops/worker.log\n" +
		"@@ -1 +1,2 @@\n" +
		"+new op\n"
	productPart := "diff --git a/internal/foo.go b/internal/foo.go\n" +
		"--- a/internal/foo.go\n" +
		"+++ b/internal/foo.go\n" +
		"@@ -1 +1,2 @@\n" +
		"+func foo() {}\n"

	diff := armaturePart + productPart

	result := review.FilterDiff(diff, []string{".armature/", ".arm/"})
	assert.Equal(t, productPart, result, "only product sections should be kept")
}

func TestFilterDiff_NoExcludedSections_Unchanged(t *testing.T) {
	t.Parallel()

	diff := "diff --git a/internal/foo.go b/internal/foo.go\n" +
		"--- a/internal/foo.go\n" +
		"+++ b/internal/foo.go\n" +
		"@@ -1 +1,2 @@\n" +
		"+func foo() {}\n"

	result := review.FilterDiff(diff, []string{".armature/", ".arm/"})
	assert.Equal(t, diff, result, "diff with no excluded sections should be unchanged")
}

func TestFilterDiff_EmptyDiff_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	result := review.FilterDiff("", []string{".armature/"})
	assert.Empty(t, result)
}

func TestFilterDiff_NoExcludePrefixes_Unchanged(t *testing.T) {
	t.Parallel()

	diff := "diff --git a/.armature/foo b/.armature/foo\n+line\n"
	result := review.FilterDiff(diff, nil)
	assert.Equal(t, diff, result, "with no exclude prefixes, diff must be returned unchanged")
}

func TestPrepare_DiffFiltersExcludedPaths(t *testing.T) {
	t.Parallel()

	baseSHA := "abc123abc123abc123abc123abc123abc123abc1"
	headSHA := "def456def456def456def456def456def456def4"

	armatureDiff := "diff --git a/.armature/ops/worker.log b/.armature/ops/worker.log\n" +
		"--- a/.armature/ops/worker.log\n" +
		"+++ b/.armature/ops/worker.log\n" +
		"@@ -1 +1,2 @@\n" +
		"+new op\n"
	productDiff := "diff --git a/internal/foo.go b/internal/foo.go\n" +
		"--- a/internal/foo.go\n" +
		"+++ b/internal/foo.go\n" +
		"@@ -1 +1,2 @@\n" +
		"+func foo() {}\n"

	git := &mockGitAdapter{
		resolveRevisionFn: func(rev string) (string, error) {
			if rev == "main" {
				return baseSHA, nil
			}
			return headSHA, nil
		},
		diffRangeFn: func(base, head string) (string, error) {
			return armatureDiff + productDiff, nil
		},
		diffNameOnlyRangeFn: func(base, head string) ([]string, error) {
			return []string{".armature/ops/worker.log", "internal/foo.go"}, nil
		},
	}

	bundle, err := review.Prepare(git, "SMTC-S1-T2", "Add feature", "dod", "task", "done",
		[]string{}, []string{}, "main", "HEAD", "")

	require.NoError(t, err)
	assert.Equal(t, productDiff, bundle.Delivery.Diff,
		"Delivery.Diff must not contain excluded coordination paths")
	assert.NotContains(t, bundle.Delivery.Diff, ".armature/",
		"Delivery.Diff must not reference .armature/ paths")
}

func TestPrepare_MultipleArmaturePaths(t *testing.T) {
	t.Parallel()

	baseSHA := "abc123abc123abc123abc123abc123abc123abc1"
	headSHA := "def456def456def456def456def456def456def4"

	git := &mockGitAdapter{
		resolveRevisionFn: func(rev string) (string, error) {
			if rev == "main" {
				return baseSHA, nil
			}
			return headSHA, nil
		},
		diffRangeFn: func(base, head string) (string, error) {
			return "diff content", nil
		},
		diffNameOnlyRangeFn: func(base, head string) ([]string, error) {
			return []string{
				".armature/ops/worker1.log",
				".arm/config.json",
				"internal/service.go",
				".armature/state/task123.json",
				"pkg/util.go",
			}, nil
		},
	}

	bundle, err := review.Prepare(git, "SMTC-S1-T2", "Feature", "", "task", "", []string{}, []string{}, "main", "HEAD", "")

	require.NoError(t, err)
	require.NotNil(t, bundle)
	assert.Equal(t, []string{"internal/service.go", "pkg/util.go"}, bundle.Delivery.ChangedFiles)
}

func TestPrepare_WithActivityLog_REQ_EXECEV_T2(t *testing.T) {
	t.Parallel()

	baseSHA := "abc123abc123abc123abc123abc123abc123abc1"
	headSHA := "def456def456def456def456def456def456def4"

	// Create a temporary activity log file
	tmpDir := t.TempDir()
	logPath := tmpDir + "/armature-activity.log"

	// Write a sample activity log with multiple entries
	logContent := activityLogLineJSON(t, map[string]any{
		"command": "make build", "head_sha": "def456def456def456def456def456def456def4",
		"output_hash": "abc123", "output_head": "Build succeeded",
	}) + "\n" + activityLogLineJSON(t, map[string]any{
		"command": "make test", "head_sha": "def456def456def456def456def456def456def4",
		"output_hash": "def789", "output_head": "Tests passed",
	}) + "\n" + activityLogLineJSON(t, map[string]any{
		"command": "go lint", "head_sha": "abc123abc123abc123abc123abc123abc123abc1",
		"output_hash": "ghi012", "output_head": "Lint clean",
	})
	err := os.WriteFile(logPath, []byte(logContent), 0o644)
	require.NoError(t, err)

	git := &mockGitAdapter{
		resolveRevisionFn: func(rev string) (string, error) {
			if rev == "main" {
				return baseSHA, nil
			}
			return headSHA, nil
		},
		diffRangeFn: func(base, head string) (string, error) {
			return "diff --git a/file.go b/file.go\n", nil
		},
		diffNameOnlyRangeFn: func(base, head string) ([]string, error) {
			return []string{"file.go"}, nil
		},
	}

	bundle, err := review.Prepare(git, "EXECEV-T2", "Add activity support", "dod", "task", "done",
		[]string{}, []string{}, "main", "HEAD", logPath)

	require.NoError(t, err)
	require.NotNil(t, bundle)
	require.NotNil(t, bundle.Activity, "Activity section should be present")

	// Verify activity properties
	assert.NotEmpty(t, bundle.Activity.Digest, "Activity digest should be set")
	assert.Equal(t, 3, bundle.Activity.EntryCount, "Should have 3 activity entries")
	assert.Equal(t, 2, bundle.Activity.DeliveryHeadCount, "Should have 2 entries at delivery HEAD")
	assert.Equal(t, 1, bundle.Activity.EarlierCount, "Should have 1 entry at earlier commit")
	assert.NotEmpty(t, bundle.Activity.LogPath, "LogPath should be set")
}

// TestPrepare_ActivityLogPathIsAbsolute_REQ_EXECEV verifies that Activity.LogPath is
// stored as an absolute path even when Prepare is given a relative path (m3):
// ValidateActivityDigestAndLoadEntries re-reads LogPath at record time, potentially from a
// different working directory than prepare ran in, so a relative path would
// silently point at the wrong file (or nothing) and break digest validation.
//
//nolint:paralleltest // mutates process-wide cwd via os.Chdir; must not run concurrently with other tests
func TestPrepare_ActivityLogPathIsAbsolute_REQ_EXECEV(t *testing.T) {
	baseSHA := "abc123abc123abc123abc123abc123abc123abc1"
	headSHA := "def456def456def456def456def456def456def4"

	tmpDir := t.TempDir()
	logContent := activityLogLineJSON(t, map[string]any{"head_sha": headSHA})
	require.NoError(t, os.WriteFile(tmpDir+"/armature-activity.log", []byte(logContent), 0o644))

	git := &mockGitAdapter{
		resolveRevisionFn: func(rev string) (string, error) {
			if rev == "main" {
				return baseSHA, nil
			}
			return headSHA, nil
		},
		diffRangeFn: func(base, head string) (string, error) {
			return "diff --git a/file.go b/file.go\n", nil
		},
		diffNameOnlyRangeFn: func(base, head string) ([]string, error) {
			return []string{"file.go"}, nil
		},
	}

	// Change into tmpDir so a relative activity log path resolves there, then
	// pass a relative path to Prepare.
	origWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { require.NoError(t, os.Chdir(origWD)) }()

	bundle, err := review.Prepare(git, "EXECEV-T2", "Relative path", "dod", "task", "done",
		[]string{}, []string{}, "main", "HEAD", "armature-activity.log")

	require.NoError(t, err)
	require.NotNil(t, bundle)
	require.NotNil(t, bundle.Activity)
	assert.True(t, filepath.IsAbs(bundle.Activity.LogPath), "LogPath must be absolute, got %q", bundle.Activity.LogPath)
}

func TestPrepare_WithoutActivityLog_REQ_EXECEV_T2(t *testing.T) {
	t.Parallel()

	baseSHA := "abc123abc123abc123abc123abc123abc123abc1"
	headSHA := "def456def456def456def456def456def456def4"

	git := &mockGitAdapter{
		resolveRevisionFn: func(rev string) (string, error) {
			if rev == "main" {
				return baseSHA, nil
			}
			return headSHA, nil
		},
		diffRangeFn: func(base, head string) (string, error) {
			return "diff --git a/file.go b/file.go\n", nil
		},
		diffNameOnlyRangeFn: func(base, head string) ([]string, error) {
			return []string{"file.go"}, nil
		},
	}

	// Pass non-existent activity log path
	bundle, err := review.Prepare(git, "EXECEV-T2", "No activity", "dod", "task", "done",
		[]string{}, []string{}, "main", "HEAD", "/nonexistent/path.log")

	require.NoError(t, err)
	require.NotNil(t, bundle)
	// Activity section should be omitted when log doesn't exist
	assert.Nil(t, bundle.Activity, "Activity section should be nil when log does not exist")
}

func TestPrepare_EmptyActivityLog_REQ_EXECEV_T2(t *testing.T) {
	t.Parallel()

	baseSHA := "abc123abc123abc123abc123abc123abc123abc1"
	headSHA := "def456def456def456def456def456def456def4"

	tmpDir := t.TempDir()
	logPath := tmpDir + "/armature-activity.log"

	// Create an empty activity log
	err := os.WriteFile(logPath, []byte(""), 0o644)
	require.NoError(t, err)

	git := &mockGitAdapter{
		resolveRevisionFn: func(rev string) (string, error) {
			if rev == "main" {
				return baseSHA, nil
			}
			return headSHA, nil
		},
		diffRangeFn: func(base, head string) (string, error) {
			return "diff --git a/file.go b/file.go\n", nil
		},
		diffNameOnlyRangeFn: func(base, head string) ([]string, error) {
			return []string{"file.go"}, nil
		},
	}

	bundle, err := review.Prepare(git, "EXECEV-T2", "Empty activity", "dod", "task", "done",
		[]string{}, []string{}, "main", "HEAD", logPath)

	require.NoError(t, err)
	require.NotNil(t, bundle)
	require.NotNil(t, bundle.Activity, "Activity section should be present for empty log")
	assert.Equal(t, 0, bundle.Activity.EntryCount, "Empty log should have zero entries")
	assert.Equal(t, 0, bundle.Activity.DeliveryHeadCount, "No entries at delivery head")
	assert.Equal(t, 0, bundle.Activity.EarlierCount, "No earlier entries")
}

func TestPrepare_MalformedActivityLog_REQ_EXECEV_T2(t *testing.T) {
	t.Parallel()

	baseSHA := "abc123abc123abc123abc123abc123abc123abc1"
	headSHA := "def456def456def456def456def456def456def4"

	tmpDir := t.TempDir()
	logPath := tmpDir + "/armature-activity.log"

	// Write a malformed activity log (a non-JSON line interleaved with valid JSONL entries)
	logContent := activityLogLineJSON(t, map[string]any{
		"command": "make build", "head_sha": "def456def456def456def456def456def456def4", "output_hash": "abc123",
	}) + "\n" +
		"this is a completely malformed line\n" +
		activityLogLineJSON(t, map[string]any{
			"command": "make test", "head_sha": "def456def456def456def456def456def456def4", "output_hash": "def789",
		})
	err := os.WriteFile(logPath, []byte(logContent), 0o644)
	require.NoError(t, err)

	git := &mockGitAdapter{
		resolveRevisionFn: func(rev string) (string, error) {
			if rev == "main" {
				return baseSHA, nil
			}
			return headSHA, nil
		},
		diffRangeFn: func(base, head string) (string, error) {
			return "diff --git a/file.go b/file.go\n", nil
		},
		diffNameOnlyRangeFn: func(base, head string) ([]string, error) {
			return []string{"file.go"}, nil
		},
	}

	bundle, err := review.Prepare(git, "EXECEV-T2", "Malformed activity", "dod", "task", "done",
		[]string{}, []string{}, "main", "HEAD", logPath)

	require.NoError(t, err)
	require.NotNil(t, bundle)
	require.NotNil(t, bundle.Activity, "Activity section should be present")
	// Malformed lines are skipped, so we should have 2 valid entries
	assert.Equal(t, 2, bundle.Activity.EntryCount, "Should skip malformed lines and parse valid ones")
}

func TestActivityDigestDeterministic_REQ_EXECEV_T2(t *testing.T) {
	t.Parallel()

	baseSHA := "abc123abc123abc123abc123abc123abc123abc1"
	headSHA := "def456def456def456def456def456def456def4"

	logContent := activityLogLineJSON(t, map[string]any{
		"command": "make build", "head_sha": "def456def456def456def456def456def456def4", "output_hash": "abc123",
	})

	// Create two identical activity logs and verify they produce the same digest
	tmpDir := t.TempDir()
	logPath1 := tmpDir + "/log1.log"
	logPath2 := tmpDir + "/log2.log"

	err := os.WriteFile(logPath1, []byte(logContent), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(logPath2, []byte(logContent), 0o644)
	require.NoError(t, err)

	git := &mockGitAdapter{
		resolveRevisionFn: func(rev string) (string, error) {
			if rev == "main" {
				return baseSHA, nil
			}
			return headSHA, nil
		},
		diffRangeFn: func(base, head string) (string, error) {
			return "diff --git a/file.go b/file.go\n", nil
		},
		diffNameOnlyRangeFn: func(base, head string) ([]string, error) {
			return []string{"file.go"}, nil
		},
	}

	bundle1, err1 := review.Prepare(git, "EXECEV-T2", "Test1", "dod", "task", "done",
		[]string{}, []string{}, "main", "HEAD", logPath1)
	require.NoError(t, err1)

	bundle2, err2 := review.Prepare(git, "EXECEV-T2", "Test2", "dod", "task", "done",
		[]string{}, []string{}, "main", "HEAD", logPath2)
	require.NoError(t, err2)

	// Activity digests should be identical for identical log content
	require.NotNil(t, bundle1.Activity)
	require.NotNil(t, bundle2.Activity)
	assert.Equal(t, bundle1.Activity.Digest, bundle2.Activity.Digest, "Activity digests should be deterministic")
}
