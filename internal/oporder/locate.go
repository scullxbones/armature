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

func IsFromCommitMissing(err error) bool {
	return errors.Is(err, ErrFromCommitMissing)
}

const DefaultPublishedRef = "origin/_armature"

const (
	epochTimestampFilenameLine = 0
	epochCommitThenLine        = 1
)

type Seq struct {
	Epoch     int
	CommitN   int64
	Line      int
	Timestamp int64
	Filename  string
}

type LocatedOp struct {
	Op            ops.Op
	Seq           Seq
	CommitSHA     string
	CommitterUnix int64
	Published     bool
}

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
		return locateAllPublishedFromWorktreeFiles(in)
	}
	gc := adapters.New(in.OpsWorktree)
	located, err := locateByCommitWalk(gc, in)
	if err != nil {
		return nil, err
	}
	SortLocated(located)
	return located, nil
}

func locateAllPublishedFromWorktreeFiles(in LocateInput) ([]LocatedOp, error) {
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

func locateByCommitWalk(gc *adapters.Client, in LocateInput) ([]LocatedOp, error) {
	// Flat git budget (independent of commit / op-log-touching commit count):
	// one CommitWalkReverse, one RevListReverse each for published/cutover
	// ancestor sets, one ShowFilesBatch (cat-file --batch) for all log blobs.
	// Error text says "commit walk HEAD" (not "rev-list HEAD") by design.
	walk, err := gc.CommitWalkReverse("HEAD")
	if err != nil {
		return nil, fmt.Errorf("commit walk HEAD: %w", err)
	}
	pubSHA, err := resolvePublishedTip(gc, in)
	if err != nil {
		return nil, err
	}
	publishedSet, err := ancestorSet(gc, pubSHA)
	if err != nil {
		return nil, err
	}
	cutover := resolveCutover(gc, in)
	cutoverSet, err := ancestorSet(gc, cutover)
	if err != nil {
		return nil, err
	}
	from := strings.TrimSpace(in.FromCommit)
	pastFrom := from == ""
	if from != "" {
		found := false
		for _, e := range walk {
			if e.SHA == from {
				found = true
				break
			}
		}
		if !found {
			return nil, &fromCommitMissingError{SHA: from}
		}
	}
	prefs := prefixes(in)

	type fileJob struct {
		commitIdx int
		path      string
		newRef    adapters.BlobRef
		oldRef    *adapters.BlobRef
	}
	var jobs []fileJob
	var blobRefs []adapters.BlobRef
	for i, e := range walk {
		if !pastFrom {
			if e.SHA == from {
				pastFrom = true
			}
			continue
		}
		for _, f := range filterOpLogPaths(e.ChangedFiles, prefs) {
			job := fileJob{
				commitIdx: i,
				path:      f,
				newRef:    adapters.BlobRef{Rev: e.SHA, Path: f},
			}
			blobRefs = append(blobRefs, job.newRef)
			if e.FirstParent != "" {
				old := adapters.BlobRef{Rev: e.FirstParent, Path: f}
				job.oldRef = &old
				blobRefs = append(blobRefs, old)
			}
			jobs = append(jobs, job)
		}
	}
	blobs, err := gc.ShowFilesBatch(blobRefs)
	if err != nil {
		return nil, err
	}
	if len(blobs) != len(blobRefs) {
		return nil, fmt.Errorf("show files batch: got %d results for %d refs", len(blobs), len(blobRefs))
	}

	blobAt := 0
	var located []LocatedOp
	for _, job := range jobs {
		e := walk[job.commitIdx]
		var neu []byte
		if blobs[blobAt].OK {
			neu = blobs[blobAt].Content
		}
		blobAt++
		var old []byte
		if job.oldRef != nil {
			if blobs[blobAt].OK {
				old = blobs[blobAt].Content
			}
			blobAt++
		}
		if neu == nil {
			continue
		}
		opsInCommit, lErr := opsIntroducedFromBytes(neu, old, filepath.Base(job.path))
		if lErr != nil {
			return nil, lErr
		}
		_, inPublished := publishedSet[e.SHA]
		published := pubSHA != "" && inPublished
		epoch := epochTimestampFilenameLine
		if cutover != "" {
			_, inCutover := cutoverSet[e.SHA]
			if !inCutover || e.SHA == cutover {
				epoch = epochCommitThenLine
			}
		}
		commitN := int64(job.commitIdx + 1)
		for _, loc := range opsInCommit {
			loc.CommitSHA = e.SHA
			loc.CommitterUnix = e.CommitterUnix
			loc.Published = published
			loc.Seq.Epoch = epoch
			loc.Seq.CommitN = commitN
			located = append(located, loc)
		}
	}
	return located, nil
}

// ancestorSet returns every commit reachable from tip (inclusive) in one
// rev-list. An empty tip yields a nil map (caller treats membership as false).
func ancestorSet(gc *adapters.Client, tip string) (map[string]struct{}, error) {
	tip = strings.TrimSpace(tip)
	if tip == "" {
		return nil, nil
	}
	shas, err := gc.RevListReverse(tip)
	if err != nil {
		return nil, fmt.Errorf("rev-list %s: %w", tip, err)
	}
	set := make(map[string]struct{}, len(shas))
	for _, sha := range shas {
		set[sha] = struct{}{}
	}
	return set, nil
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

func filterOpLogPaths(files []string, opsPrefixes []string) []string {
	var logs []string
	for _, f := range files {
		if !isOpLogPath(f, opsPrefixes) {
			continue
		}
		logs = append(logs, f)
	}
	return logs
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

func opsIntroducedFromBytes(neu, old []byte, filename string) ([]LocatedOp, error) {
	oldLines := countNonEmptyLines(old)
	return parseLogBytesFromLine(neu, filename, oldLines)
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
		locs, err := parseLogBytesFromLine(content, filepath.Base(path), 0)
		located = append(located, locs...)
		if err != nil {
			continue
		}
	}
	return located, nil
}

func SortLocated(located []LocatedOp) {
	slices.SortStableFunc(located, func(a, b LocatedOp) int {
		if n := cmp.Compare(a.Seq.Epoch, b.Seq.Epoch); n != 0 {
			return n
		}
		if a.Seq.Epoch == epochCommitThenLine {
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
	if loc.Seq.Epoch == epochCommitThenLine && loc.CommitterUnix > 0 {
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

func Ops(located []LocatedOp) []ops.Op {
	out := make([]ops.Op, len(located))
	for i, loc := range located {
		out[i] = loc.Op
	}
	return out
}
