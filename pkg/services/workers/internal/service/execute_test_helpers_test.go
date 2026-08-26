package service_test

import (
	"context"
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
