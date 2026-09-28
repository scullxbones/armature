#!/bin/bash
set -euo pipefail

REPO_ROOT="${1:-.}"
cd "$REPO_ROOT"

if ! command -v rg >/dev/null 2>&1; then
    echo "FAIL: rg is required for check-git-test-hermetic" >&2
    exit 1
fi

# Catch `git init` even when -C / -c (and Go argv equivalents) precede init.
# Do not match `git commit -m init` or `"git", "commit", "-m", "init"`.
hits=$(rg -n --glob '*_test.go' --glob '!internal/gittest/**' \
    -e '\bgit(?:\s+-[Cc]\s+\S+)+\s+init\b' \
    -e '\bgit\s+init\b' \
    -e '"git"(?:,\s*"-C",\s*[^,]+|,\s*"-c",\s*[^,]+)*,\s*"init"' \
    -e '"init", "--bare"' \
    -e '"init", "-b"' \
    -e '"init", "-q"' \
    . || true)

if [[ -n "$hits" ]]; then
    echo "FAIL: raw git init in test files outside internal/gittest:" >&2
    echo "$hits" >&2
    echo "Use gittest.Init / gittest.InitRepo instead." >&2
    exit 1
fi

echo "OK: no raw git init in _test.go files outside internal/gittest"

git_pkgs=$(mktemp)
iso_pkgs=$(mktemp)
trap 'rm -f "$git_pkgs" "$iso_pkgs"' RETURN

while IFS= read -r f; do
    pkg=$(awk '/^package / { print $2; exit }' "$f")
    [[ -n "$pkg" ]] || continue
    key="$(dirname "$f") ${pkg}"
    if rg -q \
        -e 'gittest\.(Init|InitRepo|InitWithOrigin|Git|Main|IsolateGit)' \
        -e '"git"' \
        -e 'GitInitMain' \
        -e 'GitInitBareMain' \
        -e 'NonInteractiveGitCommand' \
        "$f"; then
        printf '%s\n' "$key" >> "$git_pkgs"
    fi
    if rg -q 'func TestMain' "$f" && rg -q 'gittest\.Main|gittest\.IsolateGit|IsolateGit\(|os\.Exit\(Main\(' "$f"; then
        printf '%s\n' "$key" >> "$iso_pkgs"
    fi
done < <(rg --files --glob '*_test.go' .)

sort -u "$git_pkgs" -o "$git_pkgs"
sort -u "$iso_pkgs" -o "$iso_pkgs"

missing=""
while IFS= read -r key; do
    [[ -n "$key" ]] || continue
    if ! rg -F -x -q "$key" "$iso_pkgs"; then
        missing+="$key"$'\n'
    fi
done < "$git_pkgs"

if [[ -n "$missing" ]]; then
    echo "FAIL: packages exec git in tests without TestMain isolation (gittest.IsolateGit / gittest.Main):" >&2
    printf '%s' "$missing" >&2
    echo "Add a TestMain that calls gittest.Main or gittest.IsolateGit." >&2
    exit 1
fi

echo "OK: every git-executing test package has TestMain isolation"
