//go:build functionallong

package watcher

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/http/apitypes"
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
	provider := testutil.NewMockWorkerMapProviderWithDefault(map[string][]testutil.WorkResponse{
		"processor": {
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
			{Error: errors.New("processing failed")},
			{Content: "Done. COMPLETE"},
			{Error: errors.New("processing failed")},
		},
	})

	session, listed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
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

// TestWatcherParentChildBatchFanIn proves that submitting a canonical
// PARENT_CHILD batch through a watched file admits the batch via watcher ingress
// and routes parent-aware failure when a child story fails, leaving Work in the
// documented terminal states with no non-terminal Work leak.
func TestWatcherParentChildBatchFanIn(t *testing.T) {
	support.SkipLongFunctional(t, "slow watcher parent-child batch fan-in sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "submitted_parent_child_filewatcher"))
	writeSubmittedParentChildBatch(t, dir)

	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("Story completed. COMPLETE")},
		platformprocess.CommandResult{ExitCode: 1, Stderr: []byte("story processing failed")},
		platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("Story set failed. COMPLETE")},
	)

	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		15*time.Second,
	)

	wantStates := map[string]int{
		support.WorkCustomerLocation("story", "complete"):     1,
		support.WorkCustomerLocation("story", "failed"):       1,
		support.WorkCustomerLocation("story-set", "failed"):   1,
		support.WorkCustomerLocation("story", "init"):         0,
		support.WorkCustomerLocation("story-set", "waiting"):  0,
		support.WorkCustomerLocation("story-set", "complete"): 0,
	}
	for location, want := range wantStates {
		if got := support.CountWorkAtCustomerState(listed, location); got != want {
			t.Fatalf("CountWorkAtCustomerState(%s) = %d, want %d; listed=%#v", location, got, want, listed)
		}
	}

	if runner.CallCount() != 3 {
		t.Fatalf("provider command runner calls = %d, want 3 (two story dispatches then parent failure handler)", runner.CallCount())
	}
	assertWatchedParentChildProviderRequests(t, runner)

	assertWatchedParentChildRequestRecorded(t, events)
	assertParentFailedOnlyAfterChildFailure(t, events)
}

func assertWatchedParentChildProviderRequests(t *testing.T, runner *testutil.ProviderCommandRunner) {
	t.Helper()

	requests := runner.Requests()
	if len(requests) != 3 {
		t.Fatalf("provider command requests = %d, want 3", len(requests))
	}
	for index, request := range requests {
		if strings.TrimSpace(request.Command) == "" {
			t.Fatalf("provider command request %d missing command: %#v", index, request)
		}
		if len(request.Args) == 0 {
			t.Fatalf("provider command request %d missing args: %#v", index, request)
		}
	}
}

// TestWatcherExecutionFollowsCurrentFactorySwitch proves that after activating
// a second Current Factory, a watched file under the activated Factory admits
// and completes Work on the active run while a watched file under the
// deactivated Factory watch path does not create additional Work.
func TestWatcherExecutionFollowsCurrentFactorySwitch(t *testing.T) {
	support.SkipLongFunctional(t, "slow current-factory watcher switch sweep")

	rootDir := t.TempDir()
	srcDir := support.LegacyFixtureDir(t, "filewatcher_flow")
	sourcePath := filepath.Join(srcDir, interfaces.FactoryConfigFile)

	_ = support.CreateAndActivateNamedFactoryAtRoot(t, srcDir, rootDir, "alpha", sourcePath)
	betaDir := support.CreateNamedFactoryAtRoot(t, srcDir, rootDir, "beta", sourcePath)

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"processor": {
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
		},
	})

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                rootDir,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})
	baseURL := server.URL()
	support.WaitForRuntimeIdle(t, baseURL, 10*time.Second)

	activateNamedFilewatcherFactoryOverHTTP(t, baseURL, namedFilewatcherFactoryPayload(t, "beta"))
	assertCurrentFactoryReadback(t, baseURL, "beta", betaDir)

	testutil.WriteSeedFile(t, betaDir, "task", []byte(`{"title":"beta watched work"}`))
	waitForWatcherWorkSettlement(t, baseURL, provider, 1, 10*time.Second)

	listed := support.ListDefaultSessionWork(t, baseURL)
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "complete")); got != 1 {
		t.Fatalf("CountWorkAtCustomerState(task:complete) = %d, want 1 after beta watched file; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "init")); got != 0 {
		t.Fatalf("CountWorkAtCustomerState(task:init) = %d, want 0 after beta completion", got)
	}
	if provider.CallCount("processor") != 1 {
		t.Fatalf("provider call count = %d, want 1 after beta watched file", provider.CallCount("processor"))
	}

	alphaDir := filepath.Join(rootDir, "alpha")
	testutil.WriteSeedFile(t, alphaDir, "task", []byte(`{"title":"alpha watched work"}`))
	assertNoAdditionalWatcherWork(t, baseURL, provider, 10*time.Second)

	listed = support.ListDefaultSessionWork(t, baseURL)
	if got := len(listed.Results); got != 1 {
		t.Fatalf("listed Work count = %d, want 1 after deactivated-factory write; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "complete")); got != 1 {
		t.Fatalf("CountWorkAtCustomerState(task:complete) = %d, want 1 after deactivated-factory write; listed=%#v", got, listed)
	}
	if provider.CallCount("processor") != 1 {
		t.Fatalf("provider call count = %d, want 1 after deactivated-factory write", provider.CallCount("processor"))
	}
}

func namedFilewatcherFactoryPayload(t *testing.T, name string) []byte {
	t.Helper()

	sourcePath := filepath.Join(support.LegacyFixtureDir(t, "filewatcher_flow"), interfaces.FactoryConfigFile)
	factory, err := support.LoadedFactory(t, sourcePath)
	if err != nil {
		t.Fatalf("LoadedFactory: %v", err)
	}
	factory.Name = factoryapi.FactoryName(name)
	id := name
	factory.Id = &id
	payload, err := json.Marshal(factory)
	if err != nil {
		t.Fatalf("marshal named filewatcher factory payload: %v", err)
	}
	return payload
}

func activateNamedFilewatcherFactoryOverHTTP(t *testing.T, baseURL string, payload []byte) {
	t.Helper()

	var factory factoryapi.Factory
	if err := json.Unmarshal(payload, &factory); err != nil {
		t.Fatalf("decode named factory API payload: %v", err)
	}
	factory.Version = &factoryapi.HybridLogicalTimestamp{
		Logical:  apitypes.Int64String(1<<62 - 1),
		Physical: time.Now().UTC().Add(time.Hour),
	}
	mode := factoryapi.FactorySaveModeUpsertNamedAndActivate
	body, err := json.Marshal(factoryapi.SaveFactoryForSessionRequest{
		Factory: factory,
		Mode:    &mode,
	})
	if err != nil {
		t.Fatalf("encode named factory activation: %v", err)
	}
	request, err := http.NewRequest(
		http.MethodPut,
		baseURL+"/factory-sessions/~default/factory",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("build named factory activation request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("activate named factory over HTTP: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"activate named factory status = %d, want 200: %s",
			response.StatusCode,
			string(responseBody),
		)
	}
}

func assertCurrentFactoryReadback(t *testing.T, baseURL, wantName, wantDir string) {
	t.Helper()

	current := support.GetJSON[factoryapi.Factory](t, baseURL+"/factory-sessions/~default/factory")
	if current.Name != factoryapi.FactoryName(wantName) {
		t.Fatalf("current factory name = %q, want %q", current.Name, wantName)
	}
	if current.FactoryDirectory == nil || *current.FactoryDirectory != wantDir {
		t.Fatalf("current factory directory = %#v, want %q", current.FactoryDirectory, wantDir)
	}
}

func waitForWatcherWorkSettlement(
	t *testing.T,
	baseURL string,
	provider *testutil.MockWorkerMapProvider,
	wantCalls int,
	timeout time.Duration,
) {
	t.Helper()

	support.WaitForStatus(t, baseURL, timeout, func(status factoryapi.StatusResponse) bool {
		if status.RuntimeStatus != string(interfaces.RuntimeStatusIdle) {
			return false
		}
		if provider.CallCount("processor") != wantCalls {
			return false
		}
		listed := support.ListDefaultSessionWork(t, baseURL)
		return len(listed.Results) == wantCalls &&
			support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "complete")) == wantCalls &&
			support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "init")) == 0 &&
			support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "processing")) == 0
	})
}

func assertNoAdditionalWatcherWork(
	t *testing.T,
	baseURL string,
	provider *testutil.MockWorkerMapProvider,
	timeout time.Duration,
) {
	t.Helper()

	const stableWindow = 300 * time.Millisecond
	deadline := time.Now().Add(timeout)
	var stableSince time.Time

	for time.Now().Before(deadline) {
		if provider.CallCount("processor") > 1 {
			t.Fatalf(
				"deactivated factory watch path triggered additional work: provider calls = %d, want 1",
				provider.CallCount("processor"),
			)
		}

		listed := support.ListDefaultSessionWork(t, baseURL)
		status := support.GetJSON[factoryapi.StatusResponse](t, baseURL+"/status")
		stable := provider.CallCount("processor") == 1 &&
			len(listed.Results) == 1 &&
			support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "complete")) == 1 &&
			support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "init")) == 0 &&
			support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "processing")) == 0 &&
			status.RuntimeStatus == string(interfaces.RuntimeStatusIdle)

		if stable {
			if stableSince.IsZero() {
				stableSince = time.Now()
			} else if time.Since(stableSince) >= stableWindow {
				return
			}
		} else {
			stableSince = time.Time{}
		}
		time.Sleep(25 * time.Millisecond)
	}

	listed := support.ListDefaultSessionWork(t, baseURL)
	t.Fatalf(
		"timed out waiting for stable no-additional-work observation: provider_calls=%d listed=%#v",
		provider.CallCount("processor"),
		listed.Results,
	)
}

func writeSubmittedParentChildBatch(t *testing.T, dir string) {
	t.Helper()

	batchDir := filepath.Join(dir, "inputs", "BATCH", "default")
	if err := os.MkdirAll(batchDir, 0o755); err != nil {
		t.Fatalf("create batch input dir: %v", err)
	}
	batchJSON := []byte(`{
  "requestId": "release-story-set",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "story-set",
      "workTypeName": "story-set",
      "state": "waiting",
      "payload": {"title": "April release story set"},
      "tags": {"project": "sample-service", "branch": "ralph/april-release"}
    },
    {
      "name": "story-auth",
      "workTypeName": "story",
      "payload": {"title": "Harden auth session handling"},
      "tags": {"project": "sample-service", "branch": "ralph/april-release"}
    },
    {
      "name": "story-billing",
      "workTypeName": "story",
      "payload": {"title": "Polish billing retry UX"},
      "tags": {"project": "sample-service", "branch": "ralph/april-release"}
    }
  ],
  "relations": [
    {"type": "PARENT_CHILD", "sourceWorkName": "story-auth", "targetWorkName": "story-set"},
    {"type": "PARENT_CHILD", "sourceWorkName": "story-billing", "targetWorkName": "story-set"}
  ]
}`)
	if err := os.WriteFile(filepath.Join(batchDir, "release-story-set.json"), batchJSON, 0o644); err != nil {
		t.Fatalf("write batch file: %v", err)
	}
}

func assertWatchedParentChildRequestRecorded(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	requestIndex := -1
	requestEvents := 0
	firstRelationIndex := -1
	parentChildRelations := map[string]bool{}

	for i, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest:
			if support.StringPointerValue(event.Context.RequestId) != "release-story-set" {
				continue
			}
			payload, err := event.Payload.AsWorkRequestEventPayload()
			if err != nil {
				t.Fatalf("decode WORK_REQUEST event %q: %v", event.Id, err)
			}
			requestIndex = i
			requestEvents++
			if payload.Type != factoryapi.WorkRequestTypeFactoryRequestBatch {
				t.Fatalf("request type = %q, want FACTORY_REQUEST_BATCH", payload.Type)
			}
			if support.StringPointerValue(payload.Source) != "external-submit" {
				t.Fatalf("request source = %q, want external-submit", support.StringPointerValue(payload.Source))
			}
			works := support.FactoryWorksValue(payload.Works)
			if len(works) != 3 {
				t.Fatalf("request work items = %d, want 3", len(works))
			}
			assertRequestIncludesStorySetParent(t, works)
		case factoryapi.FactoryEventTypeRelationshipChangeRequest:
			if support.StringPointerValue(event.Context.RequestId) != "release-story-set" {
				continue
			}
			payload, err := event.Payload.AsRelationshipChangeRequestEventPayload()
			if err != nil {
				t.Fatalf("decode RELATIONSHIP_CHANGE event %q: %v", event.Id, err)
			}
			if payload.Relation.Type != factoryapi.RelationTypeParentChild {
				t.Fatalf("relation type = %q, want PARENT_CHILD", payload.Relation.Type)
			}
			if payload.Relation.TargetWorkName != "story-set" {
				t.Fatalf("relation target = %q, want story-set", payload.Relation.TargetWorkName)
			}
			parentChildRelations[payload.Relation.SourceWorkName] = true
			if firstRelationIndex == -1 {
				firstRelationIndex = i
			}
		}
	}

	if requestEvents != 1 {
		t.Fatalf("WORK_REQUEST events for release-story-set = %d, want 1", requestEvents)
	}
	if !parentChildRelations["story-auth"] || !parentChildRelations["story-billing"] || len(parentChildRelations) != 2 {
		t.Fatalf("PARENT_CHILD relations = %#v, want story-auth and story-billing under story-set", parentChildRelations)
	}
	if firstRelationIndex <= requestIndex {
		t.Fatalf("WORK_REQUEST index %d should precede RELATIONSHIP_CHANGE index %d", requestIndex, firstRelationIndex)
	}
}

func assertRequestIncludesStorySetParent(t *testing.T, works []factoryapi.Work) {
	t.Helper()

	for _, work := range works {
		if work.Name != "story-set" {
			continue
		}
		if support.StringPointerValue(work.WorkTypeName) != "story-set" {
			t.Fatalf("story-set work_type_name = %q, want story-set", support.StringPointerValue(work.WorkTypeName))
		}
		return
	}

	t.Fatal("WORK_REQUEST missing story-set parent work item")
}

func assertParentFailedOnlyAfterChildFailure(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	childFailureIndex := -1
	childFailureDispatchID := ""
	parentFailureDispatchIndex := -1
	parentFailureCompletionIndex := -1

	for i, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				t.Fatalf("decode DISPATCH_COMPLETED event %q: %v", event.Id, err)
			}
			switch {
			case payload.TransitionId == "process-story" &&
				payload.Outcome == factoryapi.WorkOutcomeFailed &&
				support.StringPointerValue(event.Context.DispatchId) == childFailureDispatchID:
				childFailureIndex = i
			case payload.TransitionId == "fail-story-set-from-child" &&
				payload.Outcome == factoryapi.WorkOutcomeAccepted:
				parentFailureCompletionIndex = i
			}
		case factoryapi.FactoryEventTypeDispatchRequest:
			payload, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil {
				t.Fatalf("decode DISPATCH_CREATED event %q: %v", event.Id, err)
			}
			if payload.TransitionId == "process-story" &&
				dispatchHistoryIncludesWorkName(t, events, event, payload, "story-billing") {
				childFailureDispatchID = support.StringPointerValue(event.Context.DispatchId)
			}
			if payload.TransitionId == "fail-story-set-from-child" &&
				dispatchHistoryIncludesWorkName(t, events, event, payload, "story-set") {
				parentFailureDispatchIndex = i
			}
		}
	}

	if childFailureIndex == -1 {
		t.Fatal("missing failed child dispatch completion for story-billing")
	}
	if parentFailureDispatchIndex == -1 {
		t.Fatal("missing parent failure dispatch creation")
	}
	if parentFailureCompletionIndex == -1 {
		t.Fatal("missing parent failure dispatch completion")
	}
	if parentFailureDispatchIndex <= childFailureIndex {
		t.Fatalf("parent failure dispatch index %d should be after child failure index %d", parentFailureDispatchIndex, childFailureIndex)
	}
	if parentFailureCompletionIndex <= childFailureIndex {
		t.Fatalf("parent failure completion index %d should be after child failure index %d", parentFailureCompletionIndex, childFailureIndex)
	}
}

func dispatchHistoryIncludesWorkName(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	event factoryapi.FactoryEvent,
	payload factoryapi.DispatchRequestEventPayload,
	workName string,
) bool {
	t.Helper()

	for _, work := range support.DispatchInputWorksFromHistory(t, events, event, payload) {
		if work.Name == workName {
			return true
		}
	}
	return false
}
