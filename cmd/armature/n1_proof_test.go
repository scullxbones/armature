package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestN1ProofBarInventory_REQ_483(t *testing.T) {
	t.Parallel()
	// ADR 0026 N1 proof bar (fixture-repo squash-merge suite). Each name must
	// remain present so slices 1–3 stay covered after #483 closeout.
	wantTests := []string{
		"TestPromotionReadyCLI_FreshPass_REQ_480",
		"TestPromotionReadyCLI_EmptyContractFails_REQ_480",
		"TestPromotionReadyCLI_UnknownScopeFails_REQ_480",
		"TestPromotionReadyCLI_StaleHeadAssessmentFails_REQ_480",
		"TestPromotionReadyCLI_SharedDiffOneAssessmentSeveralLeaves_REQ_480",
		"TestLandPR_SquashApplyThenNoOp_REQ_481",
		"TestJidokaDriftTreeStopOneAndon_REQ_482",
		"TestLandPR_AlreadyPromotedDryRunNoOps_REQ_483",
	}
	dir, err := os.Getwd()
	require.NoError(t, err)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	found := map[string]bool{}
	fset := token.NewFileSet()
	for _, ent := range entries {
		name := ent.Name()
		if ent.IsDir() || !strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		require.NoError(t, err)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Name == nil {
				continue
			}
			found[fn.Name.Name] = true
		}
	}
	for _, want := range wantTests {
		assert.True(t, found[want], "N1 proof-bar fixture missing: %s", want)
	}
}

func TestLandPR_AlreadyPromotedDryRunNoOps_REQ_483(t *testing.T) {
	t.Parallel()
	fx := setupLandFixture(t)
	repo := fx.Dir

	_, err := runTrls(t, repo, "claim", "task-01", "--worktree")
	require.NoError(t, err)
	wt := filepath.Join(repo, ".worktrees", "task-01")
	scopeFile := filepath.Join(wt, "cmd", "armature", "task_01.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(scopeFile), 0o755))
	require.NoError(t, os.WriteFile(scopeFile, []byte("package armature\n"), 0o644))
	run(t, wt, "git", "add", "cmd/armature/task_01.go")
	run(t, wt, "git", "commit", "-m", "feat(task-01): implement")

	_, err = runTrls(t, wt, "transition", "--issue", "task-01", "--to", "done",
		"--outcome", "Completed", "--force")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	issue, err := materialize.LoadIssue(filepath.Join(getTestStateDir(t, repo), "issues", "task-01.json"))
	require.NoError(t, err)
	require.NotEmpty(t, issue.Base)
	require.NotEmpty(t, issue.Tip)
	tip := issue.Tip
	attestDeliveryForTest(t, repo, "task-01", issue.Base, tip)

	run(t, repo, "git", "checkout", "main")
	run(t, repo, "git", "-c", "core.hooksPath=/dev/null", "merge", "--squash", tip)
	run(t, repo, "git", "-c", "core.hooksPath=/dev/null", "commit", "-m", "feat(task-01): squash (#464)")
	run(t, repo, "git", "push", "origin", tip+":refs/pull/464/head")
	run(t, repo, "git", "worktree", "remove", "--force", wt)
	run(t, repo, "git", "branch", "-D", "task/task-01")

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"land", "--repo", repo, "--pr", "464", "--format", "agent")
	assert.Equal(t, 0, code, "out=%s", stdout.String())
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	status, err := runTrls(t, repo, "show", "task-01", "--field", "status")
	require.NoError(t, err)
	assert.Equal(t, "merged\n", status)

	opsBefore := countAllOps(t, repo)
	stdout2 := new(bytes.Buffer)
	code = executeThenHandleRootError(t, stdout2, new(bytes.Buffer),
		"land", "--repo", repo, "--pr", "464", "--dry-run", "--format", "agent")
	assert.Equal(t, 0, code, "already-promoted dry-run must exit 0; out=%s", stdout2.String())
	assert.Equal(t, opsBefore, countAllOps(t, repo), "already-promoted dry-run must write no ops")
	_, rows := decodeSyncIssues(t, stdout2.String())
	assert.Empty(t, rows, "already-promoted dry-run must select no done leaves")
}
