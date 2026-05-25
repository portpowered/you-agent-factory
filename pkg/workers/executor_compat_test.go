package workers

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestRunnerFromProviderUsesProviderInference(t *testing.T) {
	provider := &stubProvider{
		response: interfaces.InferenceResponse{
			Content: "runner-output",
		},
	}
	request := interfaces.RunnerExecutionRequest{
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

func TestNewWorkerPoolDispatchesRegisteredExecutor(t *testing.T) {
	pool := NewWorkerPool(nil)
	result := interfaces.WorkResult{
		DispatchID:   "dispatch-1",
		TransitionID: "transition-1",
		Outcome:      interfaces.OutcomeAccepted,
	}
	pool.Register("worker-a", stubWorkerExecutor{result: result})
	pool.Start()
	defer pool.Stop()

	dispatched := pool.Dispatch("worker-a", interfaces.WorkDispatch{
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
	result := PanicAsFailedResult(interfaces.WorkDispatch{
		DispatchID:   "dispatch-1",
		TransitionID: "transition-1",
	}, "boom", duration)

	if result.DispatchID != "dispatch-1" || result.TransitionID != "transition-1" {
		t.Fatalf("panic result lost dispatch identity: %+v", result)
	}
	if result.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("panic result outcome = %q, want %q", result.Outcome, interfaces.OutcomeFailed)
	}
	if result.Error != "executor panic: boom" {
		t.Fatalf("panic result error = %q", result.Error)
	}
	if result.Metrics.Duration != duration {
		t.Fatalf("panic result duration = %s, want %s", result.Metrics.Duration, duration)
	}
}

func TestWorkLogFieldsAddsStableExecutionFields(t *testing.T) {
	fields := WorkLogFields(interfaces.ExecutionMetadata{
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

type stubProvider struct {
	lastRequest interfaces.ProviderInferenceRequest
	response    interfaces.InferenceResponse
}

func (s *stubProvider) Infer(_ context.Context, req interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	s.lastRequest = req
	return s.response, nil
}

type stubWorkerExecutor struct {
	result interfaces.WorkResult
}

func (s stubWorkerExecutor) Execute(_ context.Context, _ interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	return s.result, nil
}
