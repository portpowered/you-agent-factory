package providers

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/pkg/workers/agypty"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/workers/provider/agy"
)

type functionalAgyAllocator struct {
	launch agypty.ProcessLaunch
}

func (a *functionalAgyAllocator) Allocate(
	_ context.Context,
	launch agypty.ProcessLaunch,
	_ agypty.SessionConfig,
) (agypty.PTYSession, error) {
	a.launch = launch
	return &functionalAgySession{}, nil
}

type functionalAgySession struct{}

func (*functionalAgySession) Run(context.Context) (agypty.SessionResult, error) {
	return agypty.SessionResult{ExitCode: 0, CleanedText: "functional AGY response"}, nil
}

func (*functionalAgySession) Close() error { return nil }

func TestAgyRuntimeContractExecutesThroughNativePTYBoundary(t *testing.T) {
	allocator := &functionalAgyAllocator{}
	providerAdapter, err := agy.NewAdapterWithAllocator(
		t.TempDir(),
		allocator,
		agy.WithExecutable("agy"),
		agy.WithSessionConfig(agypty.DefaultSessionConfig()),
	)
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
		Command: adapter.CommandContext{Request: workerexecution.ProviderInferenceRequest{
			Dispatch:         work.WorkDispatch{DispatchID: "dispatch-functional-agy"},
			Model:            "gemini-pro",
			SessionID:        "session-functional-agy",
			WorkingDirectory: ".",
			UserMessage:      prompt,
		}},
		Decoder: adapter.DecoderContext{
			RunID:      "run-functional-agy",
			DispatchID: "dispatch-functional-agy",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Response.Content != "functional AGY response" || !result.Capabilities.FinalOnly {
		t.Fatalf("execution result = %#v, want final-only provider response", result)
	}
	if len(allocator.launch.Argv) == 0 || allocator.launch.Argv[len(allocator.launch.Argv)-1] != prompt {
		t.Fatalf("PTY launch argv = %#v, want prompt preserved as one argument", allocator.launch.Argv)
	}
}
