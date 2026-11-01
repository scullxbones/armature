package adapters

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// MinGitVersion is the documented and enforced Git floor. Armature requires
// Git 2.34+ (November 2021) so production walks can rely on
// --diff-merges=separate without older-git shims. Callers that need a
// higher feature gate (for example log.diffMerges=remerge, Git ≥2.36)
// should use Version.AtLeast.
var MinGitVersion = Version{Major: 2, Minor: 34, Patch: 0} //nolint:gochecknoglobals //immutable minimum Git version

// Version is a parsed Git X.Y.Z triple. Extra dotted segments after patch
// (for example Apple Git or Windows builds) are ignored for comparison.
// A prerelease marker on the same X.Y.Z (for example 2.34.0-rc1) is strictly
// below the matching release.
type Version struct {
	Major      int
	Minor      int
	Patch      int
	PreRelease bool
}

// String renders the X.Y.Z form used in docs and error messages. Prereleases
// append "-pre" so the floor message distinguishes them from the release.
func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.PreRelease {
		return s + "-pre"
	}
	return s
}

// AtLeast reports whether v is greater than or equal to want. A prerelease of
// the same X.Y.Z is below the release (2.34.0-rc1 < 2.34.0).
func (v Version) AtLeast(want Version) bool {
	if v.Major != want.Major {
		return v.Major > want.Major
	}
	if v.Minor != want.Minor {
		return v.Minor > want.Minor
	}
	if v.Patch != want.Patch {
		return v.Patch > want.Patch
	}
	if v.PreRelease && !want.PreRelease {
		return false
	}
	return true
}

// ParseGitVersionOutput parses `git --version` stdout (for example
// "git version 2.34.0", "git version 2.43.0 (Apple Git-155)",
// "git version 2.34.0.windows.1", "git version 2.34.0-rc1").
func ParseGitVersionOutput(out string) (Version, error) {
	line := strings.TrimSpace(out)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	lower := strings.ToLower(line)
	const prefix = "git version "
	if !strings.HasPrefix(lower, prefix) {
		return Version{}, fmt.Errorf("unrecognized git --version output %q", line)
	}
	rest := strings.TrimSpace(line[len(prefix):])
	if rest == "" {
		return Version{}, fmt.Errorf("unrecognized git --version output %q", line)
	}
	if i := strings.IndexByte(rest, ' '); i >= 0 {
		rest = rest[:i]
	}
	parts := strings.Split(rest, ".")
	if len(parts) < 2 {
		return Version{}, fmt.Errorf("unrecognized git version %q", rest)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return Version{}, fmt.Errorf("unrecognized git version %q", rest)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return Version{}, fmt.Errorf("unrecognized git version %q", rest)
	}
	patch := 0
	pre := false
	if len(parts) >= 3 {
		patchPart := parts[2]
		digits := 0
		for digits < len(patchPart) && unicode.IsDigit(rune(patchPart[digits])) {
			digits++
		}
		if digits == 0 {
			return Version{}, fmt.Errorf("unrecognized git version %q", rest)
		}
		patch, err = strconv.Atoi(patchPart[:digits])
		if err != nil {
			return Version{}, fmt.Errorf("unrecognized git version %q", rest)
		}
		// "0-rc1" / "0rc1" → prerelease of this X.Y.Z.
		// "0" with later dotted segments like "windows.1" is a release build.
		if digits < len(patchPart) {
			pre = true
		}
	}
	return Version{Major: major, Minor: minor, Patch: patch, PreRelease: pre}, nil
}

// DetectGitVersion runs `git --version` under environ (nil = process env)
// and parses the installed triple. Shared by the CLI min gate and by tests
// that feature-gate on higher versions (for example Git ≥2.36).
func DetectGitVersion(environ []string) (Version, error) {
	cmd := CommandGit(context.Background(), environ, "--version")
	out, err := cmd.Output()
	if err != nil {
		return Version{}, fmt.Errorf("git --version: %w", err)
	}
	return ParseGitVersionOutput(string(bytes.TrimSpace(out)))
}

// CheckMinGitVersion returns a clear error when have is below MinGitVersion.
// The message always names the installed and required versions.
func CheckMinGitVersion(have Version) error {
	if have.AtLeast(MinGitVersion) {
		return nil
	}
	return fmt.Errorf("installed git %s is below the required minimum git %s", have, MinGitVersion)
}

// RequireMinGit detects the installed git and enforces MinGitVersion.
func RequireMinGit(environ []string) error {
	have, err := DetectGitVersion(environ)
	if err != nil {
		return err
	}
	return CheckMinGitVersion(have)
}
