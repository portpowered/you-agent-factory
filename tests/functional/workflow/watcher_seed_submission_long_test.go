//go:build functionallong

package workflow

import (
	"fmt"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestFileWatcherFlowSingle drops 1 seed file and verifies it is picked up
// by preseed, processed through the service pipeline, and reaches the terminal state.
func TestFileWatcherFlowSingle(t *testing.T) {
	support.SkipLongFunctional(t, "slow file-watcher single submission sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "filewatcher_flow"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "single item"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"processor": {
			{Content: "Done. COMPLETE"},
		},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing").
		TokenCount(1)
}

// TestFileWatcherFlowSequential drops 3 seed files and verifies all 3
// are picked up by preseed and reach terminal state.
func TestFileWatcherFlowSequential(t *testing.T) {
	support.SkipLongFunctional(t, "slow file-watcher sequential submission sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "filewatcher_flow"))
	for i := 1; i <= 3; i++ {
		testutil.WriteSeedFile(t, dir, "task", fmt.Appendf(nil, `{"title": "sequential item %d"}`, i))
	}

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"processor": {
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
		},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing").
		PlaceTokenCount("task:complete", 3).
		TokenCount(3)
}

// TestFileWatcherFlowConcurrent drops 5 seed files simultaneously and verifies
// all 5 are picked up by preseed and reach terminal state.
func TestFileWatcherFlowConcurrent(t *testing.T) {
	support.SkipLongFunctional(t, "slow file-watcher concurrent submission sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "filewatcher_flow"))
	for i := 1; i <= 5; i++ {
		testutil.WriteSeedFile(t, dir, "task", fmt.Appendf(nil, `{"title": "concurrent item %d"}`, i))
	}

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"processor": {
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
		},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing").
		PlaceTokenCount("task:complete", 5).
		TokenCount(5)
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
		workers.CommandResult{Stdout: []byte("Done. COMPLETE")},
		workers.CommandResult{Stdout: []byte("Done. COMPLETE")},
		workers.CommandResult{Stderr: []byte("error"), ExitCode: 1},
		workers.CommandResult{Stdout: []byte("Done. COMPLETE")},
		workers.CommandResult{Stderr: []byte("error"), ExitCode: 1},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:complete", 3).
		PlaceTokenCount("task:failed", 2).
		TokenCount(5)

	snap := h.Marking()
	for _, tok := range snap.Tokens {
		if tok.PlaceID != "task:complete" && tok.PlaceID != "task:failed" {
			t.Errorf("token leak: token %s stuck in non-terminal place %s", tok.ID, tok.PlaceID)
		}
	}
}
