package workers

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/work"
)

func TestRunnerFromProviderUsesProviderInference(t *testing.T) {
	provider := &stubProvider{
		response: workerexecution.InferenceResponse{
			Content: "runner-output",
		},
	}
	request := workerexecution.RunnerExecutionRequest{
		SystemPrompt: "system",
		UserMessage:  "user",
	}

	result, err := RunnerFromProvider(provider).Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("runner execution returned error: %v", err)
	}
	if provider.lastRequest.SystemPrompt != request.SystemPrompt || provider.lastRequest.UserMessage != request.UserMessage {
		t.Fatalf("runner passed unexpected request to provider: %+v", provider.lastRequest)
	}
	if result.Content != provider.response.Content {
		t.Fatalf("runner returned content %q, want %q", result.Content, provider.response.Content)
	}
}

func TestRunnerFromProviderPreservesProviderFailureAndRequest(t *testing.T) {
	providerErr := errors.New("canonical provider failure")
	provider := &stubProvider{err: providerErr}
	request := workerexecution.RunnerExecutionRequest{
		SystemPrompt:     "system",
		UserMessage:      "user",
		ModelProvider:    string(modelprovider.Cursor),
		Model:            "cursor-model",
		WorkingDirectory: "/tmp/runtime-worktree",
		SessionID:        "session-1",
	}

	result, err := RunnerFromProvider(provider).Execute(context.Background(), request)
	if !errors.Is(err, providerErr) {
		t.Fatalf("runner error = %v, want provider error %v", err, providerErr)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
	if !reflect.DeepEqual(provider.lastRequest, request) {
		t.Fatalf("provider request = %#v, want %#v", provider.lastRequest, request)
	}
	if !reflect.DeepEqual(result, (workerexecution.RunnerExecutionResult{})) {
		t.Fatalf("runner result = %#v, want zero result on provider failure", result)
	}
}

func TestNewWorkerPoolDispatchesRegisteredExecutor(t *testing.T) {
	pool := NewWorkerPool(nil)
	result := workerexecution.WorkResult{
		DispatchID:   "dispatch-1",
		TransitionID: "transition-1",
		Outcome:      workerexecution.OutcomeAccepted,
	}
	pool.Register("worker-a", stubWorkerExecutor{result: result})
	pool.Start()
	defer pool.Stop()

	dispatched := pool.Dispatch("worker-a", work.WorkDispatch{
		DispatchID:   "dispatch-1",
		TransitionID: "transition-1",
		WorkerType:   "worker-a",
	})
	if !dispatched {
		t.Fatal("expected dispatch to succeed")
	}

	select {
	case got := <-pool.ResultCh():
		if got.DispatchID != result.DispatchID || got.TransitionID != result.TransitionID || got.Outcome != result.Outcome {
			t.Fatalf("pool returned unexpected result: %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for worker pool result")
	}
}

func TestPanicAsFailedResultPreservesDispatchIdentity(t *testing.T) {
	duration := 25 * time.Millisecond
	result := PanicAsFailedResult(work.WorkDispatch{
		DispatchID:   "dispatch-1",
		TransitionID: "transition-1",
	}, "boom", duration)

	if result.DispatchID != "dispatch-1" || result.TransitionID != "transition-1" {
		t.Fatalf("panic result lost dispatch identity: %+v", result)
	}
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("panic result outcome = %q, want %q", result.Outcome, workerexecution.OutcomeFailed)
	}
	if result.Error != "executor panic: boom" {
		t.Fatalf("panic result error = %q", result.Error)
	}
	if result.Metrics.Duration != duration {
		t.Fatalf("panic result duration = %s, want %s", result.Metrics.Duration, duration)
	}
}

func TestWorkLogFieldsAddsStableExecutionFields(t *testing.T) {
	fields := WorkLogFields(work.ExecutionMetadata{
		RequestID: "request-1",
		TraceID:   "trace-1",
		WorkIDs:   []string{"work-1", "work-2"},
	}, "extra", "value")

	expected := []any{
		"request_id", "request-1",
		"trace_id", "trace-1",
		"work_id", "work-1",
		"work_ids", []string{"work-1", "work-2"},
		"extra", "value",
	}
	if len(fields) != len(expected) {
		t.Fatalf("field count = %d, want %d: %#v", len(fields), len(expected), fields)
	}
	for i := range expected {
		switch want := expected[i].(type) {
		case []string:
			got, ok := fields[i].([]string)
			if !ok || len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
				t.Fatalf("field %d = %#v, want %#v", i, fields[i], expected[i])
			}
		default:
			if fields[i] != expected[i] {
				t.Fatalf("field %d = %#v, want %#v", i, fields[i], expected[i])
			}
		}
	}
}

func TestRootExecutionInterfaceAliasesAcceptImplementations(t *testing.T) {
	worker := WorkerExecutor(stubWorkerExecutor{
		result: workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted},
	})
	workerResult, err := worker.Execute(context.Background(), work.WorkDispatch{})
	if err != nil {
		t.Fatalf("worker executor returned error: %v", err)
	}
	if workerResult.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("worker result outcome = %q, want %q", workerResult.Outcome, workerexecution.OutcomeAccepted)
	}

	workstation := WorkstationRequestExecutor(stubWorkstationRequestExecutor{})
	workstationResult, err := workstation.Execute(context.Background(), workerexecution.WorkstationExecutionRequest{})
	if err != nil {
		t.Fatalf("workstation request executor returned error: %v", err)
	}
	if workstationResult.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("workstation result outcome = %q, want %q", workstationResult.Outcome, workerexecution.OutcomeAccepted)
	}

	runner := Runner(stubRunner{})
	runnerResult, err := runner.Execute(context.Background(), workerexecution.RunnerExecutionRequest{})
	if err != nil {
		t.Fatalf("runner returned error: %v", err)
	}
	if runnerResult.Content != "ok" {
		t.Fatalf("runner content = %q, want %q", runnerResult.Content, "ok")
	}
}

var _ WorkerExecutor = stubWorkerExecutor{}
var _ WorkstationRequestExecutor = stubWorkstationRequestExecutor{}
var _ Runner = stubRunner{}

type stubProvider struct {
	lastRequest workerexecution.ProviderInferenceRequest
	response    workerexecution.InferenceResponse
	err         error
	calls       int
}

func (s *stubProvider) Infer(_ context.Context, req workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	s.calls++
	s.lastRequest = req
	return s.response, s.err
}

type stubWorkerExecutor struct {
	result workerexecution.WorkResult
}

func (s stubWorkerExecutor) Execute(_ context.Context, _ work.WorkDispatch) (workerexecution.WorkResult, error) {
	return s.result, nil
}

type stubWorkstationRequestExecutor struct{}

func (stubWorkstationRequestExecutor) Execute(_ context.Context, _ workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
	return workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted}, nil
}

type stubRunner struct{}

func (stubRunner) Execute(_ context.Context, _ workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
	return workerexecution.RunnerExecutionResult{Content: "ok"}, nil
}
