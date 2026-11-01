package adapters

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
)

// MinGitVersion is the documented and enforced Git floor. Armature requires
// Git 2.34+ (November 2021) so production walks can rely on
// --diff-merges=separate without older-git shims. Callers that need a
// higher feature gate (for example log.diffMerges=remerge, Git ≥2.36)
// should use Version.AtLeast.
var MinGitVersion = Version{Major: 2, Minor: 34, Patch: 0}

// Version is a parsed Git X.Y.Z triple. Extra dotted segments after patch
// (for example Apple Git or Windows builds) are ignored for comparison.
type Version struct {
	Major int
	Minor int
	Patch int
}

// String renders the X.Y.Z form used in docs and error messages.
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// AtLeast reports whether v is greater than or equal to want.
func (v Version) AtLeast(want Version) bool {
	if v.Major != want.Major {
		return v.Major > want.Major
	}
	if v.Minor != want.Minor {
		return v.Minor > want.Minor
	}
	return v.Patch >= want.Patch
}

// ParseGitVersionOutput parses `git --version` stdout (for example
// "git version 2.34.0", "git version 2.43.0 (Apple Git-155)",
// "git version 2.34.0.windows.1").
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
	if len(parts) >= 3 {
		patchPart := parts[2]
		if j := strings.IndexFunc(patchPart, func(r rune) bool {
			return r < '0' || r > '9'
		}); j >= 0 {
			patchPart = patchPart[:j]
		}
		if patchPart == "" {
			return Version{}, fmt.Errorf("unrecognized git version %q", rest)
		}
		patch, err = strconv.Atoi(patchPart)
		if err != nil {
			return Version{}, fmt.Errorf("unrecognized git version %q", rest)
		}
	}
	return Version{Major: major, Minor: minor, Patch: patch}, nil
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
