package review_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		[]string{"file1", "file2"}, []string{"criteria1", "criteria2"}, "main", "HEAD",
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

	_, err := review.Prepare(git, "SMTC-S1-T2", "Test", "", "task", "", []string{}, []string{}, "main", "HEAD")

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

	_, err := review.Prepare(git, "SMTC-S1-T2", "Test", "", "task", "", []string{}, []string{}, "main", "HEAD")

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

	_, err := review.Prepare(git, "SMTC-S1-T2", "Test", "", "task", "", []string{}, []string{}, "main", "HEAD")

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
	bundle1, err1 := review.Prepare(git, "SMTC-S1-T2", "Test", "", "task", "", []string{}, []string{}, "main", "HEAD")
	require.NoError(t, err1)

	bundle2, err2 := review.Prepare(git, "SMTC-S1-T2", "Test", "", "task", "", []string{}, []string{}, "main", "HEAD")
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

	bundle, err := review.Prepare(git, "SMTC-S1-T2", "Add feature", "", "task", "", []string{}, criteria, "main", "HEAD")

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

	_, err := review.Prepare(git, "SMTC-S1-T2", "Test", "", "task", "", []string{}, []string{}, "main", "HEAD")

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

	bundle, err := review.Prepare(git, "SMTC-S1-T2", issueTitle, "", "task", "", []string{}, []string{}, "main", "HEAD")

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

	bundle, err := review.Prepare(git, "SMTC-S1-T2", "Test", "", "task", "", []string{}, []string{}, "main", "HEAD")

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

	bundle, err := review.Prepare(git, "SMTC-S1-T2", "Add feature", "", "task", "", []string{}, []string{}, "main", "HEAD")

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

	bundle, err := review.Prepare(git, "SMTC-S1-T2", "Add feature", "", "task", "", []string{}, []string{}, "main", "HEAD")

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

	_, err := review.Prepare(git, "SMTC-S1-T2", "Task", "", "task", "", []string{}, []string{}, "main", "HEAD")

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
		[]string{}, []string{}, "main", "HEAD")

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

	bundle, err := review.Prepare(git, "SMTC-S1-T2", "Feature", "", "task", "", []string{}, []string{}, "main", "HEAD")

	require.NoError(t, err)
	require.NotNil(t, bundle)
	assert.Equal(t, []string{"internal/service.go", "pkg/util.go"}, bundle.Delivery.ChangedFiles)
}
