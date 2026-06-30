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

	// Binary files should be indexed at the path level
	assert.True(t, idx.ContainsFile("image.png"))
	// But should not have line-level changes
	assert.False(t, idx.ContainsLine("image.png", 1))
	// Text file should still work normally
	assert.True(t, idx.ContainsLine("text.go", 2))
}

func TestBuildDiffIndex_BinaryOnlyDelivery(t *testing.T) {
	t.Parallel()
	diff := `Binary files a/logo.svg and b/logo.svg differ
`

	idx, err := review.BuildDiffIndex(diff)
	require.NoError(t, err)

	// Binary file should be indexed
	assert.True(t, idx.ContainsFile("logo.svg"))
	// Should appear in the files list
	files := idx.Files()
	assert.Len(t, files, 1)
	assert.Contains(t, files, "logo.svg")
	// But should not have line-level changes
	assert.False(t, idx.ContainsLine("logo.svg", 1))
}

func TestBuildDiffIndex_MixedBinaryAndText(t *testing.T) {
	t.Parallel()
	diff := `Binary files a/image.png and b/image.png differ
Binary files a/data.bin and b/data.bin differ
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
+new import
 import "fmt"
 func main() {
`

	idx, err := review.BuildDiffIndex(diff)
	require.NoError(t, err)

	// Both binary files should be indexed
	assert.True(t, idx.ContainsFile("image.png"))
	assert.True(t, idx.ContainsFile("data.bin"))
	assert.True(t, idx.ContainsFile("main.go"))

	// Binary files should not have line changes
	assert.False(t, idx.ContainsLine("image.png", 1))
	assert.False(t, idx.ContainsLine("data.bin", 1))

	// Text file should have line changes
	assert.True(t, idx.ContainsLine("main.go", 2))

	files := idx.Files()
	assert.Len(t, files, 3)
	assert.Equal(t, []string{"data.bin", "image.png", "main.go"}, files)
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

func TestDiffIndexContainsFile(t *testing.T) {
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
@@ -5,3 +5,4 @@
 context
+added line
 more context
`

	idx, err := review.BuildDiffIndex(diff)
	require.NoError(t, err)

	// Files that are in the diff should return true
	assert.True(t, idx.ContainsFile("file1.go"))
	assert.True(t, idx.ContainsFile("file2.go"))

	// Files that are not in the diff should return false
	assert.False(t, idx.ContainsFile("file3.go"))
	assert.False(t, idx.ContainsFile("nonexistent.go"))
}

func TestDiffIndexContainsFile_EmptyDiff(t *testing.T) {
	t.Parallel()
	idx, err := review.BuildDiffIndex("")
	require.NoError(t, err)

	// No files in empty diff
	assert.False(t, idx.ContainsFile("any_file.go"))
}

func TestBuildDiffIndex_DeletedFile(t *testing.T) {
	t.Parallel()
	// Diff showing an entire file being deleted
	diff := `--- a/deleted_file.go
+++ /dev/null
@@ -1,5 +1,0 @@
-line 1
-line 2
-line 3
-line 4
-line 5
`

	idx, err := review.BuildDiffIndex(diff)
	require.NoError(t, err)

	// The deleted file should be in the index
	assert.True(t, idx.ContainsFile("deleted_file.go"))

	// Files list should contain the deleted file
	files := idx.Files()
	assert.Contains(t, files, "deleted_file.go")
}

func TestBuildDiffIndex_BinaryFileDeleted(t *testing.T) {
	t.Parallel()
	// Diff showing a binary file being deleted
	diff := `Binary files a/image.png and /dev/null differ
--- a/image.png
+++ /dev/null
`

	idx, err := review.BuildDiffIndex(diff)
	require.NoError(t, err)

	// The deleted binary file should be in the index
	assert.True(t, idx.ContainsFile("image.png"))
}

func TestBuildDiffIndex_MixedAdditionAndDeletion(t *testing.T) {
	t.Parallel()
	// Diff with both file addition and deletion
	diff := `--- a/deleted.go
+++ /dev/null
@@ -1,3 +1,0 @@
-old line 1
-old line 2
-old line 3
--- a/new.go
+++ b/new.go
@@ -0,0 +1,2 @@
+new line 1
+new line 2
`

	idx, err := review.BuildDiffIndex(diff)
	require.NoError(t, err)

	// Both files should be in the index
	assert.True(t, idx.ContainsFile("deleted.go"))
	assert.True(t, idx.ContainsFile("new.go"))

	files := idx.Files()
	assert.Len(t, files, 2)
	assert.Equal(t, []string{"deleted.go", "new.go"}, files)

	// The new file should have line markers for added lines
	assert.True(t, idx.ContainsLine("new.go", 1))
	assert.True(t, idx.ContainsLine("new.go", 2))
}

func TestBuildDiffIndex_DeletedFileWithContext(t *testing.T) {
	t.Parallel()
	// This is unusual but valid - deleted file with lines shown
	diff := `--- a/partial.go
+++ /dev/null
@@ -1,10 +1,0 @@
-package main
-
-func main() {
-	fmt.Println("hello")
-}
-
-func helper() {
-	// helper
-}
-
`

	idx, err := review.BuildDiffIndex(diff)
	require.NoError(t, err)

	// The deleted file should be in the index
	assert.True(t, idx.ContainsFile("partial.go"))
}
