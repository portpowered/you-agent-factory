package kiro_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	kiro "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/kiro"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
	executiontest "github.com/portpowered/infinite-you/pkg/services/providers/internal/testutil/execution"
)

const (
	kiroFailureSecret  = "prompt-secret-that-must-not-escape"
	kiroResumedSession = "675f9238-5f05-456c-9a9f-f8fe486f49e4"
	kiroEmittedSession = "f2946a26-3735-4b08-8d05-c928010302d5"
)

func TestKiroAdapterConformance(t *testing.T) {
	executiontest.Run(t, executiontest.Subject{
		Provider:   providers.IDKiro,
		NewAdapter: newKiroConformanceAdapter,
		NewRoot:    newKiroConformanceRoot,
	})
}

func newKiroConformanceRoot(
	attempt execution.Attempt,
) (providers.Service, error) {
	catalog, err := catalogwire.NewService()
	if err != nil {
		return nil, err
	}
	executionService, err := executionwire.NewService(
		catalog,
		execution.Registration{
			Provider: providers.IDKiro,
			Attempt:  attempt,
		},
	)
	if err != nil {
		return nil, err
	}
	return providerservice.New(catalog, executionService)
}

type kiroConformanceState struct {
	mu          sync.Mutex
	plan        executiontest.Plan
	observation executiontest.Observation
	started     chan struct{}
	startOnce   sync.Once
}

func newKiroConformanceAdapter(plan executiontest.Plan) executiontest.Adapter {
	state := &kiroConformanceState{
		plan:    plan,
		started: make(chan struct{}),
	}
	effect := kiro.EffectFunc(state.execute)
	return executiontest.Adapter{
		Attempt: kiro.NewRegistration(effect).Attempt,
		Observe: state.observe,
		Started: state.started,
	}
}

func (state *kiroConformanceState) execute(
	ctx context.Context,
	request providers.ExecuteRequest,
	observe func([]byte) error,
) (kiro.EffectResult, error) {
	state.mu.Lock()
	state.observation.Calls++
	state.observation.Requests = append(
		state.observation.Requests,
		request.Clone(),
	)
	state.mu.Unlock()
	state.startOnce.Do(func() { close(state.started) })
	defer func() {
		state.mu.Lock()
		state.observation.Cleanups++
		state.mu.Unlock()
	}()
	if state.plan.MutateRequest {
		request.ResumeSession.ID = "kiro-effect-mutated"
	}
	if state.plan.WaitForContext {
		<-ctx.Done()
		return kiro.EffectResult{}, ctx.Err()
	}
	if state.plan.ReturnSuccessAfterContext {
		<-ctx.Done()
	}
	if state.plan.Failure != nil {
		return kiro.EffectResult{}, state.plan.Failure
	}
	if state.plan.Result.Content != "" {
		if err := observe([]byte(state.plan.Result.Content)); err != nil {
			return kiro.EffectResult{}, err
		}
	}
	effectResult := kiro.EffectResult{}
	if state.plan.Result.Diagnostics != nil {
		effectResult.DurationMillis = state.plan.Result.Diagnostics.DurationMillis
		effectResult.Metadata = state.plan.Result.Diagnostics.Metadata
	}
	if state.plan.Result.SessionRef != nil {
		session := state.plan.Result.SessionRef.Clone()
		effectResult.SessionRef = &session
	}
	return effectResult, nil
}

func (state *kiroConformanceState) observe() executiontest.Observation {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.observation.Clone()
}

func TestKiroRootPreservesRequestFinalStdoutAndSession(t *testing.T) {
	t.Parallel()

	request := providers.ExecuteRequest{
		Provider:     providers.IDKiro,
		AttemptID:    "attempt-kiro-success",
		SystemPrompt: "Follow the factory instructions.",
		UserMessage:  "perform the accepted work",
		ResumeSession: &providers.SessionRef{
			Provider: providers.IDKiro,
			Kind:     providers.SessionIDKind,
			ID:       kiroResumedSession,
		},
	}
	const content = "kiro final answer"
	var received providers.ExecuteRequest
	effect := kiro.EffectFunc(func(
		_ context.Context,
		got providers.ExecuteRequest,
		observe func([]byte) error,
	) (kiro.EffectResult, error) {
		received = got.Clone()
		if err := observe([]byte(content)); err != nil {
			return kiro.EffectResult{}, err
		}
		return kiro.EffectResult{
			DurationMillis: 17,
			SessionRef: &providers.SessionRef{
				Provider: providers.IDKiro,
				Kind:     providers.SessionIDKind,
				ID:       kiroEmittedSession,
			},
		}, nil
	})
	root := newKiroRoot(t, effect)

	result, err := root.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !reflect.DeepEqual(received, request) {
		t.Fatalf("native request = %#v, want %#v", received, request)
	}
	if result.Content != content {
		t.Fatalf("Content = %q, want %q", result.Content, content)
	}
	if result.SessionRef == nil || result.SessionRef.ID != kiroEmittedSession {
		t.Fatalf("SessionRef = %#v, want emitted session %s", result.SessionRef, kiroEmittedSession)
	}
	if result.Diagnostics == nil || result.Diagnostics.DurationMillis != 17 {
		t.Fatalf("Diagnostics = %#v", result.Diagnostics)
	}
}

func TestKiroRootNormalizesFailureStagesAndSuppressesResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		effectErr error
		wantKind  providers.ExecuteFailureKind
	}{
		{
			name:      "native exit",
			effectErr: errors.New("exit included " + kiroFailureSecret),
			wantKind:  providers.ExecuteFailureKindUnknown,
		},
		{
			name: "declared throttling beats native exit",
			effectErr: providers.ExecuteFailure{
				Kind: providers.ExecuteFailureKindThrottled,
			},
			wantKind: providers.ExecuteFailureKindThrottled,
		},
		{
			name:      "recognized native failure",
			effectErr: providers.ExecuteFailure{Kind: providers.ExecuteFailureKindAuthentication},
			wantKind:  providers.ExecuteFailureKindAuthentication,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var cleanups atomic.Int32
			effect := kiro.EffectFunc(func(
				_ context.Context,
				_ providers.ExecuteRequest,
				_ func([]byte) error,
			) (kiro.EffectResult, error) {
				defer cleanups.Add(1)
				return kiro.EffectResult{}, test.effectErr
			})

			for iteration := 0; iteration < 10; iteration++ {
				result, err := newKiroRoot(t, effect).Execute(
					t.Context(),
					kiroFailureRequest(),
				)
				assertKiroFailure(t, result, err, test.wantKind)
			}
			if got := cleanups.Load(); got != 10 {
				t.Fatalf("cleanup calls = %d, want 10", got)
			}
		})
	}
}

func TestKiroRootCancellationAndDeadlineReachEffectAndCleanUpOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		newContext func() (context.Context, context.CancelFunc)
		want       error
	}{
		{
			name: "cancellation",
			newContext: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(t.Context())
			},
			want: providers.ErrExecuteCancelled,
		},
		{
			name: "deadline",
			newContext: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(t.Context(), 50*time.Millisecond)
			},
			want: providers.ErrExecuteTimeout,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			started := make(chan struct{})
			var cleanups atomic.Int32
			effect := kiro.EffectFunc(func(
				ctx context.Context,
				_ providers.ExecuteRequest,
				_ func([]byte) error,
			) (kiro.EffectResult, error) {
				close(started)
				defer cleanups.Add(1)
				<-ctx.Done()
				return kiro.EffectResult{}, ctx.Err()
			})
			ctx, cancel := test.newContext()
			defer cancel()
			root := newKiroRoot(t, effect)
			outcome := make(chan error, 1)
			go func() {
				_, err := root.Execute(ctx, kiroFailureRequest())
				outcome <- err
			}()
			<-started
			if test.want == providers.ErrExecuteCancelled {
				cancel()
			}

			select {
			case err := <-outcome:
				if !errors.Is(err, test.want) {
					t.Fatalf("Execute() error = %v, want %v", err, test.want)
				}
			case <-time.After(time.Second):
				t.Fatal("Execute() did not stop after context ended")
			}
			if got := cleanups.Load(); got != 1 {
				t.Fatalf("cleanup calls = %d, want 1", got)
			}
		})
	}
}

func assertKiroFailure(
	t *testing.T,
	result providers.ExecuteResult,
	err error,
	wantKind providers.ExecuteFailureKind,
) {
	t.Helper()
	if !reflect.DeepEqual(result, providers.ExecuteResult{}) {
		t.Fatalf("failed Execute() result = %#v, want zero result", result)
	}
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) || failure.Kind != wantKind {
		t.Fatalf("Execute() error = %#v, want kind %q", err, wantKind)
	}
	if strings.Contains(err.Error(), kiroFailureSecret) {
		t.Fatalf("Execute() error leaked sensitive facts: %v", err)
	}
}

func kiroFailureRequest() providers.ExecuteRequest {
	return providers.ExecuteRequest{
		Provider:    providers.IDKiro,
		AttemptID:   "attempt-kiro-failure",
		UserMessage: "deterministic failure prompt",
	}
}

func newKiroRoot(t *testing.T, effect kiro.Effect) providers.Service {
	t.Helper()
	root, err := newKiroConformanceRoot(kiro.NewRegistration(effect).Attempt)
	if err != nil {
		t.Fatal(err)
	}
	return root
}
