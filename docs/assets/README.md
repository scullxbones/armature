# Docs assets

| File | Purpose |
| --- | --- |
| [`adopter-demo.cast`](adopter-demo.cast) | Asciinema recording of the paved-road demo (coordinator + two worker tasks) |
| [`record-adopter-demo.sh`](record-adopter-demo.sh) | Script that produced the cast; re-run after CLI changes |

Replay locally (asciinema 3.x):

```bash
asciinema play docs/assets/adopter-demo.cast
```

Re-record from the repo root (requires `arm` and `asciinema` on `PATH`):

```bash
./docs/assets/record-adopter-demo.sh --record docs/assets/adopter-demo.cast
```
