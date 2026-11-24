package materialize

import (
	"fmt"
	"slices"
	"strings"

	"github.com/scullxbones/armature/internal/ops"
)

// MaterializeAtSHA replays all op log files at the given commit SHA and returns
// the resulting materialized state. opsPrefixes are the paths within the git
// tree where log files are stored (e.g., "ops" or ".armature/ops") — a file is
// included if it falls under any of them. Multiple prefixes let a single
// replay span a repo's collapse migration, where commits before the collapse
// store logs under a nested prefix and commits after it store them at the root.
func MaterializeAtSHA(history historyReader, sha string, opsPrefixes ...string) (*State, error) {
	files, err := history.ListFilesAtCommit(sha)
	if err != nil {
		return nil, fmt.Errorf("list files at %s: %w", sha, err)
	}

	var allOps []ops.Op

	prefixes := make([]string, len(opsPrefixes))
	for i, p := range opsPrefixes {
		prefixes[i] = p + "/"
	}

	for _, f := range files {
		if !slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasPrefix(f, p) }) {
			continue
		}
		if !strings.HasSuffix(f, ".log") {
			continue
		}

		content, err := history.ShowFileAtCommit(sha, f)
		if err != nil {
			return nil, fmt.Errorf("show file %s at %s: %w", f, sha, err)
		}

		parsed, scanErr := parseOwnedLogOps(content, f)
		if scanErr != nil {
			return nil, fmt.Errorf("scan file %s: %w", f, scanErr)
		}
		allOps = append(allOps, parsed...)
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
