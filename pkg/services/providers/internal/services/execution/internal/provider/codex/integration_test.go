package codex_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/codex"
	inference "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/inferencecontract"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

func TestIntegrationAuthoritativeMessageMatchesSanitizedProgress(t *testing.T) {
	t.Parallel()

	userMessage := "Original request:\nconfigured quorum request\n\nBranch A output:\nbranch A"
	echoedContent := "merged quorum response:\n" + userMessage + "\nCOMPLETE"
	item, err := json.Marshal(map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"id":   "codex-functional-message",
			"type": "agent_message",
			"text": echoedContent,
		},
	})
	if err != nil {
		t.Fatalf("marshal item.completed: %v", err)
	}
	turnCompleted, err := json.Marshal(map[string]any{
		"type":  "turn.completed",
		"usage": map[string]any{"input_tokens": 1, "output_tokens": 1},
	})
	if err != nil {
		t.Fatalf("marshal turn.completed: %v", err)
	}
	stdout := []byte(`{"type":"turn.started"}` + "\n" + string(item) + "\n" + string(turnCompleted) + "\n")

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: stdout})
	providersService, err := providerswire.NewService(providerswire.WithCommandRunner(runner))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	integration := codex.NewIntegration(codex.IntegrationDependencies{
		ProvidersService: providersService,
	})
	request := inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: "codex-echo-test",
		Model:        "gpt-5",
		UserMessage:  userMessage,
	})
	destination := &orderedWriter{}
	if err := inference.ExecuteInvocation(context.Background(), integration, request, destination); err != nil {
		t.Fatalf("ExecuteInvocation: %v", err)
	}
	if destination.completion == nil || destination.completion.Response() == nil {
		t.Fatalf("completion = %#v, want success", destination.completion)
	}
	if got := destination.completion.Response().Content(); got != echoedContent {
		t.Fatalf("terminal content = %q, want echoed content", got)
	}
}

type orderedWriter struct {
	order      []string
	completion *inference.Completion
	closes     int
}

func (w *orderedWriter) WriteEvent(_ context.Context, event inference.EventDraft) error {
	draft := event.Draft()
	w.order = append(w.order, string(draft.Kind)+":"+string(draft.Phase))
	return nil
}

func (w *orderedWriter) Close(_ context.Context, completion inference.Completion) error {
	w.order = append(w.order, "CLOSE")
	w.closes++
	w.completion = &completion
	return nil
}
