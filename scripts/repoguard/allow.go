package main

const allowGitHermeticName = "git-hermetic-name"

const allowGitHermeticInit = "git-hermetic-init"

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
