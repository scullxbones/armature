package doctor_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/doctor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckIDsDocMatchesLiveAndReservations_REQ_TOPTIER_S18_T3(t *testing.T) {
	want, err := doctor.RenderCheckIDsDoc()
	require.NoError(t, err)

	root := repoRoot(t)
	if os.Getenv("UPDATE_CHECK_IDS_DOC") == "1" {
		require.NoError(t, doctor.WriteCheckIDsDoc(root),
			"UPDATE_CHECK_IDS_DOC=1 must rewrite %s from LiveCheckIDs + OpenReservations",
			doctor.CheckIDsDocRelPath)
	}

	got, err := os.ReadFile(filepath.Join(root, doctor.CheckIDsDocRelPath))
	require.NoError(t, err, "committed registry doc must exist at %s; regenerate with go generate ./internal/doctor", doctor.CheckIDsDocRelPath)
	require.Equal(t, want, string(got),
		"docs/design/doctor-check-ids.md drifted; regenerate with go generate ./internal/doctor (or UPDATE_CHECK_IDS_DOC=1 go test ./internal/doctor -run TestCheckIDsDocMatchesLiveAndReservations_REQ_TOPTIER_S18_T3)")

	liveFromDoc := parseLiveCheckIDs(t, string(got))
	assert.Equal(t, doctor.LiveCheckIDs(), liveFromDoc,
		"live table must list doctor.LiveCheckIDs() in Run order")

	reserved := parseReservedRows(t, string(got))
	require.NotEmpty(t, reserved, "open-reservation table must not be empty")
	assert.Contains(t, reserved, reservedRow{id: "D11", issue: "TOPTIER-S12-T2"},
		"TOPTIER-S12-T2 must reserve planned D11 (S12 remains held; do not implement T2 here)")

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
	stalePath := filepath.Join(root, doctor.CheckIDsDocRelPath)
	require.NoError(t, doctor.WriteCheckIDsDoc(root), "WriteCheckIDsDoc must create CheckIDsDocRelPath")
	require.NoError(t, os.WriteFile(stalePath, []byte("STALE HAND-COPIED REGISTRY\n"), 0o644))

	require.NoError(t, doctor.WriteCheckIDsDoc(root))

	got, err := os.ReadFile(stalePath)
	require.NoError(t, err)
	want, err := doctor.RenderCheckIDsDoc()
	require.NoError(t, err)
	assert.Equal(t, want, string(got), "WriteCheckIDsDoc must rewrite CheckIDsDocRelPath from RenderCheckIDsDoc")
	assert.NotContains(t, string(got), "STALE HAND-COPIED")
}

func TestWriteCheckIDsDocReportsWriteError_REQ_TOPTIER_S18_T3(t *testing.T) {
	t.Parallel()

	blocked := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(blocked, []byte("file"), 0o644))
	err := doctor.WriteCheckIDsDoc(blocked)
	require.Error(t, err)
}

func TestCheckIDsDocDocumentsRegenCommand_REQ_TOPTIER_S18_T3(t *testing.T) {
	t.Parallel()

	doc, err := doctor.RenderCheckIDsDoc()
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

func TestOpenReservationsIncludesS12T2D11_REQ_TOPTIER_S18_T3(t *testing.T) {
	t.Parallel()

	res := doctor.OpenReservations()
	require.NotEmpty(t, res)
	found := false
	for _, r := range res {
		if r.ID == "D11" && r.Issue == "TOPTIER-S12-T2" {
			found = true
			assert.NotEmpty(t, r.Planned)
		}
	}
	assert.True(t, found, "OpenReservations must include TOPTIER-S12-T2 planned D11")

	first := doctor.OpenReservations()
	first[0].ID = "D999"
	second := doctor.OpenReservations()
	assert.Equal(t, "D11", second[0].ID, "OpenReservations must return a copy")
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
