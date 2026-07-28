package cursor_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/cursor"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

func TestIntegrationRoutesThroughProvidersRoot(t *testing.T) {
	t.Parallel()

	stdout := []byte(`{"type":"result","subtype":"success","is_error":false,"result":"cursor conductor answer","session_id":"cursor-session-123"}` + "\n")
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: stdout})
	providersService, err := providerswire.NewService(providerswire.WithCommandRunner(runner))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	integration := cursor.NewIntegration(cursor.IntegrationDependencies{
		ProvidersService: providersService,
	})
	request := inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: "cursor-integration-test",
		Model:        "cursor-test-model",
		UserMessage:  "hello cursor",
	})
	destination := &orderedWriter{}
	if err := inference.ExecuteInvocation(context.Background(), integration, request, destination); err != nil {
		t.Fatalf("ExecuteInvocation: %v", err)
	}
	if destination.completion == nil || destination.completion.Response() == nil {
		t.Fatalf("completion = %#v, want success", destination.completion)
	}
	if got := destination.completion.Response().Content(); got != "cursor conductor answer" {
		t.Fatalf("terminal content = %q, want cursor conductor answer", got)
	}
}

type orderedWriter struct {
	order      []string
	completion *inference.Completion
}

func (w *orderedWriter) WriteEvent(_ context.Context, event inference.EventDraft) error {
	draft := event.Draft()
	w.order = append(w.order, string(draft.Kind)+":"+string(draft.Phase))
	return nil
}

func (w *orderedWriter) Close(_ context.Context, completion inference.Completion) error {
	w.order = append(w.order, "CLOSE")
	clone := completion
	w.completion = &clone
	return nil
}
