// Package oporder recovers published _armature commit order without rewriting JSONL.
package oporder

import (
	"bufio"
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/ops"
)

// ErrFromCommitMissing is returned when LocateInput.FromCommit is set but is
// not an ancestor of HEAD. Callers must cold-replay rather than treating the
// whole walk as incremental on cached snapshots.
var ErrFromCommitMissing = errors.New("from-commit is not in HEAD history")

type fromCommitMissingError struct {
	SHA string
}

func (e *fromCommitMissingError) Error() string {
	return fmt.Sprintf("from-commit %s is not in HEAD history", e.SHA)
}

func (e *fromCommitMissingError) Unwrap() error { return ErrFromCommitMissing }

// IsFromCommitMissing reports whether err means the incremental checkpoint SHA
// is absent from HEAD (rebase/amend/orphan).
func IsFromCommitMissing(err error) bool {
	return errors.Is(err, ErrFromCommitMissing)
}

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
	located, err := locateByCommitWalk(gc, in)
	if err != nil {
		return nil, err
	}
	SortLocated(located)
	return located, nil
}

func locateByCommitWalk(gc *adapters.Client, in LocateInput) ([]LocatedOp, error) {
	shas, err := gc.RevListReverse("HEAD")
	if err != nil {
		return nil, fmt.Errorf("rev-list HEAD: %w", err)
	}
	pubSHA, err := resolvePublishedTip(gc, in)
	if err != nil {
		return nil, err
	}
	cutover := resolveCutover(gc, in)
	from := strings.TrimSpace(in.FromCommit)
	pastFrom := from == ""
	if from != "" {
		found := false
		for _, sha := range shas {
			if sha == from {
				found = true
				break
			}
		}
		if !found {
			return nil, &fromCommitMissingError{SHA: from}
		}
	}
	prefs := prefixes(in)
	var located []LocatedOp
	for i, sha := range shas {
		if !pastFrom {
			if sha == from {
				pastFrom = true
			}
			continue
		}
		committer, ctErr := gc.CommitterUnix(sha)
		if ctErr != nil {
			return nil, ctErr
		}
		parent, hasParent, pErr := gc.FirstParent(sha)
		if pErr != nil {
			return nil, pErr
		}
		files, fErr := commitLogFiles(gc, sha, parent, hasParent, prefs)
		if fErr != nil {
			return nil, fErr
		}
		published := false
		if pubSHA != "" {
			if sha == pubSHA {
				published = true
			} else {
				anc, aErr := gc.IsAncestor(sha, pubSHA)
				if aErr != nil {
					return nil, aErr
				}
				published = anc
			}
		}
		epoch := 0
		if cutover != "" {
			before, bErr := commitStrictlyBefore(gc, sha, cutover)
			if bErr != nil {
				return nil, bErr
			}
			if !before {
				epoch = 1
			}
		}
		commitN := int64(i + 1)
		for _, f := range files {
			opsInCommit, lErr := opsIntroducedInCommit(gc, sha, parent, hasParent, f)
			if lErr != nil {
				return nil, lErr
			}
			for _, loc := range opsInCommit {
				loc.CommitSHA = sha
				loc.CommitterUnix = committer
				loc.Published = published
				loc.Seq.Epoch = epoch
				loc.Seq.CommitN = commitN
				located = append(located, loc)
			}
		}
	}
	return located, nil
}

const CutoverConfigKey = "armature.oporder-cutover"

func resolveCutover(gc *adapters.Client, in LocateInput) string {
	if c := strings.TrimSpace(in.Cutover); c != "" {
		return c
	}
	v, err := gc.ReadGitConfig(CutoverConfigKey)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(v)
}

func commitStrictlyBefore(gc *adapters.Client, sha, cutover string) (bool, error) {
	if sha == cutover {
		return false, nil
	}
	return gc.IsAncestor(sha, cutover)
}

func commitLogFiles(gc *adapters.Client, sha, parent string, hasParent bool, opsPrefixes []string) ([]string, error) {
	var files []string
	if !hasParent {
		all, err := gc.ListFilesAtCommit(sha)
		if err != nil {
			return nil, err
		}
		files = all
	} else {
		changed, err := gc.DiffNameOnlyRange(parent, sha)
		if err != nil {
			return nil, err
		}
		files = changed
	}
	var logs []string
	for _, f := range files {
		if !isOpLogPath(f, opsPrefixes) {
			continue
		}
		logs = append(logs, f)
	}
	return logs, nil
}

func isOpLogPath(f string, opsPrefixes []string) bool {
	if !strings.HasSuffix(f, ".log") {
		return false
	}
	if !strings.Contains(f, "/") {
		return true
	}
	for _, p := range opsPrefixes {
		pref := strings.TrimSuffix(p, "/") + "/"
		if strings.HasPrefix(f, pref) {
			return true
		}
	}
	return false
}

func opsIntroducedInCommit(gc *adapters.Client, sha, parent string, hasParent bool, path string) ([]LocatedOp, error) {
	var old []byte
	if hasParent {
		b, err := gc.ShowFileAtCommit(parent, path)
		if err == nil {
			old = b
		}
	}
	neu, err := gc.ShowFileAtCommit(sha, path)
	if err != nil {
		return nil, nil
	}
	oldLines := countNonEmptyLines(old)
	return parseLogBytesFromLine(neu, filepath.Base(path), oldLines)
}

func countNonEmptyLines(b []byte) int {
	n := 0
	scanner := bufio.NewScanner(bytes.NewReader(b))
	for scanner.Scan() {
		if len(scanner.Bytes()) > 0 {
			n++
		}
	}
	return n
}

func parseLogBytesFromLine(content []byte, filename string, skipNonEmpty int) ([]LocatedOp, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	seen := 0
	line := 0
	var located []LocatedOp
	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		seen++
		if seen <= skipNonEmpty {
			continue
		}
		line++
		op, parseErr := ops.ParseLine(raw)
		if parseErr != nil {
			continue
		}
		if !ops.WorkerOwnsLog(op.WorkerID, filename) {
			continue
		}
		located = append(located, LocatedOp{
			Op: op,
			Seq: Seq{
				Line:      line,
				Timestamp: op.Timestamp,
				Filename:  filename,
			},
		})
	}
	return located, scanner.Err()
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
			if !ops.WorkerOwnsLog(op.WorkerID, path) {
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

// SortLocated orders pre-C0 (epoch 0) by timestamp/filename/line and post-C0
// (epoch 1) by commit sequence then line.
func SortLocated(located []LocatedOp) {
	slices.SortStableFunc(located, func(a, b LocatedOp) int {
		if n := cmp.Compare(a.Seq.Epoch, b.Seq.Epoch); n != 0 {
			return n
		}
		if a.Seq.Epoch == 1 {
			if n := cmp.Compare(a.Seq.CommitN, b.Seq.CommitN); n != 0 {
				return n
			}
			return cmp.Compare(a.Seq.Line, b.Seq.Line)
		}
		if n := cmp.Compare(a.Seq.Timestamp, b.Seq.Timestamp); n != 0 {
			return n
		}
		if n := cmp.Compare(a.Seq.Filename, b.Seq.Filename); n != 0 {
			return n
		}
		return cmp.Compare(a.Seq.Line, b.Seq.Line)
	})
}

func stealAt(loc LocatedOp) int64 {
	if loc.Seq.Epoch == 1 && loc.CommitterUnix > 0 {
		return loc.CommitterUnix
	}
	return loc.Op.Timestamp
}

// OwnerOf folds claim.ApplyAt over located ops for one issue (commit-order after C0).
func OwnerOf(located []LocatedOp, issueID string) claim.Lease {
	var held claim.Lease
	for _, loc := range located {
		if loc.Op.TargetID != issueID {
			continue
		}
		held = claim.ApplyAt(held, loc.Op, stealAt(loc))
	}
	return held
}

// RequireOwner fails unless workerID holds the published lease (token set).
func RequireOwner(located []LocatedOp, issueID, workerID string) error {
	lease := OwnerOf(Published(located), issueID)
	if workerID == "" || lease.Holder != workerID || lease.Token == "" {
		return claim.ErrNotClaimOwner
	}
	return nil
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
