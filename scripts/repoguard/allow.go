package main

// allowGitHermeticName permits mentioning the git binary name without treating
// the test package as git-executing. Raw git init and exec.Command("git", ...)
// still fail. Add rows here; do not concatenate around the letters git.
const allowGitHermeticName = "git-hermetic-name"

type allowEntry struct {
	RelPath string
	Check   string
	Reason  string
}

// allowlist is the reviewable exception list for repo guards. Syntax tricks
// are not exceptions.
var allowlist = []allowEntry{
	{
		RelPath: "internal/adapters/git_internal_test.go",
		Check:   allowGitHermeticName,
		Reason:  "PATH overlay uses the git binary name; TestMain lives in package adapters_test",
	},
}

func allowed(relPath, check string) bool {
	for _, a := range allowlist {
		if a.RelPath == relPath && a.Check == check {
			return true
		}
	}
	return false
}
