package functional_test

import (
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestResourceGated_DispatchTokenName verifies that when a workstation uses
// resource_usage (a transition that requires both a work token and a resource
// token), the DispatchEntry.TokenName in the engine state snapshot reflects the
// work-item name, not the resource-slot name.
//
// The resource_contention fixture defines a single resource "slot" with
// capacity=1, forcing serialised processing. We submit two work items and
// verify that every CompletedDispatch in the history carries the work-item
// name (not "slot:0" or similar resource token names).
func TestResourceGated_DispatchTokenName(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "resource_contention"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "item-alpha"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "item-beta"}`))

	provider := testutil.NewMockProvider(
		acceptedProviderResponse(),
		acceptedProviderResponse(),
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 10*time.Second)

	// All work items should have completed.
	h.Assert().
		PlaceTokenCount("task:complete", 2).
		HasNoTokenInPlace("task:init")

	// Verify that every derived token name in the dispatch history is a
	// work-item name, not a resource-slot name.
	rtSnap, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot failed: %v", err)
	}

	if len(rtSnap.DispatchHistory) == 0 {
		t.Fatal("expected at least 1 CompletedDispatch in DispatchHistory, got 0")
	}

	for _, cd := range rtSnap.DispatchHistory {
		tokenIdentities := deriveTokenIdentities(cd.ConsumedTokens, cd.OutputMutations)
		if len(tokenIdentities.TokenNames) == 0 {
			t.Errorf("CompletedDispatch %s has no work token name", cd.TransitionID)
			continue
		}
		for _, tokenName := range tokenIdentities.TokenNames {
			if strings.HasPrefix(tokenName, "slot:") {
				t.Errorf("CompletedDispatch %s TokenName %q looks like a resource-slot name, expected a work-item name",
					cd.TransitionID, tokenName)
			}
		}
	}

	// Verify the marking still has the resource token in its available place.
	snap := h.Marking()
	resourceTokens := snap.TokensInPlace("slot:available")
	if len(resourceTokens) != 1 {
		t.Errorf("expected 1 resource token in slot:available, got %d", len(resourceTokens))
	}

	// Extra check: verify resource tokens have DataTypeResource.
	for _, tok := range resourceTokens {
		if tok.Color.DataType != interfaces.DataTypeResource {
			t.Errorf("resource token %s has DataType %q, expected %q",
				tok.ID, tok.Color.DataType, interfaces.DataTypeResource)
		}
	}
}
