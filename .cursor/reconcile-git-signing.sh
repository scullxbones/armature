#!/usr/bin/env bash
# Reconcile git commit signing for Armature development.
#
# The Cloud Agent VM ships a managed global ~/.gitconfig that enables SSH
# commit signing (gpg.format=ssh, commit.gpgsign=true) so the agent's own
# commits are signed. Cursor re-injects this config on every boot/provision
# (its "configure-git" setup step), so it cannot be fixed once at build time.
#
# Armature's unit suite shells out to real git, and one test
# (cmd/armature TestMigrateLegacySingleBranchOpsRollsBackOnCommitFailure_P1)
# forces a commit *failure* by pointing gpg.program (the OpenPGP signer) at a
# nonexistent binary. Under the managed SSH-signing config that override is
# ignored, git signs successfully, and the test fails only inside the VM
# (GitHub CI runs with a clean git config where the OpenPGP path is used).
#
# Reconcile by preserving the managed signing settings on the working repo
# (so the agent's own commits stay signed and pushable) while removing them
# from the global scope, so git subprocesses spawned by the test suite behave
# like a clean CI environment. The managed GitHub URL-rewrite auth is left
# untouched. Idempotent and safe to run from both install and start, in any
# order relative to Cursor's git provisioning.
set -euo pipefail

REPO_DIR="${1:-${CURSOR_WORKSPACE:-/workspace}}"

if git -C "$REPO_DIR" rev-parse --git-dir >/dev/null 2>&1; then
	fmt="$(git config --global --get gpg.format 2>/dev/null || true)"
	key="$(git config --global --get user.signingkey 2>/dev/null || true)"
	sshprog="$(git config --global --get gpg.ssh.program 2>/dev/null || true)"
	sign="$(git config --global --get commit.gpgsign 2>/dev/null || true)"

	[ -n "$fmt" ] && git -C "$REPO_DIR" config --local gpg.format "$fmt"
	[ -n "$key" ] && git -C "$REPO_DIR" config --local user.signingkey "$key"
	[ -n "$sshprog" ] && git -C "$REPO_DIR" config --local gpg.ssh.program "$sshprog"
	[ -n "$sign" ] && git -C "$REPO_DIR" config --local commit.gpgsign "$sign"
fi

# Neutralize signing globally so test-spawned git subprocesses match CI.
# Leaves the managed URL rewrite (github auth) untouched.
git config --global --unset-all gpg.format 2>/dev/null || true
git config --global --unset-all gpg.ssh.program 2>/dev/null || true
git config --global commit.gpgsign false 2>/dev/null || true

echo "Git signing reconciled: global signing disabled for clean test runs; ${REPO_DIR} keeps SSH signing."
