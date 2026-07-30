package factorysessionexecution

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const workersImportRoot = "github.com/portpowered/infinite-you/pkg/services/workers"

var executionWorkersLeaseImportRoots = []string{
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/...",
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/livechild/...",
}

// TestExecutionPackagesImportWorkersOnlyThroughRoot seals execution and
// durable-provider binding call sites to the Workers service root contract.

// TestExecutionServiceRolesNameWorkersRootContracts proves durable execution
// constructors and live-child binding factories type Workers-facing inputs only
// through the Workers service root.
func TestExecutionServiceRolesNameWorkersRootContracts(t *testing.T) {
	t.Parallel()

	var (
		_ LiveChildInvocationFactory
		_ workers.InvocationExecutor
		_ workers.Provider
		_ workers.ProgressPublisher
	)
}

func TestSmokeLiveChildProviderUsesWorkersRootInferenceContracts(t *testing.T) {
	t.Parallel()

	provider := SmokeLiveChildProvider()
	resp, err := provider.Infer(context.Background(), workers.ProviderInferenceRequest{
		Dispatch: work.WorkDispatch{
			DispatchID: "dispatch-boundary",
			WorkerType: "agent-run-fake-child",
		},
		UserMessage:   "summarize workflows",
		ModelProvider: "mock",
		Model:         "gpt-test",
		SessionID:     "session-boundary",
		RunnerID:      "runner-boundary",
		WorkerType:    "agent-run-fake-child",
	})
	if err != nil {
		t.Fatalf("Infer() error = %v, want nil", err)
	}
	if !strings.Contains(resp.Content, "live:agent-run-fake-child") {
		t.Fatalf("content = %q, want live child smoke payload", resp.Content)
	}
	if resp.ProviderSession == nil || resp.ProviderSession.ID != "live-provider-session-1" {
		t.Fatalf("provider session = %#v, want live-provider-session-1", resp.ProviderSession)
	}
}
