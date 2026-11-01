package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckIDsDocMatchesLiveAndReservations_REQ_TOPTIER_S18_T3(t *testing.T) {
	t.Parallel()

	want, err := RenderCheckIDsDoc()
	require.NoError(t, err)

	root := repoRoot(t)
	if os.Getenv("UPDATE_CHECK_IDS_DOC") == "1" {
		require.NoError(t, WriteCheckIDsDoc(root),
			"UPDATE_CHECK_IDS_DOC=1 must rewrite %s from LiveCheckIDs + OpenReservations",
			CheckIDsDocRelPath)
	}

	got, err := os.ReadFile(filepath.Join(root, CheckIDsDocRelPath))
	require.NoError(t, err, "committed registry doc must exist at %s; regenerate with go generate ./internal/doctor", CheckIDsDocRelPath)
	require.Equal(t, want, string(got),
		"docs/design/doctor-check-ids.md drifted; regenerate with go generate ./internal/doctor")

	liveFromDoc := parseLiveCheckIDs(t, string(got))
	assert.Equal(t, LiveCheckIDs(), liveFromDoc,
		"live table must list LiveCheckIDs() in Run order")

	reserved := parseReservedRows(t, string(got))
	require.NotEmpty(t, reserved, "open-reservation table must not be empty")
	assert.NotContains(t, reserved, reservedRow{id: "D11", issue: "TOPTIER-S12-T2"},
		"D11 is live for TOPTIER-S12-T2; reservation must be removed")
	assert.Contains(t, liveFromDoc, "D11", "D11 must be listed as a live check ID")

	liveSet := make(map[string]struct{}, len(liveFromDoc))
	for _, id := range liveFromDoc {
		_, dup := liveSet[id]
		assert.False(t, dup, "duplicate live check ID %s", id)
		liveSet[id] = struct{}{}
	}
	for _, row := range reserved {
		_, clash := liveSet[row.id]
		assert.False(t, clash, "reserved %s (%s) collides with a live check ID", row.id, row.issue)
	}
}

func TestWriteCheckIDsDocRewritesRegistry_REQ_TOPTIER_S18_T3(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	stalePath := filepath.Join(root, CheckIDsDocRelPath)
	require.NoError(t, WriteCheckIDsDoc(root), "WriteCheckIDsDoc must create CheckIDsDocRelPath")
	require.NoError(t, os.WriteFile(stalePath, []byte("STALE HAND-COPIED REGISTRY\n"), 0o644))

	require.NoError(t, WriteCheckIDsDoc(root))

	got, err := os.ReadFile(stalePath)
	require.NoError(t, err)
	want, err := RenderCheckIDsDoc()
	require.NoError(t, err)
	assert.Equal(t, want, string(got), "WriteCheckIDsDoc must rewrite CheckIDsDocRelPath from RenderCheckIDsDoc")
	assert.NotContains(t, string(got), "STALE HAND-COPIED")
}

func TestWriteCheckIDsDocReportsWriteError_REQ_TOPTIER_S18_T3(t *testing.T) {
	t.Parallel()

	blocked := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(blocked, []byte("file"), 0o644))
	err := WriteCheckIDsDoc(blocked)
	require.Error(t, err)
}

func TestCheckIDsDocDocumentsRegenCommand_REQ_TOPTIER_S18_T3(t *testing.T) {
	t.Parallel()

	doc, err := RenderCheckIDsDoc()
	require.NoError(t, err)
	assert.Contains(t, doc, "`go generate ./internal/doctor`",
		"registry must name the executable regeneration command")
	assert.Contains(t, doc, "UPDATE_CHECK_IDS_DOC=1",
		"registry must name the drift-test update mode")
}

func TestCheckIDsDocGoGenerateDirective_REQ_TOPTIER_S18_T3(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile(filepath.Join(repoRoot(t), "internal", "doctor", "checkids_doc.go"))
	require.NoError(t, err)
	assert.Contains(t, string(src), "//go:generate go run generate_checkids_doc.go")
}

func TestOpenReservationsExcludesLiveD11_REQ_TOPTIER_S12_T2(t *testing.T) {
	t.Parallel()

	res := OpenReservations()
	require.NotEmpty(t, res)
	for _, r := range res {
		assert.NotEqual(t, "D11", r.ID, "D11 must not remain reserved once live")
	}

	first := OpenReservations()
	orig := first[0].ID
	first[0].ID = "D999"
	second := OpenReservations()
	assert.Equal(t, orig, second[0].ID, "OpenReservations must return a copy")
}

// TestValidationCodesDocListsEveryLiveCheck_REQ_TOPTIER_S12_T2 derives the live
// doctor code list from LiveCheckIDs() and fails when docs/validation-codes.md
// is missing any of those codes (heading or quick-reference row).
func TestValidationCodesDocListsEveryLiveCheck_REQ_TOPTIER_S12_T2(t *testing.T) {
	t.Parallel()

	path := filepath.Join(repoRoot(t), "docs", "validation-codes.md")
	doc, err := os.ReadFile(path)
	require.NoError(t, err, "docs/validation-codes.md must exist")

	body := string(doc)
	live := LiveCheckIDs()
	require.NotEmpty(t, live)
	for _, id := range live {
		heading := regexp.MustCompile(`(?m)^### ` + regexp.QuoteMeta(id) + `:`)
		tableRow := regexp.MustCompile(`(?m)^\| ` + regexp.QuoteMeta(id) + ` \|`)
		assert.True(t, heading.MatchString(body),
			"docs/validation-codes.md missing ### %s: section (registry LiveCheckIDs)", id)
		assert.True(t, tableRow.MatchString(body),
			"docs/validation-codes.md missing quick-reference row for %s (registry LiveCheckIDs)", id)
	}
	assert.Contains(t, body, "checks D1–D12",
		"validation-codes intro must name the full live doctor range including D11")
}

// TestDocsDoNotReserveLiveCheckIDs_REQ_TOPTIER_S12_T2 fails when non-archive
// docs/ prose places "reserved" next to a live doctor check ID (e.g. calling
// live D11 reserved). Archive/ and historical "no longer reserved" phrasing
// are allowed.
func TestDocsDoNotReserveLiveCheckIDs_REQ_TOPTIER_S12_T2(t *testing.T) {
	t.Parallel()

	live := LiveCheckIDs()
	require.NotEmpty(t, live)
	idRes := make([]*regexp.Regexp, 0, len(live))
	for _, id := range live {
		// Word-boundary so D1 does not match inside D13.
		idRes = append(idRes, regexp.MustCompile(`(?i)\b`+regexp.QuoteMeta(id)+`\b`))
	}
	reservedRE := regexp.MustCompile(`(?i)reserved`)

	root := filepath.Join(repoRoot(t), "docs")
	var mdPaths []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if rel == "archive" || strings.HasPrefix(rel, "archive"+string(filepath.Separator)) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(strings.ToLower(path), ".md") {
			mdPaths = append(mdPaths, path)
		}
		return nil
	})
	require.NoError(t, err)

	var offenders []string
	for _, path := range mdPaths {
		rel, err := filepath.Rel(root, path)
		require.NoError(t, err)
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		for i, line := range strings.Split(string(raw), "\n") {
			lower := strings.ToLower(line)
			if !reservedRE.MatchString(line) {
				continue
			}
			// Explicitly historical / negated wording is fine.
			if strings.Contains(lower, "no longer reserved") ||
				strings.Contains(lower, "not reserved") ||
				strings.Contains(lower, "was reserved") ||
				strings.Contains(lower, "formerly reserved") {
				continue
			}
			resIdx := reservedRE.FindStringIndex(line)
			if resIdx == nil {
				continue
			}
			for _, idRE := range idRes {
				loc := idRE.FindStringIndex(line)
				if loc == nil {
					continue
				}
				gap := loc[0] - resIdx[0]
				if gap < 0 {
					gap = -gap
				}
				if gap > 48 {
					continue
				}
				offenders = append(offenders, fmt.Sprintf("%s:%d: %s", filepath.ToSlash(rel), i+1, strings.TrimSpace(line)))
			}
		}
	}
	assert.Empty(t, offenders, "docs/ (excl archive) must not call a live check ID reserved:\n%s", strings.Join(offenders, "\n"))
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

var liveRow = regexp.MustCompile(`^\| ` + "`" + `(D[0-9]+)` + "`" + ` \|$`)
var reservedRowRE = regexp.MustCompile(`^\| ` + "`" + `(D[0-9]+)` + "`" + ` \| ` + "`" + `([^` + "`" + `]+)` + "`" + ` \|`)

type reservedRow struct {
	id, issue string
}

func parseLiveCheckIDs(t *testing.T, doc string) []string {
	t.Helper()
	section := sectionBody(t, doc, "## Live checks")
	var ids []string
	for _, line := range strings.Split(section, "\n") {
		m := liveRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ids = append(ids, m[1])
	}
	require.NotEmpty(t, ids, "live checks table missing generated ID rows")
	return ids
}

func parseReservedRows(t *testing.T, doc string) []reservedRow {
	t.Helper()
	section := sectionBody(t, doc, "## Open reservations")
	var rows []reservedRow
	for _, line := range strings.Split(section, "\n") {
		m := reservedRowRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		rows = append(rows, reservedRow{id: m[1], issue: m[2]})
	}
	return rows
}

func sectionBody(t *testing.T, doc, heading string) string {
	t.Helper()
	start := strings.Index(doc, heading)
	require.GreaterOrEqual(t, start, 0, "missing heading %s", heading)
	rest := doc[start:]
	next := strings.Index(rest[len(heading):], "\n## ")
	if next < 0 {
		return rest
	}
	return rest[:len(heading)+next]
}
