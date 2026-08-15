package main

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

const (
	defaultUnitLaneJobCount        = 2
	defaultExpectedConcurrentLanes = 4
	expectedConcurrentLanesEnv     = "YOU_EXPECTED_CONCURRENT_LANES"
	logicalCPUsEnv                 = "YOU_LOGICAL_CPUS"
)

// defaultUnitLaneJobs applies the same bounded host-share policy as the
// Makefile. YOU_LOGICAL_CPUS is an optional controlled-probe override; normal
// execution uses the runtime's logical CPU count.
func defaultUnitLaneJobs() int {
	logicalCPUs := runtime.NumCPU()
	if raw, ok := os.LookupEnv(logicalCPUsEnv); ok {
		parsed, valid := positiveDecimal(raw)
		if !valid {
			return defaultUnitLaneJobCount
		}
		logicalCPUs = parsed
	}

	expectedLanes := strconv.Itoa(defaultExpectedConcurrentLanes)
	if raw, ok := os.LookupEnv(expectedConcurrentLanesEnv); ok {
		expectedLanes = raw
	}
	return boundedUnitLaneJobs(logicalCPUs, expectedLanes)
}

func boundedUnitLaneJobs(logicalCPUs int, expectedLanes string) int {
	divisor, valid := positiveDecimal(expectedLanes)
	if logicalCPUs < 1 || !valid {
		return defaultUnitLaneJobCount
	}

	jobs := logicalCPUs / divisor
	if jobs < defaultUnitLaneJobCount {
		return defaultUnitLaneJobCount
	}
	return jobs
}

func positiveDecimal(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	for _, char := range raw {
		if char < '0' || char > '9' {
			return 0, false
		}
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 0, false
	}
	return value, true
}
