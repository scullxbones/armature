#!/usr/bin/env bash
# Per-boot reconciliation for Armature Cloud Agents.
#
# The Cloud Agent VM ships a managed global ~/.gitconfig that enables SSH
# commit signing (gpg.format=ssh, commit.gpgsign=true) so the agent's own
# commits are signed. Armature's unit suite shells out to real git and one
# test (cmd/armature TestMigrateLegacySingleBranchOpsRollsBackOnCommitFailure_P1)
# forces a commit *failure* by pointing gpg.program (the OpenPGP signer) at a
# nonexistent binary. Under the managed SSH-signing config that override is
# ignored, git signs successfully, and the test fails only inside the VM
# (CI runs with a clean git config where the OpenPGP path is used).
#
# Reconcile by preserving the managed signing settings on the working repo
# (so the agent's commits stay signed and pushable) while removing them from
# the global scope, so git subprocesses spawned by the test suite behave like
# a clean CI environment. Idempotent and safe to run on every boot.
set -euo pipefail

REPO_DIR="${CURSOR_WORKSPACE:-/workspace}"

reconcile_repo_signing() {
	local repo="$1"
	git -C "$repo" rev-parse --git-dir >/dev/null 2>&1 || return 0

	local fmt key sshprog sign
	fmt="$(git config --global --get gpg.format 2>/dev/null || true)"
	key="$(git config --global --get user.signingkey 2>/dev/null || true)"
	sshprog="$(git config --global --get gpg.ssh.program 2>/dev/null || true)"
	sign="$(git config --global --get commit.gpgsign 2>/dev/null || true)"

	[ -n "$fmt" ] && git -C "$repo" config --local gpg.format "$fmt"
	[ -n "$key" ] && git -C "$repo" config --local user.signingkey "$key"
	[ -n "$sshprog" ] && git -C "$repo" config --local gpg.ssh.program "$sshprog"
	[ -n "$sign" ] && git -C "$repo" config --local commit.gpgsign "$sign"
}

reconcile_repo_signing "$REPO_DIR"

# Neutralize signing globally so test-spawned git subprocesses match CI.
# Leaves the managed URL rewrite (github auth) untouched.
git config --global --unset-all gpg.format 2>/dev/null || true
git config --global --unset-all gpg.ssh.program 2>/dev/null || true
git config --global commit.gpgsign false 2>/dev/null || true

echo "Git signing reconciled: global signing disabled for clean test runs, ${REPO_DIR} keeps SSH signing."
