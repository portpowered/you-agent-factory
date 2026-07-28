package pi_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	pipkg "github.com/portpowered/infinite-you/pkg/services/workers/provider/pi"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

func TestIntegrationRoutesThroughProvidersRoot(t *testing.T) {
	t.Parallel()

	stdout := []byte(
		`{"type":"session","id":"pi-session-123"}` + "\n" +
			`{"type":"message_start","message":{"id":"msg-1","role":"assistant","content":[]}}` + "\n" +
			`{"type":"message_end","message":{"id":"msg-1","role":"assistant","content":[{"type":"text","text":"pi conductor answer"}],"stopReason":"stop"}}` + "\n",
	)
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: stdout})
	providersService, err := providerswire.NewService(providerswire.WithCommandRunner(runner))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	integration := pipkg.NewIntegration(pipkg.IntegrationDependencies{
		ProvidersService: providersService,
	})
	request := inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: "pi-integration-test",
		Model:        "pi-test-model",
		UserMessage:  "hello pi",
	})
	destination := &orderedWriter{}
	if err := inference.ExecuteInvocation(context.Background(), integration, request, destination); err != nil {
		t.Fatalf("ExecuteInvocation: %v", err)
	}
	if destination.completion == nil || destination.completion.Response() == nil {
		t.Fatalf("completion = %#v, want success", destination.completion)
	}
	if got := destination.completion.Response().Content(); got != "pi conductor answer" {
		t.Fatalf("terminal content = %q, want pi conductor answer", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("command runner calls = %d, want 1", runner.CallCount())
	}
	command := runner.LastRequest()
	if command.Command != "pi" {
		t.Fatalf("command = %q, want pi", command.Command)
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
