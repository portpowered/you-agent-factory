package run

import (
	"context"
	"errors"
	"testing"
	"time"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimecli "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/cli"
)

func TestValidateInvocationOutputModeDelegatesToInjectedRuntimeCLIAdapter(t *testing.T) {
	t.Parallel()

	called := false
	adapter := stubRuntimeCLIAdapter{
		validateInvocationOutputMode: func(req factoryruntimecli.ValidateInvocationOutputModeRequest) error {
			called = true
			if !req.Continuously || !req.InvocationMode ||
				req.InvocationOutputMode != InvocationOutputResponseStream {
				t.Fatalf("request = %#v", req)
			}
			return &factoryruntimecli.InvocationError{
				Code:    InvocationOutputUnsupportedCode,
				Message: "injected adapter rejection",
			}
		},
	}

	text := "Plan the sprint"
	err := validateInvocationOutputMode(RunConfig{
		RuntimeCLI:               adapter,
		InvocationOutputMode:     InvocationOutputResponseStream,
		Continuously:             true,
		InvocationPositionalText: &text,
	}, true)
	if !called {
		t.Fatal("expected injected Runtime CLI adapter to validate invocation output mode")
	}
	var invocationErr *InvocationError
	if !errors.As(err, &invocationErr) {
		t.Fatalf("error = %#v, want InvocationError", err)
	}
	if invocationErr.Code != InvocationOutputUnsupportedCode ||
		invocationErr.Message != "injected adapter rejection" {
		t.Fatalf("error = %#v, want injected adapter rejection", invocationErr)
	}
}

func TestRuntimeCLIServiceFallsBackToBoundAdapterWhenUnset(t *testing.T) {
	t.Parallel()

	service := runtimeCLIService(RunConfig{})
	if service == nil {
		t.Fatal("runtimeCLIService() = nil, want bound Runtime CLI adapter")
	}
	got, err := service.NormalizeInvocationOutputMode("primary")
	if err != nil {
		t.Fatalf("NormalizeInvocationOutputMode() error = %v", err)
	}
	if got != InvocationOutputPrimaryResult {
		t.Fatalf("mode = %q, want %q", got, InvocationOutputPrimaryResult)
	}
}

type stubRuntimeCLIAdapter struct {
	validateInvocationOutputMode func(factoryruntimecli.ValidateInvocationOutputModeRequest) error
}

func (adapter stubRuntimeCLIAdapter) NormalizeInvocationOutputMode(raw string) (string, error) {
	return factoryruntimecli.NormalizeInvocationOutputMode(raw)
}

func (adapter stubRuntimeCLIAdapter) ValidateInvocationOutputSelection(quiet, jsonOutput, explicitOutput bool) error {
	return factoryruntimecli.ValidateInvocationOutputSelection(quiet, jsonOutput, explicitOutput)
}

func (adapter stubRuntimeCLIAdapter) ValidateInvocationOutputMode(
	req factoryruntimecli.ValidateInvocationOutputModeRequest,
) error {
	if adapter.validateInvocationOutputMode != nil {
		return adapter.validateInvocationOutputMode(req)
	}
	return factoryruntimecli.ValidateInvocationOutputMode(req)
}

func (adapter stubRuntimeCLIAdapter) MapCurrentFactoryFailure(err error) error {
	return factoryruntimecli.MapCurrentFactoryFailure(err)
}

func (adapter stubRuntimeCLIAdapter) MapServerFailure(err error) error {
	return factoryruntimecli.MapServerFailure(err)
}

func (adapter stubRuntimeCLIAdapter) MapInvocationFailure(err error) error {
	return factoryruntimecli.MapInvocationFailure(err)
}

func (adapter stubRuntimeCLIAdapter) MapRuntimeRootFailure(err error) error {
	return factoryruntimecli.MapRuntimeRootFailure(nil, err)
}

func (adapter stubRuntimeCLIAdapter) ObserveRuntime(
	ctx context.Context,
	req factoryruntime.ObserveRequest,
) error {
	return factoryruntimecli.ObserveRuntime(ctx, nil, req)
}

func (adapter stubRuntimeCLIAdapter) CountTokenStates(
	snap *factoryruntime.PetriMarkingSnapshot,
) (wip, completed, failed int) {
	return factoryruntimecli.CountTokenStates(snap)
}

func (adapter stubRuntimeCLIAdapter) FormatDuration(d time.Duration) string {
	return factoryruntimecli.FormatDuration(d)
}
