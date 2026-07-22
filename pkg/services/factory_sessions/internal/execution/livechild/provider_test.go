package livechild

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// scriptedChildValues keeps these Factory Session tests on the Factory Runtime
// root contract. Digest algorithm invariants remain owner-local to Factory
// Runtime; this package only verifies that the injected values flow into child
// dispatch records and that provider outputs are copied before publication.
type scriptedChildValues struct{}

func (scriptedChildValues) TextDigest(string) string { return "scripted-prompt-digest" }

func (scriptedChildValues) SchemaDigest(map[string]any) string {
	return "scripted-schema-digest"
}

func (scriptedChildValues) CloneOutputMap(output map[string]any) map[string]any {
	cloned := make(map[string]any, len(output))
	for key, value := range output {
		cloned[key] = value
	}
	return cloned
}

func TestProviderChildExecutor_Execute_RecordsLiveProviderDispatch(t *testing.T) {
	provider := newScriptedInvocationExecutor(successfulInvocation(workerexecution.InferenceResponse{
		Content: `{"text":"bridged-child-output"}`,
		ProviderSession: &workerexecution.ProviderSessionMetadata{
			Provider: "mock",
			Kind:     "session_id",
			ID:       "provider-session-42",
		},
	}))
	collectorSink := newTestChildRecordSink()
	executor := NewProviderChildExecutor("session-live-child", provider, collectorSink, scriptedChildValues{})

	result, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{
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
	if result.ExecutionMode != factory.JavaScriptChildExecutionModeLive {
		t.Fatalf("executionMode = %q, want %q", result.ExecutionMode, factory.JavaScriptChildExecutionModeLive)
	}
	if result.DispatchID != "dispatch-1" {
		t.Fatalf("dispatchId = %q, want dispatch-1", result.DispatchID)
	}
	if result.ProviderSessionRef != "provider-session-42" {
		t.Fatalf("providerSessionRef = %q, want provider-session-42", result.ProviderSessionRef)
	}
	if got := collectorSink.executionModes(); len(got) == 0 || got[len(got)-1] != factory.JavaScriptChildExecutionModeLive {
		t.Fatalf("recorded execution modes = %#v, want terminal live-provider", got)
	}
	if provider.callCount != 1 {
		t.Fatalf("provider call count = %d, want 1", provider.callCount)
	}
	if provider.lastInput.Request.ModelProvider != "CODEX" || provider.lastInput.Request.Model != "gpt-test" {
		t.Fatalf("provider worker settings = (%q, %q), want (CODEX, gpt-test)", provider.lastInput.Request.ModelProvider, provider.lastInput.Request.Model)
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

type invocationStep struct {
	result workerexecution.InvocationResult
	err    error
}

type scriptedInvocationExecutor struct {
	steps     []invocationStep
	callCount int
	lastInput workerexecution.InvocationInput
	mu        sync.Mutex
}

func newScriptedInvocationExecutor(steps ...invocationStep) *scriptedInvocationExecutor {
	return &scriptedInvocationExecutor{steps: steps}
}

func (m *scriptedInvocationExecutor) Execute(
	ctx context.Context,
	input workerexecution.InvocationInput,
) (workerexecution.InvocationResult, error) {
	if err := ctx.Err(); err != nil {
		return canceledInvocationResult(input.Attempt, err), err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	index := m.callCount
	m.callCount++
	m.lastInput = input
	if index >= len(m.steps) {
		return workerexecution.InvocationResult{Attempt: input.Attempt}, errors.New("unexpected scripted invocation")
	}
	step := m.steps[index]
	if step.result.Attempt == 0 {
		step.result.Attempt = input.Attempt
	}
	return step.result, step.err
}

func canceledInvocationResult(attempt int, err error) workerexecution.InvocationResult {
	reason := workerexecution.WorkFailureTypeUnknown
	message := "Provider execution failed."
	if errors.Is(err, context.DeadlineExceeded) {
		reason = workerexecution.WorkFailureTypeTimeout
		message = "Provider request timed out."
	}
	decision := workerexecution.WorkFailureDecision{}
	return workerexecution.InvocationResult{
		Attempt:         attempt,
		FailureMetadata: &workerexecution.WorkFailureMetadata{Family: workerexecution.WorkFailureFamilyTerminal, Type: reason},
		FailureDecision: &decision,
		FailureDetail:   &workerexecution.FailureDetail{Reason: reason, Message: message},
	}
}

func successfulInvocation(response workerexecution.InferenceResponse) invocationStep {
	return invocationStep{
		result: workerexecution.InvocationResult{
			Response:        response,
			ProviderSession: workerexecution.CloneProviderSessionMetadata(response.ProviderSession),
		},
	}
}

func failedInvocation(reason workerexecution.WorkFailureType, retryable bool) invocationStep {
	message := "Provider execution failed."
	switch reason {
	case workerexecution.WorkFailureTypePermanentBadRequest:
		message = "Provider rejected the request as invalid."
	case workerexecution.WorkFailureTypeTimeout:
		message = "Provider request timed out."
	}
	family := workerexecution.WorkFailureFamilyTerminal
	if retryable {
		family = workerexecution.WorkFailureFamilyRetryable
	}
	decision := workerexecution.WorkFailureDecision{Retryable: retryable}
	return invocationStep{
		result: workerexecution.InvocationResult{
			FailureMetadata: &workerexecution.WorkFailureMetadata{Family: family, Type: reason},
			FailureDecision: &decision,
			FailureDetail:   &workerexecution.FailureDetail{Reason: reason, Message: message},
		},
		err: errors.New("scripted invocation failure"),
	}
}

var _ workerexecution.InvocationExecutor = (*scriptedInvocationExecutor)(nil)

type testChildRecordSink struct {
	records []factory.JavaScriptRuntimeRecord
}

func newTestChildRecordSink() *testChildRecordSink {
	return &testChildRecordSink{}
}

func (s *testChildRecordSink) Append(record factory.JavaScriptRuntimeRecord) {
	s.records = append(s.records, record)
}

func (s *testChildRecordSink) AppendChildDispatch(base factory.JavaScriptChildDispatchRecord, status string) {
	record := base
	record.Status = status
	s.Append(factory.JavaScriptRuntimeRecord{
		Kind:          factory.JavaScriptRecordKindChildDispatch,
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

func (s *testChildRecordSink) completedDispatchRecord(dispatchID string) *factory.JavaScriptChildDispatchRecord {
	for i := len(s.records) - 1; i >= 0; i-- {
		record := s.records[i]
		if record.ChildDispatch == nil || record.ChildDispatch.DispatchID != dispatchID {
			continue
		}
		if record.ChildDispatch.Status == factory.JavaScriptChildDispatchStatusCompleted {
			child := *record.ChildDispatch
			return &child
		}
	}
	return nil
}

func (s *testChildRecordSink) failedDispatchRecords(dispatchID string) []factory.JavaScriptChildDispatchRecord {
	records := make([]factory.JavaScriptChildDispatchRecord, 0)
	for _, record := range s.records {
		if record.ChildDispatch == nil || record.ChildDispatch.DispatchID != dispatchID {
			continue
		}
		if record.ChildDispatch.Status == factory.JavaScriptChildDispatchStatusFailed {
			records = append(records, *record.ChildDispatch)
		}
	}
	return records
}

func TestProviderChildExecutor_Execute_FailedChild_RecordsTypedFailureDetail(t *testing.T) {
	const failureMessage = "The 'gpt-5.6-sol' model requires a newer version of Codex. Please upgrade to the latest app or CLI and try again."
	provider := newScriptedInvocationExecutor(failedInvocation(
		workerexecution.WorkFailureTypePermanentBadRequest,
		false,
	))
	collectorSink := newTestChildRecordSink()
	executor := NewProviderChildExecutor("session-live-child-failure", provider, collectorSink, scriptedChildValues{})

	_, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{
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
	if failed.Status != factory.JavaScriptChildDispatchStatusFailed {
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
	retryable := workerexecution.NewProviderError(workerexecution.WorkFailureTypeInternalServerError, "temporary server error", nil)
	_ = retryable
	mock := newScriptedInvocationExecutor(
		failedInvocation(workerexecution.WorkFailureTypeInternalServerError, true),
		successfulInvocation(workerexecution.InferenceResponse{Content: "recovered"}),
	)
	executor := NewRetryingProviderChildExecutor("session-retry-success", mock, newTestChildRecordSink(), 1, scriptedChildValues{})
	executor.sleep = func(context.Context, time.Duration) error { return nil }

	result, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{Prompt: "retry me"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mock.callCount != 2 {
		t.Fatalf("provider call count = %d, want 2", mock.callCount)
	}
	if result.Status != factory.JavaScriptChildDispatchStatusCompleted {
		t.Fatalf("status = %q, want COMPLETED", result.Status)
	}
	completed := executor.records.(*testChildRecordSink).completedDispatchRecord("dispatch-1")
	if completed == nil || completed.Attempt != 2 {
		t.Fatalf("completed dispatch = %#v, want attempt 2", completed)
	}
}

func TestProviderChildExecutor_Execute_RetryExhaustionUsesConfiguredLimit(t *testing.T) {
	retryable := workerexecution.NewProviderError(workerexecution.WorkFailureTypeTimeout, "provider timed out", nil)
	_ = retryable
	mock := newScriptedInvocationExecutor(
		failedInvocation(workerexecution.WorkFailureTypeTimeout, true),
		failedInvocation(workerexecution.WorkFailureTypeTimeout, true),
		failedInvocation(workerexecution.WorkFailureTypeTimeout, true),
	)
	sink := newTestChildRecordSink()
	executor := NewRetryingProviderChildExecutor("session-retry-exhausted", mock, sink, 2, scriptedChildValues{})
	executor.sleep = func(context.Context, time.Duration) error { return nil }

	_, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{Prompt: "keep retrying"})
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
	retryable := workerexecution.NewProviderError(workerexecution.WorkFailureTypeInternalServerError, "temporary server error", nil)
	_ = retryable
	mock := newScriptedInvocationExecutor(
		failedInvocation(workerexecution.WorkFailureTypeInternalServerError, true),
		successfulInvocation(workerexecution.InferenceResponse{Content: "must not run"}),
	)
	executor := NewRetryingProviderChildExecutor("session-retry-canceled", mock, newTestChildRecordSink(), 1, scriptedChildValues{})
	ctx, cancel := context.WithCancel(context.Background())
	executor.sleep = func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	}

	_, err := executor.Execute(ctx, factory.JavaScriptChildExecutionRequest{Prompt: "cancel retry"})
	if err == nil || err.Error() != "Provider execution failed." {
		t.Fatalf("Execute error = %v, want canonical cancellation diagnostic", err)
	}
	if mock.callCount != 1 {
		t.Fatalf("provider call count = %d, want 1", mock.callCount)
	}
}

func TestProviderChildExecutor_Execute_CanceledContext_InterruptsProviderInfer(t *testing.T) {
	provider := newBlockingInvocationExecutor()
	collectorSink := newTestChildRecordSink()
	executor := NewProviderChildExecutor("session-live-child-cancel", provider, collectorSink, scriptedChildValues{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := executor.Execute(ctx, factory.JavaScriptChildExecutionRequest{
			Prompt:       "summarize workflows",
			WorkflowName: "agent-run-fake-child",
		})
		done <- err
	}()

	provider.waitForInvocationStart(t)
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
	if provider.invocationContextsHonored() == 0 {
		t.Fatal("Worker invocation did not observe canceled context")
	}
}

func TestProviderChildExecutor_Execute_TimedOutContext_InterruptsProviderInfer(t *testing.T) {
	provider := newBlockingInvocationExecutor()
	collectorSink := newTestChildRecordSink()
	executor := NewProviderChildExecutor("session-live-child-timeout", provider, collectorSink, scriptedChildValues{})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := executor.Execute(ctx, factory.JavaScriptChildExecutionRequest{
		Prompt:       "summarize workflows",
		WorkflowName: "agent-run-fake-child",
	})
	if err == nil {
		t.Fatal("Execute: error = nil, want context deadline exceeded")
	}
	if err.Error() != "Provider request timed out." {
		t.Fatalf("Execute error = %v, want canonical timeout diagnostic", err)
	}
	if provider.invocationContextsHonored() == 0 {
		t.Fatal("Worker invocation did not observe timed-out context")
	}
}

type blockingInvocationExecutor struct {
	mu              sync.Mutex
	started         chan struct{}
	startedSet      bool
	contextCanceled int
}

func newBlockingInvocationExecutor() *blockingInvocationExecutor {
	return &blockingInvocationExecutor{
		started: make(chan struct{}),
	}
}

func (m *blockingInvocationExecutor) Execute(
	ctx context.Context,
	input workerexecution.InvocationInput,
) (workerexecution.InvocationResult, error) {
	m.mu.Lock()
	if !m.startedSet {
		m.startedSet = true
		close(m.started)
	}
	m.mu.Unlock()

	<-ctx.Done()
	m.mu.Lock()
	m.contextCanceled++
	m.mu.Unlock()
	return canceledInvocationResult(input.Attempt, ctx.Err()), ctx.Err()
}

func (m *blockingInvocationExecutor) waitForInvocationStart(t *testing.T) {
	t.Helper()
	select {
	case <-m.started:
	case <-time.After(2 * time.Second):
		t.Fatal("Worker invocation did not start")
	}
}

func (m *blockingInvocationExecutor) invocationContextsHonored() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.contextCanceled
}

var _ workerexecution.InvocationExecutor = (*blockingInvocationExecutor)(nil)
