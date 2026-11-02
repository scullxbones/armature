package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const allowGitHermeticName = "git-hermetic-name"

const allowGitHermeticInit = "git-hermetic-init"

const allowObjectID = "object-id"

const allowResidualTrls = "residual-trls"

// allowEntry is the single waiver type for every repoguard check.
// Line must be > 0 (1-based). Line == 0 is rejected by validateAllowlist —
// whole-file waivers are forbidden for every check. Matching is exact
// file + line + needle (hit must equal needle or contain it; never the reverse).
type allowEntry struct {
	RelPath string
	Check   string
	Reason  string
	Line    int
	// Needle must still appear in the hit (or on that line). Empty → line-only.
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
			Line:    375,
			Needle:  `^[0-9a-f]{40}$`,
			Reason:  "GitHub Actions uses: pin SHAs are always 40-hex by GitHub's API, not repo object-format OIDs",
		},
		{
			RelPath: "scripts/test_check_jobs.sh",
			Check:   allowObjectID,
			Line:    820,
			Needle:  `@[0-9a-f]{40}\b`,
			Reason:  "GitHub Actions uses: pin SHAs are always 40-hex by GitHub's API, not repo object-format OIDs",
		},
		// Schema sha256 *content* digests are width-64 by algorithm, not git OIDs.
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
			Line:    16,
			Needle:  `^sha256:[a-f0-9]{64}$`,
			Reason:  "bundle_id is a SHA-256 content digest prefix, not a git object id",
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
			Line:    16,
			Needle:  `^sha256:[a-f0-9]{64}$`,
			Reason:  "assessment id is a SHA-256 content digest, not a git object id",
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

// validateAllowlist fails closed: any Line == 0 entry is a hard error;
// on the real armature repo, each entry's needle must still appear on its
// exact line (stale → FAIL). Fixture trees (module fixture) skip presence.
func validateAllowlist(root string) error {
	entries := allowlist()
	if err := validateAllowEntries(entries); err != nil {
		return err
	}
	if !isArmatureRoot(root) {
		return nil
	}
	return validateAllowEntriesPresent(root, entries)
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
		if a.RelPath == "" || a.Check == "" || a.Reason == "" {
			return fmt.Errorf("allowlist[%d]: RelPath, Check, and Reason are required", i)
		}
	}
	return nil
}

// validateAllowEntriesPresent fails when a waiver's needle is absent from its
// exact line (stale allow entry).
func validateAllowEntriesPresent(root string, entries []allowEntry) error {
	for i, a := range entries {
		if a.Needle == "" {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(a.RelPath))
		data, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
				continue // fixture trees omit unrelated waived paths
			}
			return fmt.Errorf("allowlist[%d] stale: %s: %w", i, a.RelPath, err)
		}
		lines := strings.Split(string(data), "\n")
		if a.Line < 1 || a.Line > len(lines) {
			return fmt.Errorf("allowlist[%d] stale: %s:%d out of range (file has %d lines)", i, a.RelPath, a.Line, len(lines))
		}
		if !strings.Contains(lines[a.Line-1], a.Needle) {
			return fmt.Errorf("allowlist[%d] stale: %s:%d missing needle %q", i, a.RelPath, a.Line, a.Needle)
		}
	}
	return nil
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

// allowedSite is the shared per-site waiver validator for every check.
// Match is exact file + line + needle (hit equals needle or contains needle;
// reverse substring match is forbidden).
func allowedSite(relPath, check string, line int, pattern string) bool {
	if line == 0 {
		return false
	}
	for _, a := range allowlist() {
		if a.Check != check || a.RelPath != relPath {
			continue
		}
		if a.Line != line {
			continue
		}
		if a.Needle != "" && pattern != a.Needle && !strings.Contains(pattern, a.Needle) {
			continue
		}
		return true
	}
	return false
}

func allowedObjectIDSite(relPath string, line int, pattern string) bool {
	return allowedSite(relPath, allowObjectID, line, pattern)
}

func allowedResidualTrlsSite(relPath string, line int, needle string) bool {
	return allowedSite(relPath, allowResidualTrls, line, needle)
}

func allowedGitHermeticSite(relPath, check string, line int, needle string) bool {
	return allowedSite(relPath, check, line, needle)
}
