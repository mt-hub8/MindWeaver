// Package platform contains the smallest shared runtime contracts used across
// MindWeaver domains.
package platform

import "time"

// Clock makes time an explicit dependency. Domain code must not call
// time.Now directly, which keeps durable transitions deterministic.
type Clock interface {
	Now() time.Time
}

// SystemClock is the production wall clock.
type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}
