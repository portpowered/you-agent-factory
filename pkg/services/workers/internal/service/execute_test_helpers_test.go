package service_test

import (
	"context"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	executeservice "github.com/portpowered/infinite-you/pkg/services/workers/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
)

func mustExecuteService(
	t *testing.T,
	runner workers.Runner,
	observe workers.ObservationSink,
) *executeservice.Service {
	return mustExecuteServiceWithEdges(t, runner, observe, nil, nil, nil)
}

func mustExecuteServiceWithEdges(
	t *testing.T,
	runner workers.Runner,
	observe workers.ObservationSink,
	worktree workers.FactoryWorktreePreparer,
	worktreeRelease func(context.Context, workers.FactoryWorktreePreparation) error,
	temporaryFiles workers.TemporaryFileSystem,
) *executeservice.Service {
	t.Helper()
	service, err := executeservice.New(
		&staticRunners{runner: runner},
		nil,
		observe,
		nil,
		func() time.Time { return time.Unix(10, 0) },
		worktree,
		worktreeRelease,
		temporaryFiles,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return service
}

type recordingWorktree struct {
	preparation workers.FactoryWorktreePreparation
	release     func(context.Context, workers.FactoryWorktreePreparation) error
}

func (worktree *recordingWorktree) Prepare(
	context.Context,
	string,
	string,
) (workers.FactoryWorktreePreparation, error) {
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
