//go:build linux

package processmemory

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// CurrentRSS returns the current process resident-set size from the Linux
// process boundary. It is intentionally separate from CurrentCommit: the
// replay resource audit must not silently substitute one measure for another.
func CurrentRSS() (uint64, error) {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, fmt.Errorf("read process RSS: %w", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0, fmt.Errorf("read process RSS: malformed statm")
	}
	residentPages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("read process RSS: resident pages: %w", err)
	}
	return residentPages * uint64(os.Getpagesize()), nil
}
