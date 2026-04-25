package materialize

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"

	"github.com/scullxbones/armature/internal/git"
	"github.com/scullxbones/armature/internal/ops"
)

// MaterializeAtSHA replays all op log files at the given commit SHA and returns
// the resulting materialized state. opsPrefix is the path within the git tree
// where log files are stored (e.g., "ops" or ".issues/ops").
func MaterializeAtSHA(gc *git.Client, sha string, opsPrefix string) (*State, error) {
	files, err := gc.ListFilesAtCommit(sha)
	if err != nil {
		return nil, fmt.Errorf("list files at %s: %w", sha, err)
	}

	var allOps []ops.Op

	prefix := opsPrefix + "/"
	for _, f := range files {
		if !strings.HasPrefix(f, prefix) {
			continue
		}
		if !strings.HasSuffix(f, ".log") {
			continue
		}

		workerID := ops.WorkerIDFromFilename(f)

		content, err := gc.ShowFileAtCommit(sha, f)
		if err != nil {
			return nil, fmt.Errorf("show file %s at %s: %w", f, sha, err)
		}

		scanner := bufio.NewScanner(bytes.NewReader(content))
		scanner.Buffer(make([]byte, 1<<20), 1<<20)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			op, err := ops.ParseLine(line)
			if err != nil {
				// Skip corrupt lines
				continue
			}
			if op.WorkerID != workerID {
				continue
			}
			allOps = append(allOps, op)
		}
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("scan file %s: %w", f, err)
		}
	}

	sortOpsByTimestamp(allOps)

	state := NewState()
	for _, op := range allOps {
		if err := state.ApplyOp(op); err != nil {
			continue
		}
	}

	return state, nil
}
