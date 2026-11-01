package gittest

import (
	"fmt"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
)

// configValueMinGit is the ONE registry of version-specific git config values
// used by test matrices. Every matrix entry that sets a listed value must call
// SkipUnlessConfigValuesSupported so older pinned CI gits skip instead of fail.
//
// Keys are "section.key=value" (exact). Floors use adapters.Version so
// comparison matches production (adapters.Version.AtLeast / DetectGitVersion).
func configValueMinGit() map[string]adapters.Version {
	return map[string]adapters.Version{
		// log.diffMerges=remerge: Git 2.36 (Documentation/RelNotes/2.36.0).
		// On 2.34, setting the value poisons .git/config
		// ("bad config variable 'log.diffmerges'").
		"log.diffMerges=remerge": {Major: 2, Minor: 36, Patch: 0},
	}
}

// SkipUnlessConfigValuesSupported gates every version-specific config value in
// pairs via configValueMinGit. Call before writing those values into a fixture.
func SkipUnlessConfigValuesSupported(t *testing.T, pairs [][2]string) {
	t.Helper()
	cur, err := adapters.DetectGitVersion(nil)
	if err != nil {
		t.Fatalf("detect git version: %v", err)
	}
	if msg, skip := configValuesSkipMessage(cur, pairs); skip {
		t.Skipf("%s", msg)
	}
}

func configValuesSkipMessage(cur adapters.Version, pairs [][2]string) (string, bool) {
	for _, p := range pairs {
		key := p[0] + "=" + p[1]
		min, ok := configValueMinGit()[key]
		if !ok || cur.AtLeast(min) {
			continue
		}
		return fmt.Sprintf("config %s needs Git ≥%s (have %s)", key, min.String(), cur.String()), true
	}
	return "", false
}

// objectFormatSHA256Min() is when `git init --object-format=sha256` landed.
func objectFormatSHA256Min() adapters.Version {
	return adapters.Version{Major: 2, Minor: 29, Patch: 0}
}

// InitSHA256Repo runs `git init --object-format=sha256` in dir.
// On Git ≥2.29 a failure is a test failure (capability must work).
// Below 2.29 the test skips with an explicit reason.
func InitSHA256Repo(t *testing.T, dir string) {
	t.Helper()
	cur, err := adapters.DetectGitVersion(nil)
	if err != nil {
		t.Fatalf("detect git version: %v", err)
	}
	cmd := adapters.NonInteractiveGitCommand(dir, "init", "--object-format=sha256")
	out, err := cmd.CombinedOutput()
	if err == nil {
		return
	}
	if !cur.AtLeast(objectFormatSHA256Min()) {
		t.Skipf("object-format=sha256 needs Git ≥%s (have %s): %v (%s)",
			objectFormatSHA256Min().String(), cur.String(), err, out)
	}
	t.Fatalf("object-format=sha256 must succeed on Git ≥%s (have %s): %v (%s)",
		objectFormatSHA256Min().String(), cur.String(), err, out)
}
