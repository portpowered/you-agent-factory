package main

import "runtime"

const (
	defaultUnitLaneJobs        = 32
	maximumUnitLaneJobs        = 32
	unitLaneJobsPerParallelism = 2
)

// effectiveUnitJobs keeps the single go test scheduler below the measured
// ceiling while allowing small hosts to avoid launching more package work
// than their configured Go parallelism can reasonably support.
func effectiveUnitJobs(requested int) int {
	return boundedUnitJobs(requested, runtime.GOMAXPROCS(0))
}

func boundedUnitJobs(requested, parallelism int) int {
	if requested < 1 {
		requested = 1
	}
	limit := unitLaneJobLimit(parallelism)
	if requested > limit {
		return limit
	}
	return requested
}

func unitLaneJobLimit(parallelism int) int {
	if parallelism < 1 {
		return 1
	}
	if parallelism > maximumUnitLaneJobs/unitLaneJobsPerParallelism {
		return maximumUnitLaneJobs
	}
	limit := parallelism * unitLaneJobsPerParallelism
	if limit > maximumUnitLaneJobs {
		return maximumUnitLaneJobs
	}
	return limit
}
