package clock

import "time"

// Clock is a function type that returns the current time as a Unix timestamp in milliseconds.
// This type enables dependency injection of time sources into domain packages,
// supporting both real wall-clock time and deterministic fixed time for testing.
type Clock func() int64

// System is a Clock that returns the current wall-clock time as a Unix timestamp in milliseconds.
var System Clock = func() int64 {
	return time.Now().UnixMilli()
}

// Fixed returns a Clock that always returns the same timestamp.
// This is used for deterministic testing and scenarios where time should not advance.
func Fixed(ts int64) Clock {
	return func() int64 {
		return ts
	}
}
