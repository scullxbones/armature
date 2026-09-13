package clock

import (
	"testing"
	"time"
)

func TestSystemReturnsWallTime(t *testing.T) {
	t.Parallel()
	before := time.Now().Unix()
	ts := System()
	after := time.Now().Unix()

	if ts < before || ts > after {
		t.Errorf("System() returned %d, expected value between %d and %d", ts, before, after)
	}
}
