//go:build !windows

package processmemory

// CurrentCommit reports unavailable rather than substituting RSS, resident
// set, or working-set measurements for process commit.
func CurrentCommit() (uint64, error) {
	return 0, ErrUnavailable
}
