package percenter

import "time"

// RebenchmarkState resets a segment to the same baseline state used by the
// automatic optimizer while preserving the current campaign/type/profit model
// and monotonically advancing point_version. It is intended for explicit
// administrative rebenchmark requests only.
func RebenchmarkState(state State, now time.Time) State {
	return resetState(state, now)
}
