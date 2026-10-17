package review

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilterDiff_OnlyExcludedSections_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	diff := "diff --git a/.armature/ops/worker.log b/.armature/ops/worker.log\n" +
		"--- a/.armature/ops/worker.log\n" +
		"+++ b/.armature/ops/worker.log\n" +
		"@@ -1 +1,2 @@\n" +
		"+new op\n" +
		" old op\n"

	result := filterDiff(diff, []string{".armature/", ".arm/"})
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

	result := filterDiff(diff, []string{".armature/", ".arm/"})
	assert.Equal(t, productPart, result, "only product sections should be kept")
}

func TestFilterDiff_NoExcludedSections_Unchanged(t *testing.T) {
	t.Parallel()

	diff := "diff --git a/internal/foo.go b/internal/foo.go\n" +
		"--- a/internal/foo.go\n" +
		"+++ b/internal/foo.go\n" +
		"@@ -1 +1,2 @@\n" +
		"+func foo() {}\n"

	result := filterDiff(diff, []string{".armature/", ".arm/"})
	assert.Equal(t, diff, result, "diff with no excluded sections should be unchanged")
}

func TestFilterDiff_EmptyDiff_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	result := filterDiff("", []string{".armature/"})
	assert.Empty(t, result)
}

func TestFilterDiff_NoExcludePrefixes_Unchanged(t *testing.T) {
	t.Parallel()

	diff := "diff --git a/.armature/foo b/.armature/foo\n+line\n"
	result := filterDiff(diff, nil)
	assert.Equal(t, diff, result, "with no exclude prefixes, diff must be returned unchanged")
}
