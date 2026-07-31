package wire

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/services/agent"
)

func TestAgentRunnerNormalizesProviderFailureKindsWithoutRetry(t *testing.T) {
	tests := []struct {
		kind       providers.ExecuteFailureKind
		wantType   workers.WorkFailureType
		wantFamily workers.WorkFailureFamily
		wantCause  error
	}{
		{providers.ExecuteFailureKindAuthentication, workers.WorkFailureTypeAuthFailure, workers.WorkFailureFamilyTerminal, providers.ErrExecuteFailed},
		{providers.ExecuteFailureKindInvalidRequest, workers.WorkFailureTypePermanentBadRequest, workers.WorkFailureFamilyTerminal, providers.ErrExecuteFailed},
		{providers.ExecuteFailureKindMisconfigured, workers.WorkFailureTypeMisconfigured, workers.WorkFailureFamilyTerminal, providers.ErrExecuteFailed},
		{providers.ExecuteFailureKindThrottled, workers.WorkFailureTypeThrottled, workers.WorkFailureFamilyThrottle, providers.ErrExecuteFailed},
		{providers.ExecuteFailureKindDependency, workers.WorkFailureTypeInternalServerError, workers.WorkFailureFamilyRetryable, providers.ErrExecuteFailed},
		{providers.ExecuteFailureKindUnknown, workers.WorkFailureTypeUnknown, workers.WorkFailureFamilyTerminal, providers.ErrExecuteFailed},
		{providers.ExecuteFailureKindTimeout, workers.WorkFailureTypeTimeout, workers.WorkFailureFamilyRetryable, context.DeadlineExceeded},
	}
	for _, test := range tests {
		t.Run(string(test.kind), func(t *testing.T) {
			failure := providerFailureFixture(test.kind)
			fake := &failingAgentProvidersFake{failure: failure}
			var published []workers.ProgressFragment
			var observedOrder []string
			runner := resolveAgentRunner(t, fake, func(fragment workers.ProgressFragment) {
				published = append(published, cloneProgressFragment(fragment))
				observedOrder = append(observedOrder, "progress")
			})

			result, err := runner.Execute(t.Context(), agentRequest())
			observedOrder = append(observedOrder, "terminal")
			if err == nil {
				t.Fatal("Execute() error = nil, want normalized provider failure")
			}
			if fake.calls.Load() != 1 {
				t.Fatalf("Providers.Execute calls = %d, want exactly 1", fake.calls.Load())
			}
			var providerErr *workers.ProviderError
			if !errors.As(err, &providerErr) {
				t.Fatalf("Execute() error = %T %v, want *workers.ProviderError", err, err)
			}
			if providerErr.Type != test.wantType || providerErr.Family != test.wantFamily {
				t.Fatalf("ProviderError = %#v, want type %q family %q", providerErr, test.wantType, test.wantFamily)
			}
			if !errors.Is(err, test.wantCause) {
				t.Fatalf("Execute() error = %v, want cause %v", err, test.wantCause)
			}
			var retained providers.ExecuteFailure
			if !errors.As(err, &retained) || retained.Kind != test.kind {
				t.Fatalf("Execute() retained failure = %#v, want kind %q", retained, test.kind)
			}
			assertAgentFailureFacts(t, result, providerErr, published)
			if !reflect.DeepEqual(observedOrder, []string{"progress", "progress", "terminal"}) {
				t.Fatalf("failure observation order = %v, want diagnostic, stream failure, then terminal", observedOrder)
			}

			failure.Diagnostics.Metadata["safe"] = "provider-mutated"
			failure.Diagnostics.Progress[0].Metadata["sequence"] = "provider-mutated"
			if result.Diagnostics.Metadata["safe"] != "kept" ||
				providerErr.Diagnostics.Metadata["safe"] != "kept" ||
				published[0].Metadata["sequence"] != "1" {
				t.Fatal("failure output retained Providers-owned mutable values")
			}
		})
	}
}

func TestAgentRunnerPreservesCancellationAndDeadlineContext(t *testing.T) {
	tests := []struct {
		name     string
		deadline bool
		want     error
	}{
		{name: "cancellation", want: context.Canceled},
		{name: "deadline", deadline: true, want: context.DeadlineExceeded},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for iteration := 0; iteration < 20; iteration++ {
				fake := &interruptingAgentProvidersFake{entered: make(chan struct{})}
				var publishCalls atomic.Int32
				runner := resolveAgentRunner(t, fake, func(workers.ProgressFragment) {
					publishCalls.Add(1)
				})
				ctx, cancel := context.WithCancel(context.Background())
				if test.deadline {
					ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
				}
				done := make(chan agentExecutionOutcome, 1)
				go func() {
					result, err := runner.Execute(ctx, agentRequest())
					done <- agentExecutionOutcome{result: result, err: err}
				}()
				<-fake.entered
				if !test.deadline {
					cancel()
				}
				outcome := <-done
				cancel()

				if fake.Context() != ctx {
					t.Fatal("Providers.Execute received a replacement context")
				}
				if !errors.Is(outcome.err, test.want) {
					t.Fatalf("Execute() error = %v, want %v", outcome.err, test.want)
				}
				if fake.calls.Load() != 1 || publishCalls.Load() != 2 {
					t.Fatalf("calls = provider:%d progress:%d, want one provider call and two progress publications", fake.calls.Load(), publishCalls.Load())
				}
				if outcome.result.Diagnostics == nil ||
					outcome.result.Diagnostics.Metadata["safe"] != "kept" {
					t.Fatalf("interrupted result = %#v, want detached diagnostics", outcome.result)
				}
				if test.deadline {
					var providerErr *workers.ProviderError
					if !errors.As(outcome.err, &providerErr) ||
						providerErr.Type != workers.WorkFailureTypeTimeout {
						t.Fatalf("deadline error = %#v, want timeout ProviderError", outcome.err)
					}
				} else {
					var providerErr *workers.ProviderError
					if !errors.As(outcome.err, &providerErr) || providerErr.Message != "provider invocation was canceled" {
						t.Fatalf("cancellation error = %#v, want canonical cancellation ProviderError", outcome.err)
					}
				}
			}
		})
	}
}

func TestAgentRunnerBoundsFailureMessage(t *testing.T) {
	fake := &failingAgentProvidersFake{failure: providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindUnknown,
		Message: strings.Repeat("ø", 600),
	}}
	runner := resolveAgentRunner(t, fake, agentNoopPublisher)
	_, err := runner.Execute(t.Context(), agentRequest())
	var providerErr *workers.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("Execute() error = %v, want ProviderError", err)
	}
	if got := len([]rune(providerErr.Message)); got != 512 {
		t.Fatalf("failure message runes = %d, want 512", got)
	}
}

type failingAgentProvidersFake struct {
	agentProvidersFake
	failure providers.ExecuteFailure
}

func (fake *failingAgentProvidersFake) Execute(
	_ context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	fake.calls.Add(1)
	fake.mu.Lock()
	fake.request = request.Clone()
	fake.mu.Unlock()
	return providers.ExecuteResult{}, fake.failure
}

type interruptingAgentProvidersFake struct {
	agentProvidersFake
	entered chan struct{}
	once    sync.Once
	ctx     context.Context
}

func (fake *interruptingAgentProvidersFake) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	fake.calls.Add(1)
	fake.mu.Lock()
	fake.request = request.Clone()
	fake.ctx = ctx
	fake.mu.Unlock()
	fake.once.Do(func() { close(fake.entered) })
	<-ctx.Done()
	kind := providers.ExecuteFailureKindCanceled
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		kind = providers.ExecuteFailureKindTimeout
	}
	return providers.ExecuteResult{}, providerFailureFixture(kind)
}

func (fake *interruptingAgentProvidersFake) Context() context.Context {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return fake.ctx
}

type agentExecutionOutcome struct {
	result workers.RunnerExecutionResult
	err    error
}

func resolveAgentRunner(
	t *testing.T,
	providersService providers.Service,
	publish workers.ProgressPublisher,
) workers.Runner {
	t.Helper()
	registry, err := NewAgentRegistry(runners.AgentDependencies{
		Providers: providersService,
		Publish:   publish,
	})
	if err != nil {
		t.Fatalf("NewAgentRegistry() error = %v", err)
	}
	return agentRegistryStrategy{registry: registry}
}

type agentRegistryStrategy struct {
	registry runners.Service
}

func (strategy agentRegistryStrategy) Execute(
	ctx context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	return strategy.registry.Execute(ctx, runners.ExecuteRequest{
		Identity: agent.Identity,
		Attempt:  request,
	})
}

func providerFailureFixture(
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

func assertAgentFailureFacts(
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
	if result.Diagnostics == nil ||
		result.Diagnostics.Provider == nil ||
		result.Diagnostics.Provider.Provider != string(providers.IDCodex) ||
		result.Diagnostics.Metadata["safe"] != "kept" ||
		result.Diagnostics.Metadata[workers.ProviderResponseMetadataDurationMS] != "17" {
		t.Fatalf("failure result diagnostics = %#v", result.Diagnostics)
	}
	if !reflect.DeepEqual(providerErr.Diagnostics, result.Diagnostics) {
		t.Fatalf("error diagnostics = %#v, want detached equivalent of %#v", providerErr.Diagnostics, result.Diagnostics)
	}
	if len(published) != 2 ||
		published[0].DispatchID != "dispatch-agent-1" ||
		published[0].Payload != "provider stopped" ||
		!reflect.DeepEqual(published[0].ProviderSessionRef, wantSession) ||
		published[1].Kind != workers.FailedFragmentKind ||
		published[1].Type != "FAILED" ||
		!reflect.DeepEqual(published[1].ProviderSessionRef, wantSession) {
		t.Fatalf("failure progress = %#v, want correlated diagnostic and terminal failure", published)
	}
}
