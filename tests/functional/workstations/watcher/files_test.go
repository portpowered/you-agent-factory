//go:build functionallong

package watcher

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestWatcherSingleFileCompletesOneWork proves that dropping one watched seed
// file through the public process boundary creates and completes exactly one
// Work item in the Factory-configured success state, with no Work left in
// non-terminal states.
func TestWatcherSingleFileCompletesOneWork(t *testing.T) {
	support.SkipLongFunctional(t, "slow file-watcher single submission sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "filewatcher_flow"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "single item"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"processor": {
			{Content: "Done. COMPLETE"},
		},
	})

	session, listed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		10*time.Second,
	)

	if provider.CallCount("processor") != 1 {
		t.Fatalf("provider call count = %d, want 1 for single watched file", provider.CallCount("processor"))
	}
	if session.Runtime.Progress.Categories.Terminal != 1 || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf(
			"session progress categories = %+v, want one terminal and zero failed",
			session.Runtime.Progress.Categories,
		)
	}
	if got := len(listed.Results); got != 1 {
		t.Fatalf("listed Work count = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "complete")); got != 1 {
		t.Fatalf("CountWorkAtCustomerState(task:complete) = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "init")); got != 0 {
		t.Fatalf("CountWorkAtCustomerState(task:init) = %d, want 0 after completion", got)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "processing")); got != 0 {
		t.Fatalf("CountWorkAtCustomerState(task:processing) = %d, want 0 after completion", got)
	}

	workID := support.StringPointerValue(listed.Results[0].WorkId)
	if workID == "" {
		t.Fatalf("completed Work has empty work ID: %#v", listed.Results[0])
	}
	if !support.HasWorkAtCustomerState(listed, workID, support.WorkCustomerLocation("task", "complete")) {
		t.Fatalf("HasWorkAtCustomerState(%q, task:complete) = false; listed=%#v", workID, listed)
	}
	if listed.Results[0].State == nil || listed.Results[0].State.Type != factoryapi.WorkStateTypeTERMINAL {
		t.Fatalf("completed Work state type = %#v, want TERMINAL", listed.Results[0].State)
	}
}

// TestWatcherSequentialFilesAllComplete proves that dropping multiple watched
// seed files in sequence through the public process boundary admits each file
// and completes one Work per seed in the Factory-configured success state,
// with no Work left in non-terminal states.
func TestWatcherSequentialFilesAllComplete(t *testing.T) {
	support.SkipLongFunctional(t, "slow file-watcher sequential submission sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "filewatcher_flow"))
	const seedCount = 3
	for i := 1; i <= seedCount; i++ {
		testutil.WriteSeedFile(t, dir, "task", fmt.Appendf(nil, `{"title": "sequential item %d"}`, i))
	}

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"processor": {
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
		},
	})

	session, listed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		10*time.Second,
	)

	if provider.CallCount("processor") != seedCount {
		t.Fatalf("provider call count = %d, want %d for sequential watched files", provider.CallCount("processor"), seedCount)
	}
	if session.Runtime.Progress.Categories.Terminal != seedCount || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf(
			"session progress categories = %+v, want %d terminal and zero failed",
			session.Runtime.Progress.Categories,
			seedCount,
		)
	}
	if got := len(listed.Results); got != seedCount {
		t.Fatalf("listed Work count = %d, want %d; listed=%#v", got, seedCount, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "complete")); got != seedCount {
		t.Fatalf("CountWorkAtCustomerState(task:complete) = %d, want %d; listed=%#v", got, seedCount, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "init")); got != 0 {
		t.Fatalf("CountWorkAtCustomerState(task:init) = %d, want 0 after completion", got)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "processing")); got != 0 {
		t.Fatalf("CountWorkAtCustomerState(task:processing) = %d, want 0 after completion", got)
	}

	for i, work := range listed.Results {
		workID := support.StringPointerValue(work.WorkId)
		if workID == "" {
			t.Fatalf("completed Work[%d] has empty work ID: %#v", i, work)
		}
		if !support.HasWorkAtCustomerState(listed, workID, support.WorkCustomerLocation("task", "complete")) {
			t.Fatalf("HasWorkAtCustomerState(%q, task:complete) = false; listed=%#v", workID, listed)
		}
		if work.State == nil || work.State.Type != factoryapi.WorkStateTypeTERMINAL {
			t.Fatalf("completed Work[%d] state type = %#v, want TERMINAL", i, work.State)
		}
	}
}

// TestWatcherConcurrentFilesCompleteWithoutDuplicates proves that dropping
// multiple watched seed files together through the public process boundary
// admits each file once, completes exactly one Work per seed without duplicate
// Work, and leaves no Work in non-terminal states.
func TestWatcherConcurrentFilesCompleteWithoutDuplicates(t *testing.T) {
	support.SkipLongFunctional(t, "slow file-watcher concurrent submission sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "filewatcher_flow"))
	const seedCount = 5
	for i := 1; i <= seedCount; i++ {
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

	session, listed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		10*time.Second,
	)

	if provider.CallCount("processor") != seedCount {
		t.Fatalf("provider call count = %d, want %d for concurrent watched files", provider.CallCount("processor"), seedCount)
	}
	if session.Runtime.Progress.Categories.Terminal != seedCount || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf(
			"session progress categories = %+v, want %d terminal and zero failed",
			session.Runtime.Progress.Categories,
			seedCount,
		)
	}
	if got := len(listed.Results); got != seedCount {
		t.Fatalf("listed Work count = %d, want %d; listed=%#v", got, seedCount, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "complete")); got != seedCount {
		t.Fatalf("CountWorkAtCustomerState(task:complete) = %d, want %d; listed=%#v", got, seedCount, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "init")); got != 0 {
		t.Fatalf("CountWorkAtCustomerState(task:init) = %d, want 0 after completion", got)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "processing")); got != 0 {
		t.Fatalf("CountWorkAtCustomerState(task:processing) = %d, want 0 after completion", got)
	}

	seenWorkIDs := make(map[string]struct{}, seedCount)
	for i, work := range listed.Results {
		workID := support.StringPointerValue(work.WorkId)
		if workID == "" {
			t.Fatalf("completed Work[%d] has empty work ID: %#v", i, work)
		}
		if _, duplicate := seenWorkIDs[workID]; duplicate {
			t.Fatalf("duplicate Work ID %q among concurrent watched-file outcomes; listed=%#v", workID, listed)
		}
		seenWorkIDs[workID] = struct{}{}
		if !support.HasWorkAtCustomerState(listed, workID, support.WorkCustomerLocation("task", "complete")) {
			t.Fatalf("HasWorkAtCustomerState(%q, task:complete) = false; listed=%#v", workID, listed)
		}
		if work.State == nil || work.State.Type != factoryapi.WorkStateTypeTERMINAL {
			t.Fatalf("completed Work[%d] state type = %#v, want TERMINAL", i, work.State)
		}
	}
}

// TestWatcherMixedOutcomesLeaveNoNonTerminalWorkLeak proves that when watched
// seed files produce a mix of successful and failed Work outcomes, every item
// settles in a configured terminal state and no Work remains in non-terminal
// states such as init or processing.
func TestWatcherMixedOutcomesLeaveNoNonTerminalWorkLeak(t *testing.T) {
	support.SkipLongFunctional(t, "slow file-watcher mixed-outcome settlement sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "filewatcher_flow"))
	const seedCount = 5
	for i := 1; i <= seedCount; i++ {
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

	session, listed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		10*time.Second,
	)

	if session.Runtime.Progress.Categories.Terminal != 3 || session.Runtime.Progress.Categories.Failed != 2 {
		t.Fatalf(
			"session progress categories = %+v, want 3 terminal and 2 failed",
			session.Runtime.Progress.Categories,
		)
	}
	if got := len(listed.Results); got != seedCount {
		t.Fatalf("listed Work count = %d, want %d; listed=%#v", got, seedCount, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "complete")); got != 3 {
		t.Fatalf("CountWorkAtCustomerState(task:complete) = %d, want 3; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "failed")); got != 2 {
		t.Fatalf("CountWorkAtCustomerState(task:failed) = %d, want 2; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "init")); got != 0 {
		t.Fatalf("CountWorkAtCustomerState(task:init) = %d, want 0 after settlement", got)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "processing")); got != 0 {
		t.Fatalf("CountWorkAtCustomerState(task:processing) = %d, want 0 after settlement", got)
	}

	for i, work := range listed.Results {
		workID := support.StringPointerValue(work.WorkId)
		if workID == "" {
			t.Fatalf("Work[%d] has empty work ID: %#v", i, work)
		}
		if work.State == nil {
			t.Fatalf("Work[%d] state is nil: %#v", i, work)
		}
		switch work.State.Type {
		case factoryapi.WorkStateTypeTERMINAL:
			if work.State.Name != "complete" {
				t.Fatalf("terminal Work[%d] state name = %q, want complete; %#v", i, work.State.Name, work)
			}
		case factoryapi.WorkStateTypeFAILED:
			if work.State.Name != "failed" {
				t.Fatalf("failed Work[%d] state name = %q, want failed; %#v", i, work.State.Name, work)
			}
		default:
			t.Fatalf("Work[%d] state type = %q, want TERMINAL or FAILED; %#v", i, work.State.Type, work)
		}
	}
}

// TestWatcherDefaultChannelSubmission proves that a watched seed file dropped
// through the default channel path admits exactly one Work and completes it in
// the Factory-configured success state with no Work left in non-terminal states.
func TestWatcherDefaultChannelSubmission(t *testing.T) {
	support.SkipLongFunctional(t, "slow file-watcher default-channel submission sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "filewatcher_flow"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "default item"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"processor": {
			{Content: "Done. COMPLETE"},
		},
	})

	session, listed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		10*time.Second,
	)

	if provider.CallCount("processor") != 1 {
		t.Fatalf("provider call count = %d, want 1 for default-channel watched file", provider.CallCount("processor"))
	}
	if session.Runtime.Progress.Categories.Terminal != 1 || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf(
			"session progress categories = %+v, want one terminal and zero failed",
			session.Runtime.Progress.Categories,
		)
	}
	if got := len(listed.Results); got != 1 {
		t.Fatalf("listed Work count = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "complete")); got != 1 {
		t.Fatalf("CountWorkAtCustomerState(task:complete) = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "init")); got != 0 {
		t.Fatalf("CountWorkAtCustomerState(task:init) = %d, want 0 after completion", got)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "processing")); got != 0 {
		t.Fatalf("CountWorkAtCustomerState(task:processing) = %d, want 0 after completion", got)
	}
}

// TestWatcherExecutionIDDirectorySubmission proves that a watched seed file
// placed under an execution-id directory admits exactly one Work and completes
// it in the Factory-configured success state.
func TestWatcherExecutionIDDirectorySubmission(t *testing.T) {
	support.SkipLongFunctional(t, "slow file-watcher execution-id directory submission sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "filewatcher_flow"))

	execDir := filepath.Join(dir, "inputs", "task", "exec-123")
	if err := os.MkdirAll(execDir, 0o755); err != nil {
		t.Fatalf("create exec dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(execDir, "work-1.json"), []byte(`{"title": "executor work"}`), 0o644); err != nil {
		t.Fatalf("write work file: %v", err)
	}

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"processor": {
			{Content: "Done. COMPLETE"},
		},
	})

	session, listed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		10*time.Second,
	)

	if provider.CallCount("processor") != 1 {
		t.Fatalf("provider call count = %d, want 1 for execution-id directory watched file", provider.CallCount("processor"))
	}
	if session.Runtime.Progress.Categories.Terminal != 1 || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf(
			"session progress categories = %+v, want one terminal and zero failed",
			session.Runtime.Progress.Categories,
		)
	}
	if got := len(listed.Results); got != 1 {
		t.Fatalf("listed Work count = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "complete")); got != 1 {
		t.Fatalf("CountWorkAtCustomerState(task:complete) = %d, want 1; listed=%#v", got, listed)
	}
}

// TestWatcherCombinedDefaultAndDynamicExecDirectory proves that watched seed
// files submitted through both the default channel and a dynamic execution-id
// directory admit two Work items, each completing in the Factory-configured
// success state with no Work left in non-terminal states.
func TestWatcherCombinedDefaultAndDynamicExecDirectory(t *testing.T) {
	support.SkipLongFunctional(t, "slow file-watcher combined default and dynamic-exec-dir submission sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "filewatcher_flow"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "default work"}`))

	execDir := filepath.Join(dir, "inputs", "task", "exec-dynamic")
	if err := os.MkdirAll(execDir, 0o755); err != nil {
		t.Fatalf("create exec dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(execDir, "work.json"), []byte(`{"title": "exec work"}`), 0o644); err != nil {
		t.Fatalf("write work file: %v", err)
	}

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"processor": {
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
		},
	})

	session, listed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		10*time.Second,
	)

	if provider.CallCount("processor") != 2 {
		t.Fatalf("provider call count = %d, want 2 for combined default and exec-dir watched files", provider.CallCount("processor"))
	}
	if session.Runtime.Progress.Categories.Terminal != 2 || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf(
			"session progress categories = %+v, want two terminal and zero failed",
			session.Runtime.Progress.Categories,
		)
	}
	if got := len(listed.Results); got != 2 {
		t.Fatalf("listed Work count = %d, want 2; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "complete")); got != 2 {
		t.Fatalf("CountWorkAtCustomerState(task:complete) = %d, want 2; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "init")); got != 0 {
		t.Fatalf("CountWorkAtCustomerState(task:init) = %d, want 0 after completion", got)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "processing")); got != 0 {
		t.Fatalf("CountWorkAtCustomerState(task:processing) = %d, want 0 after completion", got)
	}
}
