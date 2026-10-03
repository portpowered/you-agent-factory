package support

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// TimeoutScaleEnv overrides the functional deadline multiplier. A value
	// of 1 restores the unscaled budget.
	TimeoutScaleEnv = "FUNCTIONAL_TIMEOUT_SCALE"
	// ciTimeoutScale is the default multiplier on CI runners, where many
	// package binaries and root-built processes start concurrently on shared
	// cores and bootstrap/readiness runs several times slower than locally.
	ciTimeoutScale = 4
	// windowsLocalTimeoutScale covers local Windows hosts, where root-built
	// process construction (antivirus scanning, slow file creation, many
	// concurrent lanes) measured ~10-13 s against the unscaled 15 s readiness
	// budget. Linux hosts keep the unscaled budget.
	windowsLocalTimeoutScale = 4
)

func localTimeoutScale() int {
	if runtime.GOOS == "windows" {
		return windowsLocalTimeoutScale
	}
	return 1
}

var functionalTimeoutScale = resolveTimeoutScale(os.Getenv)

// ScaledTimeout is the one shared startup/readiness budget for functional
// tests. Deadlines passed through it are failure ceilings, not sleeps:
// every wait still returns the moment its condition is observed. It must not
// be used for negative assertions that wait out a window to prove absence.
func ScaledTimeout(base time.Duration) time.Duration {
	return base * time.Duration(functionalTimeoutScale)
}

func resolveTimeoutScale(getenv func(string) string) int {
	if raw := strings.TrimSpace(getenv(TimeoutScaleEnv)); raw != "" {
		if scale, err := strconv.Atoi(raw); err == nil && scale >= 1 {
			return scale
		}
	}
	if getenv("GITHUB_ACTIONS") == "true" || getenv("CI") == "true" {
		return ciTimeoutScale
	}
	return localTimeoutScale()
}
