// Package deliverygate evaluates a worktree against an issue's delivery
// requirements (clean tree, scope containment, commit reference) without
// mutating any state.
package deliverygate

import (
	"bytes"
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

// commitChangeSurvives reports whether at least one path the commit sha
// touched still carries that commit's own contribution at HEAD. Two kinds of
// evidence are checked, both derived from git blob content rather than diff
// text/patch-id comparison:
//
//   - Exact survival (added/modified/renamed content, including
//     100%-similarity content-preserving renames): the commit's post-image
//     blob OID at a path (BlobOIDAtRev(sha, path)) equals HEAD's blob OID at
//     the same path. A content-preserving rename needs no special case
//     here: its post-image blob OID is identical to the pre-rename blob
//     (git produced no textual hunk for it in the first place), so this
//     same OID comparison against the destination path at HEAD naturally
//     recognizes it as surviving.
//   - Deletion survival: content present at a path immediately before the
//     commit but absent immediately after (whether the whole path was
//     deleted, or only some of its lines were removed as part of a larger
//     modification) that is still genuinely absent from that path's current
//     HEAD content. This is what lets a deletion count as delivered even
//     when a later, unrelated commit further edits the same file: exact
//     whole-blob comparison alone would call that "not surviving" (the
//     blobs differ) even though this commit's own removed content never
//     came back.
//
// Both kinds of evidence come from reading blob content at specific,
// already-known-literal paths (via ShowFileAtCommit / BlobOIDAtRev) rather
// than parsing diff text — so unlike the prior patch-id/hunk-parsing
// implementation, no decoding of git's quoted/octal-escaped diff path
// headers is needed for non-ASCII filenames, and a content-preserving
// rename (no textual hunk to find) is handled by construction rather than
// as a special case.
//
// Note: exact survival is intentionally exact-content, not
// cosmetic-reformatting-tolerant. The prior patch-id-based implementation
// whitespace-normalized hunks so a later purely-cosmetic reformat (e.g.
// gofmt rewrap) of survived content wouldn't count as "lost" — that
// tolerance no longer exists here: a byte-for-byte reformat changes the
// blob OID, so it now reads as content no longer surviving verbatim. This
// was traded away deliberately for simplicity: reintroducing a
// text-normalization layer on top of blob content would reintroduce the
// same class of diff-text-parsing bugs blob-based comparison exists to
// eliminate.
func commitChangeSurvives(git *adapters.Client, sha string) (bool, error) {
	entries, err := git.CommitDiffTreeStatus(sha)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		// A copy ("C...") status's OldPath is the copy *source*: that path
		// was never touched by this commit — its content is still exactly
		// where it was before, so it is never "removed content" to check for
		// survival. Only a rename's OldPath (the path that stops existing)
		// or a plain modify/delete's own Path represents genuinely removed
		// content. Treating a copy's OldPath as removed, as an earlier
		// version of this function did, would wrongly report the source
		// path's untouched content as something this commit deleted.
		isCopy := strings.HasPrefix(e.Status, "C")
		preImagePath := e.Path
		if e.OldPath != "" && !isCopy {
			preImagePath = e.OldPath
		}

		if !strings.HasPrefix(e.Status, "D") {
			commitOID, ok, err := git.BlobOIDAtRev(sha, e.Path)
			if err == nil && ok {
				headOID, headOK, err := git.BlobOIDAtRev("HEAD", e.Path)
				if err == nil && headOK && headOID == commitOID {
					return true, nil
				}
			}
		}

		if isCopy {
			// The copy's destination path was already covered by the exact
			// blob-OID check above; there is no separate "removed content"
			// evidence to look for at the (untouched) source path.
			continue
		}

		removed := removedLinesBetweenBlobs(git, sha, preImagePath, e.Path)
		if len(removed) == 0 {
			continue
		}
		headContent, err := git.ShowFileAtCommit("HEAD", preImagePath)
		if err != nil {
			// preImagePath doesn't exist at HEAD at all: every removed
			// line is genuinely absent, so the deletion survives.
			return true, nil
		}
		headLines := lineSet(string(headContent))
		for line := range removed {
			if !headLines[line] {
				return true, nil
			}
		}
	}
	return false, nil
}

// removedLinesBetweenBlobs returns the set of non-blank, trimmed lines
// present in preImagePath's content immediately before commit sha
// (`sha^:preImagePath`) but absent from postImagePath's content as of sha
// itself (`sha:postImagePath`) — i.e. the lines sha removed from
// preImagePath, whether by deleting the whole path or by removing some of
// its lines as part of a larger modification. Returns nil if preImagePath
// didn't exist before sha (a pure add: nothing was removed).
func removedLinesBetweenBlobs(git *adapters.Client, sha, preImagePath, postImagePath string) map[string]bool {
	parentContent, err := git.ShowFileAtCommit(sha+"^", preImagePath)
	if err != nil {
		return nil
	}
	// A binary pre-image has no meaningful notion of "removed lines":
	// splitting arbitrary binary bytes on '\n' produces garbage tokens that
	// almost never re-match at HEAD, which would make this fallback report
	// spurious survival for binary content whose exact blob OID does NOT
	// match at HEAD (i.e. content that was, in fact, further changed). For
	// binary files, the exact blob-OID comparison already performed by the
	// caller (commitChangeSurvives) is the only valid survival signal; if it
	// didn't match, there is nothing more to check here.
	if isBinaryContent(parentContent) {
		return nil
	}
	postLines := map[string]bool{}
	if postContent, err := git.ShowFileAtCommit(sha, postImagePath); err == nil {
		if isBinaryContent(postContent) {
			return nil
		}
		postLines = lineSet(string(postContent))
	}
	removed := map[string]bool{}
	for line := range lineSet(string(parentContent)) {
		if !postLines[line] {
			removed[line] = true
		}
	}
	return removed
}

// isBinaryContent reports whether content looks like binary data, using the
// same NUL-byte heuristic git itself uses to decide whether to treat a blob
// as binary (see git's own buffer_is_binary check). A text file essentially
// never contains a NUL byte, so this is a cheap, reliable-in-practice signal
// without needing to shell out to a separate `git diff --numstat`/attribute
// check just to classify a blob we've already read.
func isBinaryContent(content []byte) bool {
	return bytes.IndexByte(content, 0) >= 0
}

// lineSet splits content into a set of its non-blank, trimmed lines.
func lineSet(content string) map[string]bool {
	set := make(map[string]bool)
	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			set[line] = true
		}
	}
	return set
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
	// A matching commit having non-empty CommitDiffTreeStatus is not enough
	// on its own: a later commit in the range (e.g. a revert) can cancel out
	// exactly the change the matching commit made, leaving nothing actually
	// delivered even though the matching commit "touched files" at the time
	// it was made. Require instead that the matching commit's own
	// contribution — its post-image blob at an added/modified/renamed path,
	// or the absence at HEAD of its pre-image blob at a deleted path — is
	// still present at HEAD. See commitChangeSurvives for why blob-OID
	// comparison (rather than diff text/patch-id comparison) is what makes
	// this robust to non-ASCII filenames and content-preserving renames.
	foundMatchingCommit := false
	for _, entry := range entries {
		if !pattern.MatchString(entry.Subject) {
			continue
		}
		survives, err := commitChangeSurvives(git, entry.SHA)
		if err != nil || !survives {
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
