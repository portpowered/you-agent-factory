//go:build windows

package processmemory

// CurrentRSS returns the Windows working-set size for the current process.
// Working set is the resident-memory analogue used by the replay resource
// audit; committed memory remains available through CurrentCommit.
func CurrentRSS() (uint64, error) {
	var counters processMemoryCounters
	if err := readProcessMemoryCounters(&counters); err != nil {
		return 0, err
	}
	return uint64(counters.workingSetSize), nil
}
