package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestRejectionPath_NoRejectionArcsFailsToken(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "rejection_no_arcs"))

	testutil.WriteSeedFile(t, dir, "task", []byte("work payload"))

	provider := testutil.NewMockProvider(support.RejectedProviderResponse("not good enough"))
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 5*time.Second)
	assertWorkflowSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:done": 0})
}

func TestRejectionPath_NoRejectionArcsReleasesResources(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "rejection_no_arcs_resources"))

	testutil.WriteSeedFile(t, dir, "task", []byte("first item"))
	testutil.WriteSeedFile(t, dir, "task", []byte("second item"))

	provider := testutil.NewMockProvider(
		support.RejectedProviderResponse("not good enough"),
		support.AcceptedProviderResponse(),
	)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second)
	assertWorkflowSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:done": 1, "task:init": 0})
}

func TestRejectionPath_WithRejectionArcsRoutesViaArcs(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "rejection_with_arcs"))

	testutil.WriteSeedFile(t, dir, "task", []byte("work"))

	provider := testutil.NewMockProvider(
		support.RejectedProviderResponse("needs work"),
		support.AcceptedProviderResponse(),
	)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second)
	assertWorkflowSessionPlaces(t, listed, map[string]int{"task:done": 1, "task:init": 0, "task:failed": 0})
}

func TestRejectionPath_NoRejectionArcsFailureRecordSet(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "rejection_no_arcs"))

	testutil.WriteSeedFile(t, dir, "task", []byte("work"))

	provider := testutil.NewMockProvider(support.RejectedProviderResponse("missing tests"))
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 5*time.Second)
	listed := support.ListDefaultSessionWork(t, server.URL())
	assertWorkflowSessionPlaces(t, listed, map[string]int{"task:failed": 1})
	for _, event := range server.GetFactoryEvents(t) {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		if payload.Outcome != factoryapi.WorkOutcomeRejected || payload.Output == nil || *payload.Output != "missing tests" {
			t.Fatalf("dispatch response = %#v, want recorded rejection feedback", payload)
		}
		server.Stop(t)
		return
	}
	t.Fatal("Factory Event history has no dispatch response")
}

func assertWorkflowSessionPlaces(t *testing.T, listed factoryapi.ListWorkResponse, wants map[string]int) {
	t.Helper()
	for placeID, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
}
