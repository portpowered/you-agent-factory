package service_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	executeservice "github.com/portpowered/infinite-you/pkg/services/workers/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
)

func mustExecuteService(
	t *testing.T,
	runner workers.Runner,
	observe workers.ObservationSink,
	selectedLogger ...logging.Logger,
) *executeservice.Service {
	return mustExecuteServiceWithEdges(t, runner, observe, nil, nil, nil, selectedLogger...)
}

func mustExecuteServiceWithEdges(
	t *testing.T,
	runner workers.Runner,
	observe workers.ObservationSink,
	worktree workers.FactoryWorktreePreparer,
	worktreeRelease func(context.Context, workers.FactoryWorktreePreparation) error,
	temporaryFiles workers.TemporaryFileSystem,
	selectedLogger ...logging.Logger,
) *executeservice.Service {
	t.Helper()
	var logger logging.Logger = logging.NoopLogger{}
	if len(selectedLogger) > 0 {
		logger = selectedLogger[0]
	}
	service, err := executeservice.NewWithProviderOverride(
		&staticRunners{runner: runner},
		nil,
		observe,
		logger,
		func() time.Time { return time.Unix(10, 0) }, platformclock.Real{},
		worktree,
		worktreeRelease,
		temporaryFiles,
		nil, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("NewWithProviderOverride() error = %v", err)
	}
	return service
}

type recordingWorktree struct {
	prepares    atomic.Int32
	preparation workers.FactoryWorktreePreparation
	release     func(context.Context, workers.FactoryWorktreePreparation) error
}

func (worktree *recordingWorktree) Prepare(
	context.Context,
	string,
	string,
) (workers.FactoryWorktreePreparation, error) {
	worktree.prepares.Add(1)
	return worktree.preparation, nil
}

func (worktree *recordingWorktree) Release(
	ctx context.Context,
	preparation workers.FactoryWorktreePreparation,
) error {
	if worktree.release == nil {
		return nil
	}
	return worktree.release(ctx, preparation)
}

type recordingTemporaryFiles struct {
	mu      sync.Mutex
	next    int
	removed []string
	remove  func(string) error
}

func (files *recordingTemporaryFiles) CreateTemp(_, _ string) (workers.TemporaryFile, error) {
	files.mu.Lock()
	defer files.mu.Unlock()
	files.next++
	return &recordingTemporaryFile{name: "attempt-temp-" + strconv.Itoa(files.next)}, nil
}

func (files *recordingTemporaryFiles) Remove(path string) error {
	files.mu.Lock()
	files.removed = append(files.removed, path)
	files.mu.Unlock()
	if files.remove == nil {
		return nil
	}
	return files.remove(path)
}

func (files *recordingTemporaryFiles) Removed() []string {
	files.mu.Lock()
	defer files.mu.Unlock()
	return append([]string(nil), files.removed...)
}

type recordingTemporaryFile struct {
	name string
}

func (file *recordingTemporaryFile) Name() string {
	return file.name
}

func (*recordingTemporaryFile) WriteString(value string) (int, error) {
	return len(value), nil
}

func (*recordingTemporaryFile) Close() error {
	return nil
}

type stubRunner struct {
	content        string
	proposedOutput *workers.ProposedOutput
	execute        func(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error)
}

func (runner *stubRunner) Execute(
	ctx context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	if runner.execute != nil {
		return runner.execute(ctx, request)
	}
	return workers.RunnerExecutionResult{Content: runner.content, ProposedOutput: runner.proposedOutput}, nil
}

type staticRunners struct {
	runner workers.Runner
}

func (registry *staticRunners) Resolve(
	request runners.ResolutionRequest,
) (runners.Binding, error) {
	return runners.Binding{
		Identity: request.Identity,
		Metadata: workers.RunnerMetadata{ID: request.Identity},
		Runner:   registry.runner,
	}, nil
}

func (registry *staticRunners) Execute(
	ctx context.Context,
	request runners.ExecuteRequest,
) (runners.ExecuteResult, error) {
	binding, err := registry.Resolve(runners.ResolutionRequest{
		Identity:             request.Identity,
		RequiredCapabilities: request.RequiredCapabilities,
	})
	if err != nil {
		return runners.ExecuteResult{}, err
	}
	return binding.Runner.Execute(ctx, request.Attempt)
}

func validExecuteRequest(dispatchID, attemptID string) workers.ExecuteRequest {
	return workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-1",
			RuntimeID:        "runtime-1",
			GenerationID:     "generation-1",
			DispatchID:       dispatchID,
			AttemptID:        attemptID,
			RequestID:        "request-1",
			TraceID:          "trace-1",
		},
		Target: workers.ExecutionTarget{
			WorkerName:      "writer",
			WorkstationName: "review",
			RunnerID:        runners.ScriptIdentity,
		},
	}
}

func TestExecuteCheckoutLifetimeSuccess(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		retain bool
		reused bool
	}{
		{"S-N_given_new_nonretained_when_success_then_release_once", false, false},
		{"S-R_given_new_retained_when_success_then_preserve_checkout", true, false},
		{"S-U_given_reused_nonretained_when_success_then_preserve_checkout", false, true},
		{"S-RU_given_retained_reused_when_success_then_preserve_checkout", true, true},
	}
	for _, scenario := range cases {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			assertCheckoutLifetimeSuccess(t, scenario.retain, scenario.reused)
		})
	}
}

func assertCheckoutLifetimeSuccess(t *testing.T, retain, reused bool) {
	t.Helper()
	preparation := workers.FactoryWorktreePreparation{CheckoutPath: "C:/fixture/checkout", Reused: reused}
	var mu sync.Mutex
	var events []string
	var released []workers.FactoryWorktreePreparation
	var releaseContextErrors []error
	var observations []workers.ExecutionObservation
	appendEvent := func(event string) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	}
	temporaryFiles := &recordingTemporaryFiles{remove: func(path string) error {
		appendEvent("removed-" + path)
		return nil
	}}
	release := func(ctx context.Context, got workers.FactoryWorktreePreparation) error {
		mu.Lock()
		defer mu.Unlock()
		released = append(released, got)
		releaseContextErrors = append(releaseContextErrors, ctx.Err())
		events = append(events, "released-checkout")
		return nil
	}
	service := mustExecuteServiceWithEdges(t, &stubRunner{
		execute: func(_ context.Context, request workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			file, err := request.TemporaryFiles.CreateTemp("", "attempt-*")
			if err != nil {
				return workers.RunnerExecutionResult{}, err
			}
			if err := file.Close(); err != nil {
				return workers.RunnerExecutionResult{}, err
			}
			return workers.RunnerExecutionResult{Content: "checkout-output"}, nil
		},
	}, func(_ context.Context, observation workers.ExecutionObservation) error {
		mu.Lock()
		defer mu.Unlock()
		observations = append(observations, observation.Clone())
		events = append(events, string(observation.Kind))
		return nil
	}, &recordingWorktree{preparation: preparation}, release, temporaryFiles)
	request := validExecuteRequest("dispatch-"+t.Name(), "attempt-"+t.Name())
	request.Target.Environment.SkipProcessInheritance = true
	request.Target.Workspace = workers.WorkspacePolicy{
		PrepareWorktree: true, FactoryDirectory: "C:/fixture",
		CheckoutIdentifier: request.Correlation.AttemptID, RetainWorktree: retain,
	}
	result, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertAcceptedResult(t, result, request.Correlation.DispatchID, request.Correlation.AttemptID, "checkout-output")
	if result.Failure != nil {
		t.Fatalf("accepted result failure = %#v, want nil", result.Failure)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(observations) != 2 {
		t.Fatalf("observations = %#v, want one STARTED and one COMPLETED", observations)
	}
	assertCompletedObservationShape(t, observations)
	assertSuccessfulCheckoutEffects(t, preparation, retain, events, released, releaseContextErrors)
	if got := temporaryFiles.Removed(); !reflect.DeepEqual(got, []string{"attempt-temp-1"}) {
		t.Fatalf("removed paths = %v, want exactly the attempt-created file once", got)
	}
}

func assertSuccessfulCheckoutEffects(
	t *testing.T,
	preparation workers.FactoryWorktreePreparation,
	retain bool,
	events []string,
	released []workers.FactoryWorktreePreparation,
	releaseContextErrors []error,
) {
	t.Helper()
	wantEvents := []string{"STARTED"}
	wantReleases := 0
	if !retain && !preparation.Reused {
		wantReleases = 1
		wantEvents = append(wantEvents, "released-checkout")
	}
	wantEvents = append(wantEvents, "removed-attempt-temp-1", "COMPLETED")
	// The ordered effect ledger proves cleanup finished before terminal delivery,
	// and its final length also catches releases or removals after that callback.
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("effects = %v, want %v", events, wantEvents)
	}
	if len(released) != wantReleases {
		t.Fatalf("release count = %d, want %d", len(released), wantReleases)
	}
	for index, got := range released {
		if got != preparation || releaseContextErrors[index] != nil {
			t.Fatalf("release = %#v, context error = %v; want %#v and usable context", got, releaseContextErrors[index], preparation)
		}
	}
}

func TestExecuteCheckoutLifetimeCancellation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		retain bool
		reused bool
	}{
		{"C-N_given_new_nonretained_when_canceled_then_release_once", false, false},
		{"C-R_given_new_retained_when_canceled_then_preserve_checkout", true, false},
		{"C-U_given_reused_nonretained_when_canceled_then_preserve_checkout", false, true},
		{"C-RU_given_retained_reused_when_canceled_then_preserve_checkout", true, true},
	}
	for _, scenario := range cases {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			assertCheckoutLifetimeCancellation(t, scenario.retain, scenario.reused)
		})
	}
}

func assertCheckoutLifetimeCancellation(t *testing.T, retain, reused bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	preparation := workers.FactoryWorktreePreparation{CheckoutPath: "C:/fixture/checkout", Reused: reused}
	var mu sync.Mutex
	var events []string
	var released []workers.FactoryWorktreePreparation
	var releaseContextErrors []error
	var observations []workers.ExecutionObservation
	ready := make(chan struct{})
	temporaryFiles := &recordingTemporaryFiles{remove: func(path string) error {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, "removed-"+path)
		return nil
	}}
	release := func(cleanupCtx context.Context, got workers.FactoryWorktreePreparation) error {
		mu.Lock()
		defer mu.Unlock()
		released = append(released, got)
		releaseContextErrors = append(releaseContextErrors, cleanupCtx.Err())
		// Complete a context-sensitive effect, rather than only recording invocation.
		if err := cleanupCtx.Err(); err != nil {
			return err
		}
		events = append(events, "released-checkout")
		return nil
	}
	service := mustExecuteServiceWithEdges(t, &stubRunner{execute: func(runnerCtx context.Context, request workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
		file, err := request.TemporaryFiles.CreateTemp("", "attempt-*")
		if err != nil {
			return workers.RunnerExecutionResult{}, err
		}
		if err := file.Close(); err != nil {
			return workers.RunnerExecutionResult{}, err
		}
		close(ready)
		<-runnerCtx.Done()
		return workers.RunnerExecutionResult{}, runnerCtx.Err()
	}}, func(_ context.Context, observation workers.ExecutionObservation) error {
		mu.Lock()
		defer mu.Unlock()
		observations = append(observations, observation.Clone())
		events = append(events, string(observation.Kind))
		return nil
	}, &recordingWorktree{preparation: preparation}, release, temporaryFiles)
	request := validExecuteRequest("dispatch-"+t.Name(), "attempt-"+t.Name())
	request.Target.Environment.SkipProcessInheritance = true
	request.Target.Workspace = workers.WorkspacePolicy{
		PrepareWorktree: true, FactoryDirectory: "C:/fixture",
		CheckoutIdentifier: request.Correlation.AttemptID, RetainWorktree: retain,
	}
	result := executeAtCancellationBarrier(t, ctx, cancel, service, request, ready)
	mu.Lock()
	defer mu.Unlock()
	assertCanceledCheckoutResult(t, request, result, observations)
	assertCanceledCheckoutEffects(t, preparation, retain, events, released, releaseContextErrors, temporaryFiles.Removed())
}

func assertCanceledCheckoutEffects(t *testing.T, preparation workers.FactoryWorktreePreparation, retain bool, events []string, released []workers.FactoryWorktreePreparation, releaseContextErrors []error, removed []string) {
	t.Helper()
	wantEvents := []string{"STARTED"}
	wantReleases := 0
	if !retain && !preparation.Reused {
		wantReleases = 1
		wantEvents = append(wantEvents, "released-checkout")
	}
	wantEvents = append(wantEvents, "removed-attempt-temp-1", "CANCELED")
	if !reflect.DeepEqual(events, wantEvents) || len(released) != wantReleases {
		t.Fatalf("effects = %v, release count = %d; want %v and %d", events, len(released), wantEvents, wantReleases)
	}
	for index, got := range released {
		if got != preparation || releaseContextErrors[index] != nil {
			t.Fatalf("release = %#v, context error = %v; want %#v and usable context", got, releaseContextErrors[index], preparation)
		}
	}
	if !reflect.DeepEqual(removed, []string{"attempt-temp-1"}) {
		t.Fatalf("removed paths = %v, want exactly the attempt-created file once", removed)
	}
}

func executeAtCancellationBarrier(t *testing.T, ctx context.Context, cancel context.CancelFunc, service *executeservice.Service, request workers.ExecuteRequest, ready <-chan struct{}) workers.ExecuteResult {
	t.Helper()
	type completion struct {
		result workers.ExecuteResult
		err    error
	}
	done := make(chan completion, 1)
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		result, err := service.Execute(ctx, request)
		done <- completion{result: result, err: err}
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-joined:
		case <-time.After(10 * time.Second):
			t.Error("Execute goroutine did not finish after cancellation")
		}
	})
	// These deadlines only bound a broken fixture; readiness triggers cancellation.
	select {
	case <-ready:
	case completed := <-done:
		t.Fatalf("Execute returned before runner readiness: %#v, %v", completed.result, completed.err)
	case <-time.After(10 * time.Second):
		t.Fatal("runner did not signal readiness")
	}
	cancel()
	select {
	case completed := <-done:
		if completed.err != nil {
			t.Fatalf("Execute() error = %v, want normalized canceled result", completed.err)
		}
		return completed.result
	case <-time.After(10 * time.Second):
		t.Fatal("Execute did not return after cancellation")
		return workers.ExecuteResult{}
	}
}

func assertCanceledCheckoutResult(t *testing.T, request workers.ExecuteRequest, result workers.ExecuteResult, observations []workers.ExecutionObservation) {
	t.Helper()
	if result.Outcome != workers.ExecutionOutcomeCanceled || result.Failure == nil ||
		result.Failure.Type != workers.WorkFailureTypeUnknown || result.Failure.Family != workers.WorkFailureFamilyTerminal {
		t.Fatalf("result = %#v, want CANCELED with unknown terminal failure", result)
	}
	if result.Correlation != request.Correlation {
		t.Fatalf("result correlation = %#v, want %#v", result.Correlation, request.Correlation)
	}
	if len(observations) != 2 || observations[0].Kind != workers.ExecutionObservationKindStarted ||
		observations[1].Kind != workers.ExecutionObservationKindCanceled {
		t.Fatalf("observations = %#v, want exactly STARTED then CANCELED", observations)
	}
	if observations[1].Sequence != 2 || observations[1].Correlation != request.Correlation {
		t.Fatalf("terminal observation = %#v, want sequence 2 and request correlation", observations[1])
	}
}

type executeLogRecord struct {
	level   string
	message string
	fields  map[string]any
}

type executeCaptureLogger struct {
	mu      sync.Mutex
	records []executeLogRecord
}

func (logger *executeCaptureLogger) Debug(message string, fields ...any) {
	logger.record("debug", message, fields)
}
func (logger *executeCaptureLogger) Error(message string, fields ...any) {
	logger.record("error", message, fields)
}
func (logger *executeCaptureLogger) Verbose(message string, fields ...any) {
	logger.record("verbose", message, fields)
}
func (logger *executeCaptureLogger) Info(message string, fields ...any) {
	logger.record("info", message, fields)
}
func (logger *executeCaptureLogger) Warn(message string, fields ...any) {
	logger.record("warn", message, fields)
}
func (logger *executeCaptureLogger) record(level, message string, fields []any) {
	logger.mu.Lock()
	defer logger.mu.Unlock()
	values := make(map[string]any)
	for i := 0; i < len(fields); i += 2 {
		values[fields[i].(string)] = fields[i+1]
	}
	logger.records = append(logger.records, executeLogRecord{level, message, values})
}

func assertExecuteLogs(t *testing.T, logger *executeCaptureLogger, correlation workers.ExecutionCorrelation, outcome workers.ExecutionOutcome, categories ...string) {
	t.Helper()
	logger.mu.Lock()
	defer logger.mu.Unlock()
	var warnings []string
	var info []string
	for _, record := range logger.records {
		want := map[string]any{
			"factory_session_id": correlation.FactorySessionID, "runtime_id": correlation.RuntimeID,
			"generation_id": correlation.GenerationID, "dispatch_id": correlation.DispatchID,
			"attempt_id": correlation.AttemptID, "request_id": correlation.RequestID, "trace_id": correlation.TraceID,
		}
		switch record.message {
		case "workers execute started":
			want["runner_id"] = "script"
		case "workers execute finished":
			want["outcome"] = string(outcome)
			want["duration_ms"] = int64(0)
		case "workers execute cleanup failed":
			want["error"] = record.fields["error"]
		case "workers observation delivery failed":
			kind := "STARTED"
			if len(warnings) > 0 {
				kind = "COMPLETED"
			}
			want["kind"] = kind
			want["error"] = record.fields["error"]
		default:
			t.Fatalf("unexpected log: %#v", record)
		}
		if !reflect.DeepEqual(record.fields, want) {
			t.Fatalf("fields = %#v, want %#v", record.fields, want)
		}
		wantLevel := "info"
		if record.message == "workers execute cleanup failed" || record.message == "workers observation delivery failed" {
			wantLevel = "warn"
		}
		if record.level != wantLevel {
			t.Fatalf("level = %q, want %q", record.level, wantLevel)
		}
		if record.level == "warn" {
			warnings = append(warnings, record.fields["error"].(string))
		} else {
			info = append(info, record.message)
		}
		if strings.Contains(fmt.Sprint(record), "secret") {
			t.Fatalf("sensitive log: %#v", record)
		}
	}
	if !reflect.DeepEqual(warnings, categories) {
		t.Fatalf("warnings = %v, want %v", warnings, categories)
	}
	if !reflect.DeepEqual(info, []string{"workers execute started", "workers execute finished"}) {
		t.Fatalf("info = %v", info)
	}
}

func assertSafeCleanupAttempt(t *testing.T, panics, runnerFails bool) {
	t.Helper()
	capture := &executeCaptureLogger{}
	var effects []string
	var observations []workers.ExecutionObservation
	workspace := &recordingWorktree{preparation: workers.FactoryWorktreePreparation{CheckoutPath: "secret/path"}}
	release := func(ctx context.Context, _ workers.FactoryWorktreePreparation) error {
		if ctx.Err() != nil {
			t.Errorf("cleanup context = %v", ctx.Err())
		}
		effects = append(effects, "release")
		if panics {
			panic("secret cleanup panic")
		}
		return errors.New("secret cleanup error")
	}
	files := &recordingTemporaryFiles{remove: func(string) error { effects = append(effects, "remove"); return nil }}
	runner := cleanupDiagnosticRunner(runnerFails)
	service := mustExecuteServiceWithEdges(t, runner, func(_ context.Context, observation workers.ExecutionObservation) error {
		observations = append(observations, observation)
		effects = append(effects, string(observation.Kind))
		return nil
	}, workspace, release, files, capture)
	request := validExecuteRequest("dispatch-cleanup-safe", "attempt-cleanup-safe")
	request.Target.Workspace = workers.WorkspacePolicy{PrepareWorktree: true, FactoryDirectory: "secret/factory", CheckoutIdentifier: "checkout"}
	result, err := service.Execute(context.Background(), request)
	if err != nil || result.Outcome != workers.ExecutionOutcomeFailed || result.Failure == nil {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	wantType, wantMessage := workers.WorkFailureTypeInternalServerError, "execution cleanup failed"
	if runnerFails {
		wantType, wantMessage = workers.WorkFailureTypeUnknown, "prior failure"
	}
	assertCleanupFailure(t, result, wantType, wantMessage)
	if !reflect.DeepEqual(effects, []string{"STARTED", "release", "remove", "FAILED"}) {
		t.Fatalf("effects = %v", effects)
	}
	if len(observations) != 2 || observations[0].Sequence != 1 || observations[1].Sequence != 2 {
		t.Fatalf("observations = %#v", observations)
	}
	category := "cleanup_error"
	if panics {
		category = "cleanup_panic"
	}
	assertExecuteLogs(t, capture, request.Correlation, result.Outcome, category)
}

func TestExecuteRequestLoggerSelectionRemainsOptional(t *testing.T) {
	t.Parallel()
	for _, selected := range []bool{false, true} {
		t.Run(fmt.Sprintf("selected=%v", selected), func(t *testing.T) {
			t.Parallel()
			diagnostics := &executeCaptureLogger{}
			requestSink := &executeCaptureLogger{}
			var selectedSink logging.Logger
			if selected {
				selectedSink = requestSink
			}
			runner := &stubRunner{execute: func(_ context.Context, request workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
				if request.ExecutionLogger != selectedSink {
					t.Errorf("request logger = %v, want selected sink", request.ExecutionLogger)
				}
				return workers.RunnerExecutionResult{Content: "accepted"}, nil
			}}
			service := mustExecuteService(t, runner, nil, diagnostics)
			request := validExecuteRequest("dispatch-optional", "attempt-optional")
			request.Input.ExecutionLogger = selectedSink
			result, err := service.Execute(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			assertAcceptedResult(t, result, request.Correlation.DispatchID, request.Correlation.AttemptID, "accepted")
			assertExecuteLogs(t, diagnostics, request.Correlation, result.Outcome)
			if len(requestSink.records) != 0 {
				t.Fatalf("service diagnostics reached request sink: %#v", requestSink.records)
			}
		})
	}
}

func cleanupDiagnosticRunner(runnerFails bool) *stubRunner {
	return &stubRunner{execute: func(_ context.Context, request workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
		file, err := request.TemporaryFiles.CreateTemp("", "")
		if err != nil {
			return workers.RunnerExecutionResult{}, err
		}
		if err := file.Close(); err != nil {
			return workers.RunnerExecutionResult{}, err
		}
		if runnerFails {
			return workers.RunnerExecutionResult{}, workers.NewProviderError(workers.WorkFailureTypeUnknown, "prior failure", nil)
		}
		return workers.RunnerExecutionResult{Content: "secret provider output"}, nil
	}}
}

func assertCleanupFailure(t *testing.T, result workers.ExecuteResult, wantType workers.WorkFailureType, wantMessage string) {
	t.Helper()
	if result.Failure.Type != wantType || result.Failure.Message != wantMessage || result.Failure.Family != workers.WorkFailureFamilyTerminal || result.Failure.RetryHint {
		t.Fatalf("failure = %#v", result.Failure)
	}
}
