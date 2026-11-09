package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scullxbones/armature/internal/review"
)

func TestCommandCodeBansProcessStdio_REQ_MCPSTDIO_S1_T2(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	pkgDir := filepath.Dir(thisFile)

	allowedFiles := map[string]bool{
		"invocation.go": true,
		"main.go":       true,
	}
	bannedIdents := []string{"os.Stdin", "os.Stdout"}

	entries, err := os.ReadDir(pkgDir)
	require.NoError(t, err)

	fset := token.NewFileSet()
	var violations []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if allowedFiles[name] {
			continue
		}
		src, readErr := os.ReadFile(filepath.Join(pkgDir, name))
		require.NoError(t, readErr, name)
		_, parseErr := parser.ParseFile(fset, name, src, 0)
		require.NoError(t, parseErr, name)
		astText := string(src)
		for _, ident := range bannedIdents {
			if strings.Contains(astText, ident) {
				violations = append(violations, fmt.Sprintf("%s references %s (only invocation.go and main.go may)", name, ident))
			}
		}
	}
	require.Empty(t, violations, strings.Join(violations, "\n"))
}

func TestReviewAssessmentDashReadsInvocationStdin_REQ_MCPSTDIO_S1_T2(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	run(t, repo, "git", "commit", "--allow-empty", "-m", "commit 1")
	baseCmd := newCmdInDir(repo, "rev-parse", "HEAD")
	baseOut, err := baseCmd.Output()
	require.NoError(t, err)
	base := strings.TrimSpace(string(baseOut))

	require.NoError(t, os.WriteFile(filepath.Join(repo, "impl.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "impl.go")
	run(t, repo, "git", "commit", "-m", "commit 2 — add implementation")
	headCmd := newCmdInDir(repo, "rev-parse", "HEAD")
	headOut, err := headCmd.Output()
	require.NoError(t, err)
	head := strings.TrimSpace(string(headOut))

	bundleOut, err := runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head)
	require.NoError(t, err)

	var bundle review.ReviewBundle
	err = json.Unmarshal([]byte(strings.TrimSpace(bundleOut)), &bundle)
	require.NoError(t, err)

	assessment := review.ConformanceAssessment{
		SchemaVersion:       review.SchemaVersion,
		BundleID:            bundle.BundleID,
		ContractFingerprint: bundle.Fingerprints.Contract,
		DeliveryFingerprint: bundle.Fingerprints.Delivery,
		Results: []review.CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    review.Satisfied,
				Rationale: "Implementation is complete and tested.",
				Citations: []review.Citation{
					review.FileCitation("impl.go", 1, 0),
				},
			},
		},
	}
	assessmentJSON, err := json.Marshal(&assessment)
	require.NoError(t, err)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	code := Run(t.Context(), Invocation{
		Args:    []string{"review", "record", "--repo", repo, "--issue", "task-01", "--assessment", "-"},
		Dir:     repo,
		Env:     OverlayEnv(OSEnv(), cliEnvFor(t)),
		Stdin:   bytes.NewReader(assessmentJSON),
		Stdout:  stdout,
		Stderr:  stderr,
		TempDir: t.TempDir(),
	})
	require.Equal(t, 0, code, "stderr=%s", stderr.String())
	assert.Contains(t, stdout.String(), "recorded")
}
