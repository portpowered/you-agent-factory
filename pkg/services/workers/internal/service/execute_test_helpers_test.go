package service_test

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
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

func mustExecuteServiceWithContentMaterializer(
	t *testing.T,
	runner workers.Runner,
	materializer work.ContentMaterializer,
) *executeservice.Service {
	t.Helper()
	service, err := executeservice.NewWithProviderOverrideAndContentMaterializer(
		&staticRunners{runner: runner},
		nil,
		nil,
		nil,
		func() time.Time { return time.Unix(10, 0) },
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		materializer,
	)
	if err != nil {
		t.Fatalf("NewWithProviderOverrideAndContentMaterializer() error = %v", err)
	}
	return service
}

func TestExecuteMaterializesWorkContentBeforeRunnerAndCleansItUp(t *testing.T) {
	t.Parallel()

	const materializedPath = "C:/attempt-content/image.png"
	var materializedURL string
	var cleanupCalls atomic.Int32
	var runnerCalls atomic.Int32
	service := mustExecuteServiceWithContentMaterializer(
		t,
		&stubRunner{execute: func(
			_ context.Context,
			request workers.RunnerExecutionRequest,
		) (workers.RunnerExecutionResult, error) {
			runnerCalls.Add(1)
			assertMaterializedContentRequest(t, request, materializedPath)
			return workers.RunnerExecutionResult{Content: "content accepted"}, nil
		}},
		work.ContentMaterializeFunc(func(
			_ context.Context,
			rawURL string,
		) (string, work.ContentCleanup, error) {
			materializedURL = rawURL
			return materializedPath, func() { cleanupCalls.Add(1) }, nil
		}),
	)

	request := validExecuteRequest("dispatch-content", "attempt-content")
	request.Target.Environment.WorkingDirectory = "C:/workspace"
	request.Input.Work = []workers.WorkInput{{
		Name: "submitted-image",
		Content: []work.WorkContentPart{{
			Type:        work.WorkContentPartTypeImage,
			URL:         "file://submitted/image.png",
			ContentType: "image/png",
			Metadata:    map[string]any{"origin": "submitted-work"},
		}},
	}}

	result, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", result.Outcome)
	}
	wantMaterializedURL, err := work.ResolveDispatchContentURL(
		"C:/workspace",
		"file://submitted/image.png",
	)
	if err != nil {
		t.Fatalf("resolve expected materialized URL: %v", err)
	}
	if materializedURL != wantMaterializedURL {
		t.Fatalf("materialized URL = %q, want %q", materializedURL, wantMaterializedURL)
	}
	if runnerCalls.Load() != 1 || cleanupCalls.Load() != 1 {
		t.Fatalf("runner calls = %d, cleanup calls = %d, want one each", runnerCalls.Load(), cleanupCalls.Load())
	}
	original := request.Input.Work[0].Content[0]
	if original.URL != "file://submitted/image.png" || original.File != "" {
		t.Fatalf("caller content mutated = %#v, want original URL-only content", original)
	}
}

func assertMaterializedContentRequest(
	t *testing.T,
	request workers.RunnerExecutionRequest,
	wantPath string,
) {
	t.Helper()
	if len(request.InputTokens) != 1 {
		t.Fatalf("input token count = %d, want 1", len(request.InputTokens))
	}
	token, ok := request.InputTokens[0].(workers.Token)
	if !ok || len(token.Color.Content) != 1 {
		t.Fatalf("runner input token = %#v, want one typed content token", request.InputTokens[0])
	}
	part := token.Color.Content[0]
	if part.File != wantPath || part.URL != "" {
		t.Fatalf("runner content = %#v, want materialized file without source URL", part)
	}
	if part.ContentType != "image/png" || part.Metadata["origin"] != "submitted-work" {
		t.Fatalf("runner content metadata = %#v, want content type and metadata preserved", part.Metadata)
	}
}

type unsafeRemoteContentMaterializer struct{}

func (*unsafeRemoteContentMaterializer) MaterializeContentURL(
	context.Context,
	string,
) (string, work.ContentCleanup, error) {
	return "", nil, fmt.Errorf("materialization should not be reached")
}

func (*unsafeRemoteContentMaterializer) ValidateContentURLSafety(
	context.Context,
	string,
) error {
	return fmt.Errorf("reject private target: %w", work.ErrUnsafeContentURL)
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
	content string
	execute func(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error)
}

func (runner *stubRunner) Execute(
	ctx context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	if runner.execute != nil {
		return runner.execute(ctx, request)
	}
	return workers.RunnerExecutionResult{Content: runner.content}, nil
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
