package runtime_api

import (
	"context"
	"fmt"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

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

	generatedFactory, err := factorymapping.GeneratedFactoryFromOpenAPIJSON(flattened)
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
		status := support.GetJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
		if status.RuntimeStatus == string(interfaces.RuntimeStatusFinished) {
			return
		}
		time.Sleep(runtimeConfigAlignmentPollInterval)
	}

	status := support.GetJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
	session := support.GetDefaultSession(t, server.URL())
	t.Fatalf("timed out waiting %s for runtime completion; status=%s marking=%#v", timeout, status.RuntimeStatus, session.Runtime.Petri)
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
	if stringValueFromFunctionalPtr(cronWorker.Type) != interfaces.WorkerTypeInference {
		t.Fatalf("cron-worker type = %q, want %q", stringValueFromFunctionalPtr(cronWorker.Type), interfaces.WorkerTypeInference)
	}
	if stringValueFromFunctionalPtr(cronWorker.StopToken) != "COMPLETE" {
		t.Fatalf("cron-worker stop token = %q, want COMPLETE", stringValueFromFunctionalPtr(cronWorker.StopToken))
	}
	reviewer := runtimeConfigAlignmentRequireGeneratedWorker(t, *generated.Workers, "reviewer")
	if stringValueFromFunctionalPtr(reviewer.Type) != interfaces.WorkerTypeInference {
		t.Fatalf("reviewer type = %q, want %q", stringValueFromFunctionalPtr(reviewer.Type), interfaces.WorkerTypeInference)
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
	if cron.Behavior == nil || string(*cron.Behavior) != interfaces.CanonicalPublicWorkstationKind(interfaces.WorkstationKindCron) {
		t.Fatalf("%s kind = %#v, want CRON", runtimeConfigAlignmentCronWorkstation, cron.Behavior)
	}
	if cron.Cron == nil || stringValueFromFunctionalPtr(cron.Cron.Schedule) != "0 * * * *" {
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
	if review.Behavior == nil || string(*review.Behavior) != interfaces.CanonicalPublicWorkstationKind(interfaces.WorkstationKindRepeater) {
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

func (r *runtimeConfigAlignmentProviderRunner) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.callCount++
	r.mu.Unlock()

	prompt := commandPrompt(request)
	switch {
	case strings.Contains(prompt, "Review the task and return DONE"):
		return platformprocess.CommandResult{Stdout: []byte("review complete DONE")}, nil
	case strings.Contains(prompt, "Complete the scheduled task and return COMPLETE"):
		return platformprocess.CommandResult{Stdout: []byte("cron task COMPLETE")}, nil
	default:
		return platformprocess.CommandResult{Stdout: []byte("unexpected workstation COMPLETE")}, nil
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

func (r *runtimeConfigAlignmentScriptRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
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
		return platformprocess.CommandResult{}, ctx.Err()
	}

	if call == 2 {
		select {
		case <-r.releaseSecondAttempt:
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	}

	return platformprocess.CommandResult{Stdout: []byte("script-output-after-retry")}, nil
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
		dispatches := support.ObserveDispatchEvents(t, server.GetFactoryEvents(t))
		for _, dispatch := range dispatches {
			if dispatch.Request.TransitionId == runtimeConfigAlignmentReviewWorkstation &&
				dispatch.Response != nil &&
				dispatch.Response.Outcome == factoryapi.WorkOutcomeAccepted {
				return
			}
		}
		time.Sleep(runtimeConfigAlignmentPollInterval)
	}

	dispatches := support.ObserveDispatchEvents(t, server.GetFactoryEvents(t))
	t.Fatalf("expected %s to accept via stopWords before timeout stage; public dispatch events=%#v", runtimeConfigAlignmentReviewWorkstation, dispatches)
}

func waitForRuntimeConfigAlignmentInFlightResourceConsumption(
	t *testing.T,
	server *functionalAPIServer,
	runner *runtimeConfigAlignmentScriptRunner,
) {
	t.Helper()

	if !runner.waitForFirstDispatch(runtimeConfigAlignmentSignalTimeout) {
		session := support.GetDefaultSession(t, server.URL())
		t.Fatalf(
			"timed out waiting for %s to start; marking=%#v",
			runtimeConfigAlignmentExecuteWorkstation,
			session.Runtime.Petri,
		)
	}

	deadline := time.Now().Add(runtimeConfigAlignmentSignalTimeout)
	for time.Now().Before(deadline) {
		session := support.GetDefaultSession(t, server.URL())
		available, _, ok := runtimeConfigAlignmentResourceUsage(session, "agent-slot")
		if session.Runtime.Progress.InFlightCount > 0 && ok && available == 0 {
			return
		}
		time.Sleep(runtimeConfigAlignmentPollInterval)
	}

	session := support.GetDefaultSession(t, server.URL())
	available, total, _ := runtimeConfigAlignmentResourceUsage(session, "agent-slot")
	t.Fatalf(
		"expected %s to consume agent-slot while in flight; in_flight=%d resource=%d/%d",
		runtimeConfigAlignmentExecuteWorkstation,
		session.Runtime.Progress.InFlightCount,
		available,
		total,
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
		session := support.GetDefaultSession(t, server.URL())
		listed := support.ListDefaultSessionWork(t, server.URL())
		dispatches := support.ObserveDispatchEvents(t, server.GetFactoryEvents(t))
		available, _, resourceFound := runtimeConfigAlignmentResourceUsage(session, "agent-slot")
		if _, ok := runtimeConfigAlignmentFindDispatch(
			dispatches,
			runtimeConfigAlignmentExecuteWorkstation,
			factoryapi.WorkOutcomeFailed,
			"execution timeout",
		); ok &&
			support.CountWorkAtCustomerState(listed, "task:reviewed") == 1 &&
			resourceFound && available == 1 {
			return
		}
		time.Sleep(runtimeConfigAlignmentPollInterval)
	}

	session := support.GetDefaultSession(t, server.URL())
	t.Fatalf(
		"expected timed-out %s dispatch to requeue task:reviewed and restore agent-slot; dispatches=%#v session=%#v",
		runtimeConfigAlignmentExecuteWorkstation,
		support.ObserveDispatchEvents(t, server.GetFactoryEvents(t)),
		session.Runtime,
	)
}

func runtimeConfigAlignmentFindDispatch(
	dispatches []support.DispatchEventObservation,
	workstation string,
	outcome factoryapi.WorkOutcome,
	reason string,
) (support.DispatchEventObservation, bool) {
	for _, dispatch := range dispatches {
		if dispatch.Request.TransitionId != workstation || dispatch.Response == nil {
			continue
		}
		if dispatch.Response.Outcome != outcome {
			continue
		}
		if reason != "" && !runtimeConfigAlignmentDispatchHasReason(dispatch, reason) {
			continue
		}
		return dispatch, true
	}
	return support.DispatchEventObservation{}, false
}

func runtimeConfigAlignmentHasDispatch(
	dispatches []support.DispatchEventObservation,
	workstation string,
	outcome factoryapi.WorkOutcome,
	reason string,
) bool {
	_, ok := runtimeConfigAlignmentFindDispatch(dispatches, workstation, outcome, reason)
	return ok
}

func runtimeConfigAlignmentDispatchHasReason(
	dispatch support.DispatchEventObservation,
	reason string,
) bool {
	if dispatch.Response == nil {
		return false
	}
	if dispatch.Response.Error != nil && strings.Contains(*dispatch.Response.Error, reason) {
		return true
	}
	return dispatch.Response.FailureDetail != nil &&
		strings.Contains(dispatch.Response.FailureDetail.Message, reason)
}

func runtimeConfigAlignmentResourceUsage(
	session factoryapi.FactorySession,
	name string,
) (available int, total int, ok bool) {
	for _, resource := range session.Runtime.Usage.Resources {
		if resource.Name == name {
			return resource.Available, resource.Total, true
		}
	}
	return 0, 0, false
}

func runtimeConfigAlignmentPayloadText(payload any) string {
	switch value := payload.(type) {
	case string:
		return value
	case []byte:
		return string(value)
	case nil:
		return ""
	default:
		return fmt.Sprint(value)
	}
}

func assertRuntimeConfigAlignmentDispatchHistory(
	t *testing.T,
	dispatches []support.DispatchEventObservation,
) {
	t.Helper()

	if len(dispatches) < 4 {
		t.Fatalf("public dispatch event count = %d, want at least 4", len(dispatches))
	}
	if !runtimeConfigAlignmentHasDispatch(dispatches, runtimeConfigAlignmentReviewWorkstation, factoryapi.WorkOutcomeAccepted, "") {
		t.Fatalf("public dispatch events missing accepted %s: %#v", runtimeConfigAlignmentReviewWorkstation, dispatches)
	}
	timeoutDispatch, ok := runtimeConfigAlignmentFindDispatch(dispatches, runtimeConfigAlignmentExecuteWorkstation, factoryapi.WorkOutcomeFailed, "execution timeout")
	if !ok {
		t.Fatalf("public dispatch events missing execution-timeout failure for %s: %#v", runtimeConfigAlignmentExecuteWorkstation, dispatches)
	}
	if timeoutDispatch.Response == nil || timeoutDispatch.Response.FailureDetail == nil {
		t.Fatalf("%s failed dispatch FailureDetail = nil, want timeout detail", runtimeConfigAlignmentExecuteWorkstation)
	}
	if timeoutDispatch.Response.FailureDetail.Reason != factoryapi.WorkFailureTypeTimeout {
		t.Fatalf("%s failed dispatch reason = %q, want %q", runtimeConfigAlignmentExecuteWorkstation, timeoutDispatch.Response.FailureDetail.Reason, factoryapi.WorkFailureTypeTimeout)
	}
	if !runtimeConfigAlignmentHasDispatch(dispatches, runtimeConfigAlignmentExecuteWorkstation, factoryapi.WorkOutcomeAccepted, "") {
		t.Fatalf("public dispatch events missing accepted retry for %s: %#v", runtimeConfigAlignmentExecuteWorkstation, dispatches)
	}
	if !runtimeConfigAlignmentHasDispatch(dispatches, runtimeConfigAlignmentCronWorkstation, factoryapi.WorkOutcomeAccepted, "") {
		t.Fatalf("public dispatch events missing accepted %s: %#v", runtimeConfigAlignmentCronWorkstation, dispatches)
	}
	if !runtimeConfigAlignmentDispatchIncludesWork(dispatches, runtimeConfigAlignmentCronWorkstation, "runtime-config-alignment-cron-time") {
		t.Fatalf("public dispatch events missing %s consumption of cron time Work: %#v", runtimeConfigAlignmentCronWorkstation, dispatches)
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
}

func assertRuntimeConfigAlignmentCompleteWorkPayload(t *testing.T, work factoryapi.ListWorkResponse) {
	t.Helper()

	for _, item := range work.Results {
		if item.WorkId == nil || *item.WorkId != "runtime-config-alignment-work" {
			continue
		}
		if item.State == nil || item.State.Name != "complete" {
			t.Fatalf("completed Work state = %#v, want complete", item.State)
		}
		if got := strings.TrimSpace(runtimeConfigAlignmentPayloadText(item.Payload)); got != "script-output-after-retry" {
			t.Fatalf("completed Work payload = %q, want script-output-after-retry", got)
		}
		return
	}

	t.Fatal("expected completed public Work for runtime-config-alignment-work")
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

func runtimeConfigAlignmentDispatchIncludesWork(
	dispatches []support.DispatchEventObservation,
	workstation string,
	workID string,
) bool {
	for _, dispatch := range dispatches {
		if dispatch.Request.TransitionId != workstation {
			continue
		}
		if support.DispatchObservationIncludesWork(dispatch, workID) {
			return true
		}
	}
	return false
}

func assertRuntimeConfigAlignmentTopologyProjection(t *testing.T, server *functionalAPIServer) {
	t.Helper()

	for _, event := range server.GetFactoryEvents(t) {
		if event.Type != factoryapi.FactoryEventTypeInitialStructureRequest {
			continue
		}
		payload, err := event.Payload.AsInitialStructureRequestEventPayload()
		if err != nil {
			t.Fatalf("decode public INITIAL_STRUCTURE_REQUEST %q: %v", event.Id, err)
		}
		assertRuntimeConfigAlignmentGeneratedBoundary(t, payload.Factory)
		return
	}
	t.Fatal("public Factory Events missing INITIAL_STRUCTURE_REQUEST")
}
