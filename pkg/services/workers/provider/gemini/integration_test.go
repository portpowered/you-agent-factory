package gemini_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	geminipkg "github.com/portpowered/infinite-you/pkg/services/workers/provider/gemini"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

func TestIntegrationRoutesThroughProvidersRoot(t *testing.T) {
	t.Parallel()

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("gemini conductor answer"),
	})
	providersService, err := providerswire.NewService(providerswire.WithCommandRunner(runner))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	integration := geminipkg.NewIntegration(geminipkg.IntegrationDependencies{
		ProvidersService: providersService,
	})
	request := inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: "gemini-integration-test",
		Model:        "gemini-2.5-pro",
		UserMessage:  "say hello",
	})
	destination := &orderedWriter{}
	if err := inference.ExecuteInvocation(context.Background(), integration, request, destination); err != nil {
		t.Fatalf("ExecuteInvocation: %v", err)
	}
	if destination.completion == nil || destination.completion.Response() == nil {
		t.Fatalf("completion = %#v, want success", destination.completion)
	}
	if got := destination.completion.Response().Content(); got != "gemini conductor answer" {
		t.Fatalf("terminal content = %q, want gemini conductor answer", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("command runner calls = %d, want 1", runner.CallCount())
	}
	command := runner.LastRequest()
	if command.Command != "gemini" {
		t.Fatalf("command = %q, want gemini", command.Command)
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
