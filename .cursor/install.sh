#!/usr/bin/env bash
# Idempotent repository bootstrap for Armature Cloud Agents.
#
# Installs the two developer tools that `make check` needs beyond the base
# image (golangci-lint and gremlins), primes the Go module cache, and builds
# the `arm` binary. Pins live in the Makefile (`GOLANGCI_LINT_VERSION`,
# `GREMLINS_VERSION`); `make install-check-tools` is the install path.
#
# Runs after the repository is checked out. Must be safe to run repeatedly.
set -euo pipefail

cd "$(dirname "$0")/.."

# go install writes to GOBIN when set; GOPATH/bin is only the default.
GOBIN_DIR="$(go env GOBIN)"
if [ -z "${GOBIN_DIR}" ]; then
	GOBIN_DIR="$(go env GOPATH)/bin"
fi
export PATH="${GOBIN_DIR}:${PATH}"

# Pins live in the Makefile (GOLANGCI_LINT_VERSION / GREMLINS_VERSION).
make install-check-tools

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
