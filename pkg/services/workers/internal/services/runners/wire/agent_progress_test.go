package wire

import (
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/services/agent"
)

func TestAgentRunnerPublishesDetachedProviderProgressBeforeSuccess(t *testing.T) {
	fake := newAgentProvidersFake()
	fake.result.Diagnostics.Progress = []providers.ExecuteProgress{
		{
			Phase:    "planning",
			Detail:   "first",
			Metadata: map[string]string{"sequence": "1"},
		},
		{
			Phase:    "responding",
			Detail:   "second",
			Metadata: map[string]string{"sequence": "2"},
		},
	}

	var published []workers.ProgressFragment
	var observedOrder []string
	registry, err := NewAgentRegistry(runners.AgentDependencies{
		Providers: fake,
		Publish: func(fragment workers.ProgressFragment) {
			published = append(published, cloneProgressFragment(fragment))
			observedOrder = append(observedOrder, "progress:"+fragment.Payload)
			if fragment.Metadata != nil {
				fragment.Metadata["sequence"] = "publisher-mutated"
			}
			fragment.ProviderSessionRef.ID = "publisher-mutated"
		},
	})
	if err != nil {
		t.Fatalf("NewAgentRegistry() error = %v", err)
	}
	result, err := registry.Execute(t.Context(), runners.ExecuteRequest{
		Identity: agent.Identity,
		Attempt:  agentRequest(),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	observedOrder = append(observedOrder, "terminal:"+result.Content)

	want := []workers.ProgressFragment{
		{
			DispatchID: "dispatch-agent-1",
			Kind:       workers.ProgressFragmentKind,
			Type:       "planning",
			Payload:    "first",
			ProviderSessionRef: &workers.ProviderSessionMetadata{
				Provider: string(providers.IDCodex),
				Kind:     providers.SessionIDKind,
				ID:       "provider-session-1",
			},
			Metadata: map[string]string{"sequence": "1"},
		},
		{
			DispatchID: "dispatch-agent-1",
			Kind:       workers.ProgressFragmentKind,
			Type:       "responding",
			Payload:    "second",
			ProviderSessionRef: &workers.ProviderSessionMetadata{
				Provider: string(providers.IDCodex),
				Kind:     providers.SessionIDKind,
				ID:       "provider-session-1",
			},
			Metadata: map[string]string{"sequence": "2"},
		},
		{
			DispatchID: "dispatch-agent-1",
			Kind:       workers.ProgressFragmentKind,
			Type:       "message.completed",
			Payload:    "fixture output",
			ProviderSessionRef: &workers.ProviderSessionMetadata{
				Provider: string(providers.IDCodex),
				Kind:     providers.SessionIDKind,
				ID:       "provider-session-1",
			},
		},
		{
			DispatchID: "dispatch-agent-1",
			Kind:       workers.CompletedFragmentKind,
			Type:       "COMPLETED",
			ProviderSessionRef: &workers.ProviderSessionMetadata{
				Provider: string(providers.IDCodex),
				Kind:     providers.SessionIDKind,
				ID:       "provider-session-1",
			},
			ExternalEventType: "STREAM_COMPLETED",
		},
	}
	if !reflect.DeepEqual(published, want) {
		t.Fatalf("published progress = %#v, want %#v", published, want)
	}
	wantOrder := []string{
		"progress:first",
		"progress:second",
		"progress:fixture output",
		"progress:",
		"terminal:fixture output",
	}
	if !reflect.DeepEqual(observedOrder, wantOrder) {
		t.Fatalf("observation order = %v, want %v", observedOrder, wantOrder)
	}
	assertAgentResult(t, result)

	fake.result.Diagnostics.Progress[0].Metadata["sequence"] = "provider-mutated"
	fake.result.SessionRef.ID = "provider-mutated"
	if !reflect.DeepEqual(published, want) {
		t.Fatal("published progress retained Providers-owned mutable values")
	}
	if !reflect.DeepEqual(result, expectedAgentResult()) {
		t.Fatal("terminal result retained progress publisher or Providers-owned mutable values")
	}
}

func cloneProgressFragment(fragment workers.ProgressFragment) workers.ProgressFragment {
	fragment.ProviderSessionRef = workers.CloneProviderSessionMetadata(
		fragment.ProviderSessionRef,
	)
	fragment.Metadata = cloneAgentProgressMetadata(fragment.Metadata)
	return fragment
}

func cloneAgentProgressMetadata(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
