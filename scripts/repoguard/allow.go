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
	// Line is required for object-id waivers (1-based). Zero means "file-level"
	// for checks that still use whole-file allows (git-hermetic, residual-trls).
	Line int
	// Needle must still appear on that line (object-id). Empty → line-only match.
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
			RelPath: "internal/worker/residual_trls_test.go",
			Check:   allowResidualTrls,
			Reason:  "behavioral REQ tests name the banned token in assertions",
		},
		{
			RelPath: "internal/tui/app/model_test.go",
			Check:   allowResidualTrls,
			Reason:  "NavBar REQ asserts the brand string must not contain trls",
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
	for _, a := range allowlist() {
		if a.Check != allowObjectID || a.RelPath != relPath {
			continue
		}
		if a.Line == 0 {
			continue // object-id forbids whole-file waivers
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
