# Batch Strategy (Advanced)

When a task involves a large number of files (e.g. refactoring 10+ files), do not
attempt to process them all in a single turn. This leads to incomplete work and
high token usage. Instead:

1. **Build a Manifest:** Use `grep --names-only` or `glob` to find all files that
   need changes. Save this list to a temporary file or a note.
2. **Process in Chunks:** Process the files in small batches (e.g. 3-5 files at a
   time).
3. **Verify each Chunk:** Run tests/linting after each chunk to ensure no
   regressions were introduced.
4. **Heartbeat:** Call `arm heartbeat ID` after each chunk.
5. **Final Review:** Once all files are processed, run a final global check
   before transitioning the task to `done`.
