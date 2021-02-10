package util

import (
	"time"
)

// FloatSecToDuration converts a float representing number of seconds to time.Duration.
// I.e., 1.3 => 1300 milliseconds
func FloatSecToDuration(seconds float32) time.Duration {
	return time.Duration(int(seconds*1000)) * time.Millisecond
}
