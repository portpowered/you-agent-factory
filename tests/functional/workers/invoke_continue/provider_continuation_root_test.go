package acceptance

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestDirectWorkerSessionContinueUnsupportedProviderDoesNotFreshStartThroughRootProcess
// proves the canonical root-built Worker Session path preserves an opaque
// continuation and turns an unsupported Providers outcome into one terminal
// failure without invoking Execute as a fresh attempt.
func TestDirectWorkerSessionContinueUnsupportedProviderDoesNotFreshStartThroughRootProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	provider := &unsupportedContinuationProvider{
		MockProvider: testutil.NewMockProvider(workerexecution.InferenceResponse{
			Content: "initial provider output",
			ProviderSession: &providers.SessionMetadata{
				Provider: string(providers.IDCodex),
				Kind:     providers.SessionIDKind,
				ID:       "unsupported-source-thread",
			},
		}),
	}
	process := support.BuildProcess(t, serviceedges.Edges{ProviderOverride: provider})
	support.CleanupProcess(t, process)

	invoke := support.FakeInputs(ctx, []string{
		"you", "--json", "worker-sessions", "invoke", "--request-id", "unsupported-invoke-request",
		"--worker-session-id", "unsupported-source", "--dispatch-id", "unsupported-dispatch", "--workstation", "direct",
		"--worker-type", "direct-worker", "--runner", "codex", "--provider", "codex", "--model", "functional-model",
		"--user-message", "initial provider output",
	})
	if err := process.Execute(invoke.Input); err != nil {
		t.Fatalf("unsupported continuation source invoke: %v\nstdout:%s\nstderr:%s", err, invoke.Stdout(), invoke.Stderr())
	}

	continuation := support.FakeInputs(ctx, []string{
		"you", "--json", "worker-sessions", "continue", "unsupported-source", "--request-id", "unsupported-continue-request",
		"--successor-worker-session-id", "unsupported-successor", "--user-message", "must not fresh start",
	})
	err := process.Execute(continuation.Input)
	if err == nil {
		t.Fatal("unsupported provider continuation succeeded, want one terminal failure")
	}
	assertDirectWorkerSessionCLIError(t, continuation, "WORKER_SESSION_FAILED")

	if got := provider.CallCount(); got != 1 {
		t.Fatalf("provider Execute/Infer calls after unsupported continuation = %d, want initial call only", got)
	}
	if got := provider.continuationCalls.Load(); got != 1 {
		t.Fatalf("provider ContinueReference calls = %d, want one opaque continuation attempt", got)
	}
	gotReference, ok := provider.continuationReference.Load().(providers.ContinuationRef)
	if !ok {
		t.Fatal("provider ContinueReference did not receive a detached continuation reference")
	}
	wantReference := providers.ContinuationRef{
		Provider:          string(providers.IDCodex),
		Kind:              providers.SessionIDKind,
		ProviderSessionID: "unsupported-source-thread",
		ExternalRef:       "unsupported-source-thread",
	}
	if gotReference != wantReference {
		t.Fatalf("provider continuation reference = %#v, want exact opaque identity %#v", gotReference, wantReference)
	}
}

type unsupportedContinuationProvider struct {
	*testutil.MockProvider
	continuationCalls     atomic.Int32
	continuationReference atomic.Value
}

func (provider *unsupportedContinuationProvider) ContinueReference(
	_ context.Context,
	request providers.ContinueReferenceRequest,
) (providers.ContinueReferenceResult, error) {
	provider.continuationCalls.Add(1)
	provider.continuationReference.Store(request.Reference)
	return providers.ContinueReferenceResult{
		Reference: request.Reference,
		Outcome:   providers.ContinuationOutcomeUnsupported,
	}, nil
}

var _ providers.Service = (*unsupportedContinuationProvider)(nil)
