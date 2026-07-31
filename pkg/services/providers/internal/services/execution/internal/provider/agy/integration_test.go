package agy_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty"
	agypkg "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/agy"
	inference "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/inferencecontract"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestIntegrationRoutesThroughProvidersRoot(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	mock := &agypty.MockAllocator{
		Result: agypty.SessionResult{ExitCode: 0, CleanedText: "agy conductor answer"},
	}
	providersService, err := providerswire.NewService(
		providerswire.WithCommandRunner(testutil.NewProviderCommandRunner()),
		providerswire.WithAgyPTY(providerswire.AgyPTYPlatformDependencies{
			Allocator: mock,
			Locator:   platformprocess.HostExecutableLocator{},
			Inspector: platformfilesystem.Local{},
		}),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	integration := agypkg.NewIntegration(agypkg.IntegrationDependencies{
		ProvidersService: providersService,
	})
	request := inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: "agy-integration-test",
		Model:        "agy-default",
		UserMessage:  "say hello",
		Execution: workers.ProviderInferenceRequest{
			Dispatch: work.WorkDispatch{
				DispatchID: "agy-integration-test",
			},
			WorkingDirectory: factoryRoot,
		},
	})
	destination := &orderedWriter{}
	if err := inference.ExecuteInvocation(context.Background(), integration, request, destination); err != nil {
		t.Fatalf("ExecuteInvocation: %v", err)
	}
	if destination.completion == nil || destination.completion.Response() == nil {
		t.Fatalf("completion = %#v, want success", destination.completion)
	}
	if got := destination.completion.Response().Content(); got != "agy conductor answer" {
		t.Fatalf("terminal content = %q, want agy conductor answer", got)
	}
}

type orderedWriter struct {
	events     []inference.EventDraft
	completion *inference.Completion
}

func (w *orderedWriter) WriteEvent(_ context.Context, event inference.EventDraft) error {
	w.events = append(w.events, event)
	return nil
}

func (w *orderedWriter) Close(_ context.Context, completion inference.Completion) error {
	clone := completion
	w.completion = &clone
	return nil
}
