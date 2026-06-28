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

	// Pattern to match file headers: "--- a/path" and "+++ b/path"
	fileHeaderRegex := regexp.MustCompile(`^\+\+\+ b/(.+)$`)
	// Pattern to match hunk headers: "@@ -old_start,old_count +new_start,new_count @@"
	hunkHeaderRegex := regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

	for scanner.Scan() {
		line := scanner.Text()

		// Check for file header
		if fileHeaderMatch := fileHeaderRegex.FindStringSubmatch(line); fileHeaderMatch != nil {
			currentFile = fileHeaderMatch[1]
			inHunk = false
			// Initialize the file's line set if not already present
			if _, exists := idx.fileLines[currentFile]; !exists {
				idx.fileLines[currentFile] = make(map[int]bool)
			}
			continue
		}

		// Skip binary file lines
		if strings.HasPrefix(line, "Binary files") {
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

// Files returns the list of files present in the diff index, sorted alphabetically.
func (d *DiffIndex) Files() []string {
	files := make([]string, 0, len(d.fileLines))
	for file := range d.fileLines {
		files = append(files, file)
	}
	sort.Strings(files)
	return files
}
