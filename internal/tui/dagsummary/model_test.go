package dagsummary_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/armature/internal/tui/dagsummary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeItems(ids ...string) []dagsummary.Item {
	items := make([]dagsummary.Item, len(ids))
	for i, id := range ids {
		items[i] = dagsummary.Item{ID: id, Title: "Title " + id, IsCited: true}
	}
	return items
}

func keyMsg(k string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

func TestNewModel_HasItems(t *testing.T) {
	t.Parallel()
	items := makeItems("TSK-1", "TSK-2")
	m := dagsummary.New(items, "root-1")
	assert.Equal(t, 2, m.Total())
	assert.Equal(t, 0, m.Cursor())
	assert.False(t, m.Done())
	assert.False(t, m.Quitting())
}

func TestNewModel_Empty(t *testing.T) {
	t.Parallel()
	m := dagsummary.New(nil, "root-1")
	assert.Equal(t, 0, m.Total())
}

func TestApproveItem_MarksYes(t *testing.T) {
	t.Parallel()
	items := makeItems("TSK-1", "TSK-2")
	m := dagsummary.New(items, "root-1")
	m2, _ := m.Update(keyMsg("y"))
	updated := m2.(dagsummary.Model)
	assert.Equal(t, "y", updated.ActionFor("TSK-1"))
	assert.Equal(t, 1, updated.Cursor())
}

func TestRejectItem_MarksNo(t *testing.T) {
	t.Parallel()
	items := makeItems("TSK-1", "TSK-2")
	m := dagsummary.New(items, "root-1")
	m2, _ := m.Update(keyMsg("n"))
	updated := m2.(dagsummary.Model)
	assert.Equal(t, "n", updated.ActionFor("TSK-1"))
	assert.Equal(t, 1, updated.Cursor())
}

func TestSkipItem_MarksSkip(t *testing.T) {
	t.Parallel()
	items := makeItems("TSK-1", "TSK-2")
	m := dagsummary.New(items, "root-1")
	m2, _ := m.Update(keyMsg("s"))
	updated := m2.(dagsummary.Model)
	assert.Equal(t, "s", updated.ActionFor("TSK-1"))
	assert.Equal(t, 1, updated.Cursor())
}

func TestSignOffUnlocks_WhenAllActioned(t *testing.T) {
	t.Parallel()
	items := makeItems("TSK-1")
	m := dagsummary.New(items, "root-1")
	m2, _ := m.Update(keyMsg("y"))
	updated := m2.(dagsummary.Model)
	assert.True(t, updated.AwaitingSignOff())
	assert.False(t, updated.Done())
}

func TestSignOffNotUnlocked_BeforeAllActioned(t *testing.T) {
	t.Parallel()
	items := makeItems("TSK-1", "TSK-2")
	m := dagsummary.New(items, "root-1")
	m2, _ := m.Update(keyMsg("y"))
	updated := m2.(dagsummary.Model)
	assert.False(t, updated.AwaitingSignOff())
}

func TestSignOff_YConfirms(t *testing.T) {
	t.Parallel()
	items := makeItems("TSK-1")
	m := dagsummary.New(items, "root-1")
	m2, _ := m.Update(keyMsg("y"))
	m3, cmd := m2.(dagsummary.Model).Update(keyMsg("y"))
	final := m3.(dagsummary.Model)
	assert.True(t, final.Done())
	assert.False(t, final.Quitting())
	require.NotNil(t, cmd)
}

func TestSignOff_NGoesBackToReview(t *testing.T) {
	t.Parallel()
	items := makeItems("TSK-1")
	m := dagsummary.New(items, "root-1")
	m2, _ := m.Update(keyMsg("y"))
	m3, _ := m2.(dagsummary.Model).Update(keyMsg("n"))
	final := m3.(dagsummary.Model)
	assert.False(t, final.Done())
	assert.False(t, final.AwaitingSignOff())
}

func TestQuit_BeforeSignOff_NoOps(t *testing.T) {
	t.Parallel()
	items := makeItems("TSK-1")
	m := dagsummary.New(items, "root-1")
	m2, cmd := m.Update(keyMsg("q"))
	final := m2.(dagsummary.Model)
	assert.True(t, final.Quitting())
	assert.False(t, final.Done())
	require.NotNil(t, cmd)
}

func TestQuit_DuringSignOff_NoOps(t *testing.T) {
	t.Parallel()
	items := makeItems("TSK-1")
	m := dagsummary.New(items, "root-1")
	m2, _ := m.Update(keyMsg("y"))
	m3, cmd := m2.(dagsummary.Model).Update(keyMsg("q"))
	final := m3.(dagsummary.Model)
	assert.True(t, final.Quitting())
	assert.False(t, final.Done())
	require.NotNil(t, cmd)
}

func TestApprovedIDs_OnlyYes(t *testing.T) {
	t.Parallel()
	items := makeItems("TSK-1", "TSK-2", "TSK-3")
	m := dagsummary.New(items, "root-1")
	m2, _ := m.Update(keyMsg("y"))
	m3, _ := m2.(dagsummary.Model).Update(keyMsg("n"))
	m4, _ := m3.(dagsummary.Model).Update(keyMsg("s"))
	ids := m4.(dagsummary.Model).ApprovedIDs()
	assert.Equal(t, []string{"TSK-1"}, ids)
}

func TestApprovedIDs_Empty_WhenNoneApproved(t *testing.T) {
	t.Parallel()
	items := makeItems("TSK-1")
	m := dagsummary.New(items, "root-1")
	m2, _ := m.Update(keyMsg("n"))
	ids := m2.(dagsummary.Model).ApprovedIDs()
	assert.Empty(t, ids)
}

func TestUncitedNode_RequiresAckBeforeAction(t *testing.T) {
	t.Parallel()
	items := []dagsummary.Item{
		{ID: "TSK-1", Title: "Uncited task", IsCited: false},
	}
	m := dagsummary.New(items, "root-1")
	m2, _ := m.Update(keyMsg("y"))
	updated := m2.(dagsummary.Model)
	assert.Equal(t, "", updated.ActionFor("TSK-1"))
	assert.Equal(t, 0, updated.Cursor())
}

func TestUncitedNode_AcceptsActionAfterAck(t *testing.T) {
	t.Parallel()
	items := []dagsummary.Item{
		{ID: "TSK-1", Title: "Uncited task", IsCited: false},
	}
	m := dagsummary.New(items, "root-1")
	m2 := m
	for _, c := range "TSK-1" {
		next, _ := m2.Update(keyMsg(string(c)))
		m2 = next.(dagsummary.Model)
	}
	m3, _ := m2.Update(keyMsg("y"))
	updated := m3.(dagsummary.Model)
	assert.Equal(t, "y", updated.ActionFor("TSK-1"))
}

func TestUncitedNode_PartialAck_NotSufficient(t *testing.T) {
	t.Parallel()
	items := []dagsummary.Item{
		{ID: "TSK-12", Title: "Uncited", IsCited: false},
	}
	m := dagsummary.New(items, "root-1")
	m2, _ := m.Update(keyMsg("T"))
	updated := m2.(dagsummary.Model)
	m3, _ := updated.Update(keyMsg("y"))
	assert.Equal(t, "", m3.(dagsummary.Model).ActionFor("TSK-12"))
}

func TestView_ContainsCurrentItemID(t *testing.T) {
	t.Parallel()
	items := makeItems("TSK-42")
	m := dagsummary.New(items, "root-1")
	view := m.View()
	assert.Contains(t, view, "TSK-42")
}

func TestView_SignOff_ShowsPrompt(t *testing.T) {
	t.Parallel()
	items := makeItems("TSK-1")
	m := dagsummary.New(items, "root-1")
	m2, _ := m.Update(keyMsg("y"))
	view := m2.(dagsummary.Model).View()
	assert.Contains(t, view, "Sign off")
}

func TestView_UncitedNode_ShowsWarning(t *testing.T) {
	t.Parallel()
	items := []dagsummary.Item{
		{ID: "TSK-1", Title: "Uncited", IsCited: false},
	}
	m := dagsummary.New(items, "root-1")
	view := m.View()
	assert.Contains(t, view, "uncited")
}

func TestInit_ReturnsNil(t *testing.T) {
	t.Parallel()
	m := dagsummary.New(makeItems("TSK-1"), "root-1")
	assert.Nil(t, m.Init())
}
