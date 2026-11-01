package gittest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
)

func TestConfigValuesSkipMessage_RemergeGate_REQ_OPORDER_LOCATE(t *testing.T) {
	t.Parallel()
	cur34 := adapters.Version{Major: 2, Minor: 34, Patch: 0}
	if msg, skip := configValuesSkipMessage(cur34, [][2]string{{"log.diffMerges", "combined"}}); skip {
		t.Fatalf("combined must not skip on 2.34: %s", msg)
	}
	if msg, skip := configValuesSkipMessage(cur34, [][2]string{{"log.diffMerges", "remerge"}}); !skip {
		t.Fatal("remerge must skip on 2.34")
	} else if msg == "" {
		t.Fatal("remerge skip message must be non-empty")
	}
	cur36 := adapters.Version{Major: 2, Minor: 36, Patch: 0}
	if msg, skip := configValuesSkipMessage(cur36, [][2]string{{"log.diffMerges", "remerge"}}); skip {
		t.Fatalf("remerge must run on 2.36: %s", msg)
	}
	// Unknown values never skip — only the registry gates.
	SkipUnlessConfigValuesSupported(t, [][2]string{
		{"log.diffMerges", "combined"},
		{"core.quotePath", "true"},
	})
}

func TestConfigValueMinGit_RemergeRequires236_REQ_OPORDER_LOCATE(t *testing.T) {
	t.Parallel()
	min, ok := configValueMinGit()["log.diffMerges=remerge"]
	if !ok {
		t.Fatal("log.diffMerges=remerge must be in configValueMinGit() (drop-gate mutation would miss 2.34 red)")
	}
	want := adapters.Version{Major: 2, Minor: 36, Patch: 0}
	if min != want {
		t.Fatalf("remerge floor=%s want %s", min.String(), want.String())
	}
	// adapters.Version.AtLeast is the comparison authority (#414).
	cur34 := adapters.Version{Major: 2, Minor: 34, Patch: 0}
	if cur34.AtLeast(min) {
		t.Fatal("2.34 must not AtLeast remerge floor 2.36")
	}
}

func TestRemergeConfigPoisonsGitBelow236_REQ_OPORDER_LOCATE(t *testing.T) {
	t.Parallel()
	cur, err := adapters.DetectGitVersion(nil)
	if err != nil {
		t.Fatalf("DetectGitVersion: %v", err)
	}
	if cur.AtLeast(adapters.Version{Major: 2, Minor: 36, Patch: 0}) {
		t.Skipf("mutation proof for drop-gate: need Git <2.36 to observe poison (have %s)", cur.String())
	}
	repo := t.TempDir()
	Git(t, repo, "init")
	Git(t, repo, "config", "user.email", "t@t")
	Git(t, repo, "config", "user.name", "t")
	Git(t, repo, "commit", "--allow-empty", "-m", "root")
	// Ungated write — proves why SkipUnlessConfigValuesSupported must run first.
	if out, err := gitCombined(repo, "config", "log.diffMerges", "remerge"); err != nil {
		t.Fatalf("writing remerge value: %v (%s)", err, out)
	}
	if _, err := gitCombined(repo, "log", "-1"); err == nil {
		t.Fatal("expected git log to fail after ungated log.diffMerges=remerge on Git <2.36")
	}
}

func TestInitSHA256Repo_SucceedsOnSupportedGit_REQ_OPORDER_LOCATE(t *testing.T) {
	t.Parallel()
	cur, err := adapters.DetectGitVersion(nil)
	if err != nil {
		t.Fatalf("DetectGitVersion: %v", err)
	}
	if !cur.AtLeast(objectFormatSHA256Min()) {
		t.Skipf("sha256 object format needs Git ≥%s (have %s)", objectFormatSHA256Min().String(), cur.String())
	}
	dir := t.TempDir()
	InitSHA256Repo(t, dir)
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf("expected sha256 repo: %v", err)
	}
	format := GitOutput(t, dir, "rev-parse", "--show-object-format")
	if format != "sha256" {
		t.Fatalf("object format=%q want sha256", format)
	}
}

func TestSkipUnlessConfigValuesSupported_RemergeOnLiveGit_REQ_OPORDER_LOCATE(t *testing.T) {
	t.Parallel()
	cur, err := adapters.DetectGitVersion(nil)
	if err != nil {
		t.Fatalf("DetectGitVersion: %v", err)
	}
	// On Git ≥2.36 this must not skip; on 2.34 it skips (and the parent test
	// still passes). Either outcome is the gate working.
	SkipUnlessConfigValuesSupported(t, [][2]string{{"log.diffMerges", "remerge"}})
	if !cur.AtLeast(configValueMinGit()["log.diffMerges=remerge"]) {
		t.Fatal("SkipUnlessConfigValuesSupported should have skipped on Git <2.36")
	}
}
