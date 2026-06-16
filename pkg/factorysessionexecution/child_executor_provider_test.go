package factorysessionexecution

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
)

func TestProviderChildExecutor_Execute_RecordsLiveProviderDispatch(t *testing.T) {
	provider := newUnitMockProvider(interfaces.InferenceResponse{
		Content: `{"text":"bridged-child-output"}`,
		ProviderSession: &interfaces.ProviderSessionMetadata{
			Provider: "mock",
			Kind:     "session_id",
			ID:       "provider-session-42",
		},
	})
	collectorSink := newTestChildRecordSink()
	executor := NewProviderChildExecutor("session-live-child", provider, collectorSink)

	result, err := executor.Execute(context.Background(), workflowruntime.ChildExecutionRequest{
		Prompt:       "summarize workflows",
		Label:        "summarize-findings",
		Model:        "gpt-test",
		WorkflowName: "agent-run-fake-child",
		ArgsSubject:  "workflows",
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
	if result.ExecutionMode != ChildExecutorModeLive {
		t.Fatalf("executionMode = %q, want %q", result.ExecutionMode, ChildExecutorModeLive)
	}
	if result.DispatchID != "dispatch-1" {
		t.Fatalf("dispatchId = %q, want dispatch-1", result.DispatchID)
	}
	if result.ProviderSessionRef != "provider-session-42" {
		t.Fatalf("providerSessionRef = %q, want provider-session-42", result.ProviderSessionRef)
	}
	if got := collectorSink.executionModes(); len(got) == 0 || got[len(got)-1] != ChildExecutorModeLive {
		t.Fatalf("recorded execution modes = %#v, want terminal live-provider", got)
	}
	if provider.callCount != 1 {
		t.Fatalf("provider call count = %d, want 1", provider.callCount)
	}
	if got := collectorSink.statusTransitions("dispatch-1"); len(got) != 3 {
		t.Fatalf("recorded status transitions = %#v, want queued/running/completed", got)
	}
}

type unitMockProvider struct {
	response  interfaces.InferenceResponse
	callCount int
	mu        sync.Mutex
}

func newUnitMockProvider(response interfaces.InferenceResponse) *unitMockProvider {
	return &unitMockProvider{response: response}
}

func (m *unitMockProvider) Infer(_ context.Context, _ interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	return m.response, nil
}

var _ workers.Provider = (*unitMockProvider)(nil)

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

func TestProviderChildExecutor_Execute_FailedChild_RecordsTypedFailureDetail(t *testing.T) {
	provider := newUnitMockProvider(interfaces.InferenceResponse{Content: `{"text":"unused"}`})
	collectorSink := newTestChildRecordSink()
	executor := NewProviderChildExecutor("session-live-child-failure", provider, collectorSink)

	_, err := executor.Execute(context.Background(), workflowruntime.ChildExecutionRequest{
		Prompt:       "fail:simulated provider child error",
		Label:        "summarize-findings",
		WorkflowName: "parallel-child-failure",
	})
	if err == nil {
		t.Fatal("Execute: error = nil, want child failure")
	}

	projection := ProjectRuntimeExecutionRecords("session-live-child-failure", collectorSink.records, time.Now().UTC())
	if len(projection.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v, want one failed dispatch", projection.Dispatches)
	}
	dispatch := projection.Dispatches[0]
	if dispatch.Status != DispatchStatusFailed {
		t.Fatalf("dispatch status = %q, want FAILED", dispatch.Status)
	}
	if dispatch.FailureDetail == nil || dispatch.FailureDetail.Reason != workflowruntime.ChildExecutionFailureReason {
		t.Fatalf("failureDetail = %#v", dispatch.FailureDetail)
	}
	if dispatch.FailureDetail.Message == "" {
		t.Fatal("failureDetail message is empty")
	}
}

func TestProviderChildExecutor_Execute_CanceledContext_InterruptsProviderInfer(t *testing.T) {
	provider := newBlockingMockProvider()
	collectorSink := newTestChildRecordSink()
	executor := NewProviderChildExecutor("session-live-child-cancel", provider, collectorSink)

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
		if !strings.Contains(err.Error(), "context canceled") {
			t.Fatalf("Execute error = %v, want context canceled", err)
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
	executor := NewProviderChildExecutor("session-live-child-timeout", provider, collectorSink)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := executor.Execute(ctx, workflowruntime.ChildExecutionRequest{
		Prompt:       "summarize workflows",
		WorkflowName: "agent-run-fake-child",
	})
	if err == nil {
		t.Fatal("Execute: error = nil, want context deadline exceeded")
	}
	if !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("Execute error = %v, want deadline exceeded", err)
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

func (m *blockingMockProvider) Infer(ctx context.Context, _ interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
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
	return interfaces.InferenceResponse{}, ctx.Err()
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
