#!/usr/bin/env bash
# Fixture test for scripts/select-coverage-check-artifact-id.sh (fail-closed).
#
# REQ:
#   exactly-one match → exit 0 + numeric id
#   empty / expired / wrong-field-only → exit 2
#   multiple / {} / malformed / null id → exit 1 FAIL
#   every filter clause is load-bearing: neutralize each in a script copy and
#   assert a single-field-wrong fixture flips (oracle rc 2 → mutant rc 0)
#   edit workflow: no || true / set +e on any run: line; uses wait script

set -euo pipefail

REPO_ROOT="${1:-.}"
SCRIPT="$REPO_ROOT/scripts/select-coverage-check-artifact-id.sh"
EDIT_WF="$REPO_ROOT/.github/workflows/pr-coverage-quotes-on-edit.yml"

FAILURES=0
WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

run_sel() {
	local script="$1"
	local json="$2"
	shift 2
	set +e
	OUT=$("$script" "$@" "$json" 2>"$WORKDIR/err.txt")
	ST=$?
	ERR=$(cat "$WORKDIR/err.txt")
	set -e
}

pass() { echo "  PASS${1:+ ($1)}"; }
fail() { echo "  FAIL: $1"; echo "  out='$OUT' st=$ST err='$ERR'"; FAILURES=$((FAILURES + 1)); }

GOOD="$WORKDIR/good.json"
cat >"$GOOD" <<'EOF'
{
  "artifacts": [
    {
      "id": 333,
      "name": "coverage-check-deadbeef",
      "expired": false,
      "workflow_run": {
        "head_sha": "deadbeef",
        "repository_id": 42,
        "head_repository_id": 42
      }
    }
  ]
}
EOF

echo "Test 1: exactly one match → id..."
run_sel "$SCRIPT" "$GOOD" deadbeef 42
if [[ $ST -ne 0 || "$OUT" != "333" ]]; then fail "expected exit 0 id=333"
else pass; fi

echo "Test 2: empty artifacts → exit 2..."
echo '{"artifacts":[]}' >"$WORKDIR/empty.json"
run_sel "$SCRIPT" "$WORKDIR/empty.json" deadbeef 42
if [[ $ST -ne 2 ]]; then fail "expected exit 2 for empty"
else pass; fi

echo "Test 3: {} missing .artifacts → exit 1..."
echo '{}' >"$WORKDIR/brace.json"
run_sel "$SCRIPT" "$WORKDIR/brace.json" deadbeef 42
if [[ $ST -ne 1 ]] || ! grep -q '^FAIL:' <<<"$ERR"; then fail "expected FAIL exit 1 for {}"
else pass; fi

echo "Test 4: multiple matches → exit 1..."
cat >"$WORKDIR/multi.json" <<'EOF'
{
  "artifacts": [
    {"id":1,"expired":false,"workflow_run":{"head_sha":"deadbeef","repository_id":42,"head_repository_id":42}},
    {"id":2,"expired":false,"workflow_run":{"head_sha":"deadbeef","repository_id":42,"head_repository_id":42}}
  ]
}
EOF
run_sel "$SCRIPT" "$WORKDIR/multi.json" deadbeef 42
if [[ $ST -ne 1 ]] || ! grep -q 'exactly one' <<<"$ERR"; then fail "expected exactly-one FAIL"
else pass; fi

echo "Test 5: null / non-numeric id → exit 1..."
cat >"$WORKDIR/nullid.json" <<'EOF'
{
  "artifacts": [
    {"id":null,"expired":false,"workflow_run":{"head_sha":"deadbeef","repository_id":42,"head_repository_id":42}}
  ]
}
EOF
run_sel "$SCRIPT" "$WORKDIR/nullid.json" deadbeef 42
if [[ $ST -ne 1 ]] || ! grep -q 'FAIL: artifact id must be a numeric' <<<"$ERR"; then
	fail "expected numeric-id FAIL for null id"
else pass "null id"; fi

cat >"$WORKDIR/strid.json" <<'EOF'
{
  "artifacts": [
    {"id":"abc","expired":false,"workflow_run":{"head_sha":"deadbeef","repository_id":42,"head_repository_id":42}}
  ]
}
EOF
run_sel "$SCRIPT" "$WORKDIR/strid.json" deadbeef 42
if [[ $ST -ne 1 ]] || ! grep -q 'FAIL: artifact id must be a numeric' <<<"$ERR"; then
	fail "expected numeric-id FAIL for string id"
else pass "string id"; fi

echo "Test 6: malformed JSON → exit 1..."
echo 'not-json{' >"$WORKDIR/bad.json"
run_sel "$SCRIPT" "$WORKDIR/bad.json" deadbeef 42
if [[ $ST -ne 1 ]] || ! grep -q '^FAIL: malformed' <<<"$ERR"; then fail "expected malformed FAIL"
else pass; fi

echo "Test 7: each filter clause is load-bearing (neutralize → fixture flips)..."
set +e
CLAUSE_OUT=$(python3 - "$SCRIPT" "$WORKDIR" <<'PY'
import json, os, re, subprocess, sys

script, work = sys.argv[1], sys.argv[2]
src = open(script).read()

m = re.search(
    r"\| select\(\n(?P<body>(?:.*\n)*?)\t\t\t\)\n",
    src,
)
if not m:
    print("FAIL: could not find select(…) filter body in selector")
    sys.exit(1)
body = m.group("body")
raw_lines = [ln for ln in body.splitlines() if ln.strip()]
clauses = []
buf = []
for ln in raw_lines:
    stripped = ln.strip()
    if stripped.startswith("and ") and buf:
        clauses.append("\n".join(buf))
        buf = [ln]
    else:
        buf.append(ln)
if buf:
    clauses.append("\n".join(buf))
if len(clauses) < 4:
    print(f"FAIL: expected ≥4 filter clauses, got {len(clauses)}: {clauses!r}")
    sys.exit(1)

def write_fixture(name, **wrong):
    art = {
        "id": 333,
        "name": "coverage-check-deadbeef",
        "expired": False,
        "workflow_run": {
            "head_sha": "deadbeef",
            "repository_id": 42,
            "head_repository_id": 42,
        },
    }
    for k, v in wrong.items():
        if k == "expired":
            art["expired"] = v
        else:
            art["workflow_run"][k] = v
    path = os.path.join(work, name)
    with open(path, "w") as f:
        json.dump({"artifacts": [art]}, f)
    return path

fixtures = []
for clause in clauses:
    c = clause.strip()
    if "expired" in c and "not" in c:
        fixtures.append((clause, write_fixture("only_expired.json", expired=True), "expired"))
    elif "head_sha" in c:
        fixtures.append((clause, write_fixture("only_wrong_sha.json", head_sha="cafebabe"), "head_sha"))
    elif "head_repository_id" in c:
        fixtures.append((clause, write_fixture("only_fork.json", head_repository_id=99), "head_repository_id"))
    elif "repository_id" in c:
        fixtures.append((clause, write_fixture("only_wrong_repo.json", repository_id=7), "repository_id"))
    else:
        print(f"FAIL: unrecognized filter clause: {c}")
        sys.exit(1)

failures = 0
for clause, fix, label in fixtures:
    r = subprocess.run([script, "deadbeef", "42", fix], capture_output=True, text=True)
    if r.returncode != 2:
        print(f"FAIL: oracle for {label}-only-wrong expected rc 2, got {r.returncode}")
        failures += 1
        continue
    mut_path = os.path.join(work, f"mut_{label}.sh")
    if clause.strip().startswith("and "):
        replacement = "\t\t\t\tand true"
    else:
        replacement = "\t\t\t\ttrue"
    if clause not in src:
        print(f"FAIL: clause text not found in script for {label}")
        failures += 1
        continue
    mut = src.replace(clause, replacement, 1)
    open(mut_path, "w").write(mut)
    os.chmod(mut_path, 0o755)
    mr = subprocess.run([mut_path, "deadbeef", "42", fix], capture_output=True, text=True)
    if mr.returncode != 0:
        print(f"FAIL: neutralized {label} should flip to rc 0, got {mr.returncode} err={mr.stderr!r}")
        failures += 1
        continue
    print(f"PASS ({label}: oracle rc2 → neutralized rc0)")

sys.exit(1 if failures else 0)
PY
)
CLAUSE_ST=$?
set -e
if [[ $CLAUSE_ST -ne 0 ]]; then
	echo "$CLAUSE_OUT"
	FAILURES=$((FAILURES + 1))
else
	echo "$CLAUSE_OUT" | sed 's/^/  /'
fi

echo "Test 8: edit workflow — no || true / set +e / continue-on-error / shell: overrides..."
if [[ ! -f "$EDIT_WF" ]]; then
	fail "missing edit workflow"
else
	set +e
	WF_OUT=$(python3 - "$EDIT_WF" <<'PY'
import re, sys
text = open(sys.argv[1]).read()
if "wait-coverage-check-artifact.sh" not in text:
    print("FAIL: edit workflow must call wait-coverage-check-artifact.sh")
    sys.exit(1)
lines = text.splitlines()
problems = []
# Step-level keys under jobs.*.steps[] — reject continue-on-error / shell: overrides.
for i, line in enumerate(lines):
    if re.match(r"^\s+continue-on-error\s*:", line):
        problems.append(f"step continue-on-error at line {i+1}")
    if re.match(r"^\s+shell\s*:", line):
        problems.append(f"step shell: override at line {i+1}")
i = 0
while i < len(lines):
    line = lines[i]
    m = re.match(r"^(\s*)run:\s*(.*)$", line)
    if not m:
        i += 1
        continue
    indent, rest = m.group(1), m.group(2)
    block = []
    if rest in ("|", ">-", ">", "|-"):
        i += 1
        while i < len(lines):
            ln = lines[i]
            if not ln.strip():
                block.append(ln)
                i += 1
                continue
            if ln.startswith(indent + "  ") or ln.startswith(indent + "\t"):
                block.append(ln)
                i += 1
                continue
            break
    else:
        block = [rest]
        i += 1
    blob = "\n".join(block)
    if re.search(r"\|\|\s*true\b", blob):
        problems.append("run: block contains || true")
    if re.search(r"(^|[;\n])\s*set\s+\+e\b", blob):
        problems.append("run: block contains set +e")
if problems:
    print("FAIL: " + "; ".join(sorted(set(problems))))
    sys.exit(1)
print("PASS")
PY
)
	WF_ST=$?
	set -e
	if [[ $WF_ST -ne 0 ]]; then
		echo "  $WF_OUT"
		FAILURES=$((FAILURES + 1))
	else
		pass
	fi
fi

echo ""
if [[ $FAILURES -eq 0 ]]; then
	echo "All select-coverage-check-artifact-id tests passed"
	exit 0
else
	echo "FAIL: $FAILURES select-coverage-check-artifact-id test(s) failed"
	exit 1
fi
