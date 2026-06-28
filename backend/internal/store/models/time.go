package models

import "time"

// nowFn is the clock used for model-side defaults. Overridable in tests.
var nowFn = time.Now

// SetClock overrides the package clock (tests only) and returns a restore func.
func SetClock(fn func() time.Time) func() {
	prev := nowFn
	nowFn = fn
	return func() { nowFn = prev }
}
