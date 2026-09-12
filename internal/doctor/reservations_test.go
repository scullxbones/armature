package doctor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateReservationsRejectsInvalidAllocations(t *testing.T) {
	t.Parallel()

	live := []string{"D1", "D10"}
	ok := CheckReservation{ID: "D11", Issue: "TOPTIER-S12-T2", Planned: "ops-branch backup"}

	t.Run("empty_id", func(t *testing.T) {
		t.Parallel()
		err := validateReservations(live, []CheckReservation{{ID: "", Issue: "X", Planned: "p"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty check ID")
	})
	t.Run("bad_id", func(t *testing.T) {
		t.Parallel()
		err := validateReservations(live, []CheckReservation{{ID: "11", Issue: "X", Planned: "p"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid check ID")
	})
	t.Run("empty_issue", func(t *testing.T) {
		t.Parallel()
		err := validateReservations(live, []CheckReservation{{ID: "D11", Issue: "", Planned: "p"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty issue")
	})
	t.Run("empty_planned", func(t *testing.T) {
		t.Parallel()
		err := validateReservations(live, []CheckReservation{{ID: "D11", Issue: "X", Planned: ""}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty planned")
	})
	t.Run("duplicate", func(t *testing.T) {
		t.Parallel()
		err := validateReservations(live, []CheckReservation{ok, ok})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate reserved")
	})
	t.Run("collision_with_live", func(t *testing.T) {
		t.Parallel()
		err := validateReservations(live, []CheckReservation{{ID: "D10", Issue: "X", Planned: "p"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "collides with live")
	})
	t.Run("ok", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, validateReservations(live, []CheckReservation{ok}))
	})
}

func TestRenderCheckIDsDocFailsOnInvalidReservations(t *testing.T) {
	t.Parallel()
	_, err := renderCheckIDsDoc([]string{"D1"}, []CheckReservation{{ID: "D1", Issue: "X", Planned: "collide with live D1"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "collides with live")
}

func TestRenderCheckIDsDocEmitsLiveAndReservedRows(t *testing.T) {
	t.Parallel()
	doc, err := renderCheckIDsDoc([]string{"D1", "D2"}, []CheckReservation{{
		ID: "D11", Issue: "TOPTIER-S12-T2", Planned: "ops-branch backup",
	}})
	require.NoError(t, err)
	assert.Contains(t, doc, "| `D1` |\n| `D2` |\n")
	assert.Contains(t, doc, "| `D11` | `TOPTIER-S12-T2` | ops-branch backup |\n")
	assert.NotContains(t, doc, "| — |")
}

func TestRenderCheckIDsDocEmptyReservationsPlaceholder(t *testing.T) {
	t.Parallel()
	doc, err := renderCheckIDsDoc([]string{"D1"}, nil)
	require.NoError(t, err)
	assert.Contains(t, doc, "| — | — | _none_ |\n")
}
