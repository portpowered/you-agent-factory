package internal

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	runnerswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/wire"
)

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

// TestAgentRunnerThrottledFailureThroughServiceComposition proves provider
// saturation maps to one normalized throttle failure with progress before the
// terminal handoff.
func TestAgentRunnerThrottledFailureThroughServiceComposition(t *testing.T) {
	fake := &failingServiceAgentProvidersFake{
		serviceAgentProvidersFake: newServiceAgentProvidersFake(),
		failure: providerServiceFailureFixture(
			providers.ExecuteFailureKindThrottled,
		),
	}

	var published []workers.ProgressFragment
	runner := resolveServiceAgentRunner(t, fake, func(fragment workers.ProgressFragment) {
		published = append(published, cloneServiceProgressFragment(fragment))
	})

	result, err := runner.Execute(t.Context(), serviceAgentRequest())
	if err == nil {
		t.Fatal("Execute() error = nil, want normalized throttle failure")
	}
	if fake.calls.Load() != 1 {
		t.Fatalf("Providers.Execute calls = %d, want exactly one", fake.calls.Load())
	}
	var providerErr *workers.ProviderError
	if !errors.As(err, &providerErr) ||
		providerErr.Type != workers.WorkFailureTypeThrottled ||
		providerErr.Family != workers.WorkFailureFamilyThrottle {
		t.Fatalf("Execute() error = %#v, want throttle ProviderError", err)
	}
	assertServiceAgentFailureFacts(t, result, providerErr, published)
}

// TestAgentRunnerCancellationThroughServiceComposition proves caller
// cancellation wins concurrent provider failure classification without retrying.
func TestAgentRunnerCancellationThroughServiceComposition(t *testing.T) {
	fake := &interruptingServiceAgentProvidersFake{
		serviceAgentProvidersFake: newServiceAgentProvidersFake(),
		entered:                   make(chan struct{}),
	}
	runner := resolveServiceAgentRunner(t, fake, func(workers.ProgressFragment) {})

	ctx, cancel := context.WithCancel(context.Background())
	outcome := make(chan serviceAgentExecutionOutcome, 1)
	go func() {
		result, err := runner.Execute(ctx, serviceAgentRequest())
		outcome <- serviceAgentExecutionOutcome{result: result, err: err}
	}()
	<-fake.entered
	cancel()

	var got serviceAgentExecutionOutcome
	select {
	case got = <-outcome:
	case <-time.After(time.Second):
		t.Fatal("Execute() did not return after cancellation")
	}
	if !errors.Is(got.err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled", got.err)
	}
	if fake.calls.Load() != 1 {
		t.Fatalf("Providers.Execute calls = %d, want exactly one", fake.calls.Load())
	}
	if got.result.Content != "" {
		t.Fatalf("cancellation result = %#v, want no success content", got.result)
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
			Metadata:       map[string]string{"fixture": "detached"},
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
) workers.Runner {
	t.Helper()
	registry, err := runnerswire.NewAgentRegistry(runners.AgentDependencies{
		Providers: providersService,
		Publish:   publish,
	})
	if err != nil {
		t.Fatalf("NewAgentRegistry() error = %v", err)
	}
	binding, err := registry.Resolve(runners.ResolutionRequest{Identity: runners.AgentIdentity})
	if err != nil {
		t.Fatalf("Resolve(agent) error = %v", err)
	}
	return binding.Runner
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
	wantSession := &workers.ProviderSessionMetadata{
		Provider: string(providers.IDCodex),
		Kind:     providers.SessionIDKind,
		ID:       "resume-session-1",
	}
	if !reflect.DeepEqual(result.ProviderSession, wantSession) ||
		!reflect.DeepEqual(providerErr.ProviderSession, wantSession) {
		t.Fatalf("failure sessions = result:%#v error:%#v, want %#v", result.ProviderSession, providerErr.ProviderSession, wantSession)
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
	fragment.ProviderSessionRef = workers.CloneProviderSessionMetadata(
		fragment.ProviderSessionRef,
	)
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
