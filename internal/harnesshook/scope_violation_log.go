package harnesshook

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/scullxbones/armature/internal/harnesspolicy"
)

// sanitizeLogField strips newlines and carriage returns from a value before it
// is interpolated into a single-line log entry, preventing log injection where
// a crafted tool name, decision message, reason, or violation path could forge
// or mask a violation/decision line.
func sanitizeLogField(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func appendHookLogLine(gitDir string, line string) error {
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

// LogPassThroughScopeViolation checks paths against scope and, when any are
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
func LogPassThroughScopeViolation(gitDir string, scope []string, root string, paths []string, reason string) (harnesspolicy.ScopeCheckResult, error) {
	if len(paths) == 0 {
		return harnesspolicy.ScopeCheckResult{Allowed: true}, nil
	}
	result := harnesspolicy.CheckScope(scope, root, paths)
	if result.Allowed {
		return result, nil
	}
	violationPaths := make([]string, 0, len(result.Violations))
	for _, v := range result.Violations {
		violationPaths = append(violationPaths, sanitizeLogField(v.Path))
	}
	line := fmt.Sprintf("violation: out-of-scope path(s) on pass-through (%s): %s", sanitizeLogField(reason), strings.Join(violationPaths, ", "))
	return result, appendHookLogLine(gitDir, line)
}

// LogPassThrough appends a pass-through: line to <gitDir>/armature-hook.log.
func LogPassThrough(gitDir string, reason string) error {
	return appendHookLogLine(gitDir, "pass-through: "+sanitizeLogField(reason))
}

// LogViolation appends a violation: line to <gitDir>/armature-hook.log.
func LogViolation(gitDir string, reason string) error {
	return appendHookLogLine(gitDir, "violation: "+sanitizeLogField(reason))
}

// LogDecision appends a decision: line to <gitDir>/armature-hook.log.
func LogDecision(gitDir string, issueID string, resolutionStep string, eventKind string, tool string, decision string, blockReason string) error {
	entry := fmt.Sprintf("decision: issue_id=%s resolution_step=%s event=%s tool=%s decision=%s",
		sanitizeLogField(issueID), sanitizeLogField(resolutionStep),
		sanitizeLogField(eventKind), sanitizeLogField(tool),
		sanitizeLogField(decision))
	if blockReason != "" {
		entry += fmt.Sprintf(" block_reason=%s", sanitizeLogField(blockReason))
	}
	return appendHookLogLine(gitDir, entry)
}
