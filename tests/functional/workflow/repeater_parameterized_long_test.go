//go:build functionallong

package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestRepeater_RefiresOnRejectedStopsOnAccepted(t *testing.T) {
	support.SkipLongFunctional(t, "slow repeater rejection-to-acceptance sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "repeater_workstation"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "repeater test"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"exec-worker":   {{Content: "retry"}, {Content: "retry"}, {Content: "done COMPLETE"}},
		"finish-worker": {{Content: "done COMPLETE"}},
	})
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second)

	if provider.CallCount("exec-worker") != 3 {
		t.Errorf("expected exec-worker called 3 times, got %d", provider.CallCount("exec-worker"))
	}
	assertWorkflowSessionPlaces(t, listed, map[string]int{"task:complete": 1, "task:init": 0, "task:failed": 0})
}

func TestRepeater_GuardedLoopBreakerTerminatesRejectedRepeater(t *testing.T) {
	support.SkipLongFunctional(t, "slow repeater guarded loop-breaker sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "repeater_workstation"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "exhaustion test"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"exec-worker": {
			{Content: "retry"}, {Content: "retry"}, {Content: "retry"}, {Content: "retry"},
			{Content: "retry"}, {Content: "retry"}, {Content: "retry"},
		},
		"finish-worker": {{Content: "done COMPLETE"}},
	})
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	listed := support.ListDefaultSessionWork(t, server.URL())
	assertWorkflowSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:complete": 0})
	assertPublicDispatchRoute(t, server.GetFactoryEvents(t), "executor-loop-breaker", "task:failed")
	server.Stop(t)
}

func TestRepeater_ResourceReleaseBetweenIterations_ServiceHarness(t *testing.T) {
	support.SkipLongFunctional(t, "slow repeater service-harness resource-release sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "repeater_resource"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "service resource repeater test"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Still working"},
		workerexecution.InferenceResponse{Content: "Almost there"},
		workerexecution.InferenceResponse{Content: "Done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Finalized. COMPLETE"},
	)

	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 15*time.Second)
	assertWorkflowSessionPlaces(t, listed, map[string]int{"task:complete": 1, "task:init": 0, "task:failed": 0})

	if provider.CallCount() != 4 {
		t.Errorf("expected provider called 4 times, got %d", provider.CallCount())
	}
}
