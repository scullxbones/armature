package tuivalidate_test

import (
	"strings"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/tui/tuivalidate"
)

func TestValidateInit(t *testing.T) {
	t.Parallel()
	m := tuivalidate.New()
	if cmd := m.Init(); cmd != nil {
		t.Error("Init should return nil")
	}
}

func TestValidateSetSize(t *testing.T) {
	t.Parallel()
	m := tuivalidate.New()
	m.SetSize(80, 24) // must not panic
}

func TestValidateHelpBar(t *testing.T) {
	t.Parallel()
	m := tuivalidate.New()
	h := m.HelpBar()
	if !strings.Contains(h, "q quit") {
		t.Errorf("help bar missing q quit, got: %s", h)
	}
}

func TestValidateUpdate(t *testing.T) {
	t.Parallel()
	m := tuivalidate.New()
	screen, cmd := m.Update(nil)
	if screen == nil {
		t.Error("Update should return the model")
	}
	if cmd != nil {
		t.Error("Update should return nil cmd")
	}
}

func TestValidateNilStateView(t *testing.T) {
	t.Parallel()
	m := tuivalidate.New()
	v := m.View()
	if !strings.Contains(v, "No state available") {
		t.Errorf("expected nil-state message, got: %s", v)
	}
}

// TestValidateScreenAgesOutExpiredAggregateClaim verifies SetState supplies a
// clock to validate: an aggregate story whose claim outlived its TTL must not
// render a W1 overlap against unrelated live work. With no Now injected,
// validate cannot evaluate expiry and the stale claim reads as active forever.
func TestValidateScreenAgesOutExpiredAggregateClaim(t *testing.T) {
	t.Parallel()
	now := time.Now().Unix()
	state := materialize.NewState()
	state.Issues["STORY-EXPIRED"] = &materialize.Issue{
		ID: "STORY-EXPIRED", Type: "story", Status: ops.StatusClaimed,
		ClaimedBy: "worker-a", ClaimedAt: now - 7200, LastHeartbeat: now - 7200, ClaimTTL: 60,
		Scope: []string{"cmd/armature/claim.go"}, Children: []string{"TSK-DONE"},
	}
	state.Issues["TSK-DONE"] = &materialize.Issue{
		ID: "TSK-DONE", Type: "task", Parent: "STORY-EXPIRED", Status: "done",
		Scope: []string{"cmd/armature/claim.go"},
	}
	state.Issues["TSK-NEW"] = &materialize.Issue{
		ID: "TSK-NEW", Type: "task", Scope: []string{"cmd/armature/claim.go"},
	}

	m := tuivalidate.New()
	m.SetState(state)
	if v := m.View(); strings.Contains(v, "scope overlap") {
		t.Errorf("expired aggregate claim must not render a W1 overlap, got:\n%s", v)
	}
}

func TestValidateScreenRendersIssues(t *testing.T) {
	t.Parallel()
	m := tuivalidate.New()
	state := materialize.NewState()
	// No issues -> should show OK.
	m.SetState(state)
	v := m.View()
	if !strings.Contains(v, "No issues found") {
		t.Errorf("expected OK, got:\n%s", v)
	}

	// Add an issue that causes an error.
	state.Issues["T1"] = &materialize.Issue{ID: "T1", Type: "task", Parent: "E1"} // E1 missing
	m.SetState(state)
	v = m.View()
	if !strings.Contains(v, "ERROR:") {
		t.Errorf("expected ERROR, got:\n%s", v)
	}
}
