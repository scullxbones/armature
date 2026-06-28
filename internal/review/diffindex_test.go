package review_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildDiffIndex_SimpleFile(t *testing.T) {
	t.Parallel()
	// Simple diff with one file and one hunk
	diff := `--- a/internal/review/test.go
+++ b/internal/review/test.go
@@ -5,7 +5,7 @@ package review
 func TestFunc() {
 	x := 1
-	y := 2
+	y := 3
 	return x + y
 }
 extra line
`

	idx, err := review.BuildDiffIndex(diff)
	require.NoError(t, err)
	require.NotNil(t, idx)

	// In the new file, after the hunk starting at line 5:
	// Line 5: func TestFunc() { (context, space)
	// Line 6:   x := 1 (context, space)
	// Line 7:   y := 3 (added, +) - this is the modified line
	// Line 8:   return x + y (context, space)
	// Line 9: } (context, space)
	// Line 10: extra line (context, space)

	// Line 7 was added (the + line)
	assert.True(t, idx.ContainsLine("internal/review/test.go", 7))
	// Line 5 is context, should not be marked as changed
	assert.False(t, idx.ContainsLine("internal/review/test.go", 5))
	// Line 6 is context, should not be marked as changed
	assert.False(t, idx.ContainsLine("internal/review/test.go", 6))

	files := idx.Files()
	assert.Len(t, files, 1)
	assert.Contains(t, files, "internal/review/test.go")
}

func TestBuildDiffIndex_MultipleFiles(t *testing.T) {
	t.Parallel()
	diff := `--- a/file1.go
+++ b/file1.go
@@ -1,3 +1,4 @@
+new line
 line 1
 line 2
 line 3
--- a/file2.go
+++ b/file2.go
@@ -10,3 +10,4 @@
 context
+added line
 more context
 another line
`

	idx, err := review.BuildDiffIndex(diff)
	require.NoError(t, err)

	files := idx.Files()
	assert.Len(t, files, 2)
	assert.Contains(t, files, "file1.go")
	assert.Contains(t, files, "file2.go")

	// file1.go has a + at line 1
	assert.True(t, idx.ContainsLine("file1.go", 1))
	// file2.go has a + at line 11 (10 is context, 11 is the added line)
	assert.True(t, idx.ContainsLine("file2.go", 11))
}

func TestBuildDiffIndex_MultipleHunks(t *testing.T) {
	t.Parallel()
	diff := `--- a/multi.go
+++ b/multi.go
@@ -1,3 +1,4 @@
 line 1
+added at line 2
 line 2
 line 3
@@ -10,3 +11,4 @@
 context
+added at line 12
 more context
 another line
`

	idx, err := review.BuildDiffIndex(diff)
	require.NoError(t, err)

	assert.True(t, idx.ContainsLine("multi.go", 2))
	assert.True(t, idx.ContainsLine("multi.go", 12))
	assert.False(t, idx.ContainsLine("multi.go", 3))
}

func TestBuildDiffIndex_DeletedLines(t *testing.T) {
	t.Parallel()
	diff := `--- a/deleted.go
+++ b/deleted.go
@@ -1,5 +1,3 @@
 line 1
-removed line 2
-removed line 3
 line 4
 line 5
`

	idx, err := review.BuildDiffIndex(diff)
	require.NoError(t, err)

	// Deleted lines (with -) should not be in the index (they don't exist in new file)
	// In the new file, after deleting lines 2-3 from original, line 4 becomes line 2
	// So the deleted lines don't appear in the new file's line numbers
	// Deleted lines are skipped (don't increment new file line number)
	// Line 1 (context) -> stays line 1 in new file
	// Line 2 (deleted) -> skipped in new file
	// Line 3 (deleted) -> skipped in new file
	// Line 4 (context) -> becomes line 2 in new file
	// Line 5 (context) -> becomes line 3 in new file

	// Nothing in the diff is marked with +, so no lines are changed
	assert.False(t, idx.ContainsLine("deleted.go", 1))
	assert.False(t, idx.ContainsLine("deleted.go", 2))
	assert.False(t, idx.ContainsLine("deleted.go", 3))
}

func TestBuildDiffIndex_EmptyDiff(t *testing.T) {
	t.Parallel()
	diff := ""

	idx, err := review.BuildDiffIndex(diff)
	require.NoError(t, err)
	require.NotNil(t, idx)

	files := idx.Files()
	assert.Len(t, files, 0)
}

func TestBuildDiffIndex_InvalidFormat(t *testing.T) {
	t.Parallel()
	// Malformed diff - missing file headers
	diff := `@@ -1,3 +1,4 @@
+new line
`

	idx, err := review.BuildDiffIndex(diff)
	// Should handle gracefully - either error or skip malformed sections
	// Implementation choice: continue parsing what we can
	require.NoError(t, err)
	require.NotNil(t, idx)
}

func TestBuildDiffIndex_RenamedFile(t *testing.T) {
	t.Parallel()
	diff := `--- a/old_name.go
+++ b/new_name.go
@@ -1,3 +1,4 @@
+new line
 line 1
 line 2
`

	idx, err := review.BuildDiffIndex(diff)
	require.NoError(t, err)

	// The new file name should be in the index
	assert.True(t, idx.ContainsLine("new_name.go", 1))
	// The old file name should not be in the index
	assert.False(t, idx.ContainsLine("old_name.go", 1))
}

func TestBuildDiffIndex_BinaryFile(t *testing.T) {
	t.Parallel()
	diff := `Binary files a/image.png and b/image.png differ
--- a/text.go
+++ b/text.go
@@ -1,2 +1,3 @@
 line
+added
`

	idx, err := review.BuildDiffIndex(diff)
	require.NoError(t, err)

	// Should skip binary files
	assert.False(t, idx.ContainsLine("image.png", 1))
	assert.True(t, idx.ContainsLine("text.go", 2))
}

func TestContainsLine_NonexistentFile(t *testing.T) {
	t.Parallel()
	diff := `--- a/file.go
+++ b/file.go
@@ -1,2 +1,3 @@
+added
 line
`

	idx, err := review.BuildDiffIndex(diff)
	require.NoError(t, err)

	assert.False(t, idx.ContainsLine("nonexistent.go", 1))
}

func TestFiles_Sorted(t *testing.T) {
	t.Parallel()
	diff := `--- a/z.go
+++ b/z.go
@@ -1,1 +1,2 @@
+added
--- a/a.go
+++ b/a.go
@@ -1,1 +1,2 @@
+added
--- a/m.go
+++ b/m.go
@@ -1,1 +1,2 @@
+added
`

	idx, err := review.BuildDiffIndex(diff)
	require.NoError(t, err)

	files := idx.Files()
	assert.Len(t, files, 3)
	// Files should be returned in sorted order
	assert.Equal(t, []string{"a.go", "m.go", "z.go"}, files)
}
