package main

import "testing"

func TestBoundedUnitJobsUsesMeasuredCeilingAndHostParallelism(t *testing.T) {
	tests := []struct {
		name        string
		requested   int
		parallelism int
		want        int
	}{
		{name: "invalid request", requested: 0, parallelism: 24, want: 1},
		{name: "invalid host parallelism", requested: 32, parallelism: 0, want: 1},
		{name: "small host cap", requested: 32, parallelism: 1, want: 2},
		{name: "measured host ceiling", requested: 32, parallelism: 24, want: 32},
		{name: "oversized request", requested: 1000, parallelism: 24, want: 32},
		{name: "explicit lower request", requested: 7, parallelism: 24, want: 7},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := boundedUnitJobs(test.requested, test.parallelism); got != test.want {
				t.Fatalf("boundedUnitJobs(%d, %d) = %d, want %d", test.requested, test.parallelism, got, test.want)
			}
		})
	}
}
