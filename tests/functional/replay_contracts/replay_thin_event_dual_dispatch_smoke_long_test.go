//go:build functionallong

package replay_contracts

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestReplayThinEventDualDispatchSmoke_ReplayAndReadersReuseSharedArtifact(t *testing.T) {
	support.SkipLongFunctional(t, "slow replay dual-dispatch artifact reuse sweep")

	smoke := runThinEventDualDispatchSmoke(t)

	replay := observeReplayThroughRoot(t, smoke.artifactPath, 10*time.Second)
	assertReplayPlaceCounts(t, replay.Work, map[string]int{
		"task:complete": 1,
		dualDispatchSmokeScriptWorkType + ":done": 1,
		"task:failed": 0,
		dualDispatchSmokeScriptWorkType + ":failed": 0,
	})
	if !replayWorkIncludesID(replay.Work, smoke.modelWorkID) || !replayWorkIncludesID(replay.Work, smoke.scriptWorkID) {
		t.Fatalf("replay Work listing = %#v, want model work %q and script work %q", replay.Work.Results, smoke.modelWorkID, smoke.scriptWorkID)
	}
	assertThinEventDispatchLifecycleForWork(t, replay.Events, smoke.modelWorkID, factoryapi.FactoryEventTypeInferenceRequest, factoryapi.FactoryEventTypeInferenceResponse)
	assertThinEventDispatchLifecycleForWork(t, replay.Events, smoke.scriptWorkID, factoryapi.FactoryEventTypeScriptRequest, factoryapi.FactoryEventTypeScriptResponse)
	assertThinEventModelAttemptCorrelation(t, replay.Events, smoke.modelWorkID)
	assertThinEventScriptBoundaryFacts(t, replay.Events, smoke.scriptWorkID)
}
