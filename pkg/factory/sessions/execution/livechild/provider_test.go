package livechild

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"github.com/portpowered/infinite-you/pkg/workers/provider"
	"github.com/portpowered/infinite-you/pkg/workers/providerexecution"
)

func TestProviderChildExecutor_Execute_RecordsLiveProviderDispatch(t *testing.T) {
	provider := newUnitMockProvider(workerexecution.InferenceResponse{
		Content: `{"text":"bridged-child-output"}`,
		ProviderSession: &workerexecution.ProviderSessionMetadata{
			Provider: "mock",
			Kind:     "session_id",
			ID:       "provider-session-42",
		},
	})
	collectorSink := newTestChildRecordSink()
	executor := NewProviderChildExecutor("session-live-child", providerexecution.NewProviderExecutor(provider), collectorSink)

	result, err := executor.Execute(context.Background(), workflowruntime.ChildExecutionRequest{
		Prompt:        "summarize workflows",
		Label:         "summarize-findings",
		ModelProvider: "CODEX",
		Model:         "gpt-test",
		WorkflowName:  "agent-run-fake-child",
		ArgsSubject:   "workflows",
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{"type": "string"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.ExecutionMode != workflowruntime.ChildExecutionModeLive {
		t.Fatalf("executionMode = %q, want %q", result.ExecutionMode, workflowruntime.ChildExecutionModeLive)
	}
	if result.DispatchID != "dispatch-1" {
		t.Fatalf("dispatchId = %q, want dispatch-1", result.DispatchID)
	}
	if result.ProviderSessionRef != "provider-session-42" {
		t.Fatalf("providerSessionRef = %q, want provider-session-42", result.ProviderSessionRef)
	}
	if got := collectorSink.executionModes(); len(got) == 0 || got[len(got)-1] != workflowruntime.ChildExecutionModeLive {
		t.Fatalf("recorded execution modes = %#v, want terminal live-provider", got)
	}
	if provider.callCount != 1 {
		t.Fatalf("provider call count = %d, want 1", provider.callCount)
	}
	if provider.lastReq.ModelProvider != "CODEX" || provider.lastReq.Model != "gpt-test" {
		t.Fatalf("provider worker settings = (%q, %q), want (CODEX, gpt-test)", provider.lastReq.ModelProvider, provider.lastReq.Model)
	}
	if got := collectorSink.statusTransitions("dispatch-1"); len(got) != 3 {
		t.Fatalf("recorded status transitions = %#v, want queued/running/completed", got)
	}
	completed := collectorSink.completedDispatchRecord("dispatch-1")
	if completed == nil {
		t.Fatal("expected completed child dispatch record")
	}
	if completed.Output["text"] != "bridged-child-output" {
		t.Fatalf("completed output text = %#v, want bridged-child-output", completed.Output["text"])
	}
}

type unitMockProvider struct {
	response  workerexecution.InferenceResponse
	callCount int
	lastReq   workerexecution.ProviderInferenceRequest
	mu        sync.Mutex
}

func newUnitMockProvider(response workerexecution.InferenceResponse) *unitMockProvider {
	return &unitMockProvider{response: response}
}

func (m *unitMockProvider) Infer(_ context.Context, req workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	m.lastReq = req
	return m.response, nil
}

type failingUnitMockProvider struct {
	err       error
	callCount int
	mu        sync.Mutex
}

type sequencedUnitMockProvider struct {
	results []struct {
		response workerexecution.InferenceResponse
		err      error
	}
	callCount int
}

func (m *sequencedUnitMockProvider) Infer(_ context.Context, _ workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	index := m.callCount
	m.callCount++
	if index >= len(m.results) {
		return workerexecution.InferenceResponse{}, provider.NewProviderError(workerexecution.WorkFailureTypeUnknown, "unexpected provider call", nil)
	}
	return m.results[index].response, m.results[index].err
}

func newFailingUnitMockProvider(err error) *failingUnitMockProvider {
	return &failingUnitMockProvider{err: err}
}

func (m *failingUnitMockProvider) Infer(_ context.Context, _ workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	return workerexecution.InferenceResponse{}, m.err
}

var _ workers.Provider = (*unitMockProvider)(nil)
var _ workers.Provider = (*failingUnitMockProvider)(nil)
var _ workers.Provider = (*sequencedUnitMockProvider)(nil)

type testChildRecordSink struct {
	records []workflowruntime.RuntimeRecord
}

func newTestChildRecordSink() *testChildRecordSink {
	return &testChildRecordSink{}
}

func (s *testChildRecordSink) Append(record workflowruntime.RuntimeRecord) {
	s.records = append(s.records, record)
}

func (s *testChildRecordSink) AppendChildDispatch(base workflowruntime.ChildDispatchRecord, status string) {
	record := base
	record.Status = status
	s.Append(workflowruntime.RuntimeRecord{
		Kind:          workflowruntime.RecordKindChildDispatch,
		ChildDispatch: &record,
	})
}

func (s *testChildRecordSink) NextChildDispatchIdentity() (string, int) {
	return "dispatch-1", 1
}

func (s *testChildRecordSink) NextChildArtifactID() string {
	return "child-artifact-1"
}

func (s *testChildRecordSink) executionModes() []string {
	modes := make([]string, 0)
	for _, record := range s.records {
		if record.ChildDispatch == nil {
			continue
		}
		if mode := record.ChildDispatch.ExecutionMode; mode != "" {
			modes = append(modes, mode)
		}
	}
	return modes
}

func (s *testChildRecordSink) statusTransitions(dispatchID string) []string {
	transitions := make([]string, 0)
	for _, record := range s.records {
		if record.ChildDispatch == nil || record.ChildDispatch.DispatchID != dispatchID {
			continue
		}
		status := record.ChildDispatch.Status
		if status == "" {
			continue
		}
		if len(transitions) > 0 && transitions[len(transitions)-1] == status {
			continue
		}
		transitions = append(transitions, status)
	}
	return transitions
}

func (s *testChildRecordSink) completedDispatchRecord(dispatchID string) *workflowruntime.ChildDispatchRecord {
	for i := len(s.records) - 1; i >= 0; i-- {
		record := s.records[i]
		if record.ChildDispatch == nil || record.ChildDispatch.DispatchID != dispatchID {
			continue
		}
		if record.ChildDispatch.Status == workflowruntime.ChildDispatchStatusCompleted {
			child := *record.ChildDispatch
			return &child
		}
	}
	return nil
}

func (s *testChildRecordSink) failedDispatchRecords(dispatchID string) []workflowruntime.ChildDispatchRecord {
	records := make([]workflowruntime.ChildDispatchRecord, 0)
	for _, record := range s.records {
		if record.ChildDispatch == nil || record.ChildDispatch.DispatchID != dispatchID {
			continue
		}
		if record.ChildDispatch.Status == workflowruntime.ChildDispatchStatusFailed {
			records = append(records, *record.ChildDispatch)
		}
	}
	return records
}

func TestProviderChildExecutor_Execute_FailedChild_RecordsTypedFailureDetail(t *testing.T) {
	const failureMessage = "The 'gpt-5.6-sol' model requires a newer version of Codex. Please upgrade to the latest app or CLI and try again."
	provider := newFailingUnitMockProvider(provider.NewProviderError(
		workerexecution.WorkFailureTypePermanentBadRequest,
		failureMessage,
		nil,
	))
	collectorSink := newTestChildRecordSink()
	executor := NewProviderChildExecutor("session-live-child-failure", providerexecution.NewProviderExecutor(provider), collectorSink)

	_, err := executor.Execute(context.Background(), workflowruntime.ChildExecutionRequest{
		Prompt:       "summarize workflows",
		Label:        "summarize-findings",
		WorkflowName: "parallel-child-failure",
	})
	if err == nil {
		t.Fatal("Execute: error = nil, want child failure")
	}
	if provider.callCount != 1 {
		t.Fatalf("provider call count = %d, want 1", provider.callCount)
	}

	failedRecords := collectorSink.failedDispatchRecords("dispatch-1")
	if len(failedRecords) != 1 {
		t.Fatalf("failed dispatch records = %d, want 1", len(failedRecords))
	}
	failed := failedRecords[0]
	if failed.Status != workflowruntime.ChildDispatchStatusFailed {
		t.Fatalf("dispatch status = %q, want FAILED", failed.Status)
	}
	if failed.FailureDetail == nil || failed.FailureDetail.Reason != workerexecution.WorkFailureTypePermanentBadRequest {
		t.Fatalf("failureDetail = %#v, want %q", failed.FailureDetail, workerexecution.WorkFailureTypePermanentBadRequest)
	}
	if failed.FailureDetail.Message != "Provider rejected the request as invalid." {
		t.Fatalf("failure message = %q", failed.FailureDetail.Message)
	}
	if failed.Retryable == nil || *failed.Retryable || failed.FailureClassification != workerexecution.WorkFailureTypePermanentBadRequest {
		t.Fatalf("retry diagnostics = retryable:%v classification:%q", failed.Retryable, failed.FailureClassification)
	}
	if strings.Contains(failed.FailureDetail.Message, "gpt-5.6-sol") {
		t.Fatalf("public failure message leaked provider detail: %q", failed.FailureDetail.Message)
	}
	encoded, marshalErr := json.Marshal(failed)
	if marshalErr != nil {
		t.Fatalf("marshal failed record: %v", marshalErr)
	}
	for _, legacy := range []string{"failureReason", "failureMessage", "failureErrorClass", "errorClass"} {
		if strings.Contains(string(encoded), legacy) {
			t.Fatalf("serialized failed record contains legacy field %q: %s", legacy, encoded)
		}
	}
}

func TestProviderChildExecutor_Execute_RetryThenSuccessRecordsSuccessfulAttempt(t *testing.T) {
	retryable := provider.NewProviderError(workerexecution.WorkFailureTypeInternalServerError, "temporary server error", nil)
	mock := &sequencedUnitMockProvider{results: []struct {
		response workerexecution.InferenceResponse
		err      error
	}{
		{err: retryable},
		{response: workerexecution.InferenceResponse{Content: "recovered"}},
	}}
	executor := NewRetryingProviderChildExecutor("session-retry-success", providerexecution.NewProviderExecutor(mock), newTestChildRecordSink(), 1)
	executor.sleep = func(context.Context, time.Duration) error { return nil }

	result, err := executor.Execute(context.Background(), workflowruntime.ChildExecutionRequest{Prompt: "retry me"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mock.callCount != 2 {
		t.Fatalf("provider call count = %d, want 2", mock.callCount)
	}
	if result.Status != workflowruntime.ChildDispatchStatusCompleted {
		t.Fatalf("status = %q, want COMPLETED", result.Status)
	}
	completed := executor.records.(*testChildRecordSink).completedDispatchRecord("dispatch-1")
	if completed == nil || completed.Attempt != 2 {
		t.Fatalf("completed dispatch = %#v, want attempt 2", completed)
	}
}

func TestProviderChildExecutor_Execute_RetryExhaustionUsesConfiguredLimit(t *testing.T) {
	retryable := provider.NewProviderError(workerexecution.WorkFailureTypeTimeout, "provider timed out", nil)
	mock := &sequencedUnitMockProvider{results: []struct {
		response workerexecution.InferenceResponse
		err      error
	}{{err: retryable}, {err: retryable}, {err: retryable}}}
	sink := newTestChildRecordSink()
	executor := NewRetryingProviderChildExecutor("session-retry-exhausted", providerexecution.NewProviderExecutor(mock), sink, 2)
	executor.sleep = func(context.Context, time.Duration) error { return nil }

	_, err := executor.Execute(context.Background(), workflowruntime.ChildExecutionRequest{Prompt: "keep retrying"})
	if err == nil {
		t.Fatal("Execute: error = nil, want exhausted retry failure")
	}
	if mock.callCount != 3 {
		t.Fatalf("provider call count = %d, want 3", mock.callCount)
	}
	failed := sink.failedDispatchRecords("dispatch-1")
	if len(failed) != 1 || failed[0].Attempt != 3 {
		t.Fatalf("failed dispatches = %#v, want one failure at attempt 3", failed)
	}
	if failed[0].Retryable == nil || !*failed[0].Retryable || failed[0].FailureClassification != workerexecution.WorkFailureTypeTimeout {
		t.Fatalf("retry diagnostics = retryable:%v classification:%q", failed[0].Retryable, failed[0].FailureClassification)
	}
}

func TestProviderChildExecutor_Execute_CancellationPreventsNextAttempt(t *testing.T) {
	retryable := provider.NewProviderError(workerexecution.WorkFailureTypeInternalServerError, "temporary server error", nil)
	mock := &sequencedUnitMockProvider{results: []struct {
		response workerexecution.InferenceResponse
		err      error
	}{{err: retryable}, {response: workerexecution.InferenceResponse{Content: "must not run"}}}}
	executor := NewRetryingProviderChildExecutor("session-retry-canceled", providerexecution.NewProviderExecutor(mock), newTestChildRecordSink(), 1)
	ctx, cancel := context.WithCancel(context.Background())
	executor.sleep = func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	}

	_, err := executor.Execute(ctx, workflowruntime.ChildExecutionRequest{Prompt: "cancel retry"})
	if err == nil || err.Error() != "Provider execution failed." {
		t.Fatalf("Execute error = %v, want canonical cancellation diagnostic", err)
	}
	if mock.callCount != 1 {
		t.Fatalf("provider call count = %d, want 1", mock.callCount)
	}
}

func TestProviderChildExecutor_Execute_CanceledContext_InterruptsProviderInfer(t *testing.T) {
	provider := newBlockingMockProvider()
	collectorSink := newTestChildRecordSink()
	executor := NewProviderChildExecutor("session-live-child-cancel", providerexecution.NewProviderExecutor(provider), collectorSink)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := executor.Execute(ctx, workflowruntime.ChildExecutionRequest{
			Prompt:       "summarize workflows",
			WorkflowName: "agent-run-fake-child",
		})
		done <- err
	}()

	provider.waitForInferStart(t)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Execute: error = nil, want context cancellation")
		}
		if err.Error() != "Provider execution failed." {
			t.Fatalf("Execute error = %v, want canonical cancellation diagnostic", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Execute did not return after context cancellation")
	}
	if provider.inferContextsHonored() == 0 {
		t.Fatal("provider Infer did not observe canceled context")
	}
}

func TestProviderChildExecutor_Execute_TimedOutContext_InterruptsProviderInfer(t *testing.T) {
	provider := newBlockingMockProvider()
	collectorSink := newTestChildRecordSink()
	executor := NewProviderChildExecutor("session-live-child-timeout", providerexecution.NewProviderExecutor(provider), collectorSink)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := executor.Execute(ctx, workflowruntime.ChildExecutionRequest{
		Prompt:       "summarize workflows",
		WorkflowName: "agent-run-fake-child",
	})
	if err == nil {
		t.Fatal("Execute: error = nil, want context deadline exceeded")
	}
	if err.Error() != "Provider request timed out." {
		t.Fatalf("Execute error = %v, want canonical timeout diagnostic", err)
	}
	if provider.inferContextsHonored() == 0 {
		t.Fatal("provider Infer did not observe timed-out context")
	}
}

type blockingMockProvider struct {
	mu              sync.Mutex
	inferStarted    chan struct{}
	inferStartedSet bool
	contextCanceled int
}

func newBlockingMockProvider() *blockingMockProvider {
	return &blockingMockProvider{
		inferStarted: make(chan struct{}),
	}
}

func (m *blockingMockProvider) Infer(ctx context.Context, _ workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	m.mu.Lock()
	if !m.inferStartedSet {
		m.inferStartedSet = true
		close(m.inferStarted)
	}
	m.mu.Unlock()

	<-ctx.Done()
	m.mu.Lock()
	m.contextCanceled++
	m.mu.Unlock()
	return workerexecution.InferenceResponse{}, ctx.Err()
}

func (m *blockingMockProvider) waitForInferStart(t *testing.T) {
	t.Helper()
	select {
	case <-m.inferStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("provider Infer did not start")
	}
}

func (m *blockingMockProvider) inferContextsHonored() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.contextCanceled
}
