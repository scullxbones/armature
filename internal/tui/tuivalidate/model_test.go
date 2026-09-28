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
	m.SetSize(80, 24)
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

func TestValidateScreenUsesInjectedClock_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()
	claimedAt := int64(1_700_000_000)
	expiredState := materialize.NewState()
	expiredState.Issues["STORY-EXPIRED"] = &materialize.Issue{
		ID: "STORY-EXPIRED", Type: "story", Status: ops.StatusClaimed,
		ClaimedBy: "worker-a", ClaimedAt: claimedAt - 7200, LastHeartbeat: claimedAt - 7200, ClaimTTL: 60,
		Scope: []string{"cmd/armature/claim.go"}, Children: []string{"TSK-DONE"},
	}
	expiredState.Issues["TSK-DONE"] = &materialize.Issue{
		ID: "TSK-DONE", Type: "task", Parent: "STORY-EXPIRED", Status: "done",
		Scope: []string{"cmd/armature/claim.go"},
	}
	expiredState.Issues["TSK-NEW"] = &materialize.Issue{
		ID: "TSK-NEW", Type: "task", Scope: []string{"cmd/armature/claim.go"},
	}

	stale := tuivalidate.NewWithClock(func() int64 { return claimedAt })
	stale.SetState(expiredState)
	if v := stale.View(); strings.Contains(v, "scope overlap") {
		t.Errorf("injected clock past TTL must not render a W1 overlap, got:\n%s", v)
	}

	liveState := materialize.NewState()
	liveState.Issues["STORY-LIVE"] = &materialize.Issue{
		ID: "STORY-LIVE", Type: "story", Status: ops.StatusClaimed,
		ClaimedBy: "worker-a", ClaimedAt: claimedAt, LastHeartbeat: claimedAt, ClaimTTL: 60,
		Scope: []string{"cmd/armature/claim.go"}, Children: []string{"TSK-DONE"},
	}
	liveState.Issues["TSK-DONE"] = &materialize.Issue{
		ID: "TSK-DONE", Type: "task", Parent: "STORY-LIVE", Status: "done",
		Scope: []string{"cmd/armature/claim.go"},
	}
	liveState.Issues["TSK-NEW"] = &materialize.Issue{
		ID: "TSK-NEW", Type: "task", Scope: []string{"cmd/armature/claim.go"},
	}

	live := tuivalidate.NewWithClock(func() int64 { return claimedAt + 10 })
	live.SetState(liveState)
	if v := live.View(); !strings.Contains(v, "scope overlap") {
		t.Errorf("injected clock within TTL must render a W1 overlap, got:\n%s", v)
	}
}

func TestValidateScreenRendersIssues(t *testing.T) {
	t.Parallel()
	m := tuivalidate.New()
	state := materialize.NewState()
	m.SetState(state)
	v := m.View()
	if !strings.Contains(v, "No issues found") {
		t.Errorf("expected OK, got:\n%s", v)
	}

	state.Issues["T1"] = &materialize.Issue{ID: "T1", Type: "task", Parent: "E1"}
	m.SetState(state)
	v = m.View()
	if !strings.Contains(v, "ERROR:") {
		t.Errorf("expected ERROR, got:\n%s", v)
	}
}
