package review

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildDiffIndex_SimpleFile(t *testing.T) {
	t.Parallel()

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

	idx := buildDiffIndex(diff)

	assert.True(t, idx.containsLine("internal/review/test.go", 7))

	assert.False(t, idx.containsLine("internal/review/test.go", 5))

	assert.False(t, idx.containsLine("internal/review/test.go", 6))

	assert.True(t, idx.containsFile("internal/review/test.go"))
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

	idx := buildDiffIndex(diff)

	assert.True(t, idx.containsFile("file1.go"))
	assert.True(t, idx.containsFile("file2.go"))

	assert.True(t, idx.containsLine("file1.go", 1))

	assert.True(t, idx.containsLine("file2.go", 11))
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

	idx := buildDiffIndex(diff)

	assert.True(t, idx.containsLine("multi.go", 2))
	assert.True(t, idx.containsLine("multi.go", 12))
	assert.False(t, idx.containsLine("multi.go", 3))
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

	idx := buildDiffIndex(diff)

	assert.False(t, idx.containsLine("deleted.go", 1))
	assert.False(t, idx.containsLine("deleted.go", 2))
	assert.False(t, idx.containsLine("deleted.go", 3))
}

func TestBuildDiffIndex_EmptyDiff(t *testing.T) {
	t.Parallel()
	diff := ""

	idx := buildDiffIndex(diff)
	require.NotNil(t, idx)
}

func TestBuildDiffIndex_InvalidFormat(t *testing.T) {
	t.Parallel()

	diff := `@@ -1,3 +1,4 @@
+new line
`

	idx := buildDiffIndex(diff)
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

	idx := buildDiffIndex(diff)

	assert.True(t, idx.containsLine("new_name.go", 1))

	assert.False(t, idx.containsLine("old_name.go", 1))
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

	idx := buildDiffIndex(diff)

	assert.True(t, idx.containsFile("image.png"))

	assert.False(t, idx.containsLine("image.png", 1))

	assert.True(t, idx.containsLine("text.go", 2))
}

func TestBuildDiffIndex_BinaryOnlyDelivery(t *testing.T) {
	t.Parallel()
	diff := `Binary files a/logo.svg and b/logo.svg differ
`

	idx := buildDiffIndex(diff)

	assert.True(t, idx.containsFile("logo.svg"))

	assert.False(t, idx.containsLine("logo.svg", 1))
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

	idx := buildDiffIndex(diff)

	assert.True(t, idx.containsFile("image.png"))
	assert.True(t, idx.containsFile("data.bin"))
	assert.True(t, idx.containsFile("main.go"))

	assert.False(t, idx.containsLine("image.png", 1))
	assert.False(t, idx.containsLine("data.bin", 1))

	assert.True(t, idx.containsLine("main.go", 2))
}

func TestContainsLine_NonexistentFile(t *testing.T) {
	t.Parallel()
	diff := `--- a/file.go
+++ b/file.go
@@ -1,2 +1,3 @@
+added
 line
`

	idx := buildDiffIndex(diff)

	assert.False(t, idx.containsLine("nonexistent.go", 1))
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

	idx := buildDiffIndex(diff)

	assert.True(t, idx.containsFile("file1.go"))
	assert.True(t, idx.containsFile("file2.go"))

	assert.False(t, idx.containsFile("file3.go"))
	assert.False(t, idx.containsFile("nonexistent.go"))
}

func TestDiffIndexContainsFile_EmptyDiff(t *testing.T) {
	t.Parallel()
	idx := buildDiffIndex("")

	assert.False(t, idx.containsFile("any_file.go"))
}

func TestBuildDiffIndex_DeletedFile(t *testing.T) {
	t.Parallel()

	diff := `--- a/deleted_file.go
+++ /dev/null
@@ -1,5 +1,0 @@
-line 1
-line 2
-line 3
-line 4
-line 5
`

	idx := buildDiffIndex(diff)

	assert.True(t, idx.containsFile("deleted_file.go"))
}

func TestBuildDiffIndex_BinaryFileDeleted(t *testing.T) {
	t.Parallel()

	diff := `Binary files a/image.png and /dev/null differ
--- a/image.png
+++ /dev/null
`

	idx := buildDiffIndex(diff)

	assert.True(t, idx.containsFile("image.png"))
}

func TestBuildDiffIndex_MixedAdditionAndDeletion(t *testing.T) {
	t.Parallel()

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

	idx := buildDiffIndex(diff)

	assert.True(t, idx.containsFile("deleted.go"))
	assert.True(t, idx.containsFile("new.go"))

	assert.True(t, idx.containsLine("new.go", 1))
	assert.True(t, idx.containsLine("new.go", 2))
}

func TestBuildDiffIndex_DeletedFileWithContext(t *testing.T) {
	t.Parallel()

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

	idx := buildDiffIndex(diff)

	assert.True(t, idx.containsFile("partial.go"))
}
