//go:build functionallong

package replay_contracts

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestReplayThinEventDualDispatchSmoke_ReplayAndReadersReuseSharedArtifact(t *testing.T) {
	support.SkipLongFunctional(t, "slow replay dual-dispatch artifact reuse sweep")

	smoke := runThinEventDualDispatchSmoke(t)

	replayHarness := testutil.AssertReplaySucceeds(t, smoke.artifactPath, 10*time.Second)
	replayHarness.Service.Assert().
		PlaceTokenCount("task:complete", 1).
		PlaceTokenCount(dualDispatchSmokeScriptWorkType+":done", 1).
		HasNoTokenInPlace("task:failed").
		HasNoTokenInPlace(dualDispatchSmokeScriptWorkType + ":failed")

	finalTick := lastFactoryEventTick(smoke.artifact.Events)
	worldState, err := projections.ReconstructFactoryWorldState(smoke.artifact.Events, finalTick)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}

	assertThinEventReconstructedModelReader(t, smoke, worldState)
	assertThinEventReconstructedScriptReader(t, smoke, worldState)
	assertThinEventWorkstationRequestProjection(t, smoke, worldState)
}
