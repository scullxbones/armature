package tui_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/tui"
)

func TestNewSpinner_ReturnsModel(t *testing.T) {
	s := tui.NewSpinner()
	// Spinner model is a value type; verify it has a non-zero state by checking
	// that the ID is set (bubbletea assigns an ID > 0 on construction).
	if s.ID() == 0 {
		t.Error("expected non-zero spinner ID")
	}
}

func TestNewProgressBar_ReturnsModel(t *testing.T) {
	// Smoke test: construction must not panic.
	_ = tui.NewProgressBar()
}

func TestNewTable_ReturnsModel(t *testing.T) {
	// Smoke test: construction must not panic.
	_ = tui.NewTable()
}

func TestNewViewport_ReturnsModel(t *testing.T) {
	vp := tui.NewViewport(80, 24)
	if vp.Width != 80 {
		t.Errorf("expected Width=80, got %d", vp.Width)
	}
	if vp.Height != 24 {
		t.Errorf("expected Height=24, got %d", vp.Height)
	}
}
