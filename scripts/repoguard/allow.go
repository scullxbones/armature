package main

const allowGitHermeticName = "git-hermetic-name"

const allowGitHermeticInit = "git-hermetic-init"

const allowObjectID = "object-id"

const allowResidualTrls = "residual-trls"

type allowEntry struct {
	RelPath string
	Check   string
	Reason  string
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
			RelPath: "internal/gitoid/oid.go",
			Check:   allowObjectID,
			Reason:  "canonical PatternLower home — the one allowed 40|64 literal",
		},
		{
			RelPath: "internal/gitoid/oid_test.go",
			Check:   allowObjectID,
			Reason:  "asserts PatternLower contains {40} and {64}; not a second predicate",
		},
		{
			RelPath: "scripts/test_check_jobs.sh",
			Check:   allowObjectID,
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
		if a.RelPath == relPath && a.Check == check {
			return true
		}
	}
	return false
}
