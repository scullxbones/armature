package review

import (
	"bufio"
	"regexp"
	"strconv"
	"strings"
)

type DiffIndex struct {
	fileLines map[string]map[int]bool
}

func BuildDiffIndex(unifiedDiff string) (*DiffIndex, error) {
	idx := &DiffIndex{
		fileLines: make(map[string]map[int]bool),
	}

	if unifiedDiff == "" {
		return idx, nil
	}

	scanner := bufio.NewScanner(strings.NewReader(unifiedDiff))
	var currentFile string
	var currentLineNum int
	var inHunk bool
	var lastOldFile string

	oldFileHeaderRegex := regexp.MustCompile(`^--- a/(.+)$`)
	newFileHeaderRegex := regexp.MustCompile(`^\+\+\+ (?:b/(.+)|/dev/null)$`)
	binaryFileRegex := regexp.MustCompile(`^Binary files (.+) and (.+) differ$`)
	hunkHeaderRegex := regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

	for scanner.Scan() {
		line := scanner.Text()

		if oldFileHeaderMatch := oldFileHeaderRegex.FindStringSubmatch(line); oldFileHeaderMatch != nil {
			lastOldFile = oldFileHeaderMatch[1]
			continue
		}

		if binaryFileMatch := binaryFileRegex.FindStringSubmatch(line); binaryFileMatch != nil {
			firstPath := binaryFileMatch[1]
			secondPath := binaryFileMatch[2]

			var binaryFilePath string
			switch {
			case strings.HasPrefix(secondPath, "b/"):
				binaryFilePath = strings.TrimPrefix(secondPath, "b/")
			case strings.HasPrefix(firstPath, "a/"):
				binaryFilePath = strings.TrimPrefix(firstPath, "a/")
			}

			if binaryFilePath != "" {
				inHunk = false
				if _, exists := idx.fileLines[binaryFilePath]; !exists {
					idx.fileLines[binaryFilePath] = make(map[int]bool)
				}
			}
			continue
		}

		if newFileHeaderMatch := newFileHeaderRegex.FindStringSubmatch(line); newFileHeaderMatch != nil {
			if newFileHeaderMatch[1] != "" {
				currentFile = newFileHeaderMatch[1]
			} else {
				currentFile = lastOldFile
			}
			inHunk = false
			if _, exists := idx.fileLines[currentFile]; !exists {
				idx.fileLines[currentFile] = make(map[int]bool)
			}
			continue
		}

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

		if inHunk && currentFile != "" {
			if len(line) == 0 {
				continue
			}

			firstChar := line[0]
			switch firstChar {
			case ' ':
				currentLineNum++
			case '+':
				idx.fileLines[currentFile][currentLineNum] = true
				currentLineNum++
			case '-':
			case '\\':
			}
		}
	}

	return idx, nil
}

func (d *DiffIndex) ContainsLine(file string, line int) bool {
	fileLines, exists := d.fileLines[file]
	if !exists {
		return false
	}
	return fileLines[line]
}

func (d *DiffIndex) ContainsFile(file string) bool {
	_, exists := d.fileLines[file]
	return exists
}
