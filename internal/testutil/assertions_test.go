package testutil_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func TestMarkingAssertCountsExplicitSnapshotTokens(t *testing.T) {
	t.Parallel()

	marking := &factoryruntime.PetriMarkingSnapshot{
		Tokens: map[string]*factoryruntime.RuntimeToken{
			"done-1": {ID: "done-1", PlaceID: "item:done"},
		},
		PlaceTokens: map[string][]string{
			"item:done": {"done-1"},
		},
	}

	testutil.AssertMarking(t, marking).
		HasTokenInPlace("item:done").
		HasNoTokenInPlace("item:new").
		PlaceTokenCount("item:done", 1).
		TokenCount(1)
}
