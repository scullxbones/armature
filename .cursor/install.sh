#!/usr/bin/env bash
# Idempotent repository bootstrap for Armature Cloud Agents.
#
# Installs the two developer tools that `make check` needs beyond the base
# image (golangci-lint and gremlins), primes the Go module cache, and builds
# the `arm` binary. Mirrors the tool set pinned in .github/workflows/ci.yml.
#
# Runs after the repository is checked out. Must be safe to run repeatedly.
set -euo pipefail

cd "$(dirname "$0")/.."

GOBIN_DIR="$(go env GOPATH)/bin"
export PATH="${GOBIN_DIR}:${PATH}"

# gremlins is pinned to the version CI uses; golangci-lint tracks CI's @latest.
GREMLINS_VERSION="v0.6.0"
GOLANGCI_LINT_VERSION="${GOLANGCI_LINT_VERSION:-latest}"

if ! command -v golangci-lint >/dev/null 2>&1; then
	echo "Installing golangci-lint@${GOLANGCI_LINT_VERSION}..."
	go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}"
else
	echo "golangci-lint already present: $(golangci-lint version 2>/dev/null | head -1)"
fi

if ! command -v gremlins >/dev/null 2>&1; then
	echo "Installing gremlins@${GREMLINS_VERSION}..."
	go install "github.com/go-gremlins/gremlins/cmd/gremlins@${GREMLINS_VERSION}"
else
	echo "gremlins already present"
fi

# Expose the Go tool binaries on PATH for every interactive shell without
# mutating a shell profile: symlink them into a directory already on PATH.
# /usr/local/bin is world-readable and on the default PATH.
if [ -w /usr/local/bin ] || sudo -n true 2>/dev/null; then
	for tool in golangci-lint gremlins; do
		src="${GOBIN_DIR}/${tool}"
		dst="/usr/local/bin/${tool}"
		if [ -x "${src}" ] && [ ! -e "${dst}" ]; then
			if [ -w /usr/local/bin ]; then
				ln -sf "${src}" "${dst}"
			else
				sudo ln -sf "${src}" "${dst}"
			fi
		fi
	done
fi

echo "Downloading Go modules..."
go mod download

echo "Building arm binary..."
make build

# Reconcile git commit signing so `make check`'s git-shelling tests match a
# clean CI environment. Also invoked per-boot by start.sh; running it here
# covers non-build environments where install runs during agent provisioning
# (after Cursor's git setup) and start-only builds alike. See the script for
# the full rationale.
./.cursor/reconcile-git-signing.sh "$(pwd)"

echo "Install complete: $(./bin/arm version)"
