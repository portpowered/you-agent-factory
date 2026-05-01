package guards_batch

import (
	"strings"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestResourceGated_DispatchTokenName(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "resource_contention"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "item-alpha"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "item-beta"}`))

	provider := testutil.NewMockProvider(
		support.AcceptedProviderResponse(),
		support.AcceptedProviderResponse(),
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:complete", 2).
		HasNoTokenInPlace("task:init")

	rtSnap, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot failed: %v", err)
	}

	if len(rtSnap.DispatchHistory) == 0 {
		t.Fatal("expected at least 1 CompletedDispatch in DispatchHistory, got 0")
	}

	for _, cd := range rtSnap.DispatchHistory {
		tokenIdentities := support.DeriveTokenIdentities(cd.ConsumedTokens, cd.OutputMutations)
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

	snap := h.Marking()
	resourceTokens := snap.TokensInPlace("slot:available")
	if len(resourceTokens) != 1 {
		t.Errorf("expected 1 resource token in slot:available, got %d", len(resourceTokens))
	}

	for _, tok := range resourceTokens {
		if tok.Color.DataType != interfaces.DataTypeResource {
			t.Errorf("resource token %s has DataType %q, expected %q",
				tok.ID, tok.Color.DataType, interfaces.DataTypeResource)
		}
	}
}
