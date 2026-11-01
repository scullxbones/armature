package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/promotion"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Owner/lease gates the printed remediation path uses (real code — no injected
// Decide/Producer). Each has a named product test; TestDisableMutationsFailNamedTests
// proves a source disable-mutation fails that test (includes locate.go RequireOwner).
var remediationPrintGates = []struct {
	Name     string
	Source   string
	Needle   string
	TestName string
}{
	{
		Name:     "RequireOwner",
		Source:   "internal/oporder/locate.go",
		Needle:   `workerID == "" || lease.Holder != workerID || lease.Token == ""`,
		TestName: "TestPrintedRemediation_RequireOwnerBlocksForeignCancel_REQ_TOPTIER_S18_T4",
	},
	{
		Name:     "ForeignLiveLeaseBlocks",
		Source:   "internal/claim/owner.go",
		Needle:   "return LeaseLive(held, at)",
		TestName: "TestPrintedRemediation_ForeignLiveLeaseBlocksClaim_REQ_TOPTIER_S18_T4",
	},
	{
		Name:     "WouldViolateTreeConsistency",
		Source:   "internal/promotion/remediate.go",
		Needle:   "return nearestMergedAncestor(id, g) != \"\"",
		TestName: "TestPrintedRemediation_TreeConsistencyBlocksClaimUnderMerged_REQ_TOPTIER_S18_T4",
	},
}

// TestPrintedRemediation_RequireOwnerBlocksForeignCancel_REQ_TOPTIER_S18_T4
// drives REAL print path: planOptsForRemediation → remediationDecide →
// requirePublishedOwner → oporder.RequireOwner. No injected Decide/Producer.
// Disable-mutation `if false && (workerID…)` at RequireOwner must FAIL this test.
func TestPrintedRemediation_RequireOwnerBlocksForeignCancel_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	repo, workerID := setupOwnedForeignMergedFixture(t)
	ctx := getTestContext(t, repo)
	opts := planOptsForRemediation(ctx, workerID)
	require.NotNil(t, opts.Decide)
	g := loadPromotionGraph(t, repo)
	require.Equal(t, ops.StatusClaimed, g["OS"].Status)
	require.NotEqual(t, workerID, g["OS"].ClaimedBy)

	plan := promotion.RemediationsPlanOpts("OS", g, opts)
	joined := strings.Join(plan.RenderNextActions(), "\n")
	assert.NotContains(t, joined, "arm transition --issue OS --to cancelled --force",
		"RequireOwner must hide cancel-other on live foreign (real requirePublishedOwner path)")
	require.Len(t, plan.Alternatives, 1,
		"live foreign must drop Option B via Decide/RequireOwner")
	assert.True(t, containsSubstring(plan.Guidance, "live claimed lease on OS"))
}

// TestPrintedRemediation_ForeignLiveLeaseBlocksClaim_REQ_TOPTIER_S18_T4
// drives REAL remediationDecide → claim.ForeignLiveLeaseBlocks. Disable-mutation
// ForeignLiveLeaseBlocks→false must FAIL this test.
func TestPrintedRemediation_ForeignLiveLeaseBlocksClaim_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	now := time.Now().Unix()
	g := promotion.Graph{
		"P": {Status: ops.StatusDone, Children: []string{"C"}},
		"C": {
			Status: ops.StatusClaimed, Parent: "P", ClaimedBy: "other",
			ClaimToken: "tok", ClaimedAt: now - 30, LastActivity: now - 30, TTLMinutes: 60,
		},
	}
	opts := promotion.PlanOpts{
		Viewer: "me",
		Now:    now,
		Decide: remediationDecide(nil, "me", now),
	}
	plan := promotion.RemediationsPlanOpts("P", g, opts)
	for _, line := range plan.RenderNextActions() {
		assert.NotEqual(t, "arm claim --issue C --worktree", promotion.RunnableArgv(line),
			"ForeignLiveLeaseBlocks must omit claim on foreign live child")
	}
}

// TestPrintedRemediation_TreeConsistencyBlocksClaimUnderMerged_REQ_TOPTIER_S18_T4
// drives REAL Decide → WouldViolateTreeConsistency on a viewer-owned claim under
// merged (ForeignLiveLeaseBlocks does not apply). Disable-mutation of the tree
// gate must FAIL this test (Decide would return nil).
func TestPrintedRemediation_TreeConsistencyBlocksClaimUnderMerged_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	repo, workerID := setupOwnedForeignMergedFixture(t)
	ctx := getTestContext(t, repo)
	opts := planOptsForRemediation(ctx, workerID)
	require.NotNil(t, opts.Decide)
	g := loadPromotionGraph(t, repo)
	require.Equal(t, workerID, g["OWN"].ClaimedBy)
	require.True(t, promotion.WouldViolateTreeConsistency("OWN", g))

	err := opts.Decide("arm claim --issue OWN --worktree", g)
	require.Error(t, err, "Decide must refuse claim under merged via WouldViolateTreeConsistency")
	assert.Contains(t, err.Error(), "tree consistency")

	plan := promotion.RemediationsPlanOpts("OWN", g, opts)
	for _, line := range plan.RenderNextActions() {
		cmd := promotion.RunnableArgv(line)
		assert.NotEqual(t, "arm claim --issue OWN --worktree", cmd)
		assert.NotEqual(t, "arm claim --issue OS --worktree", cmd)
	}

	out := new(bytes.Buffer)
	code := executeThenHandleRootError(t, out, new(bytes.Buffer),
		"claim", "--issue", "OWN", "--force", "--worktree", "--repo", repo, "--format", "agent")
	require.Equal(t, 1, code)
	cf := agentFailureFromStdout(t, out.String())
	for _, a := range cf.NextActions {
		assert.NotEqual(t, "arm claim --issue OWN --worktree", promotion.RunnableArgv(a))
	}
}

// TestDisableMutationsFailNamedTests_REQ_TOPTIER_S18_T4 applies each gate
// disable-mutation via go -overlay and requires the named product test to FAIL.
func TestDisableMutationsFailNamedTests_REQ_TOPTIER_S18_T4(t *testing.T) {
	if testing.Short() {
		t.Skip("disable-mutation subprocess proof")
	}
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))

	for _, gate := range remediationPrintGates {
		t.Run(gate.Name, func(t *testing.T) {
			t.Parallel()
			srcPath := filepath.Join(repoRoot, gate.Source)
			body, err := os.ReadFile(srcPath)
			require.NoError(t, err)
			require.Contains(t, string(body), gate.Needle)

			mutDir := t.TempDir()
			mutFile := filepath.Join(mutDir, filepath.Base(gate.Source))
			mutated := disableGateMutation(gate.Name, string(body))
			// #nosec G703 -- mutFile is under t.TempDir with filepath.Base only
			require.NoError(t, os.WriteFile(mutFile, []byte(mutated), 0o644))
			require.True(t, gateDisabled(gate.Name, mutated),
				"disable mutation must neutralize gate %s", gate.Name)

			overlayPath := filepath.Join(mutDir, "overlay.json")
			overlay, err := json.Marshal(map[string]map[string]string{
				"Replace": {srcPath: mutFile},
			})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(overlayPath, overlay, 0o644))

			ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
			t.Cleanup(cancel)
			cmd := exec.CommandContext(ctx, "go", "test", "./cmd/armature/",
				"-count=1", "-timeout", "170s",
				"-overlay", overlayPath,
				"-run", "^"+gate.TestName+"$")
			cmd.Dir = repoRoot
			out, err := cmd.CombinedOutput()
			require.Error(t, err,
				"disable-mutation of %s must FAIL %s; out=\n%s", gate.Name, gate.TestName, out)
			joined := string(out)
			assert.True(t, strings.Contains(joined, "FAIL"),
				"expected FAIL from %s after disabling %s; out=\n%s", gate.TestName, gate.Name, joined)
		})
	}
}

func disableGateMutation(name, src string) string {
	switch name {
	case "RequireOwner":
		// PJ concrete: if false && (…) disables requirePublishedOwner/RequireOwner.
		old := `if workerID == "" || lease.Holder != workerID || lease.Token == "" {
		return claim.ErrNotClaimOwner
	}`
		neu := `if false && (workerID == "" || lease.Holder != workerID || lease.Token == "") {
		return claim.ErrNotClaimOwner
	}`
		if !strings.Contains(src, old) {
			panic("RequireOwner check missing")
		}
		return strings.Replace(src, old, neu, 1)
	case "ForeignLiveLeaseBlocks":
		sig := "func ForeignLiveLeaseBlocks(held Lease, challengerID string, at int64) bool {"
		i := strings.Index(src, sig)
		if i < 0 {
			panic("ForeignLiveLeaseBlocks missing")
		}
		rest := src[i+len(sig):]
		end := strings.Index(rest, "\n}")
		if end < 0 {
			panic("ForeignLiveLeaseBlocks body end missing")
		}
		return src[:i+len(sig)] + "\n\treturn false\n}" + rest[end+2:]
	case "WouldViolateTreeConsistency":
		old := "return nearestMergedAncestor(id, g) != \"\""
		neu := "return false // mutated: tree gate off"
		if !strings.Contains(src, old) {
			panic("WouldViolateTreeConsistency return missing")
		}
		return strings.Replace(src, old, neu, 1)
	default:
		panic("unknown gate " + name)
	}
}

func gateDisabled(name, src string) bool {
	switch name {
	case "RequireOwner":
		return strings.Contains(src, "if false && (workerID")
	case "ForeignLiveLeaseBlocks":
		return strings.Contains(src, "func ForeignLiveLeaseBlocks(held Lease, challengerID string, at int64) bool {\n\treturn false\n}")
	case "WouldViolateTreeConsistency":
		return strings.Contains(src, "return false // mutated: tree gate off")
	default:
		return false
	}
}

// TestReintroducedMutableHookFailsGochecknoglobals_REQ_TOPTIER_S18_T4 proves
// gochecknoglobals is enabled in .golangci.yml: a package-level mutable func
// hook fails golangci-lint (same class as reintroduced RequireOwnerForTest hook).
func TestReintroducedMutableHookFailsGochecknoglobals_REQ_TOPTIER_S18_T4(t *testing.T) {
	if testing.Short() {
		t.Skip("golangci-lint subprocess proof")
	}
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))

	cfg, err := os.ReadFile(filepath.Join(repoRoot, ".golangci.yml"))
	require.NoError(t, err)
	require.Contains(t, string(cfg), "- gochecknoglobals",
		".golangci.yml must enable gochecknoglobals")

	// Probe package must live inside the module for typecheck.
	pkgDir := filepath.Join(repoRoot, "cmd", "armature", "testdata", "gochecknoglobals_hookprobe")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))
	t.Cleanup(func() {
		if remErr := os.RemoveAll(pkgDir); remErr != nil {
			t.Logf("cleanup RemoveAll: %v", remErr)
		}
	})
	src := "package gochecknoglobals_hookprobe\n\nvar requireOwnerHook func(string) error\n"
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "hook.go"), []byte(src), 0o644))

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, "golangci-lint", "run",
		"--enable-only=gochecknoglobals",
		"--max-issues-per-linter=0", "--max-same-issues=0",
		"./cmd/armature/testdata/gochecknoglobals_hookprobe/...")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "XDG_CACHE_HOME=/tmp/golangci-lint-cache")
	out, err := cmd.CombinedOutput()
	joined := string(out)
	require.Error(t, err, "reintroduced mutable hook must fail golangci-lint; out=\n%s", joined)
	assert.Contains(t, joined, "gochecknoglobals",
		"expected gochecknoglobals hit for requireOwnerHook; out=\n%s", joined)
	assert.Contains(t, joined, "requireOwnerHook")
}
