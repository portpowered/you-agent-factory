package kiro_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	kiropkg "github.com/portpowered/infinite-you/pkg/services/workers/provider/kiro"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

func TestIntegrationRoutesThroughProvidersRoot(t *testing.T) {
	t.Parallel()

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("kiro conductor answer"),
	})
	providersService, err := providerswire.NewService(providerswire.WithCommandRunner(runner))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	integration := kiropkg.NewIntegration(kiropkg.IntegrationDependencies{
		ProvidersService: providersService,
	})
	session := inference.NewProviderSession("kiro", "session_id", kiroSessionID, nil)
	request := inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID:    "kiro-integration-test",
		Model:           "claude-sonnet-4",
		UserMessage:     "say hello",
		ProviderSession: &session,
	})
	destination := &orderedWriter{}
	if err := inference.ExecuteInvocation(context.Background(), integration, request, destination); err != nil {
		t.Fatalf("ExecuteInvocation: %v", err)
	}
	if destination.completion == nil || destination.completion.Response() == nil {
		t.Fatalf("completion = %#v, want success", destination.completion)
	}
	if got := destination.completion.Response().Content(); got != "kiro conductor answer" {
		t.Fatalf("terminal content = %q, want kiro conductor answer", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("command runner calls = %d, want 1", runner.CallCount())
	}
	command := runner.LastRequest()
	if command.Command != "kiro-cli" {
		t.Fatalf("command = %q, want kiro-cli", command.Command)
	}
	if !containsConformanceArgPair(command.Args, "--resume-id", kiroSessionID) {
		t.Fatalf("args = %#v, want --resume-id %s", command.Args, kiroSessionID)
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
