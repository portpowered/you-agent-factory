package guards_batch

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestResourceGated_DispatchTokenName(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "resource_contention"))

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{Name: "item-alpha", WorkTypeID: "task", Payload: []byte("alpha")})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{Name: "item-beta", WorkTypeID: "task", Payload: []byte("beta")})

	provider := testutil.NewMockProvider(
		support.AcceptedProviderResponse(),
		support.AcceptedProviderResponse(),
	)
	session, listedWork, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		10*time.Second,
	)
	assertGuardSessionPlaces(t, session, map[string]int{"task:complete": 2, "task:init": 0})
	assertGuardResourceAvailability(t, session, "slot", 1)

	dispatchedWorkIDs := map[string]bool{}
	for _, dispatch := range support.ObserveDispatchEvents(t, events) {
		for _, workID := range dispatch.WorkIDs {
			dispatchedWorkIDs[workID] = true
		}
		for _, input := range dispatch.Request.Inputs {
			dispatchedWorkIDs[input.WorkId] = true
		}
	}
	names := map[string]bool{}
	for _, item := range listedWork.Results {
		if item.WorkId != nil && dispatchedWorkIDs[*item.WorkId] {
			names[item.Name] = true
		}
	}
	for _, want := range []string{"item-alpha", "item-beta"} {
		if !names[want] {
			t.Errorf("publicly dispatched Work names = %v, missing %q", names, want)
		}
	}
}
