package cli_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimecli "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/cli"
)

type stubRuntimeRoot struct {
	observeErr error
}

func (stub stubRuntimeRoot) ControlPause(context.Context, factoryruntime.PauseRequest) (factoryruntime.PauseResult, error) {
	return factoryruntime.PauseResult{}, factoryruntime.ErrNotRunning
}

func (stub stubRuntimeRoot) ControlResume(context.Context, factoryruntime.ResumeRequest) (factoryruntime.ResumeResult, error) {
	return factoryruntime.ResumeResult{}, factoryruntime.ErrNotRunning
}

func (stub stubRuntimeRoot) ControlTerminate(context.Context, factoryruntime.TerminateRequest) (factoryruntime.TerminateResult, error) {
	return factoryruntime.TerminateResult{}, factoryruntime.ErrNotRunning
}

func (stub stubRuntimeRoot) ControlWaitToComplete(factoryruntime.WaitToCompleteRequest) factoryruntime.WaitToCompleteResult {
	return factoryruntime.WaitToCompleteResult{}
}

func (stub stubRuntimeRoot) ControlMoveWork(context.Context, factoryruntime.MoveWorkRequest) (factoryruntime.MoveWorkResult, error) {
	return factoryruntime.MoveWorkResult{}, factoryruntime.ErrNotRunning
}

func (stub stubRuntimeRoot) Observe(context.Context, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	return factoryruntime.ObserveResult{}, stub.observeErr
}

func (stub stubRuntimeRoot) PlanDispatch(context.Context, factoryruntime.PlanDispatchRequest) (factoryruntime.PlanDispatchResult, error) {
	return factoryruntime.PlanDispatchResult{}, factoryruntime.ErrNotRunning
}

func (stub stubRuntimeRoot) AcceptDispatchResult(context.Context, factoryruntime.AcceptDispatchResultRequest) (factoryruntime.AcceptDispatchResultResult, error) {
	return factoryruntime.AcceptDispatchResultResult{}, factoryruntime.ErrNotRunning
}

func (stub stubRuntimeRoot) CaptureCheckpoint(context.Context, factoryruntime.CaptureCheckpointRequest) (factoryruntime.CaptureCheckpointResult, error) {
	return factoryruntime.CaptureCheckpointResult{}, factoryruntime.ErrNotRunning
}

func (stub stubRuntimeRoot) LoadCheckpoint(context.Context, factoryruntime.LoadCheckpointRequest) (factoryruntime.LoadCheckpointResult, error) {
	return factoryruntime.LoadCheckpointResult{}, factoryruntime.ErrNotRunning
}

func (stub stubRuntimeRoot) RestoreCheckpoint(context.Context, factoryruntime.RestoreCheckpointRequest) (factoryruntime.RestoreCheckpointResult, error) {
	return factoryruntime.RestoreCheckpointResult{}, factoryruntime.ErrNotRunning
}

func constructedRuntimeCLIService(
	t *testing.T,
	runtime factoryruntime.Service,
) factoryruntimecli.Service {
	t.Helper()
	return factoryruntimecli.New(factoryruntimecli.Config{Runtime: runtime})
}

func TestConstructedService_RequiresRuntimeRootForObservation(t *testing.T) {
	t.Parallel()

	service := constructedRuntimeCLIService(t, nil)
	err := service.ObserveRuntime(context.Background(), factoryruntime.ObserveRequest{})
	var invocationErr *factoryruntimecli.InvocationError
	if !errors.As(err, &invocationErr) {
		t.Fatalf("error = %#v, want InvocationError", err)
	}
	if invocationErr.Code != factoryruntimecli.InvocationErrorCodeFailed ||
		invocationErr.Message != "factory runtime root is required" {
		t.Fatalf("error = %#v, want missing runtime root failure", err)
	}
}

func TestConstructedService_ObserveRuntimeMapsRuntimeRootRejection(t *testing.T) {
	t.Parallel()

	service := constructedRuntimeCLIService(t, stubRuntimeRoot{observeErr: factoryruntime.ErrNotRunning})
	err := service.ObserveRuntime(context.Background(), factoryruntime.ObserveRequest{})
	var invocationErr *factoryruntimecli.InvocationError
	if !errors.As(err, &invocationErr) {
		t.Fatalf("error = %#v, want InvocationError", err)
	}
	if invocationErr.Message != "factory runtime is not running" {
		t.Fatalf("message = %q, want factory runtime is not running", invocationErr.Message)
	}
	if !errors.Is(invocationErr, factoryruntime.ErrNotRunning) {
		t.Fatalf("cause = %v, want ErrNotRunning", invocationErr.Unwrap())
	}
}

func TestConstructedService_NormalizeInvocationOutputModeMatchesPackageFunction(t *testing.T) {
	t.Parallel()

	service := constructedRuntimeCLIService(t, nil)
	assertInvocationOutputModeParity(t, service, "")
}

func TestConstructedService_ValidateInvocationOutputSelectionMatchesPackageFunction(t *testing.T) {
	t.Parallel()

	service := constructedRuntimeCLIService(t, nil)
	err := service.ValidateInvocationOutputSelection(true, true, false)
	assertInvocationOutputSelectionParity(t, service, true, true, false, err)
}

func TestConstructedService_ValidateInvocationOutputModeMatchesPackageFunction(t *testing.T) {
	t.Parallel()

	service := constructedRuntimeCLIService(t, nil)
	req := factoryruntimecli.ValidateInvocationOutputModeRequest{
		InvocationOutputMode: factoryruntimecli.InvocationOutputResponseStream,
		Continuously:         true,
		InvocationMode:       true,
	}
	err := service.ValidateInvocationOutputMode(req)
	assertInvocationOutputModeValidationParity(t, service, req, err)
}

func TestConstructedService_MapInvocationFailureMatchesPackageFunction(t *testing.T) {
	t.Parallel()

	service := constructedRuntimeCLIService(t, nil)
	err := service.MapInvocationFailure(context.Canceled)
	assertInvocationFailureParity(t, service, context.Canceled, err)
}

func TestConstructedService_CountTokenStatesMatchesPackageFunction(t *testing.T) {
	t.Parallel()

	service := constructedRuntimeCLIService(t, nil)
	snap := &factoryruntime.PetriMarkingSnapshot{
		Tokens: map[string]*factoryruntime.RuntimeToken{
			"t1": {ID: "t1", PlaceID: "task:todo"},
			"t2": {ID: "t2", PlaceID: "task:completed"},
			"t3": {ID: "t3", PlaceID: "task:failed"},
		},
	}
	assertCountTokenStatesParity(t, service, snap)
}

func TestConstructedService_FormatDurationMatchesPackageFunction(t *testing.T) {
	t.Parallel()

	service := constructedRuntimeCLIService(t, nil)
	duration := 90 * time.Minute
	if got := service.FormatDuration(duration); got != factoryruntimecli.FormatDuration(duration) {
		t.Fatalf("FormatDuration() = %q, want %q", got, factoryruntimecli.FormatDuration(duration))
	}
}

func assertInvocationOutputModeParity(
	t *testing.T,
	service factoryruntimecli.Service,
	raw string,
) {
	t.Helper()
	gotService, errService := service.NormalizeInvocationOutputMode(raw)
	gotPackage, errPackage := factoryruntimecli.NormalizeInvocationOutputMode(raw)
	if (errService == nil) != (errPackage == nil) {
		t.Fatalf("service error = %v, package error = %v", errService, errPackage)
	}
	if gotService != gotPackage {
		t.Fatalf("service mode = %q, package mode = %q", gotService, gotPackage)
	}
}

func assertInvocationOutputSelectionParity(
	t *testing.T,
	service factoryruntimecli.Service,
	quiet, jsonOutput, explicitOutput bool,
	err error,
) {
	t.Helper()
	gotPackage := factoryruntimecli.ValidateInvocationOutputSelection(quiet, jsonOutput, explicitOutput)
	if (err == nil) != (gotPackage == nil) {
		t.Fatalf("service error = %v, package error = %v", err, gotPackage)
	}
}

func assertInvocationOutputModeValidationParity(
	t *testing.T,
	service factoryruntimecli.Service,
	req factoryruntimecli.ValidateInvocationOutputModeRequest,
	err error,
) {
	t.Helper()
	gotPackage := factoryruntimecli.ValidateInvocationOutputMode(req)
	if (err == nil) != (gotPackage == nil) {
		t.Fatalf("service error = %v, package error = %v", err, gotPackage)
	}
	if err != nil && gotPackage != nil {
		var serviceErr, packageErr *factoryruntimecli.InvocationError
		if !errors.As(err, &serviceErr) || !errors.As(gotPackage, &packageErr) {
			t.Fatalf("errors = %#v / %#v, want InvocationError", err, gotPackage)
		}
		if serviceErr.Code != packageErr.Code || !strings.Contains(serviceErr.Message, "continuously") {
			t.Fatalf("service error = %#v, package error = %#v", serviceErr, packageErr)
		}
	}
}

func assertInvocationFailureParity(
	t *testing.T,
	service factoryruntimecli.Service,
	cause error,
	err error,
) {
	t.Helper()
	gotPackage := factoryruntimecli.MapInvocationFailure(cause)
	if (err == nil) != (gotPackage == nil) {
		t.Fatalf("service error = %v, package error = %v", err, gotPackage)
	}
	var serviceErr, packageErr *factoryruntimecli.InvocationError
	if !errors.As(err, &serviceErr) || !errors.As(gotPackage, &packageErr) {
		t.Fatalf("errors = %#v / %#v, want InvocationError", err, gotPackage)
	}
	if serviceErr.Code != packageErr.Code {
		t.Fatalf("service code = %q, package code = %q", serviceErr.Code, packageErr.Code)
	}
}

func assertCountTokenStatesParity(
	t *testing.T,
	service factoryruntimecli.Service,
	snap *factoryruntime.PetriMarkingSnapshot,
) {
	t.Helper()
	wipService, doneService, failedService := service.CountTokenStates(snap)
	wipPackage, donePackage, failedPackage := factoryruntimecli.CountTokenStates(snap)
	if wipService != wipPackage || doneService != donePackage || failedService != failedPackage {
		t.Fatalf(
			"service counts = (%d,%d,%d), package counts = (%d,%d,%d)",
			wipService, doneService, failedService,
			wipPackage, donePackage, failedPackage,
		)
	}
}
