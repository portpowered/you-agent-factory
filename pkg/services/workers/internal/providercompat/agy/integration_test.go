package agy_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	agypkg "github.com/portpowered/infinite-you/pkg/services/workers/internal/providercompat/agy"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/internal/providercompat/inferencecontract"
)

func TestIntegrationRoutesThroughProvidersRoot(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	mock := &workers.MockPTYAllocator{
		Result: workers.PTYSessionResult{ExitCode: 0, CleanedText: "agy conductor answer"},
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

func TestIntegrationPreservesProviderDiagnosticsMetadata(t *testing.T) {
	t.Parallel()

	fake := &metadataProvidersFake{result: providers.ExecuteResult{
		Content: "media review",
		Diagnostics: &providers.ExecuteDiagnostics{
			Metadata: map[string]string{
				"input_tokens":    "89393",
				"thinking_tokens": "2312",
			},
		},
	}}
	integration := agypkg.NewIntegration(agypkg.IntegrationDependencies{
		ProvidersService: fake,
	})
	request := inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: "agy-integration-diagnostics",
		UserMessage:  "review media",
		Execution: workers.ProviderInferenceRequest{
			Dispatch: work.WorkDispatch{DispatchID: "agy-integration-diagnostics"},
		},
	})
	destination := &orderedWriter{}
	if err := inference.ExecuteInvocation(context.Background(), integration, request, destination); err != nil {
		t.Fatalf("ExecuteInvocation: %v", err)
	}
	if destination.completion == nil || destination.completion.Response() == nil {
		t.Fatalf("completion = %#v, want success", destination.completion)
	}
	metadata := destination.completion.Response().Metadata()
	if metadata["input_tokens"] != "89393" || metadata["thinking_tokens"] != "2312" {
		t.Fatalf("completion metadata = %#v, want provider usage facts", metadata)
	}
}

func TestIntegrationRoutesRequestedSessionThroughProvidersContinue(t *testing.T) {
	t.Parallel()

	fake := &continuationProvidersFake{}
	integration := agypkg.NewIntegration(agypkg.IntegrationDependencies{ProvidersService: fake})
	request := inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: "agy-integration-continue",
		UserMessage:  "continue the prior turn",
		Execution: workers.ProviderInferenceRequest{
			Dispatch:  work.WorkDispatch{DispatchID: "agy-integration-continue"},
			SessionID: "prior-session",
		},
	})
	destination := &orderedWriter{}
	if err := inference.ExecuteInvocation(context.Background(), integration, request, destination); err != nil {
		t.Fatalf("ExecuteInvocation: %v", err)
	}
	if fake.executeCalls != 0 {
		t.Fatalf("Providers.Execute calls = %d, want 0 for a requested session", fake.executeCalls)
	}
	wantReference := providers.SessionRef{
		Provider: providers.IDAntigravity,
		Kind:     providers.SessionIDKind,
		ID:       "prior-session",
	}
	if fake.continueRequest.Reference != wantReference {
		t.Fatalf("Providers.Continue reference = %#v, want %#v", fake.continueRequest.Reference, wantReference)
	}
	if fake.continueRequest.Attempt.Provider != providers.IDAntigravity ||
		fake.continueRequest.Attempt.AttemptID != "agy-integration-continue" {
		t.Fatalf("Providers.Continue attempt = %#v, want normalized Agy attempt", fake.continueRequest.Attempt)
	}
	if destination.completion == nil || destination.completion.Response() == nil ||
		destination.completion.Response().Content() != "continued Agy response" {
		t.Fatalf("completion = %#v, want continued Agy response", destination.completion)
	}
}

type continuationProvidersFake struct {
	providers.Service
	continueRequest providers.ContinueRequest
	executeCalls    int
}

type metadataProvidersFake struct {
	providers.Service
	result providers.ExecuteResult
}

func (fake *metadataProvidersFake) Execute(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return fake.result.Clone(), nil
}

func (fake *continuationProvidersFake) Execute(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	fake.executeCalls++
	return providers.ExecuteResult{}, nil
}

func (fake *continuationProvidersFake) Continue(
	_ context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	fake.continueRequest = request
	return providers.ContinueResult{
		Reference: request.Reference,
		Outcome:   providers.ContinuationOutcomeResumed,
		Result:    providers.ExecuteResult{Content: "continued Agy response"},
	}, nil
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
