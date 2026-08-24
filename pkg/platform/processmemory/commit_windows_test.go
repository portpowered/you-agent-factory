//go:build windows

package processmemory

import "testing"

func TestPagefileUsageBytesUsesCommitField(t *testing.T) {
	got := pagefileUsageBytes(processMemoryCounters{
		workingSetSize:    43_524 * 1024 * 1024,
		pagefileUsage:     70 * 1024 * 1024 * 1024,
		peakPagefileUsage: 75 * 1024 * 1024 * 1024,
	})
	want := uint64(70 * 1024 * 1024 * 1024)
	if got != want {
		t.Fatalf("PagefileUsage commit = %d, want %d", got, want)
	}
}
