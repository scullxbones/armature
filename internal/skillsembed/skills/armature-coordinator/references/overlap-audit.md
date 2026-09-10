# Parallel Branch Overlap Audit

Run after every parallel wave, even when `arm ready --waves` planned the batch (ADR 0012). File-level disjointness does not catch shared-contract drift.

Task branches are not yet merged into the story `HEAD` here. Merge is step (b) in `SKILL.md`.

```bash
declare -A TASK_FILES
for TASK_ID in $WAVE_TASK_IDS; do
  TASK_BASE="${TASK_COMMITS[$TASK_ID]%%\.\.*}"
  TASK_HEAD="${TASK_COMMITS[$TASK_ID]##*\.\.}"
  TASK_FILES["$TASK_ID"]=$(git diff --name-only "$TASK_BASE".."$TASK_HEAD")
done

OVERLAPPING_FILES=""
ALL_CHANGED_FILES=$(for TASK_ID in $WAVE_TASK_IDS; do echo "${TASK_FILES[$TASK_ID]}"; done | sort -u)
for FILE in $ALL_CHANGED_FILES; do
  TASK_COUNT=0
  for TASK_ID in $WAVE_TASK_IDS; do
    if echo "${TASK_FILES[$TASK_ID]}" | grep -q "^$FILE$"; then
      ((TASK_COUNT++))
    fi
  done
  if [ "$TASK_COUNT" -gt 1 ]; then
    OVERLAPPING_FILES="$OVERLAPPING_FILES $FILE"
  fi
done

if [ -n "$OVERLAPPING_FILES" ]; then
  echo "WARNING: Files modified by multiple parallel tasks in wave $WAVE_TASK_IDS:"
  echo "$OVERLAPPING_FILES" | tr ' ' '\n' | sort -u
fi
```

For each overlapping file, confirm diffs are additive, the combined effect keeps intended semantics, and tests cover the overlap. Contradictory edits (same flag set both ways) are high risk: escalate to reviewer dispatch with test evidence before `merged`.
