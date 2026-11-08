package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const allowGitHermeticName = "git-hermetic-name"

const allowGitHermeticInit = "git-hermetic-init"

const allowObjectID = "object-id"

const allowResidualTrls = "residual-trls"

// allowEntry is the single waiver type for every repoguard check.
// Line must be > 0 (1-based). Needle must be non-empty. Matching is exact
// file + line + needle (hit must equal needle or contain it; never the reverse).
// An entry that never waives a real hit during the scan is unused → FAIL.
type allowEntry struct {
	RelPath string
	Check   string
	Reason  string
	Line    int
	// Needle must appear in the hit. Empty is rejected by validateAllowEntries
	// (strings.Contains(hit, "") would otherwise waive any hit at file:line).
	Needle string
}

func allowlist() []allowEntry {
	return []allowEntry{
		// git-hermetic-name dropped: git_internal_test constructs the binary
		// name without a bare "git" BasicLit (see WithEnviron PATH overlay).
		{
			RelPath: "scripts/repoguard/repoguard_test.go",
			Check:   allowGitHermeticInit,
			Line:    448,
			Needle:  "init",
			Reason:  "fixture assertions quote raw git init; production packages still fail",
		},
		{
			RelPath: "scripts/repoguard/repoguard_test.go",
			Check:   allowGitHermeticInit,
			Line:    511,
			Needle:  "init",
			Reason:  "OK-line assertion quotes raw git init; production packages still fail",
		},
		{
			RelPath: "scripts/test_check_jobs.sh",
			Check:   allowObjectID,
			Line:    515,
			Needle:  `^[0-9a-f]{40}$`,
			Reason:  "GitHub Actions uses: pin SHAs are always 40-hex by GitHub's API, not repo object-format OIDs",
		},
		{
			RelPath: "scripts/test_check_jobs.sh",
			Check:   allowObjectID,
			Line:    960,
			Needle:  `@[0-9a-f]{40}\b`,
			Reason:  "GitHub Actions uses: pin SHAs are always 40-hex by GitHub's API, not repo object-format OIDs",
		},
		{
			RelPath: "scripts/workflowimagepins/main.go",
			Check:   allowObjectID,
			Line:    40,
			Needle:  `^[0-9a-f]{40}$`,
			Reason:  "GitHub Actions uses: pin SHAs are always 40-hex by GitHub's API, not repo object-format OIDs",
		},
		{
			RelPath: "scripts/workflowimagepins/main.go",
			Check:   allowObjectID,
			Line:    840,
			Needle:  "width-compare",
			Reason:  "GitHub Actions uses: pin SHAs are always 40-hex by GitHub's API, not repo object-format OIDs",
		},
		{
			RelPath: "scripts/workflowimagepins/main.go",
			Check:   allowObjectID,
			Line:    842,
			Needle:  "width-compare",
			Reason:  "GitHub Actions uses: pin SHAs are always 40-hex by GitHub's API, not repo object-format OIDs",
		},
		// Schema SHA-256 *content* digests that the scanner flags as 64-only
		// (sha256:-prefixed patterns are not hits under oidPatternBanReason today;
		// unused-entry audit forbids waiving them).
		{
			RelPath: "docs/schemas/activity-index.schema.json",
			Check:   allowObjectID,
			Line:    20,
			Needle:  `^[a-f0-9]{64}$`,
			Reason:  "activity-index content digest is SHA-256 hex, not a git object id",
		},
		{
			RelPath: "docs/schemas/review-bundle.schema.json",
			Check:   allowObjectID,
			Line:    105,
			Needle:  `^[a-f0-9]{64}$`,
			Reason:  "fingerprint fields are SHA-256 content digests, not git object ids",
		},
		{
			RelPath: "docs/schemas/review-bundle.schema.json",
			Check:   allowObjectID,
			Line:    110,
			Needle:  `^[a-f0-9]{64}$`,
			Reason:  "fingerprint fields are SHA-256 content digests, not git object ids",
		},
		{
			RelPath: "docs/schemas/review-bundle.schema.json",
			Check:   allowObjectID,
			Line:    122,
			Needle:  `^[a-f0-9]{64}$`,
			Reason:  "fingerprint fields are SHA-256 content digests, not git object ids",
		},
		{
			RelPath: "docs/schemas/conformance-assessment.schema.json",
			Check:   allowObjectID,
			Line:    114,
			Needle:  `^[a-f0-9]{64}$`,
			Reason:  "fingerprint fields are SHA-256 content digests, not git object ids",
		},
		{
			RelPath: "docs/schemas/conformance-assessment.schema.json",
			Check:   allowObjectID,
			Line:    119,
			Needle:  `^[a-f0-9]{64}$`,
			Reason:  "fingerprint fields are SHA-256 content digests, not git object ids",
		},
		{
			RelPath: "internal/worker/residual_trls_test.go",
			Check:   allowResidualTrls,
			Line:    17,
			Needle:  "trls",
			Reason:  "behavioral REQ asserts the banned token is absent from the error",
		},
		{
			RelPath: "internal/tui/app/model_test.go",
			Check:   allowResidualTrls,
			Line:    76,
			Needle:  "trls",
			Reason:  "NavBar REQ asserts the brand string must not contain trls",
		},
		{
			RelPath: "internal/tui/app/model_test.go",
			Check:   allowResidualTrls,
			Line:    77,
			Needle:  "trls",
			Reason:  "NavBar REQ error text names the banned token",
		},
	}
}

// allowIndex tracks which allowlist entries waived a hit during one scan.
// Missing files and needle-on-line-but-no-hit entries stay unused → FAIL.
type allowIndex struct {
	entries []allowEntry
	used    []bool
}

func newAllowIndex(entries []allowEntry) *allowIndex {
	return &allowIndex{entries: entries, used: make([]bool, len(entries))}
}

// validateAllowlist fails closed on malformed entries (Line == 0, empty needle).
// Unused-entry audit happens after the real scan via allowIndex.unused — not by
// skipping fixture trees via module name or ErrNotExist.
func validateAllowlist() error {
	return validateAllowEntries(allowlist())
}

func isArmatureRoot(root string) bool {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(string(data)), "module github.com/scullxbones/armature")
}

func validateAllowEntries(entries []allowEntry) error {
	for i, a := range entries {
		if a.Line == 0 {
			return fmt.Errorf("allowlist[%d] %s %s: Line == 0 forbidden (no whole-file waivers)", i, a.Check, a.RelPath)
		}
		if a.Needle == "" {
			return fmt.Errorf("allowlist[%d] %s %s: empty Needle forbidden (would waive any hit at file:line)", i, a.Check, a.RelPath)
		}
		if a.RelPath == "" || a.Check == "" || a.Reason == "" {
			return fmt.Errorf("allowlist[%d]: RelPath, Check, and Reason are required", i)
		}
	}
	return nil
}

// unused fails when any entry for check never waived a hit during the scan.
// Missing files and no-hit sites never set used[i] → FAIL.
func (idx *allowIndex) unused(check string) error {
	for i, a := range idx.entries {
		if a.Check != check {
			continue
		}
		if idx.used[i] {
			continue
		}
		return fmt.Errorf("allowlist[%d] unused: %s:%d needle %q (%s) — no hit waived (missing file counts as unused)",
			i, a.RelPath, a.Line, a.Needle, a.Check)
	}
	return nil
}

// allowed records a waiver when file+line+needle match a hit. Empty needles
// never match (defense in depth; validateAllowEntries already rejects them).
func (idx *allowIndex) allowed(relPath, check string, line int, pattern string) bool {
	if line == 0 || pattern == "" {
		return false
	}
	for i, a := range idx.entries {
		if a.Check != check || a.RelPath != relPath {
			continue
		}
		if a.Line != line {
			continue
		}
		if a.Needle == "" {
			continue
		}
		if pattern != a.Needle && !strings.Contains(pattern, a.Needle) {
			continue
		}
		idx.used[i] = true
		return true
	}
	return false
}

// allowedSite is match-only against the production allowlist (no usage
// tracking). Prefer allowIndex.allowed during real scans.
func allowedSite(relPath, check string, line int, pattern string) bool {
	return newAllowIndex(allowlist()).allowed(relPath, check, line, pattern)
}

func allowedResidualTrlsSite(relPath string, line int, needle string) bool {
	return allowedSite(relPath, allowResidualTrls, line, needle)
}

func allowedGitHermeticSite(relPath, check string, line int, needle string) bool {
	return allowedSite(relPath, check, line, needle)
}

// gitNetAllow is one local-transport git call site. The production statement
// carries `gitnetallow: <site> <reason>` and the checker accepts that call
// only when file, function, site, and reason all match. A file-wide comment
// does not match.
type gitNetAllow struct {
	RelPath string
	Func    string
	Site    string
	Reason  string
}

func gitNetAllows() []gitNetAllow {
	return []gitNetAllow{
		{
			RelPath: "internal/adapters/git.go",
			Func:    "BundleCreateAdvertised",
			Site:    "bundle-create-local-bare-push",
			Reason:  "local bare path under armature-tmp-work, not a named remote",
		},
		{
			RelPath: "internal/adapters/git.go",
			Func:    "BundleVerifyRestorable",
			Site:    "bundle-verify-local-fetch",
			Reason:  "absolute bundle file into a throwaway local bare, not a remote URL",
		},
		{
			RelPath: "internal/adapters/git.go",
			Func:    "RestoreEvidence",
			Site:    "restore-local-bundle-fetch",
			Reason:  "absolute bundle file into refs/armature-tmp; origin fetch stays on networkCmd",
		},
		{
			RelPath: "internal/e2e/harness/harness.go",
			Func:    "New",
			Site:    "e2e-local-origin-push",
			Reason:  "local bare from gittest.InitWithOrigin, not a network URL",
		},
		{
			RelPath: "internal/e2e/harness/harness.go",
			Func:    "Clone",
			Site:    "e2e-local-origin-clone",
			Reason:  "local bare path in h.OriginDir, not a network URL",
		},
	}
}
