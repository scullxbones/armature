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

func TestFixedIsIdempotent(t *testing.T) {
	t.Parallel()
	fixedTime := int64(1609459200) // 2021-01-01 00:00:00 UTC in seconds
	clock := Fixed(fixedTime)

	// Call the clock multiple times
	result1 := clock()
	result2 := clock()
	result3 := clock()

	// Verify all calls return the same value
	if result1 != fixedTime {
		t.Errorf("Fixed(%d) returned %d on first call, expected %d", fixedTime, result1, fixedTime)
	}
	if result2 != fixedTime {
		t.Errorf("Fixed(%d) returned %d on second call, expected %d", fixedTime, result2, fixedTime)
	}
	if result3 != fixedTime {
		t.Errorf("Fixed(%d) returned %d on third call, expected %d", fixedTime, result3, fixedTime)
	}
}
