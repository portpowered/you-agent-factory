package factorysessionexecution

import (
	"context"
	"sync"
	"testing"

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

	result, err := executor.Execute(workflowruntime.ChildExecutionRequest{
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
