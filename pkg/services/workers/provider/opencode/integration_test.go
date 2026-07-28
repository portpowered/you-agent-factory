package opencode_test

import (
	"context"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/opencode"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

func TestIntegrationGoldenTimeoutStdout(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.MustRepoRoot(t)
	stdoutPath := repoRoot + "/docs/temp/functional/provider-sessions/opencode/timeout/stdout.jsonl"
	stdout, err := os.ReadFile(stdoutPath)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout:   stdout,
		Stderr:   nil,
		ExitCode: 1,
	})
	providersService, err := providerswire.NewService(providerswire.WithCommandRunner(runner))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	integration := opencode.NewIntegration(opencode.IntegrationDependencies{
		ProvidersService: providersService,
	})
	request := inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: "opencode-golden-timeout",
		Model:        "openai/gpt-5",
		UserMessage:  "hello opencode",
	})
	destination := &orderedWriter{}
	if err := inference.ExecuteInvocation(context.Background(), integration, request, destination); err != nil {
		t.Fatalf("ExecuteInvocation: %v", err)
	}
	if destination.completion == nil || destination.completion.Failure() == nil {
		t.Fatalf("completion = %#v, want failure", destination.completion)
	}
	if got := destination.completion.Failure().Kind(); got != inference.FailureTimeout {
		t.Fatalf("failure kind = %q, want %q", got, inference.FailureTimeout)
	}
}

func TestIntegrationGoldenStructuredSnapshotStdout(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.MustRepoRoot(t)
	stdoutPath := repoRoot + "/docs/temp/functional/provider-sessions/opencode/structured-snapshot-success/stdout.jsonl"
	stdout, err := os.ReadFile(stdoutPath)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: stdout})
	providersService, err := providerswire.NewService(providerswire.WithCommandRunner(runner))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	integration := opencode.NewIntegration(opencode.IntegrationDependencies{
		ProvidersService: providersService,
	})
	request := inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: "opencode-golden-snapshot",
		Model:        "openai/gpt-5",
		UserMessage:  "hello opencode",
	})
	destination := &orderedWriter{}
	if err := inference.ExecuteInvocation(context.Background(), integration, request, destination); err != nil {
		t.Fatalf("ExecuteInvocation: %v", err)
	}
	if destination.completion == nil || destination.completion.Response() == nil {
		t.Fatalf("completion = %#v, want success", destination.completion)
	}
	if got := destination.completion.Response().Content(); got != "Hello world COMPLETE" {
		t.Fatalf("terminal content = %q", got)
	}
}

func TestIntegrationRoutesThroughProvidersRoot(t *testing.T) {
	t.Parallel()

	stdout := openCodeStructuredSuccessStream()
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: stdout})
	providersService, err := providerswire.NewService(providerswire.WithCommandRunner(runner))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	integration := opencode.NewIntegration(opencode.IntegrationDependencies{
		ProvidersService: providersService,
	})
	request := inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: "opencode-integration-test",
		Model:        "openai/gpt-5",
		UserMessage:  "hello opencode",
	})
	destination := &orderedWriter{}
	if err := inference.ExecuteInvocation(context.Background(), integration, request, destination); err != nil {
		t.Fatalf("ExecuteInvocation: %v", err)
	}
	if destination.completion == nil || destination.completion.Response() == nil {
		t.Fatalf("completion = %#v, want success", destination.completion)
	}
	if got := destination.completion.Response().Content(); got != "Hello world" {
		t.Fatalf("terminal content = %q, want Hello world", got)
	}
}

func openCodeStructuredSuccessStream() []byte {
	return []byte(
		`{"type":"step_start","sessionID":"session-42"}` + "\n" +
			`{"type":"text","sessionID":"session-42","part":{"id":"message-7","text":"Hello world","time":{"end":1}}}` + "\n",
	)
}

type orderedWriter struct {
	completion *inference.Completion
}

func (w *orderedWriter) WriteEvent(_ context.Context, _ inference.EventDraft) error {
	return nil
}

func (w *orderedWriter) Close(_ context.Context, completion inference.Completion) error {
	clone := completion
	w.completion = &clone
	return nil
}
