#!/usr/bin/env bash
# Record arm tui DAG tree from a real armature checkout into an asciinema cast,
# then optionally render a GIF with agg (https://github.com/asciinema/agg).
#
# Usage (from repo root, with arm on PATH):
#   ./docs/assets/record-arm-tui-dag.sh
#   ./docs/assets/record-arm-tui-dag.sh --record docs/assets/arm-tui-dag.cast
#   ./docs/assets/record-arm-tui-dag.sh --record docs/assets/arm-tui-dag.cast --gif docs/assets/arm-tui-dag.gif
set -euo pipefail

RECORD_OUT="docs/assets/arm-tui-dag.cast"
GIF_OUT=""
REPO_ROOT=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --record)
      RECORD_OUT="${2:?cast output path required}"
      shift 2
      ;;
    --gif)
      GIF_OUT="${2:?gif output path required}"
      shift 2
      ;;
    --repo)
      REPO_ROOT="${2:?repo path required}"
      shift 2
      ;;
    -h|--help)
      sed -n '2,12p' "$0"
      exit 0
      ;;
    *)
      echo "unknown arg: $1" >&2
      exit 2
      ;;
  esac
done

ARM="${ARM:-$(command -v arm)}"
AGG="${AGG:-$(command -v agg || true)}"
if [[ -z "$ARM" ]]; then
  echo "arm not on PATH" >&2
  exit 1
fi

if [[ -z "$REPO_ROOT" ]]; then
  REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
fi

python3 - "$ARM" "$REPO_ROOT" "$RECORD_OUT" <<'PY'
import fcntl, json, os, pty, select, struct, sys, termios, time

arm, repo, cast_path = sys.argv[1], sys.argv[2], sys.argv[3]
cols, rows = 100, 30
os.chdir(repo)

pid, fd = pty.fork()
if pid == 0:
    os.environ["TERM"] = "xterm-256color"
    os.environ["COLORTERM"] = "truecolor"
    for k in ("CI", "GITHUB_ACTIONS"):
        os.environ.pop(k, None)
    winsize = struct.pack("HHHH", rows, cols, 0, 0)
    fcntl.ioctl(1, termios.TIOCSWINSZ, winsize)
    os.execvp(arm, [arm, "tui"])

winsize = struct.pack("HHHH", rows, cols, 0, 0)
try:
    fcntl.ioctl(fd, termios.TIOCSWINSZ, winsize)
except OSError:
    pass

header = {
    "version": 2,
    "width": cols,
    "height": rows,
    "timestamp": int(time.time()),
    "command": "arm tui",
    "title": "arm tui — DAG tree view",
    "env": {"TERM": "xterm-256color", "SHELL": os.environ.get("SHELL", "/bin/sh")},
}

wall0 = time.time()
pre = []
recording = []
started = None
phase = "warmup"
quit_sent = False
deadline = wall0 + 25

while time.time() < deadline:
    r, _, _ = select.select([fd], [], [], 0.05)
    if not r:
        if phase == "hold" and started and time.time() - started > 4 and not quit_sent:
            try:
                os.write(fd, b"q")
            except OSError:
                pass
            quit_sent = True
            deadline = min(deadline, time.time() + 1.2)
        continue
    try:
        chunk = os.read(fd, 65536)
    except OSError:
        break
    if not chunk:
        break
    text = chunk.decode("utf-8", "replace")
    if phase == "warmup":
        pre.append(text)
        joined = "".join(pre)
        if ("├──" in joined or "└──" in joined) and len(joined) > 400:
            started = time.time()
            recording.append([0.0, "o", joined])
            phase = "hold"
    else:
        recording.append([round(time.time() - started, 6), "o", text])
        if time.time() - started > 4 and not quit_sent:
            try:
                os.write(fd, b"q")
            except OSError:
                pass
            quit_sent = True
            deadline = min(deadline, time.time() + 1.2)

try:
    os.close(fd)
except OSError:
    pass
try:
    os.waitpid(pid, 0)
except ChildProcessError:
    pass

if not recording:
    print("failed to capture painted TUI frame; is this an armature repo?", file=sys.stderr)
    sys.exit(1)

recording.append([max(recording[-1][0] + 0.01, 3.5), "o", ""])

os.makedirs(os.path.dirname(os.path.abspath(cast_path)) or ".", exist_ok=True)
with open(cast_path, "w", encoding="utf-8") as f:
    f.write(json.dumps(header) + "\n")
    for e in recording:
        f.write(json.dumps(e, ensure_ascii=False) + "\n")
print(f"wrote {cast_path} ({len(recording)} events)")
PY

if [[ -n "$GIF_OUT" ]]; then
  if [[ -z "$AGG" ]]; then
    echo "agg not on PATH; install https://github.com/asciinema/agg to render GIF" >&2
    exit 1
  fi
  "$AGG" "$RECORD_OUT" "$GIF_OUT" --cols 100 --rows 30 --font-size 13 --theme monokai \
    --last-frame-duration 4 --idle-time-limit 2
  echo "wrote $GIF_OUT"
fi
