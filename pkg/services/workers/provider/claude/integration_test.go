package claude_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/claude"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

func TestIntegrationAuthoritativeMessageMatchesSanitizedProgress(t *testing.T) {
	t.Parallel()

	userMessage := "Original request:\nconfigured quorum request\n\nBranch A output:\nbranch A"
	echoedContent := "merged quorum response:\n" + userMessage + "\nCOMPLETE"
	resultRecord, err := json.Marshal(map[string]any{
		"type":       "result",
		"subtype":    "success",
		"is_error":   false,
		"result":     echoedContent,
		"session_id": "claude-integration-test-session",
	})
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	stdout := append(resultRecord, '\n')

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: stdout})
	providersService, err := providerswire.NewService(providerswire.WithCommandRunner(runner))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	integration := claude.NewIntegration(claude.IntegrationDependencies{
		ProvidersService: providersService,
	})
	request := inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: "claude-echo-test",
		Model:        "claude-sonnet-4-20250514",
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
