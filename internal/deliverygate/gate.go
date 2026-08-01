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
//
// Precondition: baseCommit must already be an actual merge-base of the
// current branch (as produced by ResolveBaseCommit), not an arbitrary ref —
// the diff below uses two-dot (baseCommit..HEAD) semantics, which silently
// includes commits reachable from baseCommit but not from HEAD if baseCommit
// is a raw branch tip rather than a merge-base.
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

// fileHunks splits a unified diff (as produced by `git diff` or `git show`)
// into per-file (using the "b/" post-image path) lists of raw hunk bodies —
// everything from one "@@ ... @@" header up to (but excluding) the next hunk
// or file-header line. Hunks are the unit of comparison used by
// addedContentSurvives: a contiguous chunk of change plus its surrounding
// context, rather than an isolated content line.
func fileHunks(diff string) map[string][]string {
	result := make(map[string][]string)
	currentFile := ""
	var hunk []string
	flush := func() {
		if currentFile != "" && len(hunk) > 0 {
			result[currentFile] = append(result[currentFile], strings.Join(hunk, "\n"))
		}
		hunk = nil
	}
	for line := range strings.SplitSeq(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "), strings.HasPrefix(line, "--- "):
			flush()
			continue
		case strings.HasPrefix(line, "+++ "):
			flush()
			path := strings.TrimPrefix(line, "+++ ")
			path = strings.TrimPrefix(path, "b/")
			currentFile = ""
			if path != "/dev/null" {
				currentFile = path
			}
			continue
		case strings.HasPrefix(line, "@@"):
			flush()
			hunk = []string{line} // kept verbatim: git patch-id already ignores hunk-header line numbers
			continue
		}
		// A line that is not a diff/file/hunk header: part of the current
		// hunk's body (context/added/removed) if one is in progress, or
		// preamble noise (e.g. an "index .." line) before the first hunk if
		// not.
		if hunk != nil {
			hunk = append(hunk, line)
		}
	}
	flush()
	return result
}

// normalizeDiffContentLine strips a hunk line down to its leading +/-/space
// marker plus its content with internal whitespace collapsed to single
// spaces and outer whitespace trimmed. This makes hunk comparison tolerant
// of purely cosmetic reformatting (gofmt rewrap, indentation, punctuation
// spacing) introduced by an intervening commit, without weakening it to a
// bare substring match: markers and relative content are still compared.
func normalizeDiffContentLine(line string) string {
	if line == "" {
		return line
	}
	marker := ""
	rest := line
	switch line[0] {
	case '+', '-', ' ':
		marker = string(line[0])
		rest = line[1:]
	}
	return marker + strings.Join(strings.Fields(rest), " ")
}

// hunkPatchIDs computes git's own patch-id (via adapters.Client.PatchID) for
// each hunk fileHunks extracts for path f from diff, after normalizing every
// hunk line's whitespace (normalizeDiffContentLine). The hunk header
// ("@@ -a,b +c,d @@") is kept verbatim rather than normalized: git patch-id
// already ignores a hunk header's line-number/count fields on its own, so
// line-number drift between two diffs of the same underlying content (e.g.
// preceding content shifted by an unrelated edit) never affects the
// resulting hash — but a malformed or placeholder header confuses patch-id's
// own hunk-boundary detection and must not be used. When requireAdded
// is true, hunks with no added ("+") line are skipped — used for the
// added-content survival check, which only has evidence to offer from hunks
// that actually added something.
//
// Using git's own patch-id, computed over a full hunk (context lines
// included) rather than a bare content line, is what lets the short-line
// coincidence problem be closed without an arbitrary length heuristic: an
// unrelated commit that happens to add an identical short line elsewhere in
// the same file will not reproduce the same surrounding hunk context, so its
// patch-id will not collide with the matching commit's hunk.
func hunkPatchIDs(git *adapters.Client, diff, f string, requireAdded bool) (map[string]bool, error) {
	ids := make(map[string]bool)
	for _, rawHunk := range fileHunks(diff)[f] {
		lines := strings.Split(rawHunk, "\n")
		hasAdded := false
		normalized := make([]string, 0, len(lines))
		for i, l := range lines {
			if i == 0 {
				normalized = append(normalized, l) // placeholder "@@ @@" header
				continue
			}
			if strings.HasPrefix(l, "+") {
				hasAdded = true
			}
			normalized = append(normalized, normalizeDiffContentLine(l))
		}
		if requireAdded && !hasAdded {
			continue
		}
		doc := fmt.Sprintf("diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n%s\n",
			f, f, f, f, strings.Join(normalized, "\n"))
		id, err := git.PatchID(doc)
		if err != nil {
			return nil, err
		}
		if id != "" {
			ids[id] = true
		}
	}
	return ids, nil
}

// addedContentSurvives reports whether at least one hunk of added content
// the matching commit introduced to file f is still present, as an
// equivalent hunk (via git patch-id, whitespace-normalized), in the net
// base..HEAD diff for that same file. See hunkPatchIDs for why hunk-level
// (rather than bare-line) comparison is what makes this git-native,
// deterministic, and resistant both to reformatting-induced false negatives
// and to short-line-coincidence false positives — the two failure modes of
// the prior line-string-matching implementation.
func addedContentSurvives(git *adapters.Client, commitDiff, netDiff, f string) (bool, error) {
	commitIDs, err := hunkPatchIDs(git, commitDiff, f, true)
	if err != nil {
		return false, err
	}
	if len(commitIDs) == 0 {
		return false, nil
	}
	netIDs, err := hunkPatchIDs(git, netDiff, f, true)
	if err != nil {
		return false, err
	}
	for id := range commitIDs {
		if netIDs[id] {
			return true, nil
		}
	}
	return false, nil
}

// removedLinesByFile parses a unified diff (as produced by `git diff` or
// `git show`) and returns, per file (using the "b/" post-image path, same
// file-identity convention as fileHunks so lookups by the paths returned
// from CommitChangedFiles line up for both), the set of non-blank content
// lines that were removed (a line beginning with "-" in a hunk, excluding
// the "---" file-header line), whitespace-normalized via
// normalizeDiffContentLine (minus its marker) so a removed line that
// reappears with only cosmetic reformatting is still recognized as restored.
// A deletion-only commit removes content without adding any, so hunk-level
// added-content comparison can never provide survival evidence for it;
// removedLinesByFile lets the survival check instead confirm that the
// matching commit's removed content is genuinely absent from the current
// HEAD version of the file.
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
			continue
		case strings.HasPrefix(line, "+++ "):
			path := strings.TrimPrefix(line, "+++ ")
			path = strings.TrimPrefix(path, "b/")
			if path != "/dev/null" {
				currentFile = path
			}
			continue
		}
		if !strings.HasPrefix(line, "-") || currentFile == "" {
			continue
		}
		content := strings.Join(strings.Fields(strings.TrimPrefix(line, "-")), " ")
		if content == "" {
			continue
		}
		if result[currentFile] == nil {
			result[currentFile] = make(map[string]bool)
		}
		result[currentFile][content] = true
	}
	return result
}

// deletionSurvives reports whether a deletion-only change the matching
// commit made to file f (removedLines, its removed content, already
// whitespace-normalized by removedLinesByFile) persists at HEAD: true if at
// least one of removedLines is genuinely absent from f's current HEAD
// content (including if f no longer exists at HEAD at all), false only if
// EVERY removed line has been re-added by a later commit, which is the only
// case where the matching commit's deletion has been fully undone. Requiring
// absence of only one line (rather than all of them) avoids rejecting a
// commit whose deletion genuinely survives just because a later, unrelated
// commit happens to reintroduce one coincidentally-identical line. HEAD's
// content is whitespace-normalized the same way as removedLines so a
// reintroduced line that differs only by cosmetic reformatting is still
// correctly recognized as restored (and so a removed line is still correctly
// recognized as genuinely absent even if unrelated surrounding lines were
// reformatted).
func deletionSurvives(git *adapters.Client, f string, removedLines map[string]bool) bool {
	content, err := git.ShowFileAtCommit("HEAD", f)
	if err != nil {
		// File does not exist at HEAD (or is otherwise unreadable): the
		// removed content cannot be present, so the deletion survives.
		return true
	}
	currentLines := make(map[string]bool)
	for line := range strings.SplitSeq(string(content), "\n") {
		normalized := strings.Join(strings.Fields(line), " ")
		if normalized == "" {
			continue
		}
		currentLines[normalized] = true
	}
	for line := range removedLines {
		if !currentLines[line] {
			return true
		}
	}
	return false
}

// CommitReferenceCheck verifies that at least one commit since baseCommit
// matches the conventional-commit format with the issue ID in the scope.
// Format: <type>(<ISSUE-ID>): ... or <type>(<ISSUE-ID>)!: ...
// where type is one of feat, fix, refactor, test, docs, style, polish
// (see docs/conventions.md).
//
// Precondition: baseCommit must already be an actual merge-base of the
// current branch (as produced by ResolveBaseCommit), not an arbitrary ref —
// LogRange and the net diff below use two-dot (baseCommit..HEAD) semantics,
// which is only correct when baseCommit is the real divergence point.
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
	// Survival itself is determined via git patch-id (hunkPatchIDs /
	// addedContentSurvives), not string/line matching: see those functions'
	// docs for why hunk-level, whitespace-normalized comparison is what
	// makes this robust to both reformatting-induced false negatives and
	// short-line-coincidence false positives.
	netDiff, err := git.DiffFrom(baseCommit)
	if err != nil {
		return CheckResult{
			Pass:        false,
			Remediation: fmt.Sprintf("Failed to get diff: %v", err),
		}
	}

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
			added, addErr := addedContentSurvives(git, commitDiff, netDiff, f)
			if addErr == nil && added {
				survives = true
				break
			}
			// Hunk-based added-content comparison can never provide
			// survival evidence for a pure removal, since there is no
			// added hunk to look for. Instead confirm the removed content
			// is genuinely still absent from the current HEAD version of
			// f: if it survives (the file no longer exists, or exists
			// without any of the removed lines), the deletion persisted;
			// if a later commit re-added the removed content, the
			// deletion was undone and must not count as delivered.
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
