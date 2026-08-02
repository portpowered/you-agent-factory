package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestConfigDrivenRetryLoopBreaker_TerminatesAfterMaxRetries(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "retry_exhaustion"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Will exhaust retries"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Processed. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Needs work"},
		workerexecution.InferenceResponse{Content: "Processed. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Still needs work"},
		workerexecution.InferenceResponse{Content: "Processed. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Not good enough"},
	)

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 15*time.Second)
	listed := support.ListDefaultSessionWork(t, server.URL())
	assertWorkflowSessionPlaces(t, listed, map[string]int{
		"task:failed": 1, "task:init": 0, "task:in-review": 0, "task:complete": 0,
	})

	if provider.CallCount() != 6 {
		t.Errorf("expected provider called 6 times, got %d", provider.CallCount())
	}

	assertPublicDispatchRoute(t, server.GetFactoryEvents(t), "review-exhaustion", "task:failed")
	server.Stop(t)
}

func TestConfigDrivenRetryLoopBreaker_SucceedsBeforeLimit(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "retry_exhaustion"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Will succeed on second try"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Processed. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Needs work"},
		workerexecution.InferenceResponse{Content: "Processed. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Looks good. ACCEPTED"},
	)

	_, listed := support.RunFactoryToCompletionWithEdgesAndWorkStable(t, dir, serviceedges.Edges{ProviderOverride: provider}, 15*time.Second)
	assertWorkflowSessionPlaces(t, listed, map[string]int{"task:complete": 1, "task:init": 0, "task:failed": 0})
}

func assertPublicDispatchRoute(t *testing.T, events []factoryapi.FactoryEvent, transitionID, toPlaceID string) {
	t.Helper()
	var sawDispatch bool
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		sawDispatch = sawDispatch || payload.TransitionId == transitionID
	}
	if !sawDispatch {
		t.Fatalf("public events missing transition %s before terminal place %s", transitionID, toPlaceID)
	}
}

func assertWorkflowSessionPlaces(t *testing.T, listed factoryapi.ListWorkResponse, wants map[string]int) {
	t.Helper()
	for placeID, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
}
