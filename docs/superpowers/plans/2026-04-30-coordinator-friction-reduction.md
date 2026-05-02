# Coordinator Friction Reduction Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce coordinator friction discovered during the RP story: noisy `arm validate` output, invisible `arm ready` exclusion reasons, the auditor missing `merged` tasks, and agents reflexively reaching for Python to parse CLI output.

**Architecture:** Three targeted tool additions (`--quiet` on validate, `--terminal` on list, `--explain` on ready) plus focused skill edits to the coordinator and auditor SKILL.md files. No new packages — all changes extend existing commands in `cmd/armature/`.

**Tech Stack:** Go, cobra, existing `internal/ops`, `internal/validate`, `internal/ready`, `internal/materialize` packages. SKILL.md edits are plain markdown.

---

## Chunk 1: `arm validate --quiet`

Suppress `INFO:` phantom-scope lines by default. Today every `arm validate` run emits 100–150 `INFO:` lines from historical renamed files, burying the `COVERAGE:` and `OK:` lines agents actually need.

**Files:**
- Modify: `cmd/armature/validate.go`
- Modify: `cmd/armature/cmd_extra_test.go` (existing validate tests live here)

### Task 1: `arm validate --quiet` flag

- [ ] **Step 1: Write the failing test**

  In `cmd/armature/cmd_extra_test.go`, add a test that runs `arm validate --quiet` and asserts `INFO:` lines are absent while `COVERAGE:` and `OK:` lines are present. Find the existing validate test block to place it near (search for `TestValidate` or `"validate"`).

  ```go
  func TestValidateQuiet(t *testing.T) {
      // Use any repo fixture that produces INFO: phantom scope lines.
      // The existing validate test setup already has one — reuse it.
      out := runCmd(t, "validate", "--quiet")
      if strings.Contains(out, "INFO:") {
          t.Fatalf("--quiet should suppress INFO lines, got:\n%s", out)
      }
      if !strings.Contains(out, "COVERAGE:") {
          t.Fatalf("--quiet should still print COVERAGE line, got:\n%s", out)
      }
  }
  ```

- [ ] **Step 2: Run to confirm it fails**

  ```bash
  go test ./cmd/armature/... -run TestValidateQuiet -v
  ```
  Expected: `FAIL` — flag doesn't exist yet.

- [ ] **Step 3: Add `--quiet` flag to `cmd/armature/validate.go`**

  Add `quiet bool` to the vars block and wire it to the flag:
  ```go
  var (
      ci     bool
      strict bool
      scope  string
      quiet  bool   // add this
  )
  ```

  In the text output block, wrap the `INFO:` loop:
  ```go
  if !quiet {
      for _, i := range result.Infos {
          _, _ = fmt.Fprintf(cmd.OutOrStdout(), "INFO: %s\n", i)
      }
  }
  ```

  Register the flag at the bottom:
  ```go
  cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress INFO lines; show only ERRORs, WARNINGs, and COVERAGE")
  ```

- [ ] **Step 4: Run to confirm it passes**

  ```bash
  go test ./cmd/armature/... -run TestValidateQuiet -v
  ```
  Expected: `PASS`.

- [ ] **Step 5: Run full check**

  ```bash
  make check
  ```
  Expected: all stages green.

- [ ] **Step 6: Commit**

  ```bash
  git add cmd/armature/validate.go cmd/armature/cmd_extra_test.go
  git commit -m "feat(validate): add --quiet flag to suppress INFO phantom-scope lines"
  ```

---

## Chunk 2: `arm list --terminal`

Add a `--terminal` flag to `arm list` that filters to all finished statuses (`done`, `merged`, `cancelled`) at once. Fixes the auditor Step 3 gap where `--status done` silently missed tasks that workers transitioned to `merged`.

**Files:**
- Modify: `cmd/armature/list.go`
- Modify: `cmd/armature/cmd_extra_test.go`

### Task 2: `arm list --terminal` flag

- [ ] **Step 1: Write the failing test**

  Add a test that creates issues with mixed statuses (done, merged, open) and confirms `--terminal` returns done and merged but not open.

  ```go
  func TestListTerminal(t *testing.T) {
      // Assumes a fixture with at least one "done", one "merged", one "open" issue.
      // Reuse the existing list-test fixture or extend it.
      out := runCmd(t, "list", "--terminal", "--format", "json")
      var entries []map[string]interface{}
      if err := json.Unmarshal([]byte(out), &entries); err != nil {
          t.Fatalf("parse json: %v\n%s", err, out)
      }
      for _, e := range entries {
          status := e["status"].(string)
          if status != "done" && status != "merged" && status != "cancelled" {
              t.Fatalf("--terminal returned non-terminal status %q", status)
          }
      }
      // Confirm at least one merged entry is present (not silently empty)
      var hasMerged bool
      for _, e := range entries {
          if e["status"].(string) == "merged" {
              hasMerged = true
          }
      }
      if !hasMerged {
          t.Fatal("--terminal should include merged issues but found none")
      }
  }
  ```

- [ ] **Step 2: Run to confirm it fails**

  ```bash
  go test ./cmd/armature/... -run TestListTerminal -v
  ```
  Expected: `FAIL` — flag doesn't exist yet.

- [ ] **Step 3: Add `--terminal` flag to `cmd/armature/list.go`**

  Add `terminal bool` to the flag vars block:
  ```go
  var filterParent string
  var filterType string
  var filterStatus string
  var terminal bool   // add this
  var group bool
  ```

  In the filter loop, after the existing `filterStatus` check, add:
  ```go
  if terminal {
      s := entry.Status
      if s != ops.StatusDone && s != ops.StatusMerged && s != ops.StatusCancelled {
          continue
      }
  }
  ```

  Note: `terminal` and `filterStatus` are independent — `filterStatus` still works alone for single-status queries.

  Register the flag at the bottom:
  ```go
  cmd.Flags().BoolVar(&terminal, "terminal", false, "Filter to all terminal statuses: done, merged, cancelled")
  ```

- [ ] **Step 4: Run to confirm it passes**

  ```bash
  go test ./cmd/armature/... -run TestListTerminal -v
  ```
  Expected: `PASS`.

- [ ] **Step 5: Run full check**

  ```bash
  make check
  ```
  Expected: all stages green.

- [ ] **Step 6: Commit**

  ```bash
  git add cmd/armature/list.go cmd/armature/cmd_extra_test.go
  git commit -m "feat(list): add --terminal flag to filter done/merged/cancelled at once"
  ```

---

## Chunk 3: `arm ready --explain`

Add `--explain` to `arm ready` that lists every `open` unclaimed task that is NOT in the ready queue, and states exactly which gate excluded it. Eliminates the invisible exclusion problem that forced direct `arm claim` without understanding why `arm ready` was silent.

**Files:**
- Modify: `cmd/armature/ready.go`
- Modify: `internal/ready/compute.go`
- Modify: `internal/ready/compute_test.go` (or `cmd/armature/cmd_extra_test.go`)

### Task 3: `ExplainNotReady` in `internal/ready/compute.go`

- [ ] **Step 1: Write the failing test**

  In `internal/ready/compute_test.go`, add a test for a new `ExplainNotReady` function. It should take the same index and issues map as `ComputeReady` and return a map of issue ID → reason string for every open unclaimed task that is excluded.

  ```go
  func TestExplainNotReady_BlockerNotMerged(t *testing.T) {
      index := materialize.Index{
          "T1": {Type: "task", Status: "open", BlockedBy: []string{"T0"}},
          "T0": {Type: "task", Status: "done"},  // done, not merged
      }
      issues := map[string]*materialize.Issue{}
      explanations := ExplainNotReady(index, issues, 0)
      reason, ok := explanations["T1"]
      if !ok {
          t.Fatal("T1 should have an explanation")
      }
      if !strings.Contains(reason, "T0") {
          t.Errorf("reason should mention blocker T0, got: %s", reason)
      }
  }

  func TestExplainNotReady_ParentNotActive(t *testing.T) {
      index := materialize.Index{
          "S1":   {Type: "story", Status: "done"},
          "T1":   {Type: "task", Status: "open", Parent: "S1"},
      }
      issues := map[string]*materialize.Issue{}
      explanations := ExplainNotReady(index, issues, 0)
      reason, ok := explanations["T1"]
      if !ok {
          t.Fatal("T1 should have an explanation")
      }
      if !strings.Contains(strings.ToLower(reason), "parent") {
          t.Errorf("reason should mention parent, got: %s", reason)
      }
  }
  ```

- [ ] **Step 2: Run to confirm it fails**

  ```bash
  go test ./internal/ready/... -run TestExplainNotReady -v
  ```
  Expected: `FAIL` — function doesn't exist yet.

- [ ] **Step 3: Implement `ExplainNotReady` in `internal/ready/compute.go`**

  Add after `ComputeReady`:

  ```go
  // ExplainNotReady returns a map of issue ID → reason for every open, unclaimed
  // task/story/feature that is NOT in the ready queue. Useful for debugging why
  // arm ready returns fewer results than expected.
  func ExplainNotReady(index materialize.Index, issues map[string]*materialize.Issue, nowEpoch int64) map[string]string {
      if nowEpoch == 0 {
          nowEpoch = time.Now().Unix()
      }
      out := make(map[string]string)
      for id, entry := range index {
          if entry.Type != "task" && entry.Type != "feature" && entry.Type != "story" {
              continue
          }
          if entry.Status != ops.StatusOpen {
              continue
          }
          issue := issues[id]
          if issue != nil && issue.ClaimedBy != "" {
              if !isClaimStale(issue.ClaimedAt, issue.LastHeartbeat, issue.ClaimTTL, nowEpoch) {
                  out[id] = fmt.Sprintf("actively claimed by %s", issue.ClaimedBy)
                  continue
              }
          }
          if issue != nil && issue.Provenance.Confidence == "draft" {
              out[id] = "draft provenance — requires confirmation before claiming"
              continue
          }
          if !allBlockersMerged(entry.BlockedBy, index) {
              var unmet []string
              for _, bid := range entry.BlockedBy {
                  e, ok := index[bid]
                  if !ok || e.Status != ops.StatusMerged {
                      unmet = append(unmet, fmt.Sprintf("%s (status: %s)", bid, func() string {
                          if !ok { return "not found" }
                          return e.Status
                      }()))
                  }
              }
              out[id] = fmt.Sprintf("waiting on blockers: %s", strings.Join(unmet, ", "))
              continue
          }
          if entry.Parent != "" {
              parentEntry, ok := index[entry.Parent]
              if !ok {
                  out[id] = fmt.Sprintf("parent %s not found in graph", entry.Parent)
                  continue
              }
              s := parentEntry.Status
              if s != ops.StatusInProgress && s != ops.StatusClaimed && s != ops.StatusOpen {
                  out[id] = fmt.Sprintf("parent %s has status %q (need open/claimed/in-progress)", entry.Parent, s)
                  continue
              }
          }
          // Passes all gates — should be in ready queue (no explanation needed)
      }
      return out
  }
  ```

  Add `"fmt"` and `"strings"` to imports if not already present.

- [ ] **Step 4: Run to confirm it passes**

  ```bash
  go test ./internal/ready/... -run TestExplainNotReady -v
  ```
  Expected: `PASS`.

- [ ] **Step 5: Write the failing CLI test**

  In `cmd/armature/cmd_extra_test.go`:
  ```go
  func TestReadyExplain(t *testing.T) {
      // Use a fixture where at least one open task is blocked or has wrong parent state.
      out := runCmd(t, "ready", "--explain")
      // Should print issue IDs with reasons, not the normal ready queue
      if strings.Contains(out, "No tasks ready") {
          t.Skip("fixture has no explainable tasks")
      }
      // Output should contain at least one ":" separator (id: reason format)
      if !strings.Contains(out, ":") {
          t.Errorf("--explain output should be 'ID: reason' pairs, got:\n%s", out)
      }
  }
  ```

- [ ] **Step 6: Add `--explain` flag to `cmd/armature/ready.go`**

  Add `explain bool` to the flag vars and cobra flags:
  ```go
  var explain bool
  // ...
  cmd.Flags().BoolVar(&explain, "explain", false, "Show why open tasks are not in the ready queue")
  ```

  Add the explain path at the top of `RunE`, before the existing format/TUI branches:
  ```go
  if explain {
      explanations := ready.ExplainNotReady(index, issues, 0)
      if len(explanations) == 0 {
          _, _ = fmt.Fprintln(cmd.OutOrStdout(), "All open tasks are either ready or actively claimed.")
          return nil
      }
      // Sort for deterministic output
      ids := make([]string, 0, len(explanations))
      for id := range explanations {
          ids = append(ids, id)
      }
      sort.Strings(ids)
      for _, id := range ids {
          _, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", id, explanations[id])
      }
      return nil
  }
  ```

- [ ] **Step 7: Run to confirm CLI test passes**

  ```bash
  go test ./cmd/armature/... -run TestReadyExplain -v
  ```
  Expected: `PASS` (or `SKIP` if fixture has no blocked tasks — that's fine).

- [ ] **Step 8: Run full check**

  ```bash
  make check
  ```
  Expected: all stages green.

- [ ] **Step 9: Commit**

  ```bash
  git add internal/ready/compute.go internal/ready/compute_test.go \
          cmd/armature/ready.go cmd/armature/cmd_extra_test.go
  git commit -m "feat(ready): add --explain flag to diagnose why open tasks are not surfacing"
  ```

---

## Chunk 4: Skill edits

No code — pure markdown edits to the coordinator and auditor skills. Run `make skill` after each file edit to deploy to `.claude/skills/` and `.gemini/skills/`.

**Files:**
- Modify: `skills/armature-coordinator/SKILL.md`
- Modify: `skills/armature-auditor/SKILL.md`

### Task 4: Auditor SKILL.md — fix Step 3 to catch `merged` tasks

The current Step 3 says `arm list --status done --parent STORY-ID`. Tasks workers transition to `merged` are silently excluded. Fix it to use either `--terminal` (once Chunk 2 ships) or the unfiltered form.

- [ ] **Step 1: Edit `skills/armature-auditor/SKILL.md` Step 3**

  Find:
  ```
  arm list --status done --parent STORY-ID
  ```
  Replace with:
  ```
  arm list --terminal --parent STORY-ID
  ```
  And update the surrounding prose to note that `--terminal` captures `done`, `merged`, and `cancelled` — the full set of finished states regardless of which transition target the worker chose.

- [ ] **Step 2: Deploy and verify**

  ```bash
  make skill
  grep -n "terminal" .claude/skills/armature-auditor/SKILL.md
  ```
  Expected: line with `--terminal` present.

- [ ] **Step 3: Commit**

  ```bash
  git add skills/armature-auditor/SKILL.md .claude/skills/ .gemini/skills/
  git commit -m "docs(auditor): use --terminal in Step 3 to catch merged tasks"
  ```

### Task 5: Coordinator SKILL.md — worker skill invocation, `arm ready` gaps, `--explain`, grep idioms, CI tag note

Four improvements to the coordinator skill:

**a. Worker skill invocation — close the dispatch loop**

The coordinator's dispatch protocol sends log slot, render-context, branch, and commit instructions — but never tells the worker to invoke the armature-worker skill. Without it workers improvise: wrong transition targets (`merged` instead of `done`), skipped heartbeats, missed citation steps. The skill is the canonical source; the dispatcher should delegate to it rather than re-state a subset of its rules.

- [ ] **Step 1: Add skill invocation as item 0 of the Dispatch Protocol**

  In the `## Dispatch Protocol` section, make the following the first numbered item (before the log slot instruction):

  ```markdown
  0. **Invoke the worker skill (FIRST instruction):**
     ```
     Invoke the armature-worker skill before doing anything else: Skill("armature-worker")
     ```
     This must appear before all other instructions. The skill establishes transition
     targets, heartbeat cadence, citation requirements, commit format, and the
     dual-branch `.armature/` rule. Do not repeat those rules in the dispatch prompt —
     the skill is the authoritative source.
  ```

  Renumber the existing items 1–6 to 2–7.

**b. `arm ready` may not surface all open tasks — diagnose with `--explain`**

- [ ] **Step 2: Add a diagnostic note to the "Find Ready Work" section**

  After the existing `arm ready` examples, add:

  ```markdown
  If `arm ready` returns fewer tasks than expected (e.g. you know open tasks exist
  but they don't appear), run:
  ```bash
  arm ready --explain
  ```
  This lists every open unclaimed task that was excluded and the gate that blocked it
  (unmet blocker, parent in wrong state, active claim, draft provenance). Use
  `arm claim --issue TASK-ID` directly only after confirming the exclusion is safe to
  bypass (e.g. you are intentionally claiming out of dependency order).
  ```

**b. Grep idioms for common JSON queries**

- [ ] **Step 3: Add a "Querying JSON Output" section to the Command Reference**

  ```markdown
  ## Querying JSON Output

  `arm list` and `arm show` output is regular JSON. Prefer `grep` over Python/jq
  for simple field extraction — the output is predictable enough that string matching
  works reliably.

  ```bash
  # Find all epics
  arm list | grep -A3 '"type": "epic"'

  # Find all tasks under a story, show status
  arm list --parent RP | grep -E '"id"|"status"'

  # Check a specific task's outcome
  arm show RP-T1 | grep "Outcome"

  # Confirm all tasks under a story are in a terminal state
  arm list --terminal --parent RP | grep '"id"'
  # (if count matches arm list --parent RP | grep '"id"', all are done/merged/cancelled)

  # Suppress INFO noise from arm validate
  arm validate --quiet
  ```
  ```

**c. CI workflow tag-push note**

- [ ] **Step 4: Add a note to the "Story Completion" → "Push and open PR" section**

  After the PR instructions, add:

  ```markdown
  **CI workflow hygiene:** If this story adds or modifies a GitHub Actions CI workflow,
  verify its `push:` trigger includes a `branches:` filter so it does not fire on tag
  pushes (which should only trigger the release workflow):
  ```yaml
  on:
    push:
      branches:
        - '**'   # branches only — not tags
    pull_request:
  ```
  A CI workflow with bare `push:` will run on every tag push, redundantly alongside
  any release workflow.
  ```

- [ ] **Step 5: Deploy and verify**

  ```bash
  make skill
  grep -n "armature-worker\|explain\|Querying\|branches:" .claude/skills/armature-coordinator/SKILL.md | head -20
  ```
  Expected: all four additions visible.

- [ ] **Step 6: Run full check**

  ```bash
  make check
  ```
  Expected: green (skill changes don't affect Go tests, but `make skill` is part of `make check`).

- [ ] **Step 7: Commit**

  ```bash
  git add skills/armature-coordinator/SKILL.md .claude/skills/ .gemini/skills/
  git commit -m "docs(coordinator): invoke worker skill at dispatch, add --explain tip, grep idioms, CI tag-push note"
  ```

---

## Completion Checklist

- [ ] `arm validate --quiet` suppresses INFO lines, COVERAGE still prints
- [ ] `arm list --terminal` returns done + merged + cancelled, not open
- [ ] `arm ready --explain` outputs `ID: reason` for each excluded open task
- [ ] Auditor SKILL.md Step 3 uses `--terminal` 
- [ ] Coordinator SKILL.md dispatch protocol item 0 invokes the armature-worker skill
- [ ] Coordinator SKILL.md has `--explain` tip, grep idioms, CI tag-push note
- [ ] `make check` green on final commit
