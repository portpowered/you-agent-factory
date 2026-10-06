package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
)

type validationRunner struct {
	runners.Service
	resolved int
	err      error
}

func (r *validationRunner) Resolve(runners.ResolutionRequest) (runners.Binding, error) {
	r.resolved++
	return runners.Binding{}, r.err
}

func TestT7WorkersPreflightGuardsBeforeRunnerResolution(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"nil-service", "missing-runners", "nil-context", "canceled", "invalid-input", "noop", "runner-unavailable"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			runner := &validationRunner{}
			svc := &Service{runners: runner}
			ctx := context.Background()
			request := workers.ExecuteRequest{
				Correlation: workers.ExecutionCorrelation{FactorySessionID: "session", RuntimeID: "runtime", GenerationID: "generation", DispatchID: "dispatch", AttemptID: "attempt", RequestID: "request"},
				Target:      workers.ExecutionTarget{WorkerName: "worker", WorkstationName: "station", RunnerID: "script"},
			}
			var want error
			switch mode {
			case "nil-service":
				svc, want = nil, workers.ErrExecuteUnavailable
			case "missing-runners":
				svc.runners, want = nil, workers.ErrExecuteUnavailable
			case "nil-context":
				ctx, want = nil, workers.ErrInvalidExecuteRequest
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
				want = context.Canceled
			case "invalid-input":
				request.Correlation.AttemptID, want = "", workers.ErrInvalidExecuteRequest
			case "noop":
				request.Target.Noop = true
			case "runner-unavailable":
				runner.err = errors.New("controlled runner unavailable")
				want = workers.ErrInvalidExecuteRequest
			}
			err := svc.ValidateExecution(ctx, request)
			if !errors.Is(err, want) || (mode != "runner-unavailable" && runner.resolved != 0) {
				t.Fatalf("preflight = %v, runner resolutions %d", err, runner.resolved)
			}
		})
	}
}

type validationProvider struct {
	providerAuthorizationFake
	request providers.ExecuteRequest
	err     error
}

func (p *validationProvider) ValidateExecution(_ context.Context, req providers.ExecuteRequest) error {
	p.request = req
	return p.err
}

func TestT7WorkersPreflightPreservesInputsAndProviderPolicy(t *testing.T) {
	t.Parallel()
	for _, unavailable := range []bool{false, true} {
		t.Run(map[bool]string{false: "supported", true: "unavailable"}[unavailable], func(t *testing.T) {
			t.Parallel()
			provider := &validationProvider{providerAuthorizationFake: providerAuthorizationFake{
				resolveResult: providers.ResolveIdentityResult{ID: providers.IDCodex},
			}}
			if unavailable {
				provider.err = providers.ErrProviderUnavailable
			}
			runner := &validationRunner{}
			svc := &Service{providers: provider, runners: runner}
			request := workers.ExecuteRequest{
				Correlation: workers.ExecutionCorrelation{FactorySessionID: "session", RuntimeID: "runtime", GenerationID: "generation", DispatchID: "dispatch", AttemptID: "attempt", RequestID: "request"},
				Target:      workers.ExecutionTarget{WorkerName: "worker", WorkstationName: "station", RunnerID: "agent", Provider: workers.ProviderReference{Alias: "openai"}, Model: workers.ModelReference{Name: "opaque-model", ReasoningEffort: "high"}},
			}
			original := request
			err := svc.ValidateExecution(context.Background(), request)
			if !errors.Is(err, provider.err) {
				t.Fatalf("ValidateExecution = %v, want %v", err, provider.err)
			}
			if !reflect.DeepEqual(request, original) {
				t.Fatal("preflight mutated caller request")
			}
			if provider.request.Provider != providers.IDCodex || provider.request.Model != "opaque-model" || provider.request.ReasoningEffort != "high" || provider.request.AttemptID != "attempt" {
				t.Fatalf("provider policy request = %#v", provider.request)
			}
			if unavailable && runner.resolved != 0 {
				t.Fatal("resolved runner after failed provider policy")
			}
		})
	}
}
