//go:build !linux && !windows

package processmemory

// CurrentRSS reports unavailable rather than substituting commit, heap, or
// another non-resident memory measure.
func CurrentRSS() (uint64, error) {
	return 0, ErrRSSUnavailable
}
