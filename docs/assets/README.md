# Docs assets

| File | Purpose |
| --- | --- |
| [`arm-tui-dag.gif`](arm-tui-dag.gif) | Animated capture of `arm tui` DAG tree from this repository (README above-the-fold visual) |
| [`arm-tui-dag.cast`](arm-tui-dag.cast) | Asciinema cast used to render the GIF |
| [`record-arm-tui-dag.sh`](record-arm-tui-dag.sh) | Re-record script for the TUI DAG cast/GIF |
| [`adopter-demo.cast`](adopter-demo.cast) | Asciinema recording of the paved-road demo (coordinator + two worker tasks) |
| [`record-adopter-demo.sh`](record-adopter-demo.sh) | Script that produced the cast; re-run after CLI changes |

## TUI DAG visual

Replay the cast locally (asciinema 2.x/3.x):

```bash
asciinema play docs/assets/arm-tui-dag.cast
```

Re-record from an armature checkout (requires `arm` on `PATH`; `agg` for GIF):

```bash
./docs/assets/record-arm-tui-dag.sh \
  --record docs/assets/arm-tui-dag.cast \
  --gif docs/assets/arm-tui-dag.gif
```

## Adopter demo

Replay locally (asciinema 3.x):

```bash
asciinema play docs/assets/adopter-demo.cast
```

Re-record from the repo root (requires `arm` and `asciinema` on `PATH`):

```bash
./docs/assets/record-adopter-demo.sh --record docs/assets/adopter-demo.cast
```
