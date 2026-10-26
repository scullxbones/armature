package main

const allowGitHermeticName = "git-hermetic-name"

const allowGitHermeticInit = "git-hermetic-init"

const allowTestExecChmod = "test-executables-chmod"

type allowEntry struct {
	RelPath string
	Check   string
	Reason  string
}

var allowlist = []allowEntry{
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
		RelPath: "cmd/armature/dag_override_release_test.go",
		Check:   allowTestExecChmod,
		Reason:  "restores opsDir after a read-only snapshot; WriteFile(opsDir) elsewhere in the package is a path-ident collision",
	},
	{
		RelPath: "cmd/armature/harness_hook_test.go",
		Check:   allowTestExecChmod,
		Reason:  "restores opsDir after chmod 0o000; WriteFile(opsDir) elsewhere in the package is a path-ident collision",
	},
	{
		RelPath: "cmd/armature/helpers_test.go",
		Check:   allowTestExecChmod,
		Reason:  "restores opsDir after chmod 0o555; WriteFile(opsDir) elsewhere in the package is a path-ident collision",
	},
	{
		RelPath: "cmd/armature/merged_test.go",
		Check:   allowTestExecChmod,
		Reason:  "restores opsDir after chmod 0o444; WriteFile(opsDir) elsewhere in the package is a path-ident collision",
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
