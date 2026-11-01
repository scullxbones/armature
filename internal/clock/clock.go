// Package clock provides a narrow time-source seam so callers can inject a
// deterministic clock in tests instead of depending on wall-clock time.
package clock

import "time"

// Clock is a function type that returns the current time as a Unix timestamp in seconds.
type Clock func() int64

// System returns the current wall-clock time as a Unix timestamp in seconds.
// The function value itself is Clock-compatible (pass clock.System where a Clock is needed).
func System() int64 {
	return time.Now().Unix()
}
