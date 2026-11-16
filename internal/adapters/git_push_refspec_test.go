package adapters_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/gittest"
)

func TestPushRefspecPublishesProofRefOnly_REQ_481(t *testing.T) {
	t.Parallel()
	fx := gittest.InitWithOrigin(t)
	gittest.Git(t, fx.Dir, "commit", "--allow-empty", "-m", "init")
	gittest.Git(t, fx.Dir, "branch", "_armature")
	gittest.Git(t, fx.Dir, "push", "origin", "main")

	proof := "refs/heads/_armature-ci-proof/99"
	if err := adapters.New(fx.Dir).Push("refs/heads/_armature:" + proof); err != nil {
		t.Fatal(err)
	}

	tip := strings.TrimSpace(gitOutput(t, fx.Dir, "rev-parse", "_armature"))
	remoteProof := strings.TrimSpace(gitOutput(t, fx.Origin, "rev-parse", proof))
	if remoteProof != tip {
		t.Fatalf("proof ref = %s, local _armature = %s", remoteProof, tip)
	}
	if _, err := gitOutputErr(fx.Origin, "rev-parse", "refs/heads/_armature"); err == nil {
		t.Fatal("origin/_armature was created by the proof push")
	}
	if err := adapters.New(fx.Dir).Push("-main"); err == nil {
		t.Fatal("flag-like refspec was accepted")
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitOutputErr(dir, args...)
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func gitOutputErr(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
