//go:build functionallong

package workflow

import (
	"fmt"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestFileWatcherFlowSingle drops 1 seed file and verifies it is picked up
// by preseed, processed through the service pipeline, and reaches the terminal state.
func TestFileWatcherFlowSingle(t *testing.T) {
	support.SkipLongFunctional(t, "slow file-watcher single submission sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "filewatcher_flow"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "single item"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"processor": {
			{Content: "Done. COMPLETE"},
		},
	})

	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second)
	assertWorkflowSessionPlaces(t, listed, map[string]int{"task:complete": 1, "task:init": 0, "task:processing": 0})
}

// TestFileWatcherFlowSequential drops 3 seed files and verifies all 3
// are picked up by preseed and reach terminal state.
func TestFileWatcherFlowSequential(t *testing.T) {
	support.SkipLongFunctional(t, "slow file-watcher sequential submission sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "filewatcher_flow"))
	for i := 1; i <= 3; i++ {
		testutil.WriteSeedFile(t, dir, "task", fmt.Appendf(nil, `{"title": "sequential item %d"}`, i))
	}

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"processor": {
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
		},
	})

	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second)
	assertWorkflowSessionPlaces(t, listed, map[string]int{"task:init": 0, "task:processing": 0, "task:complete": 3})
}

// TestFileWatcherFlowConcurrent drops 5 seed files simultaneously and verifies
// all 5 are picked up by preseed and reach terminal state.
func TestFileWatcherFlowConcurrent(t *testing.T) {
	support.SkipLongFunctional(t, "slow file-watcher concurrent submission sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "filewatcher_flow"))
	for i := 1; i <= 5; i++ {
		testutil.WriteSeedFile(t, dir, "task", fmt.Appendf(nil, `{"title": "concurrent item %d"}`, i))
	}

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"processor": {
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
		},
	})

	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second)
	assertWorkflowSessionPlaces(t, listed, map[string]int{"task:init": 0, "task:processing": 0, "task:complete": 5})
}

// TestFileWatcherFlowNoTokenLeaks verifies that after processing a mix
// of successful and failed work via seed files, no tokens remain in
// non-terminal places.
func TestFileWatcherFlowNoTokenLeaks(t *testing.T) {
	support.SkipLongFunctional(t, "slow file-watcher token-leak sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "filewatcher_flow"))
	for i := 1; i <= 5; i++ {
		testutil.WriteSeedFile(t, dir, "task", fmt.Appendf(nil, `{"title": "item %d"}`, i))
	}

	// Pre-load results: succeed, succeed, fail, succeed, fail.
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("Done. COMPLETE")},
		platformprocess.CommandResult{Stdout: []byte("Done. COMPLETE")},
		platformprocess.CommandResult{Stderr: []byte("error"), ExitCode: 1},
		platformprocess.CommandResult{Stdout: []byte("Done. COMPLETE")},
		platformprocess.CommandResult{Stderr: []byte("error"), ExitCode: 1},
	)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 10*time.Second)
	assertWorkflowSessionPlaces(t, listed, map[string]int{
		"task:complete": 3, "task:failed": 2, "task:init": 0, "task:processing": 0,
	})
}
