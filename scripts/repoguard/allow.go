package main

import "strings"

const allowGitHermeticName = "git-hermetic-name"

const allowGitHermeticInit = "git-hermetic-init"

const allowObjectID = "object-id"

const allowResidualTrls = "residual-trls"

type allowEntry struct {
	RelPath string
	Check   string
	Reason  string
	// Line is required for object-id and residual-trls (1-based). Zero means
	// "file-level" only for checks that still use whole-file allows (git-hermetic).
	Line int
	// Needle must still appear on that line. Empty → line-only match.
	// Stale when the waived pattern moves or a new pattern appears elsewhere.
	Needle string
}

func allowlist() []allowEntry {
	return []allowEntry{
		{
			RelPath: "internal/adapters/git_internal_test.go",
			Check:   allowGitHermeticName,
			Reason:  "PATH overlay uses the git binary name; TestMain lives in package adapters_test",
		},
		{
			RelPath: "scripts/repoguard/repoguard_test.go",
			Check:   allowGitHermeticInit,
			Reason:  "fixture assertions and example sources quote raw git init; production packages still fail",
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

func allowed(relPath, check string) bool {
	for _, a := range allowlist() {
		if a.RelPath == relPath && a.Check == check && a.Line == 0 {
			return true
		}
	}
	return false
}

// allowedObjectIDSite is the per-site object-id waiver: file + line + needle.
// A new 40-only pattern elsewhere in a waived file (or on a moved line without
// the needle) is NOT covered — that was the #419 per-site requirement.
func allowedObjectIDSite(relPath string, line int, pattern string) bool {
	return allowedSite(relPath, allowObjectID, line, pattern)
}

// allowedResidualTrlsSite is the per-site residual-trls waiver: file + line + needle.
// Line == 0 is rejected (no whole-file waivers).
func allowedResidualTrlsSite(relPath string, line int, needle string) bool {
	return allowedSite(relPath, allowResidualTrls, line, needle)
}

func allowedSite(relPath, check string, line int, pattern string) bool {
	if line == 0 {
		return false
	}
	for _, a := range allowlist() {
		if a.Check != check || a.RelPath != relPath {
			continue
		}
		if a.Line == 0 {
			continue // object-id / residual-trls forbid whole-file waivers
		}
		if line != a.Line {
			continue
		}
		if a.Needle != "" && pattern != a.Needle && !strings.Contains(pattern, a.Needle) {
			continue
		}
		return true
	}
	return false
}
