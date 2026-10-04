package cancel_test

import "testing"

// TestRetireExitedTargetProcessesIgnoresPIDReuse proves a target PID observed
// absent stays retired, so a later unrelated process that reuses the number
// cannot read as survival of the canceled process tree.
func TestRetireExitedTargetProcessesIgnoresPIDReuse(t *testing.T) {
	target := workerProcessTree{RootPID: 10, PIDs: []int{10, 11, 12}}
	retired := map[int]struct{}{}

	first := retireExitedTargetProcesses(cancelProcessSample{TargetPresent: []int{}}, target, retired)
	if len(first.TargetPresent) != 0 || len(retired) != 3 {
		t.Fatalf("first absent sample = %#v retired=%v, want all target PIDs retired", first, retired)
	}

	reused := retireExitedTargetProcesses(cancelProcessSample{
		TargetPresent:     []int{11},
		TargetDescendants: []int{10, 11},
	}, target, retired)
	if len(reused.TargetPresent) != 0 || len(reused.TargetDescendants) != 0 {
		t.Fatalf("reused-PID sample = %#v, want reuse masked", reused)
	}

	live := map[int]struct{}{}
	survivor := retireExitedTargetProcesses(cancelProcessSample{
		TargetPresent:     []int{11},
		TargetDescendants: []int{10, 11},
	}, target, live)
	if len(survivor.TargetPresent) != 1 || len(survivor.TargetDescendants) != 2 {
		t.Fatalf("survivor sample = %#v, want a never-absent PID reported", survivor)
	}
}
