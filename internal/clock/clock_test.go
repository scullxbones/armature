package clock

import (
	"testing"
	"time"
)

func TestSystemReturnsWallTime(t *testing.T) {
	// Get time before calling System
	before := time.Now().UnixMilli()

	// Call System Clock
	ts := System()

	// Get time after calling System
	after := time.Now().UnixMilli()

	// Verify that the returned timestamp is within the expected range
	if ts < before || ts > after {
		t.Errorf("System() returned %d, expected value between %d and %d", ts, before, after)
	}
}

func TestFixedIsIdempotent(t *testing.T) {
	// Create a fixed clock with a specific timestamp
	fixedTime := int64(1609459200000) // 2021-01-01 00:00:00 UTC in milliseconds
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
