package harnesshook

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/scullxbones/armature/internal/harnesspolicy"
)

// AppendHookLogLine appends a single line to <gitDir>/armature-hook.log, prefixed
// with an RFC3339 UTC timestamp, matching the format used by cmd/armature's
// decision/pass-through/violation loggers. Exported so pass-through-with-violation
// logging (LogPassThroughScopeViolation) can be exercised directly, and reused,
// from outside this package.
func AppendHookLogLine(gitDir string, line string) error {
	logPath := filepath.Join(gitDir, "armature-hook.log")
	// #nosec G304 - logPath is derived from a caller-supplied trusted git directory
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close() //nolint:errcheck // closing log file, error is not actionable
	}()
	ts := time.Now().UTC().Format(time.RFC3339) //nolint:forbidigo // required for hook log timestamps
	_, err = fmt.Fprintf(f, "%s %s\n", ts, line)
	return err
}

// LogPassThroughScopeViolation checks paths against scopePolicy and, when any are
// out of scope, appends a "violation:" entry to <gitDir>/armature-hook.log.
//
// This covers the pass-through-with-violation case described in
// docs/harness-hook.md's "Scope Violation Visibility" section: out-of-scope
// operations must be logged with a violation marker "even when the hook blocks
// or passes through the operation" — not only when the hook's ultimate
// decision is block. A stale binding is the canonical example: enforcement is
// skipped entirely (fail-open pass-through), but an attempted out-of-scope
// operation is still evidence of an enforcement gap worth surfacing to
// operators for merge-time audit.
//
// reason is a short human-readable label describing why enforcement was
// skipped (e.g. "stale issue binding"), included in the log line for context.
// Returns the underlying ScopeCheckResult so callers can inspect it further;
// when paths is empty or all paths are in scope, no line is written and a nil
// error is returned.
func LogPassThroughScopeViolation(gitDir string, scopePolicy harnesspolicy.ScopePolicy, paths []string, reason string) (harnesspolicy.ScopeCheckResult, error) {
	if len(paths) == 0 {
		return harnesspolicy.ScopeCheckResult{Allowed: true}, nil
	}
	result := scopePolicy.CheckPaths(paths)
	if result.Allowed {
		return result, nil
	}
	violationPaths := make([]string, 0, len(result.Violations))
	for _, v := range result.Violations {
		violationPaths = append(violationPaths, v.Path)
	}
	line := fmt.Sprintf("violation: out-of-scope path(s) on pass-through (%s): %s", reason, strings.Join(violationPaths, ", "))
	return result, AppendHookLogLine(gitDir, line)
}
