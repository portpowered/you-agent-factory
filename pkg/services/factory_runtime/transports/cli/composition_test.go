package cli_test

import (
	"context"
	"errors"
	"testing"
	"time"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimecli "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/cli"
)

func TestBindServiceDelegatesThroughAdapterService(t *testing.T) {
	t.Parallel()

	service := factoryruntimecli.BindService(factoryruntimecli.Config{
		Runtime: stubRuntimeRoot{observeErr: factoryruntime.ErrNotRunning},
	})
	if service == nil {
		t.Fatal("BindService(runtime) = nil, want Runtime CLI service")
	}
	err := service.ObserveRuntime(context.Background(), factoryruntime.ObserveRequest{})
	var invocationErr *factoryruntimecli.InvocationError
	if !errors.As(err, &invocationErr) {
		t.Fatalf("error = %#v, want InvocationError", err)
	}
	if invocationErr.Message != "factory runtime is not running" {
		t.Fatalf("message = %q, want factory runtime is not running", invocationErr.Message)
	}
}

func TestBindServiceMatchesFreeFunctionFacade(t *testing.T) {
	t.Parallel()

	service := factoryruntimecli.BindService(factoryruntimecli.Config{})
	gotService, errService := service.NormalizeInvocationOutputMode("primary")
	gotPackage, errPackage := factoryruntimecli.NormalizeInvocationOutputMode("primary")
	if (errService == nil) != (errPackage == nil) {
		t.Fatalf("service error = %v, package error = %v", errService, errPackage)
	}
	if gotService != gotPackage {
		t.Fatalf("service mode = %q, package mode = %q", gotService, gotPackage)
	}
}

func TestBindServicePreservesRuntimeRootCollaborator(t *testing.T) {
	t.Parallel()

	runtime := stubRuntimeRoot{}
	service := factoryruntimecli.BindService(factoryruntimecli.Config{Runtime: runtime})
	if err := service.ObserveRuntime(context.Background(), factoryruntime.ObserveRequest{}); err != nil {
		t.Fatalf("ObserveRuntime() error = %v, want nil for successful stub", err)
	}
}

func TestBindServiceSupportsStatelessPresentationMethods(t *testing.T) {
	t.Parallel()

	service := factoryruntimecli.BindService(factoryruntimecli.Config{})
	duration := 90 * time.Minute
	if got := service.FormatDuration(duration); got != factoryruntimecli.FormatDuration(duration) {
		t.Fatalf("FormatDuration() = %q, want %q", got, factoryruntimecli.FormatDuration(duration))
	}
}
