// Package deliverygate evaluates a worktree against an issue's delivery
// requirements (clean tree, scope containment, commit reference) without
// mutating any state.
package deliverygate

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/claim"
)

// CheckResult represents the outcome of a single gate check.
type CheckResult struct {
	Pass        bool   // Whether the check passed
	Remediation string // Remediation message if check failed (empty if passed)
}

// GateResult represents the combined results of all three delivery gate checks.
type GateResult struct {
	CleanTree        CheckResult // Check 1: git status --porcelain is empty
	ScopeContainment CheckResult // Check 2: delivery diff is subset of declared scope
	CommitReference  CheckResult // Check 3: at least one commit matches conventional format
}

// DeliveryGate evaluates a worktree against an issue via three checks:
// 1. Clean tree: git status --porcelain is empty
// 2. Scope containment: delivery diff vs base is subset of declared scope
// 3. Commit reference: at least one commit matches conventional-commit format
//
// Returns a structured GateResult with per-check results and remediations.
// Performs no state mutation — only reads and reports.
func DeliveryGate(worktreePath, issueID, baseCommit string, scope []string) *GateResult {
	return &GateResult{
		CleanTree:        CleanTreeCheck(worktreePath),
		ScopeContainment: ScopeContainmentCheck(worktreePath, baseCommit, scope),
		CommitReference:  CommitReferenceCheck(worktreePath, baseCommit, issueID),
	}
}

// CleanTreeCheck verifies that git status --porcelain is empty.
// Returns (Pass: true, Remediation: "") if the tree is clean,
// or (Pass: false, Remediation: "...message...") if there are uncommitted changes.
func CleanTreeCheck(worktreePath string) CheckResult {
	git := adapters.New(worktreePath)

	// Get all dirty entries (both tracked and untracked)
	entries, err := git.DirtyEntries()
	if err != nil {
		return CheckResult{
			Pass:        false,
			Remediation: fmt.Sprintf("Failed to check tree status: %v", err),
		}
	}

	// arm's own materialized state under .armature/ is expected to be
	// gitignored in any repo using armature; treat it as noise here rather
	// than depending on every caller's .gitignore being correctly set up.
	// A rename's OldPath must also be under .armature/ before the entry is
	// ignored: DirtyEntries only reports OldPath for renames (empty
	// otherwise), so a rename from a tracked file outside .armature/ into
	// .armature/ (e.g. `git mv outside.go .armature/outside.go`) would
	// otherwise be discarded here by checking Path alone, even though it
	// effectively deletes a tracked non-armature-state file.
	const armatureStateDir = ".armature/"
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Path, armatureStateDir) &&
			(entry.OldPath == "" || strings.HasPrefix(entry.OldPath, armatureStateDir)) {
			continue
		}
		paths = append(paths, entry.Path)
	}

	// If there are any dirty entries, the tree is not clean
	if len(paths) > 0 {
		return CheckResult{
			Pass: false,
			Remediation: fmt.Sprintf(
				"Working tree is not clean. Commit or discard changes to: %s",
				strings.Join(paths, ", "),
			),
		}
	}

	return CheckResult{Pass: true, Remediation: ""}
}

// ScopeContainmentCheck verifies that all files changed since baseCommit
// are within the declared scope globs.
// Returns (Pass: true, Remediation: "") if all files are in scope,
// or (Pass: false, Remediation: "...message...") if any file is outside scope.
func ScopeContainmentCheck(worktreePath, baseCommit string, scope []string) CheckResult {
	git := adapters.New(worktreePath)

	// Get the list of file changes since base commit, with rename detection
	// enabled so both the source and destination paths of any rename are
	// checked against scope. `git diff --name-only` alone would report only
	// the destination path of a rename, masking an out-of-scope original
	// location (e.g. a rename that moves a file from outside scope into
	// scope, silently deleting the out-of-scope original).
	entries, err := git.DiffNameStatus(baseCommit)
	if err != nil {
		return CheckResult{
			Pass:        false,
			Remediation: fmt.Sprintf("Failed to get diff: %v", err),
		}
	}

	files := make([]string, 0, len(entries)*2)
	for _, entry := range entries {
		if entry.OldPath != "" {
			files = append(files, entry.OldPath)
		}
		files = append(files, entry.Path)
	}

	// Use IsWithinScope to check if all files are within scope
	isInScope, outOfScopeFile := claim.IsWithinScope(files, scope)
	if !isInScope {
		return CheckResult{
			Pass: false,
			Remediation: fmt.Sprintf(
				"Delivery diff contains file outside declared scope: %s (scope: %v)",
				outOfScopeFile, scope,
			),
		}
	}

	return CheckResult{Pass: true, Remediation: ""}
}

// addedLinesByFile parses a unified diff (as produced by `git diff` or
// `git show`) and returns, per file (using the "b/" post-image path), the
// set of non-blank content lines that were added (a line beginning with "+"
// in a hunk, excluding the "+++" file-header line). Blank added lines are
// excluded because they are too common to reliably indicate that specific
// delivered content survived. This is used to check whether a commit's own
// added content is still present in a later diff, rather than merely
// checking that a filename appears in both diffs.
func addedLinesByFile(diff string) map[string]map[string]bool {
	result := make(map[string]map[string]bool)
	currentFile := ""
	for line := range strings.SplitSeq(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "):
			path := strings.TrimPrefix(line, "+++ ")
			path = strings.TrimPrefix(path, "b/")
			if path == "/dev/null" {
				currentFile = ""
				continue
			}
			currentFile = path
		case strings.HasPrefix(line, "+") && currentFile != "":
			content := strings.TrimSpace(strings.TrimPrefix(line, "+"))
			if content == "" {
				continue
			}
			if result[currentFile] == nil {
				result[currentFile] = make(map[string]bool)
			}
			result[currentFile][content] = true
		}
	}
	return result
}

// removedLinesByFile parses a unified diff (as produced by `git diff` or
// `git show`) and returns, per file (using the "b/" post-image path, same
// file-identity convention as addedLinesByFile so lookups by the paths
// returned from CommitChangedFiles line up for both maps, including
// renames), the set of non-blank content lines that were removed (a line
// beginning with "-" in a hunk, excluding the "---" file-header line).
// Mirrors addedLinesByFile for the pre-image side: a deletion-only commit
// removes content without adding any, so addedLinesByFile alone can never
// provide survival evidence for it. removedLinesByFile lets the survival
// check instead confirm that the matching commit's removed content is
// genuinely absent from the current HEAD version of the file.
func removedLinesByFile(diff string) map[string]map[string]bool {
	result := make(map[string]map[string]bool)
	currentFile := ""
	for line := range strings.SplitSeq(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "--- "):
			// Set from the pre-image path first; a deleted file's "+++ "
			// line is "+++ /dev/null" (no real post-image path), so this is
			// the only header line that names a deleted file. A normal
			// (non-delete) hunk immediately overwrites currentFile via its
			// "+++ " line below, keeping the "b/" path as the identity used
			// everywhere else.
			path := strings.TrimPrefix(line, "--- ")
			path = strings.TrimPrefix(path, "a/")
			if path == "/dev/null" {
				currentFile = ""
				continue
			}
			currentFile = path
		case strings.HasPrefix(line, "+++ "):
			path := strings.TrimPrefix(line, "+++ ")
			path = strings.TrimPrefix(path, "b/")
			if path != "/dev/null" {
				currentFile = path
			}
		case strings.HasPrefix(line, "-") && currentFile != "":
			content := strings.TrimSpace(strings.TrimPrefix(line, "-"))
			if content == "" {
				continue
			}
			if result[currentFile] == nil {
				result[currentFile] = make(map[string]bool)
			}
			result[currentFile][content] = true
		}
	}
	return result
}

// survivalMinLineLength is the minimum trimmed length (in characters) an
// added line must have to count as proof that a matching commit's content
// survives in the net base..HEAD diff. Short lines like "}", "return nil",
// or a common import are common enough that two unrelated commits touching
// the same file can coincidentally add an identical short line, which would
// otherwise let a fully-reverted matching commit falsely appear to survive.
const survivalMinLineLength = 15

// addedContentSurvives reports whether at least one line the matching commit
// added to a file is still present as an added line in the net base..HEAD
// diff for that same file, applying the short-line coincidence floor
// described on survivalMinLineLength.
func addedContentSurvives(commitLines, netLines map[string]bool) bool {
	if len(netLines) == 0 {
		return false
	}
	// Only apply the short-line floor when the commit added more than one
	// line to this file: if a short line is the ENTIRE content the commit
	// added, there is nothing longer to prefer and rejecting it would
	// falsely reject a legitimately tiny real change. But when several
	// lines were added, a short one among them (a lone "}", "return nil", a
	// common import) is far more likely to coincidentally collide with an
	// unrelated commit touching the same file than to genuinely prove this
	// commit's content survives — without this floor, such a coincidence
	// could make a fully-reverted matching commit's change look like it
	// survived.
	requireLongLine := len(commitLines) > 1
	for line := range commitLines {
		if requireLongLine && len(line) < survivalMinLineLength {
			continue
		}
		if netLines[line] {
			return true
		}
	}
	return false
}

// deletionSurvives reports whether a deletion-only change the matching
// commit made to file f (removedLines, its removed content) persists at
// HEAD: true if none of removedLines are present in f's current HEAD
// content (including if f no longer exists at HEAD at all), false if a
// later commit re-added the removed content, undoing the deletion.
func deletionSurvives(git *adapters.Client, f string, removedLines map[string]bool) bool {
	content, err := git.ShowFileAtCommit("HEAD", f)
	if err != nil {
		// File does not exist at HEAD (or is otherwise unreadable): the
		// removed content cannot be present, so the deletion survives.
		return true
	}
	currentLines := make(map[string]bool)
	for line := range strings.SplitSeq(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		currentLines[trimmed] = true
	}
	for line := range removedLines {
		if currentLines[line] {
			return false
		}
	}
	return true
}

// CommitReferenceCheck verifies that at least one commit since baseCommit
// matches the conventional-commit format with the issue ID in the scope.
// Format: <type>(<ISSUE-ID>): ... or <type>(<ISSUE-ID>)!: ...
// where type is one of feat, fix, refactor, test, docs, style, polish
// (see docs/conventions.md).
func CommitReferenceCheck(worktreePath, baseCommit, issueID string) CheckResult {
	git := adapters.New(worktreePath)

	// Get only commits strictly after baseCommit (exclusive) up to HEAD.
	entries, err := git.LogRange(baseCommit, "HEAD")
	if err != nil {
		return CheckResult{
			Pass:        false,
			Remediation: fmt.Sprintf("Failed to get commits: %v", err),
		}
	}

	// Build regex pattern: ^(feat|fix|refactor|test|docs|style|polish)\(ISSUE-ID\)!?:[ \t]+\S
	// Matches: type(ISSUE-ID): <description> or type(ISSUE-ID)!: <description>
	// where type is one of the commit types enumerated by
	// docs/conventions.md. Restricting the type alternation (rather than
	// accepting any lowercase word) prevents a bogus type like
	// "oops(ISSUE-ID): ..." from satisfying this check. Requiring whitespace
	// followed by a non-whitespace character after the colon prevents a
	// bare "fix(ISSUE-ID):" with no actual description from satisfying this
	// check.
	pattern := regexp.MustCompile(`^(feat|fix|refactor|test|docs|style|polish)\(` + regexp.QuoteMeta(issueID) + `\)!?:[ \t]+\S`)

	// A commit whose subject matches the conventional-commit format but has no
	// actual diff (e.g. an empty commit created with `git commit
	// --allow-empty -m "fix(ISSUE-ID): busywork"`) delivers no real content
	// and must not satisfy this check on its own. Keep scanning past it in
	// case a later matching commit does have real content.
	// A matching commit having non-empty CommitChangedFiles is not enough on
	// its own: a later commit in the range (e.g. a revert) can cancel out
	// exactly the change the matching commit made, leaving nothing actually
	// delivered even though the matching commit "touched files" at the time
	// it was made. It is also not enough to require that the net base-to-HEAD
	// diff is merely nonempty for any reason, or that the matching commit's
	// touched FILENAMES merely appear in the net diff's changed filenames: a
	// fully-reverted matching commit can be padded with an unrelated trivial
	// edit to the very same file, keeping that filename in the net diff while
	// none of the matching commit's own added content survives anywhere in
	// it. Require instead that at least one line the matching commit itself
	// added is still present as an added line in the net base..HEAD diff for
	// that same file, so neither a filename-only match nor a same-file
	// unrelated edit can smuggle a self-cancelling revert past this check.
	netDiff, err := git.DiffFrom(baseCommit)
	if err != nil {
		return CheckResult{
			Pass:        false,
			Remediation: fmt.Sprintf("Failed to get diff: %v", err),
		}
	}
	netAddedByFile := addedLinesByFile(netDiff)

	foundMatchingCommit := false
	for _, entry := range entries {
		if !pattern.MatchString(entry.Subject) {
			continue
		}
		files, err := git.CommitChangedFiles(entry.SHA)
		if err != nil || len(files) == 0 {
			continue
		}
		commitDiff, err := git.CommitDiff(entry.SHA)
		if err != nil {
			continue
		}
		commitAddedByFile := addedLinesByFile(commitDiff)
		commitRemovedByFile := removedLinesByFile(commitDiff)

		survives := false
		for _, f := range files {
			// Check added-content survival and removal survival
			// independently (OR semantics): a file where the matching
			// commit both added and removed lines (e.g. a refactor) must
			// still be checked for deletion survival even when its added
			// content was later edited away, since either kind of
			// evidence alone is sufficient to prove the commit's change
			// survives.
			if len(commitAddedByFile[f]) > 0 && addedContentSurvives(commitAddedByFile[f], netAddedByFile[f]) {
				survives = true
				break
			}
			// addedLinesByFile can never provide survival evidence for a
			// pure removal, since there is no added line to look for.
			// Instead confirm the removed content is genuinely still
			// absent from the current HEAD version of f: if it survives
			// (the file no longer exists, or exists without any of the
			// removed lines), the deletion persisted; if a later commit
			// re-added the removed content, the deletion was undone and
			// must not count as delivered.
			if removedLines := commitRemovedByFile[f]; len(removedLines) > 0 && deletionSurvives(git, f, removedLines) {
				survives = true
				break
			}
		}
		if !survives {
			continue
		}
		foundMatchingCommit = true
		break
	}

	if !foundMatchingCommit {
		return CheckResult{
			Pass: false,
			Remediation: fmt.Sprintf(
				"No commits with non-trivial, undone content found matching conventional-commit format %s(<ISSUE-ID>): ... since %s",
				"[type]", baseCommit,
			),
		}
	}

	return CheckResult{Pass: true, Remediation: ""}
}
