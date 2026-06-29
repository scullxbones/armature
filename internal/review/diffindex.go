package review

import (
	"bufio"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// DiffIndex maps file paths to sets of changed line numbers from a unified diff.
type DiffIndex struct {
	fileLines map[string]map[int]bool // file -> set of line numbers with changes
}

// BuildDiffIndex parses a unified diff string into a DiffIndex.
// It tracks which lines in each file were added or modified (lines with + prefix in output).
// It also tracks deleted files (where +++ /dev/null appears).
func BuildDiffIndex(unifiedDiff string) (*DiffIndex, error) {
	idx := &DiffIndex{
		fileLines: make(map[string]map[int]bool),
	}

	if unifiedDiff == "" {
		return idx, nil
	}

	scanner := bufio.NewScanner(strings.NewReader(unifiedDiff))
	var currentFile string
	var currentLineNum int // Current line number in the new file
	var inHunk bool
	var lastOldFile string // Track the last --- a/path for deleted files

	// Pattern to match old file header: "--- a/path"
	oldFileHeaderRegex := regexp.MustCompile(`^--- a/(.+)$`)
	// Pattern to match new file header: "+++ b/path" or "+++ /dev/null" (for deletions)
	newFileHeaderRegex := regexp.MustCompile(`^\+\+\+ (?:b/(.+)|/dev/null)$`)
	// Pattern to match binary file lines: "Binary files a/path and b/path differ"
	// Handles: modified (a/path and b/path), deleted (a/path and /dev/null), added (/dev/null and b/path)
	binaryFileRegex := regexp.MustCompile(`^Binary files (.+) and (.+) differ$`)
	// Pattern to match hunk headers: "@@ -old_start,old_count +new_start,new_count @@"
	hunkHeaderRegex := regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

	for scanner.Scan() {
		line := scanner.Text()

		// Check for old file header (--- a/path)
		if oldFileHeaderMatch := oldFileHeaderRegex.FindStringSubmatch(line); oldFileHeaderMatch != nil {
			lastOldFile = oldFileHeaderMatch[1]
			continue
		}

		// Check for binary file line
		if binaryFileMatch := binaryFileRegex.FindStringSubmatch(line); binaryFileMatch != nil {
			// Extract the file path from "Binary files a/path and b/path differ"
			// Handle modified (a/path and b/path), deleted (a/path and /dev/null), added (/dev/null and b/path)
			firstPath := binaryFileMatch[1]
			secondPath := binaryFileMatch[2]

			var binaryFilePath string
			// Determine which path to use based on prefixes
			switch {
			case strings.HasPrefix(secondPath, "b/"):
				// For modified files: use b/ path
				binaryFilePath = strings.TrimPrefix(secondPath, "b/")
			case strings.HasPrefix(firstPath, "a/"):
				// For deleted or added files: use a/ path if available
				binaryFilePath = strings.TrimPrefix(firstPath, "a/")
			case secondPath != "/dev/null":
				// For added files: use b/ path
				binaryFilePath = strings.TrimPrefix(secondPath, "b/")
			}

			if binaryFilePath != "" {
				inHunk = false
				// Initialize the file's line set if not already present
				if _, exists := idx.fileLines[binaryFilePath]; !exists {
					idx.fileLines[binaryFilePath] = make(map[int]bool)
				}
			}
			continue
		}

		// Check for new file header (file addition, modification, or deletion)
		if newFileHeaderMatch := newFileHeaderRegex.FindStringSubmatch(line); newFileHeaderMatch != nil {
			if newFileHeaderMatch[1] != "" {
				// +++ b/path (addition or modification)
				currentFile = newFileHeaderMatch[1]
			} else {
				// +++ /dev/null (deletion - use the old filename)
				currentFile = lastOldFile
			}
			inHunk = false
			// Initialize the file's line set if not already present
			if _, exists := idx.fileLines[currentFile]; !exists {
				idx.fileLines[currentFile] = make(map[int]bool)
			}
			continue
		}

		// Check for hunk header
		if hunkHeaderMatch := hunkHeaderRegex.FindStringSubmatch(line); hunkHeaderMatch != nil {
			startLineStr := hunkHeaderMatch[1]
			startLine, err := strconv.Atoi(startLineStr)
			if err != nil {
				continue
			}
			currentLineNum = startLine
			inHunk = true
			continue
		}

		// Process diff lines (only if we're in a hunk and have a current file)
		if inHunk && currentFile != "" {
			if len(line) == 0 {
				continue
			}

			firstChar := line[0]
			switch firstChar {
			case ' ':
				// Context line - increment line counter but don't mark as changed
				currentLineNum++
			case '+':
				// Added/modified line - mark as changed and increment line counter
				idx.fileLines[currentFile][currentLineNum] = true
				currentLineNum++
			case '-':
				// Deleted line - don't increment line counter (line doesn't exist in new file)
			case '\\':
				// Special marker (e.g., "\ No newline at end of file") - ignore
			}
		}
	}

	return idx, nil
}

// ContainsLine returns true if the given file+line combination appears in the diff.
func (d *DiffIndex) ContainsLine(file string, line int) bool {
	fileLines, exists := d.fileLines[file]
	if !exists {
		return false
	}
	return fileLines[line]
}

// ContainsFile returns true if the given file path appears in the diff index.
func (d *DiffIndex) ContainsFile(file string) bool {
	_, exists := d.fileLines[file]
	return exists
}

// Files returns the list of files present in the diff index, sorted alphabetically.
func (d *DiffIndex) Files() []string {
	files := make([]string, 0, len(d.fileLines))
	for file := range d.fileLines {
		files = append(files, file)
	}
	sort.Strings(files)
	return files
}
