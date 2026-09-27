// Package oporder recovers published _armature commit order without rewriting JSONL.
package oporder

import (
	"bufio"
	"bytes"
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/ops"
)

// DefaultPublishedRef is the tracking ref every clone agrees on after fetch.
const DefaultPublishedRef = "origin/_armature"

// Seq is the total order every clone agrees on after fetch.
// W1.1 fills Epoch 0 (timestamp + filename + line). CommitN is set in W1.2.
type Seq struct {
	Epoch     int
	CommitN   int64
	Line      int
	Timestamp int64
	Filename  string
}

// LocatedOp is one JSONL op with git location metadata.
type LocatedOp struct {
	Op            ops.Op
	Seq           Seq
	CommitSHA     string
	CommitterUnix int64
	Published     bool
}

// LocateInput names the ops worktree and the published tip.
type LocateInput struct {
	OpsWorktree       string
	FromCommit        string
	Cutover           string
	PublishedRef      string
	ExtraPublishedTip string
	OpsPrefixes       []string
}

func prefixes(in LocateInput) []string {
	if len(in.OpsPrefixes) > 0 {
		return in.OpsPrefixes
	}
	return []string{"ops", ".armature/ops"}
}

func publishedRef(in LocateInput) string {
	if in.PublishedRef != "" {
		return in.PublishedRef
	}
	return DefaultPublishedRef
}

// LocateOps loads ops at HEAD and marks those present at the published tip.
func LocateOps(in LocateInput) ([]LocatedOp, error) {
	if in.OpsWorktree == "" {
		return nil, fmt.Errorf("LocateOps: ops worktree is required")
	}
	if !hasGitDir(in.OpsWorktree) {
		located, err := loadOpsFromWorktreeFiles(in.OpsWorktree, prefixes(in))
		if err != nil {
			return nil, err
		}
		for i := range located {
			located[i].Published = true
		}
		SortLocated(located)
		return located, nil
	}
	gc := adapters.New(in.OpsWorktree)
	head, err := gc.ResolveRevision("HEAD")
	if err != nil {
		return nil, fmt.Errorf("resolve HEAD: %w", err)
	}
	headOps, err := loadOpsAt(gc, head, prefixes(in))
	if err != nil {
		return nil, err
	}
	pubSHA, err := resolvePublishedTip(gc, in)
	if err != nil {
		return nil, err
	}
	if pubSHA == "" {
		SortLocated(headOps)
		return headOps, nil
	}
	if pubSHA == head {
		for i := range headOps {
			headOps[i].Published = true
		}
		SortLocated(headOps)
		return headOps, nil
	}
	publishedKeys := map[string]bool{}
	pubOps, loadErr := loadOpsAt(gc, pubSHA, prefixes(in))
	if loadErr != nil {
		return nil, loadErr
	}
	for _, loc := range pubOps {
		publishedKeys[opKey(loc.Op)] = true
	}
	for i := range headOps {
		headOps[i].Published = publishedKeys[opKey(headOps[i].Op)]
	}
	SortLocated(headOps)
	return headOps, nil
}

func resolvePublishedTip(gc *adapters.Client, in LocateInput) (string, error) {
	if in.ExtraPublishedTip != "" {
		sha, err := gc.ResolveRevision(in.ExtraPublishedTip)
		if err != nil {
			return "", fmt.Errorf("resolve extra published tip %s: %w", in.ExtraPublishedTip, err)
		}
		return sha, nil
	}
	sha, err := gc.ResolveRevision(publishedRef(in))
	if err != nil {
		return "", nil
	}
	return sha, nil
}

func hasGitDir(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}

func loadOpsFromWorktreeFiles(root string, opsPrefixes []string) ([]LocatedOp, error) {
	var files []string
	for _, p := range opsPrefixes {
		dir := filepath.Join(root, p)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
				continue
			}
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	rootEntries, err := os.ReadDir(root)
	if err == nil {
		for _, e := range rootEntries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
				continue
			}
			files = append(files, filepath.Join(root, e.Name()))
		}
	}
	var located []LocatedOp
	for _, path := range files {
		content, readErr := os.ReadFile(path) //nolint:gosec // ops logs under the configured worktree
		if readErr != nil {
			continue
		}
		expectedWorkerID := strings.TrimSuffix(filepath.Base(path), ".log")
		legacyWorkerID, _, _ := strings.Cut(expectedWorkerID, "~")
		scanner := bufio.NewScanner(bytes.NewReader(content))
		scanner.Buffer(make([]byte, 1<<20), 1<<20)
		line := 0
		for scanner.Scan() {
			raw := scanner.Bytes()
			if len(raw) == 0 {
				continue
			}
			line++
			op, parseErr := ops.ParseLine(raw)
			if parseErr != nil {
				continue
			}
			if op.WorkerID != expectedWorkerID && op.WorkerID != legacyWorkerID {
				continue
			}
			located = append(located, LocatedOp{
				Op: op,
				Seq: Seq{
					Epoch:     0,
					Line:      line,
					Timestamp: op.Timestamp,
					Filename:  filepath.Base(path),
				},
			})
		}
	}
	return located, nil
}

func loadOpsAt(gc *adapters.Client, sha string, opsPrefixes []string) ([]LocatedOp, error) {
	files, err := gc.ListFilesAtCommit(sha)
	if err != nil {
		return nil, fmt.Errorf("list files at %s: %w", sha, err)
	}
	prefixed := make([]string, len(opsPrefixes))
	for i, p := range opsPrefixes {
		prefixed[i] = strings.TrimSuffix(p, "/") + "/"
	}
	var located []LocatedOp
	for _, f := range files {
		if !strings.HasSuffix(f, ".log") {
			continue
		}
		underPrefix := slices.ContainsFunc(prefixed, func(p string) bool { return strings.HasPrefix(f, p) })
		if !underPrefix && strings.Contains(f, "/") {
			continue
		}
		expectedWorkerID := strings.TrimSuffix(filepath.Base(f), ".log")
		legacyWorkerID, _, _ := strings.Cut(expectedWorkerID, "~")
		content, err := gc.ShowFileAtCommit(sha, f)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(bytes.NewReader(content))
		scanner.Buffer(make([]byte, 1<<20), 1<<20)
		line := 0
		for scanner.Scan() {
			raw := scanner.Bytes()
			if len(raw) == 0 {
				continue
			}
			line++
			op, parseErr := ops.ParseLine(raw)
			if parseErr != nil {
				continue
			}
			if op.WorkerID != expectedWorkerID && op.WorkerID != legacyWorkerID {
				continue
			}
			filename := filepath.Base(f)
			located = append(located, LocatedOp{
				Op:        op,
				CommitSHA: sha,
				Seq: Seq{
					Epoch:     0,
					Line:      line,
					Timestamp: op.Timestamp,
					Filename:  filename,
				},
			})
		}
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("scan file %s at %s: %w", f, sha, err)
		}
	}
	return located, nil
}

func opKey(op ops.Op) string {
	b, err := ops.MarshalOp(op)
	if err != nil {
		return fmt.Sprintf("%s|%s|%d|%s|%s", op.Type, op.TargetID, op.Timestamp, op.WorkerID, op.Payload.ClaimToken)
	}
	return string(b)
}

// SortLocated orders by timestamp, then filename, then line (W1.1 / pre-C0).
func SortLocated(located []LocatedOp) {
	slices.SortStableFunc(located, func(a, b LocatedOp) int {
		if n := cmp.Compare(a.Seq.Timestamp, b.Seq.Timestamp); n != 0 {
			return n
		}
		if n := cmp.Compare(a.Seq.Filename, b.Seq.Filename); n != 0 {
			return n
		}
		return cmp.Compare(a.Seq.Line, b.Seq.Line)
	})
}

// Published returns located ops whose introducing content is on the published tip.
func Published(located []LocatedOp) []LocatedOp {
	var out []LocatedOp
	for _, loc := range located {
		if loc.Published {
			out = append(out, loc)
		}
	}
	return out
}

// Pending returns located ops present at HEAD but not on the published tip.
func Pending(located []LocatedOp) []LocatedOp {
	var out []LocatedOp
	for _, loc := range located {
		if !loc.Published {
			out = append(out, loc)
		}
	}
	return out
}

// Ops unwraps LocatedOp values.
func Ops(located []LocatedOp) []ops.Op {
	out := make([]ops.Op, len(located))
	for i, loc := range located {
		out[i] = loc.Op
	}
	return out
}
