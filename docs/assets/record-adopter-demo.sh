#!/usr/bin/env bash
# Record (or re-record) the TOPTIER-S8-T3 adopter demo.
# Usage (from repo root, with arm + asciinema on PATH):
#   ./docs/assets/record-adopter-demo.sh
#   ./docs/assets/record-adopter-demo.sh --record docs/assets/adopter-demo.cast
#
# The demo shows the paved road: bootstrap → sources → plan/apply →
# dag transition (draft→verified) → ready → two claim/work/done cycles.
# Escape-hatch create is intentionally not used.
# Scratch dirs under /tmp are left for inspection; delete manually if needed.

set -euo pipefail

RECORD_OUT=""
if [[ "${1:-}" == "--record" ]]; then
  RECORD_OUT="${2:?cast output path required}"
fi

ARM="${ARM:-$(command -v arm)}"
ASCIINEMA="${ASCIINEMA:-$(command -v asciinema || true)}"
if [[ -z "$ARM" ]]; then
  echo "arm not on PATH" >&2
  exit 1
fi

run_demo() {
  scratch="$(mktemp -d /tmp/arm-s8-demo.XXXXXX)"

  echo "==> scratch: $scratch"
  cd "$scratch"
  git init -q
  git config user.email "demo@example.com"
  git config user.name "Armature Demo"
  git config commit.gpgsign false
  mkdir -p docs
  cat > docs/requirements.md <<'EOF'
# Demo requirements

## R1 Greeting
Ship a greeting file for worker A.

## R2 Farewell
Ship a farewell file for worker B.
EOF
  git add docs/requirements.md
  git commit -qm "docs: seed requirements"
  # claim publishes ops to origin/_armature; give the scratch a local bare remote
  git init -q --bare "$scratch/origin.git"
  git remote add origin "$scratch/origin.git"
  git push -q -u origin HEAD:main
  git branch -M main

  echo "==> bootstrap + worker-init"
  "$ARM" bootstrap
  "$ARM" worker-init --id demo-s8 || true

  echo "==> sources (paved road; any requirements-like doc works)"
  "$ARM" sources add --url docs/requirements.md --type filesystem
  "$ARM" sources sync
  SOURCE_UUID=$("$ARM" sources verify | awk '/OK/{print $1; exit}')
  test -n "$SOURCE_UUID"
  echo "SOURCE_UUID=$SOURCE_UUID"

  echo "==> plan with citations (draft birth)"
  cat > plan.json <<EOF
{
  "version": 1,
  "title": "S8 adopter demo",
  "issues": [
    {
      "id": "DEMO-S1",
      "title": "Demo story — two workers",
      "type": "story",
      "scope": "docs/requirements.md",
      "priority": "high",
      "dod": "hello.txt and bye.txt exist with expected contents",
      "parent": "",
      "blocked_by": [],
      "acceptance": ["hello.txt exists", "bye.txt exists"],
      "source": "$SOURCE_UUID"
    },
    {
      "id": "DEMO-S1-T1",
      "title": "Worker A: write greeting",
      "type": "task",
      "scope": "hello.txt",
      "priority": "high",
      "dod": "hello.txt contains Hello",
      "parent": "DEMO-S1",
      "blocked_by": [],
      "acceptance": ["hello.txt exists"],
      "source": "$SOURCE_UUID"
    },
    {
      "id": "DEMO-S1-T2",
      "title": "Worker B: write farewell",
      "type": "task",
      "scope": "bye.txt",
      "priority": "high",
      "dod": "bye.txt contains Bye",
      "parent": "DEMO-S1",
      "blocked_by": [],
      "acceptance": ["bye.txt exists"],
      "source": "$SOURCE_UUID"
    }
  ]
}
EOF

  "$ARM" dag apply --plan plan.json
  echo "==> ready is empty while draft (draft ≠ ready)"
  "$ARM" ready || true
  echo "==> dag transition promotes draft → verified"
  "$ARM" dag transition --issue DEMO-S1
  "$ARM" dag transition --issue DEMO-S1-T1
  "$ARM" dag transition --issue DEMO-S1-T2
  echo "==> ready now lists both leaves"
  "$ARM" ready

  echo "==> coordinator claims worker A"
  "$ARM" claim DEMO-S1-T1 --worktree
  "$ARM" render-context DEMO-S1-T1 --format agent | head -c 400
  echo
  (
    cd .worktrees/DEMO-S1-T1
    printf 'Hello\n' > hello.txt
    git add hello.txt
    git commit -qm "feat(DEMO-S1-T1): add greeting"
    "$ARM" transition DEMO-S1-T1 --to done --outcome "Worker A wrote hello.txt" --force
  )

  echo "==> coordinator claims worker B"
  "$ARM" claim DEMO-S1-T2 --worktree
  (
    cd .worktrees/DEMO-S1-T2
    printf 'Bye\n' > bye.txt
    git add bye.txt
    git commit -qm "feat(DEMO-S1-T2): add farewell"
    "$ARM" transition DEMO-S1-T2 --to done --outcome "Worker B wrote bye.txt" --force
  )

  echo "==> story close (workers done; ready empty; optional story claim)"
  "$ARM" ready || true
  "$ARM" show DEMO-S1-T1 --field status
  "$ARM" show DEMO-S1-T2 --field status
  "$ARM" claim DEMO-S1 --worktree || true
  "$ARM" transition DEMO-S1 --to done --outcome "Both worker deliveries complete for adopter demo" --force --skip-delivery-gate || \
    echo "(story close skipped; both worker tasks are done)"

  echo "==> demo complete in $scratch"
}

if [[ -n "$RECORD_OUT" ]]; then
  if [[ -z "$ASCIINEMA" ]]; then
    echo "asciinema not found; install or set ASCIINEMA=" >&2
    exit 1
  fi
  mkdir -p "$(dirname "$RECORD_OUT")"
  # Recurse without --record so the cast captures the demo body only.
  "$ASCIINEMA" record --overwrite -c "$0" "$RECORD_OUT"
  echo "Wrote $RECORD_OUT"
else
  run_demo
fi
