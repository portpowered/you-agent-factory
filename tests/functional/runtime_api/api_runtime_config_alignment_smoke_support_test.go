package runtime_api

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func runtimeConfigAlignmentSummaryFromRuntime(
	t *testing.T,
	definitionLookup interfaces.RuntimeDefinitionLookup,
	workstationLookup interfaces.RuntimeWorkstationLookup,
) runtimeConfigAlignmentSummary {
	t.Helper()

	return runtimeConfigAlignmentSummary{
		Workers: map[string]runtimeConfigAlignmentWorkerSummary{
			"cron-worker": runtimeConfigAlignmentWorkerSummaryFromLookup(t, definitionLookup.Worker, "cron-worker"),
			"reviewer":    runtimeConfigAlignmentWorkerSummaryFromLookup(t, definitionLookup.Worker, "reviewer"),
			"executor":    runtimeConfigAlignmentWorkerSummaryFromLookup(t, definitionLookup.Worker, "executor"),
		},
		Workstations: map[string]runtimeConfigAlignmentWorkstationSummary{
			runtimeConfigAlignmentCronWorkstation:    runtimeConfigAlignmentWorkstationSummaryFromLookup(t, workstationLookup.Workstation, runtimeConfigAlignmentCronWorkstation),
			runtimeConfigAlignmentReviewWorkstation:  runtimeConfigAlignmentWorkstationSummaryFromLookup(t, workstationLookup.Workstation, runtimeConfigAlignmentReviewWorkstation),
			runtimeConfigAlignmentExecuteWorkstation: runtimeConfigAlignmentWorkstationSummaryFromLookup(t, workstationLookup.Workstation, runtimeConfigAlignmentExecuteWorkstation),
		},
	}
}

func runtimeConfigAlignmentWorkerSummaryFromLookup(
	t *testing.T,
	lookup func(string) (*interfaces.WorkerConfig, bool),
	name string,
) runtimeConfigAlignmentWorkerSummary {
	t.Helper()

	worker, ok := lookup(name)
	if !ok {
		t.Fatalf("worker lookup missing %q", name)
	}
	return runtimeConfigAlignmentWorkerSummary{
		Type:      worker.Type,
		Resources: append([]interfaces.ResourceConfig(nil), worker.Resources...),
		StopToken: worker.StopToken,
	}
}

func runtimeConfigAlignmentWorkstationSummaryFromLookup(
	t *testing.T,
	lookup func(string) (*interfaces.FactoryWorkstationConfig, bool),
	name string,
) runtimeConfigAlignmentWorkstationSummary {
	t.Helper()

	workstation, ok := lookup(name)
	if !ok {
		t.Fatalf("workstation lookup missing %q", name)
	}
	summary := runtimeConfigAlignmentWorkstationSummary{
		WorkerTypeName: workstation.WorkerTypeName,
		Kind:           workstation.Kind,
		Type:           workstation.Type,
		Limits:         workstation.Limits,
		Resources:      append([]interfaces.ResourceConfig(nil), workstation.Resources...),
		StopWords:      append([]string(nil), workstation.StopWords...),
	}
	if workstation.Cron != nil {
		summary.Cron = &runtimeConfigAlignmentCronSummary{
			Schedule:       workstation.Cron.Schedule,
			TriggerAtStart: workstation.Cron.TriggerAtStart,
			Jitter:         workstation.Cron.Jitter,
			ExpiryWindow:   workstation.Cron.ExpiryWindow,
		}
	}
	return summary
}

func assertRuntimeConfigAlignmentBoundaryErrorContains(t *testing.T, err error, contextSnippet string, want string) {
	t.Helper()

	if err == nil {
		t.Fatal("expected runtime config alignment boundary error, got nil")
	}
	if !strings.Contains(err.Error(), contextSnippet) {
		t.Fatalf("runtime config alignment error = %q, want context %q", err.Error(), contextSnippet)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("runtime config alignment error = %q, want detail %q", err.Error(), want)
	}
}

func assertRuntimeConfigAlignmentCanonicalJSON(t *testing.T, flattened []byte) {
	t.Helper()

	generatedFactory, err := factoryconfig.GeneratedFactoryFromOpenAPIJSON(flattened)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON(canonical flattened): %v", err)
	}
	if generatedFactory.Workers == nil || len(*generatedFactory.Workers) != 3 {
		t.Fatalf("canonical flattened workers = %#v, want three workers", generatedFactory.Workers)
	}
	if generatedFactory.Workstations == nil || len(*generatedFactory.Workstations) != 3 {
		t.Fatalf("canonical flattened workstations = %#v, want three workstations", generatedFactory.Workstations)
	}
}

func waitForRuntimeConfigAlignmentServerCompletion(
	t *testing.T,
	server *functionalAPIServer,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snapshot := server.GetEngineStateSnapshot(t)
		if snapshot.RuntimeStatus == interfaces.RuntimeStatusFinished {
			return
		}
		time.Sleep(runtimeConfigAlignmentPollInterval)
	}

	snapshot := server.GetEngineStateSnapshot(t)
	t.Fatalf("timed out waiting %s for runtime completion; status=%s places=%#v", timeout, snapshot.RuntimeStatus, snapshot.Marking.PlaceTokens)
}

func assertRuntimeConfigAlignmentResourceManifest(t *testing.T, manifest *interfaces.PortableResourceManifestConfig) {
	t.Helper()

	if manifest == nil {
		t.Fatal("resource manifest is nil")
	}
	if len(manifest.RequiredTools) != 1 || manifest.RequiredTools[0].Name != "go" {
		t.Fatalf("resource manifest requiredTools = %#v, want go", manifest.RequiredTools)
	}
	if len(manifest.BundledFiles) != 2 {
		t.Fatalf("resource manifest bundledFiles = %#v, want bootstrap script and usage doc", manifest.BundledFiles)
	}
}

func assertRuntimeConfigAlignmentGeneratedResourceManifest(t *testing.T, manifest *factoryapi.ResourceManifest) {
	t.Helper()

	if manifest == nil {
		t.Fatal("generated resource manifest is nil")
	}
	if manifest.RequiredTools == nil || len(*manifest.RequiredTools) != 1 {
		t.Fatalf("generated requiredTools = %#v, want one go tool", manifest.RequiredTools)
	}
	if (*manifest.RequiredTools)[0].Name != "go" {
		t.Fatalf("generated required tool = %#v, want go", (*manifest.RequiredTools)[0])
	}
	if manifest.BundledFiles == nil || len(*manifest.BundledFiles) != 2 {
		t.Fatalf("generated bundled files = %#v, want bootstrap script and usage doc", manifest.BundledFiles)
	}
	runtimeConfigAlignmentRequireGeneratedBundledFile(t, *manifest.BundledFiles, "factory/scripts/bootstrap.ps1")
	runtimeConfigAlignmentRequireGeneratedBundledFile(t, *manifest.BundledFiles, "factory/docs/usage.md")
}

func runtimeConfigAlignmentRequireGeneratedBundledFile(
	t *testing.T,
	bundledFiles []factoryapi.BundledFile,
	targetPath string,
) factoryapi.BundledFile {
	t.Helper()

	for _, bundledFile := range bundledFiles {
		if bundledFile.TargetPath == targetPath {
			return bundledFile
		}
	}
	t.Fatalf("expected generated bundled file %q in %#v", targetPath, bundledFiles)
	return factoryapi.BundledFile{}
}

func assertRuntimeConfigAlignmentGeneratedBoundary(t *testing.T, generated factoryapi.Factory) {
	t.Helper()

	if generated.Workers == nil || len(*generated.Workers) != 3 {
		t.Fatalf("generated workers = %#v, want three workers", generated.Workers)
	}
	cronWorker := runtimeConfigAlignmentRequireGeneratedWorker(t, *generated.Workers, "cron-worker")
	if stringValueFromFunctionalPtr(cronWorker.Type) != interfaces.WorkerTypeModel {
		t.Fatalf("cron-worker type = %q, want %q", stringValueFromFunctionalPtr(cronWorker.Type), interfaces.WorkerTypeModel)
	}
	if stringValueFromFunctionalPtr(cronWorker.StopToken) != "COMPLETE" {
		t.Fatalf("cron-worker stop token = %q, want COMPLETE", stringValueFromFunctionalPtr(cronWorker.StopToken))
	}
	reviewer := runtimeConfigAlignmentRequireGeneratedWorker(t, *generated.Workers, "reviewer")
	if stringValueFromFunctionalPtr(reviewer.Type) != interfaces.WorkerTypeModel {
		t.Fatalf("reviewer type = %q, want %q", stringValueFromFunctionalPtr(reviewer.Type), interfaces.WorkerTypeModel)
	}
	if stringValueFromFunctionalPtr(reviewer.StopToken) != "COMPLETE" {
		t.Fatalf("reviewer stop token = %q, want COMPLETE", stringValueFromFunctionalPtr(reviewer.StopToken))
	}
	if reviewer.Resources != nil && len(*reviewer.Resources) != 0 {
		t.Fatalf("reviewer resources = %#v, want no worker-level resources in this fixture", reviewer.Resources)
	}
	executor := runtimeConfigAlignmentRequireGeneratedWorker(t, *generated.Workers, "executor")
	if stringValueFromFunctionalPtr(executor.Type) != interfaces.WorkerTypeScript {
		t.Fatalf("executor type = %q, want %q", stringValueFromFunctionalPtr(executor.Type), interfaces.WorkerTypeScript)
	}
	if executor.Resources != nil && len(*executor.Resources) != 0 {
		t.Fatalf("executor resources = %#v, want no worker-level resources in this fixture", executor.Resources)
	}

	if generated.Workstations == nil || len(*generated.Workstations) != 3 {
		t.Fatalf("generated workstations = %#v, want three workstations", generated.Workstations)
	}
	cron := runtimeConfigAlignmentRequireGeneratedWorkstation(t, *generated.Workstations, runtimeConfigAlignmentCronWorkstation)
	if cron.Worker != "cron-worker" {
		t.Fatalf("%s worker = %q, want cron-worker", runtimeConfigAlignmentCronWorkstation, cron.Worker)
	}
	if cron.Behavior == nil || *cron.Behavior != interfaces.GeneratedPublicWorkstationKind(interfaces.WorkstationKindCron) {
		t.Fatalf("%s kind = %#v, want CRON", runtimeConfigAlignmentCronWorkstation, cron.Behavior)
	}
	if cron.Cron == nil || cron.Cron.Schedule != "0 * * * *" {
		t.Fatalf("%s cron = %#v, want schedule 0 * * * *", runtimeConfigAlignmentCronWorkstation, cron.Cron)
	}
	if cron.Cron.TriggerAtStart == nil || !*cron.Cron.TriggerAtStart {
		t.Fatalf("%s cron = %#v, want triggerAtStart true", runtimeConfigAlignmentCronWorkstation, cron.Cron)
	}
	if stringValueFromFunctionalPtr(cron.Cron.Jitter) != "5s" {
		t.Fatalf("%s cron = %#v, want jitter 5s", runtimeConfigAlignmentCronWorkstation, cron.Cron)
	}
	if stringValueFromFunctionalPtr(cron.Cron.ExpiryWindow) != "1h" {
		t.Fatalf("%s cron = %#v, want expiryWindow 1h", runtimeConfigAlignmentCronWorkstation, cron.Cron)
	}
	review := runtimeConfigAlignmentRequireGeneratedWorkstation(t, *generated.Workstations, runtimeConfigAlignmentReviewWorkstation)
	if review.Worker != "reviewer" {
		t.Fatalf("%s worker = %q, want reviewer", runtimeConfigAlignmentReviewWorkstation, review.Worker)
	}
	if stringValueFromFunctionalPtr(review.Type) != interfaces.WorkstationTypeModel {
		t.Fatalf("%s type = %q, want %q", runtimeConfigAlignmentReviewWorkstation, stringValueFromFunctionalPtr(review.Type), interfaces.WorkstationTypeModel)
	}
	if review.Behavior == nil || *review.Behavior != interfaces.GeneratedPublicWorkstationKind(interfaces.WorkstationKindRepeater) {
		t.Fatalf("%s kind = %#v, want REPEATER", runtimeConfigAlignmentReviewWorkstation, review.Behavior)
	}
	if !reflect.DeepEqual(stringSliceValue(review.StopWords), []string{"DONE"}) {
		t.Fatalf("%s stopWords = %#v, want [DONE]", runtimeConfigAlignmentReviewWorkstation, review.StopWords)
	}
	if !runtimeConfigAlignmentHasGeneratedResource(review.Resources, "agent-slot", 1) {
		t.Fatalf("%s resources = %#v, want agent-slot capacity 1", runtimeConfigAlignmentReviewWorkstation, review.Resources)
	}
	execute := runtimeConfigAlignmentRequireGeneratedWorkstation(t, *generated.Workstations, runtimeConfigAlignmentExecuteWorkstation)
	if execute.Worker != "executor" {
		t.Fatalf("%s worker = %q, want executor", runtimeConfigAlignmentExecuteWorkstation, execute.Worker)
	}
	if execute.Limits == nil || stringValueFromFunctionalPtr(execute.Limits.MaxExecutionTime) != runtimeConfigAlignmentExecuteTimeout.String() {
		t.Fatalf("%s limits = %#v, want maxExecutionTime %s", runtimeConfigAlignmentExecuteWorkstation, execute.Limits, runtimeConfigAlignmentExecuteTimeout)
	}
	if !runtimeConfigAlignmentHasGeneratedResource(execute.Resources, "agent-slot", 1) {
		t.Fatalf("%s resources = %#v, want agent-slot capacity 1", runtimeConfigAlignmentExecuteWorkstation, execute.Resources)
	}
}

func runtimeConfigAlignmentRequireGeneratedWorker(
	t *testing.T,
	workers []factoryapi.Worker,
	name string,
) factoryapi.Worker {
	t.Helper()

	for _, worker := range workers {
		if worker.Name == name {
			return worker
		}
	}
	t.Fatalf("generated workers missing %q: %#v", name, workers)
	return factoryapi.Worker{}
}

func runtimeConfigAlignmentRequireGeneratedWorkstation(
	t *testing.T,
	workstations []factoryapi.Workstation,
	name string,
) factoryapi.Workstation {
	t.Helper()

	for _, workstation := range workstations {
		if workstation.Name == name {
			return workstation
		}
	}
	t.Fatalf("generated workstations missing %q: %#v", name, workstations)
	return factoryapi.Workstation{}
}

type runtimeConfigAlignmentProviderRunner struct {
	mu        sync.Mutex
	callCount int
}

func newRuntimeConfigAlignmentProviderRunner() *runtimeConfigAlignmentProviderRunner {
	return &runtimeConfigAlignmentProviderRunner{}
}

func (r *runtimeConfigAlignmentProviderRunner) Run(_ context.Context, request workers.CommandRequest) (workers.CommandResult, error) {
	r.mu.Lock()
	r.callCount++
	r.mu.Unlock()

	switch request.WorkstationName {
	case runtimeConfigAlignmentReviewWorkstation:
		return workers.CommandResult{Stdout: []byte("review complete DONE")}, nil
	case runtimeConfigAlignmentCronWorkstation:
		return workers.CommandResult{Stdout: []byte("cron task COMPLETE")}, nil
	default:
		return workers.CommandResult{Stdout: []byte("unexpected workstation COMPLETE")}, nil
	}
}

func (r *runtimeConfigAlignmentProviderRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.callCount
}

type runtimeConfigAlignmentScriptRunner struct {
	mu                   sync.Mutex
	callCount            int
	firstDispatchAt      time.Time
	firstTimeoutAt       time.Time
	firstDispatchStarted chan struct{}
	firstTimeout         chan struct{}
	releaseSecondAttempt chan struct{}
	firstStartedOnce     sync.Once
	firstTimeoutOnce     sync.Once
}

func newRuntimeConfigAlignmentScriptRunner() *runtimeConfigAlignmentScriptRunner {
	return &runtimeConfigAlignmentScriptRunner{
		firstDispatchStarted: make(chan struct{}),
		firstTimeout:         make(chan struct{}),
		releaseSecondAttempt: make(chan struct{}),
	}
}

func (r *runtimeConfigAlignmentScriptRunner) Run(ctx context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	r.mu.Lock()
	r.callCount++
	call := r.callCount
	r.mu.Unlock()

	if call == 1 {
		r.mu.Lock()
		r.firstDispatchAt = time.Now()
		r.mu.Unlock()
		r.firstStartedOnce.Do(func() { close(r.firstDispatchStarted) })
		<-ctx.Done()
		r.mu.Lock()
		r.firstTimeoutAt = time.Now()
		r.mu.Unlock()
		r.firstTimeoutOnce.Do(func() { close(r.firstTimeout) })
		return workers.CommandResult{}, ctx.Err()
	}

	if call == 2 {
		select {
		case <-r.releaseSecondAttempt:
		case <-ctx.Done():
			return workers.CommandResult{}, ctx.Err()
		}
	}

	return workers.CommandResult{Stdout: []byte("script-output-after-retry")}, nil
}

func (r *runtimeConfigAlignmentScriptRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.callCount
}

func (r *runtimeConfigAlignmentScriptRunner) firstTimeoutElapsed() (time.Duration, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.firstDispatchAt.IsZero() || r.firstTimeoutAt.IsZero() {
		return 0, false
	}
	return r.firstTimeoutAt.Sub(r.firstDispatchAt), true
}

func (r *runtimeConfigAlignmentScriptRunner) waitForFirstDispatch(timeout time.Duration) bool {
	select {
	case <-r.firstDispatchStarted:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (r *runtimeConfigAlignmentScriptRunner) waitForFirstTimeout(timeout time.Duration) bool {
	select {
	case <-r.firstTimeout:
		return true
	case <-time.After(timeout):
		return false
	}
}

func waitForRuntimeConfigAlignmentStopWordDispatch(
	t *testing.T,
	server *functionalAPIServer,
) {
	t.Helper()

	deadline := time.Now().Add(runtimeConfigAlignmentSignalTimeout)
	for time.Now().Before(deadline) {
		snapshot := server.GetEngineStateSnapshot(t)
		for _, dispatch := range snapshot.DispatchHistory {
			if dispatch.WorkstationName == runtimeConfigAlignmentReviewWorkstation && dispatch.Outcome == interfaces.OutcomeAccepted {
				return
			}
		}
		time.Sleep(runtimeConfigAlignmentPollInterval)
	}

	snapshot := server.GetEngineStateSnapshot(t)
	t.Fatalf("expected %s to accept via stopWords before timeout stage; history=%#v", runtimeConfigAlignmentReviewWorkstation, snapshot.DispatchHistory)
}

func waitForRuntimeConfigAlignmentInFlightResourceConsumption(
	t *testing.T,
	server *functionalAPIServer,
	runner *runtimeConfigAlignmentScriptRunner,
) {
	t.Helper()

	if !runner.waitForFirstDispatch(runtimeConfigAlignmentSignalTimeout) {
		snapshot := server.GetEngineStateSnapshot(t)
		t.Fatalf(
			"timed out waiting for %s to start; history=%#v places=%#v",
			runtimeConfigAlignmentExecuteWorkstation,
			snapshot.DispatchHistory,
			snapshot.Marking.PlaceTokens,
		)
	}

	deadline := time.Now().Add(runtimeConfigAlignmentSignalTimeout)
	for time.Now().Before(deadline) {
		snapshot := server.GetEngineStateSnapshot(t)
		if snapshot.InFlightCount > 0 && len(snapshot.Marking.PlaceTokens["agent-slot:available"]) == 0 {
			return
		}
		time.Sleep(runtimeConfigAlignmentPollInterval)
	}

	snapshot := server.GetEngineStateSnapshot(t)
	t.Fatalf(
		"expected %s to consume agent-slot while in flight; in_flight=%d places=%#v",
		runtimeConfigAlignmentExecuteWorkstation,
		snapshot.InFlightCount,
		snapshot.Marking.PlaceTokens,
	)
}

func waitForRuntimeConfigAlignmentTimeoutAndRequeue(
	t *testing.T,
	server *functionalAPIServer,
	runner *runtimeConfigAlignmentScriptRunner,
) {
	t.Helper()

	if !runner.waitForFirstTimeout(runtimeConfigAlignmentSignalTimeout) {
		t.Fatalf("timed out waiting for %s to hit limits.maxExecutionTime", runtimeConfigAlignmentExecuteWorkstation)
	}
	elapsed, ok := runner.firstTimeoutElapsed()
	if !ok {
		t.Fatalf("missing first timeout timing for %s", runtimeConfigAlignmentExecuteWorkstation)
	}
	if elapsed < runtimeConfigAlignmentTimeoutMinElapsed || elapsed > runtimeConfigAlignmentTimeoutMaxElapsed {
		t.Fatalf(
			"%s timeout elapsed = %s, want bounded around configured maxExecutionTime %s",
			runtimeConfigAlignmentExecuteWorkstation,
			elapsed,
			runtimeConfigAlignmentExecuteTimeout,
		)
	}

	deadline := time.Now().Add(runtimeConfigAlignmentSignalTimeout)
	for time.Now().Before(deadline) {
		snapshot := server.GetEngineStateSnapshot(t)
		if dispatch, ok := runtimeConfigAlignmentFindDispatch(snapshot.DispatchHistory, runtimeConfigAlignmentExecuteWorkstation, interfaces.OutcomeFailed, "execution timeout"); ok {
			if runtimeConfigAlignmentHasMutationToPlace(dispatch.OutputMutations, "task:reviewed") &&
				runtimeConfigAlignmentHasMutationToPlace(dispatch.OutputMutations, "agent-slot:available") {
				return
			}
		}
		time.Sleep(runtimeConfigAlignmentPollInterval)
	}

	snapshot := server.GetEngineStateSnapshot(t)
	t.Fatalf(
		"expected timed-out %s dispatch to requeue task:reviewed and restore agent-slot; history=%#v places=%#v",
		runtimeConfigAlignmentExecuteWorkstation,
		snapshot.DispatchHistory,
		snapshot.Marking.PlaceTokens,
	)
}

func runtimeConfigAlignmentFindDispatch(
	history []interfaces.CompletedDispatch,
	workstation string,
	outcome interfaces.WorkOutcome,
	reason string,
) (interfaces.CompletedDispatch, bool) {
	for _, dispatch := range history {
		if dispatch.WorkstationName != workstation {
			continue
		}
		if dispatch.Outcome != outcome {
			continue
		}
		if reason != "" && dispatch.Reason != reason {
			continue
		}
		return dispatch, true
	}
	return interfaces.CompletedDispatch{}, false
}

func runtimeConfigAlignmentHasDispatch(
	history []interfaces.CompletedDispatch,
	workstation string,
	outcome interfaces.WorkOutcome,
	reason string,
) bool {
	_, ok := runtimeConfigAlignmentFindDispatch(history, workstation, outcome, reason)
	return ok
}

func runtimeConfigAlignmentHasMutationToPlace(mutations []interfaces.TokenMutationRecord, placeID string) bool {
	for _, mutation := range mutations {
		if mutation.ToPlace == placeID {
			return true
		}
	}
	return false
}

func assertRuntimeConfigAlignmentDispatchHistory(t *testing.T, history []interfaces.CompletedDispatch) {
	t.Helper()

	if len(history) < 4 {
		t.Fatalf("dispatch history length = %d, want at least 4", len(history))
	}
	if !runtimeConfigAlignmentHasDispatch(history, runtimeConfigAlignmentReviewWorkstation, interfaces.OutcomeAccepted, "") {
		t.Fatalf("dispatch history missing accepted %s: %#v", runtimeConfigAlignmentReviewWorkstation, history)
	}
	timeoutDispatch, ok := runtimeConfigAlignmentFindDispatch(history, runtimeConfigAlignmentExecuteWorkstation, interfaces.OutcomeFailed, "execution timeout")
	if !ok {
		t.Fatalf("dispatch history missing execution-timeout failure for %s: %#v", runtimeConfigAlignmentExecuteWorkstation, history)
	}
	if timeoutDispatch.FailureMetadata == nil {
		t.Fatalf("%s failed dispatch FailureMetadata = nil, want timeout metadata", runtimeConfigAlignmentExecuteWorkstation)
	}
	if timeoutDispatch.FailureMetadata.Type != interfaces.WorkFailureTypeTimeout {
		t.Fatalf("%s failed dispatch FailureMetadata.Type = %q, want %q", runtimeConfigAlignmentExecuteWorkstation, timeoutDispatch.FailureMetadata.Type, interfaces.WorkFailureTypeTimeout)
	}
	if timeoutDispatch.FailureMetadata.Family != interfaces.WorkFailureFamilyRetryable {
		t.Fatalf("%s failed dispatch FailureMetadata.Family = %q, want %q", runtimeConfigAlignmentExecuteWorkstation, timeoutDispatch.FailureMetadata.Family, interfaces.WorkFailureFamilyRetryable)
	}
	if timeoutDispatch.ProviderFailure == nil {
		t.Fatalf("%s failed dispatch ProviderFailure = nil, want timeout metadata", runtimeConfigAlignmentExecuteWorkstation)
	}
	if timeoutDispatch.ProviderFailure.Type != interfaces.ProviderErrorTypeTimeout {
		t.Fatalf("%s failed dispatch ProviderFailure.Type = %q, want %q", runtimeConfigAlignmentExecuteWorkstation, timeoutDispatch.ProviderFailure.Type, interfaces.ProviderErrorTypeTimeout)
	}
	if timeoutDispatch.ProviderFailure.Family != interfaces.ProviderErrorFamilyRetryable {
		t.Fatalf("%s failed dispatch ProviderFailure.Family = %q, want %q", runtimeConfigAlignmentExecuteWorkstation, timeoutDispatch.ProviderFailure.Family, interfaces.ProviderErrorFamilyRetryable)
	}
	if !runtimeConfigAlignmentHasDispatch(history, runtimeConfigAlignmentExecuteWorkstation, interfaces.OutcomeAccepted, "") {
		t.Fatalf("dispatch history missing accepted retry for %s: %#v", runtimeConfigAlignmentExecuteWorkstation, history)
	}
	if !runtimeConfigAlignmentHasDispatch(history, runtimeConfigAlignmentCronWorkstation, interfaces.OutcomeAccepted, "") {
		t.Fatalf("dispatch history missing accepted %s: %#v", runtimeConfigAlignmentCronWorkstation, history)
	}
	if !runtimeConfigAlignmentDispatchConsumedPlace(history, runtimeConfigAlignmentCronWorkstation, interfaces.SystemTimePendingPlaceID) {
		t.Fatalf("dispatch history missing %s consumption of %s: %#v", runtimeConfigAlignmentCronWorkstation, interfaces.SystemTimePendingPlaceID, history)
	}
}

func assertRuntimeConfigAlignmentEventHistory(t *testing.T, server *functionalAPIServer) {
	t.Helper()

	events := server.GetFactoryEvents(t)
	for _, eventType := range []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeRunRequest,
		factoryapi.FactoryEventTypeInitialStructureRequest,
		factoryapi.FactoryEventTypeWorkRequest,
		factoryapi.FactoryEventTypeDispatchRequest,
		factoryapi.FactoryEventTypeDispatchResponse,
	} {
		if runtimeConfigAlignmentCountFactoryEvents(events, eventType) == 0 {
			t.Fatalf("GetFactoryEvents missing %s in canonical history", eventType)
		}
	}
	if got := runtimeConfigAlignmentCountFactoryEvents(events, factoryapi.FactoryEventTypeDispatchResponse); got < 4 {
		t.Fatalf("DISPATCH_RESPONSE events = %d, want at least 4", got)
	}

	worldState, err := projections.ReconstructFactoryWorldState(events, runtimeConfigAlignmentMaxTick(events))
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	assertRuntimeConfigAlignmentProjectedWorkstationKind(
		t,
		worldState.Topology,
		runtimeConfigAlignmentCronWorkstation,
		interfaces.CanonicalPublicWorkstationKind(interfaces.WorkstationKindCron),
	)
	assertRuntimeConfigAlignmentProjectedWorkstationKind(
		t,
		worldState.Topology,
		runtimeConfigAlignmentReviewWorkstation,
		interfaces.CanonicalPublicWorkstationKind(interfaces.WorkstationKindRepeater),
	)

	worldView := projections.BuildFactoryWorldView(worldState)
	if got := worldView.Runtime.PlaceTokenCounts["task:complete"]; got != 1 {
		t.Fatalf("canonical world view task:complete count = %d, want 1", got)
	}
	if got := worldView.Runtime.PlaceTokenCounts["scheduled:complete"]; got != 1 {
		t.Fatalf("canonical world view scheduled:complete count = %d, want 1", got)
	}
}

func assertRuntimeConfigAlignmentCompleteTokenPayload(t *testing.T, tokens map[string]*interfaces.Token) {
	t.Helper()

	for _, token := range tokens {
		if token == nil || token.PlaceID != "task:complete" {
			continue
		}
		if string(token.Color.Payload) != "script-output-after-retry" {
			t.Fatalf("completed token payload = %q, want script-output-after-retry", string(token.Color.Payload))
		}
		return
	}

	t.Fatal("expected completed token payload for task:complete")
}

func runtimeConfigAlignmentCountFactoryEvents(
	events []factoryapi.FactoryEvent,
	eventType factoryapi.FactoryEventType,
) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func runtimeConfigAlignmentMaxTick(events []factoryapi.FactoryEvent) int {
	maxTick := 0
	for _, event := range events {
		if event.Context.Tick > maxTick {
			maxTick = event.Context.Tick
		}
	}
	return maxTick
}

func runtimeConfigAlignmentDispatchConsumedPlace(
	history []interfaces.CompletedDispatch,
	workstation string,
	placeID string,
) bool {
	for _, dispatch := range history {
		if dispatch.WorkstationName != workstation {
			continue
		}
		for _, token := range dispatch.ConsumedTokens {
			if token.PlaceID == placeID {
				return true
			}
		}
	}
	return false
}

func assertRuntimeConfigAlignmentTopologyProjection(t *testing.T, dir string) {
	t.Helper()

	replayProjection := projectReplayInitialStructureFromEmbeddedConfig(t, dir)
	assertRuntimeConfigAlignmentTopologyPayload(t, replayProjection)
}

func assertRuntimeConfigAlignmentTopologyPayload(t *testing.T, payload interfaces.InitialStructurePayload) {
	t.Helper()

	assertRuntimeConfigAlignmentProjectedWorkstationKind(t, payload, runtimeConfigAlignmentCronWorkstation, interfaces.CanonicalPublicWorkstationKind(interfaces.WorkstationKindCron))
	assertRuntimeConfigAlignmentProjectedWorkstationKind(t, payload, runtimeConfigAlignmentReviewWorkstation, interfaces.CanonicalPublicWorkstationKind(interfaces.WorkstationKindRepeater))
	assertRuntimeConfigAlignmentConstraint(t, payload.Constraints, "workstation/"+runtimeConfigAlignmentExecuteWorkstation+"/limits", "workstation_limit", map[string]string{
		"max_execution_time": "100ms",
		"max_retries":        "2",
	})
	assertRuntimeConfigAlignmentConstraint(t, payload.Constraints, "workstation/"+runtimeConfigAlignmentReviewWorkstation+"/stop-words", "stop_words", map[string]string{
		"words": "DONE",
	})
	assertRuntimeConfigAlignmentConstraint(t, payload.Constraints, "workstation/"+runtimeConfigAlignmentCronWorkstation+"/cron", "cron_trigger", map[string]string{
		"schedule":         "0 * * * *",
		"trigger_at_start": "true",
		"jitter":           "5s",
		"expiry_window":    "1h",
	})
}

func assertRuntimeConfigAlignmentProjectedWorkstationKind(
	t *testing.T,
	payload interfaces.InitialStructurePayload,
	workstationID string,
	wantKind string,
) {
	t.Helper()

	for _, workstation := range payload.Workstations {
		if workstation.ID != workstationID {
			continue
		}
		if workstation.Kind != wantKind {
			t.Fatalf("workstation %s kind = %q, want %q in %#v", workstationID, workstation.Kind, wantKind, payload.Workstations)
		}
		return
	}

	t.Fatalf("missing workstation %s in %#v", workstationID, payload.Workstations)
}

func assertRuntimeConfigAlignmentConstraint(
	t *testing.T,
	constraints []interfaces.FactoryConstraint,
	id string,
	wantType string,
	wantValues map[string]string,
) {
	t.Helper()

	matches := 0
	for _, constraint := range constraints {
		if constraint.ID != id {
			continue
		}
		matches++
		if constraint.Type != wantType || !reflect.DeepEqual(constraint.Values, wantValues) {
			t.Fatalf("constraint %s = %#v, want type=%s values=%#v", id, constraint, wantType, wantValues)
		}
	}
	if matches != 1 {
		t.Fatalf("constraint %s count = %d, want 1 in %#v", id, matches, constraints)
	}
}
