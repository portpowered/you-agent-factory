package wire

import (
	"reflect"
	"testing"

	"context"
	"errors"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/services/agent"
	"sync"
	"sync/atomic"
)

// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-function-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
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
	registry, err := newTestAgentRegistry(runners.AgentDependencies{
		Providers: fake,
		Publish: func(fragment workers.ProgressFragment) {
			published = append(published, cloneProgressFragment(fragment))
			observedOrder = append(observedOrder, "progress:"+fragment.Payload)
			if fragment.Metadata != nil {
				fragment.Metadata["sequence"] = "publisher-mutated"
			}
			fragment.Continuation.ProviderSessionID = "publisher-mutated"
		},
	})
	if err != nil {
		t.Fatalf("newTestAgentRegistry() error = %v", err)
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
			Provider:   string(providers.IDCodex),
			Continuation: (&providers.SessionMetadata{
				Provider: string(providers.IDCodex), Kind: providers.SessionIDKind, ID: "provider-session-1",
			}).ContinuationRef(),
			Metadata: map[string]string{"sequence": "1"},
		},
		{
			DispatchID: "dispatch-agent-1",
			Kind:       workers.ProgressFragmentKind,
			Type:       "responding",
			Payload:    "second",
			Provider:   string(providers.IDCodex),
			Continuation: (&providers.SessionMetadata{
				Provider: string(providers.IDCodex), Kind: providers.SessionIDKind, ID: "provider-session-1",
			}).ContinuationRef(),
			Metadata: map[string]string{"sequence": "2"},
		},
		{
			DispatchID: "dispatch-agent-1",
			Kind:       workers.ProgressFragmentKind,
			Type:       "message.completed",
			Payload:    "fixture output",
			Provider:   string(providers.IDCodex),
			Continuation: (&providers.SessionMetadata{
				Provider: string(providers.IDCodex), Kind: providers.SessionIDKind, ID: "provider-session-1",
			}).ContinuationRef(),
		},
		{
			DispatchID: "dispatch-agent-1",
			Kind:       workers.CompletedFragmentKind,
			Type:       "COMPLETED",
			Provider:   string(providers.IDCodex),
			Continuation: (&providers.SessionMetadata{
				Provider: string(providers.IDCodex), Kind: providers.SessionIDKind, ID: "provider-session-1",
			}).ContinuationRef(),
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
	fragment.Continuation = (fragment.Continuation).ClonePtr()
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

// TestAgentRunnerSuccessThroughServiceComposition proves ordered progress and
// one detached success through the registered Agent Runner and injected
// Providers root composed from the Workers service boundary.
func TestAgentRunnerSuccessThroughServiceComposition(t *testing.T) {
	fake := newServiceAgentProvidersFake()
	fake.result.Diagnostics.Progress = []providers.ExecuteProgress{
		{Phase: "planning", Detail: "first", Metadata: map[string]string{"sequence": "1"}},
		{Phase: "responding", Detail: "second", Metadata: map[string]string{"sequence": "2"}},
	}

	var published []workers.ProgressFragment
	var observedOrder []string
	runner := resolveServiceAgentRunner(t, fake, func(fragment workers.ProgressFragment) {
		published = append(published, cloneServiceProgressFragment(fragment))
		observedOrder = append(observedOrder, "progress:"+fragment.Payload)
	})

	result, err := runner.Execute(t.Context(), serviceAgentRequest())
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	observedOrder = append(observedOrder, "terminal:"+result.Content)

	if fake.calls.Load() != 1 {
		t.Fatalf("Providers.Execute calls = %d, want 1", fake.calls.Load())
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
	if len(published) != 4 || result.Content != "fixture output" {
		t.Fatalf("terminal outcome = content:%q progress:%d, want one success after two facts and the default terminal stream", result.Content, len(published))
	}
}

type serviceAgentProvidersFake struct {
	providers.Service
	mu      sync.Mutex
	request providers.ExecuteRequest
	result  providers.ExecuteResult
	calls   atomic.Int32
}

type failingServiceAgentProvidersFake struct {
	*serviceAgentProvidersFake
	failure providers.ExecuteFailure
}

type interruptingServiceAgentProvidersFake struct {
	*serviceAgentProvidersFake
	entered chan struct{}
	once    sync.Once
}

type serviceAgentExecutionOutcome struct {
	result workers.RunnerExecutionResult
	err    error
}

var _ providers.Service = (*serviceAgentProvidersFake)(nil)

func newServiceAgentProvidersFake() *serviceAgentProvidersFake {
	return &serviceAgentProvidersFake{result: providers.ExecuteResult{
		Content: "fixture output",
		SessionRef: &providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "provider-session-1",
		},
		Diagnostics: &providers.ExecuteDiagnostics{
			DurationMillis: 42,
			Metadata: map[string]string{
				"fixture": "detached",
				workers.ProviderResponseMetadataCompletionEvidence: "provider_response",
			},
		},
	}}
}

func (fake *serviceAgentProvidersFake) Execute(
	_ context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	fake.calls.Add(1)
	fake.mu.Lock()
	fake.request = request.Clone()
	fake.mu.Unlock()
	return fake.result, nil
}

func (fake *serviceAgentProvidersFake) Continue(
	_ context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ContinueResult{}, err
	}
	fake.calls.Add(1)
	fake.mu.Lock()
	fake.request = request.Attempt.Clone()
	fake.mu.Unlock()
	return providers.ContinueResult{
		Reference: request.Reference,
		Outcome:   providers.ContinuationOutcomeResumed,
		Result:    fake.result,
	}, nil
}

func (fake *failingServiceAgentProvidersFake) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	fake.calls.Add(1)
	fake.mu.Lock()
	fake.request = request.Clone()
	fake.mu.Unlock()
	return providers.ExecuteResult{}, fake.failure
}

func (fake *failingServiceAgentProvidersFake) Continue(
	_ context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ContinueResult{}, err
	}
	fake.calls.Add(1)
	fake.mu.Lock()
	fake.request = request.Attempt.Clone()
	fake.mu.Unlock()
	return providers.ContinueResult{}, fake.failure
}

func (fake *interruptingServiceAgentProvidersFake) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	fake.calls.Add(1)
	fake.mu.Lock()
	fake.request = request.Clone()
	fake.mu.Unlock()
	fake.once.Do(func() { close(fake.entered) })
	<-ctx.Done()
	kind := providers.ExecuteFailureKindCanceled
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		kind = providers.ExecuteFailureKindTimeout
	}
	return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: kind, Message: "interrupted"}
}

func (fake *interruptingServiceAgentProvidersFake) Continue(
	ctx context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ContinueResult{}, err
	}
	fake.calls.Add(1)
	fake.mu.Lock()
	fake.request = request.Attempt.Clone()
	fake.mu.Unlock()
	fake.once.Do(func() { close(fake.entered) })
	<-ctx.Done()
	kind := providers.ExecuteFailureKindCanceled
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		kind = providers.ExecuteFailureKindTimeout
	}
	return providers.ContinueResult{}, providers.ExecuteFailure{Kind: kind, Message: "interrupted"}
}

func (*serviceAgentProvidersFake) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (*serviceAgentProvidersFake) GetProvider(
	context.Context,
	providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	return providers.GetProviderResult{}, nil
}

func resolveServiceAgentRunner(
	t *testing.T,
	providersService providers.Service,
	publish workers.ProgressPublisher,
) runners.Strategy {
	t.Helper()
	runner, err := agentImplementation(runners.AgentDependencies{Providers: providersService, Publish: publish})
	if err != nil {
		t.Fatalf("construct Agent Runner: %v", err)
	}
	return runner
}

func serviceAgentRequest() workers.RunnerExecutionRequest {
	return workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID: "dispatch-agent-1",
			InputTokens: []any{map[string]any{
				"nested": []any{"dispatch-original"},
			}},
		},
		RunnerID:     string(providers.IDCodex),
		SystemPrompt: "system fixture",
		UserMessage:  "user fixture",
		OutputSchema: `{"type":"object"}`,
		SessionID:    "resume-session-1",
		InputTokens: []any{map[string]any{
			"nested": []any{"original"},
		}},
		RequiredOptionalCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilitySessionResume,
		},
		WorkingDirectory: "C:/fixture/work",
		Worktree:         "C:/fixture/worktree",
	}
}

func providerServiceFailureFixture(
	kind providers.ExecuteFailureKind,
) providers.ExecuteFailure {
	return providers.ExecuteFailure{
		Kind:    kind,
		Message: "safe provider failure",
		Diagnostics: &providers.ExecuteDiagnostics{
			DurationMillis: 17,
			Progress: []providers.ExecuteProgress{{
				Phase:    "finishing",
				Detail:   "provider stopped",
				Metadata: map[string]string{"sequence": "1"},
			}},
			Metadata: map[string]string{"safe": "kept"},
		},
	}
}

func assertServiceAgentFailureFacts(
	t *testing.T,
	result workers.RunnerExecutionResult,
	providerErr *workers.ProviderError,
	published []workers.ProgressFragment,
) {
	t.Helper()
	wantSession := &providers.SessionMetadata{
		Provider: string(providers.IDCodex),
		Kind:     providers.SessionIDKind,
		ID:       "resume-session-1",
	}
	wantContinuation := (wantSession).ContinuationRef()
	if !reflect.DeepEqual(result.Continuation, wantContinuation) ||
		!reflect.DeepEqual(providerErr.Continuation, wantContinuation) {
		t.Fatalf("failure continuations = result:%#v error:%#v, want %#v", result.Continuation, providerErr.Continuation, wantContinuation)
	}
	if len(published) != 2 ||
		published[0].DispatchID != "dispatch-agent-1" ||
		published[0].Payload != "provider stopped" ||
		published[1].Kind != workers.FailedFragmentKind ||
		published[1].Payload != "safe provider failure" {
		t.Fatalf("failure progress = %#v, want one correlated fact followed by the default terminal failure stream", published)
	}
}

func cloneServiceProgressFragment(fragment workers.ProgressFragment) workers.ProgressFragment {
	fragment.Continuation = (fragment.Continuation).ClonePtr()
	if fragment.Metadata == nil {
		return fragment
	}
	cloned := make(map[string]string, len(fragment.Metadata))
	for key, value := range fragment.Metadata {
		cloned[key] = value
	}
	fragment.Metadata = cloned
	return fragment
}
