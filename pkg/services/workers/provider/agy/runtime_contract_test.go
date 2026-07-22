package agy_test

import (
	"context"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/agy"
)

type runtimeContractAllocator struct {
	launch agypty.ProcessLaunch
}

func (a *runtimeContractAllocator) Allocate(
	_ context.Context,
	launch agypty.ProcessLaunch,
	_ agypty.SessionConfig,
) (agypty.PTYSession, error) {
	a.launch = launch
	return runtimeContractSession{}, nil
}

type runtimeContractSession struct{}

func (runtimeContractSession) Run(context.Context) (agypty.SessionResult, error) {
	return agypty.SessionResult{ExitCode: 0, CleanedText: "Agy adapter response"}, nil
}

func (runtimeContractSession) Close() error { return nil }

func TestRuntimeContractExecutesThroughInjectedNativePTY(t *testing.T) {
	t.Parallel()

	allocator := &runtimeContractAllocator{}
	providerAdapter, err := agy.NewAdapterWithAllocator(t.TempDir(), allocator, executableDependencies(nil))
	if err != nil {
		t.Fatalf("NewAdapterWithAllocator() error = %v", err)
	}
	registry, err := adapter.NewRegistry(providerAdapter)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	runner, err := providerAdapter.PTYRunner()
	if err != nil {
		t.Fatalf("PTYRunner() error = %v", err)
	}

	const prompt = "summarize this; preserve argv boundaries"
	result, err := adapter.Execute(context.Background(), registry, runner, adapter.ExecuteInput{
		Provider: providerAdapter.Identity(),
		Command: adapter.CommandContext{Request: workers.ProviderInferenceRequest{
			Dispatch:         work.WorkDispatch{DispatchID: "dispatch-agy-contract"},
			Model:            "gemini-pro",
			SessionID:        "session-agy-contract",
			WorkingDirectory: ".",
			UserMessage:      prompt,
		}},
		Decoder: adapter.DecoderContext{
			RunID:      "run-agy-contract",
			DispatchID: "dispatch-agy-contract",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Response.Content != "Agy adapter response" || !result.Capabilities.FinalOnly {
		t.Fatalf("execution result = %#v, want final-only provider response", result)
	}
	if len(allocator.launch.Argv) == 0 || allocator.launch.Argv[len(allocator.launch.Argv)-1] != prompt {
		t.Fatalf("PTY launch argv = %#v, want prompt preserved as one argument", allocator.launch.Argv)
	}
}

func TestConstructionRetainsCallerOwnedPTYEdge(t *testing.T) {
	t.Parallel()

	allocator := &runtimeContractAllocator{}
	providerAdapter, err := agy.NewAdapterWithAllocator(t.TempDir(), allocator, executableDependencies(nil))
	if err != nil {
		t.Fatalf("construct Agy adapter: %v", err)
	}
	if got := providerAdapter.Identity(); got != adapter.Identity(modelprovider.ProviderAgy) {
		t.Fatalf("provider identity = %q, want %q", got, modelprovider.ProviderAgy)
	}
	got, err := providerAdapter.PTYAllocator()
	if err != nil {
		t.Fatalf("resolve Agy PTY allocator: %v", err)
	}
	if got != allocator {
		t.Fatal("Agy adapter did not retain the caller-owned PTY allocator")
	}
}

func TestAdapterFailsClosedWithoutCallerOwnedPTYEdge(t *testing.T) {
	t.Parallel()

	providerAdapter := &agy.Adapter{}
	if allocator, err := providerAdapter.PTYAllocator(); err == nil || allocator != nil {
		t.Fatalf("PTYAllocator() = (%T, %v), want nil and error", allocator, err)
	}
}
