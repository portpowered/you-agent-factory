//go:build functionallong

package replay_contracts

import (
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory/scheduler"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestReplayAdhocWorkInQueueScheduler_PrioritizesInitializedTraceProgressOverFIFO(t *testing.T) {
	support.SkipLongFunctional(t, "slow replay work-in-queue scheduler ordering sweep")

	artifactPath := support.AgentFactoryPath(t, filepath.Join("tests", "functional_test", "testdata", "adhoc-recording-batch-event-log.json"))
	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	assertRecordedAdhocReplayIsUnary(t, artifact)

	fifoDispatches, fifoHarness := runRecordedAdhocReplay(t, artifactPath, scheduler.NewFIFOScheduler())
	workInQueueDispatches, workInQueueHarness := runRecordedAdhocReplay(t, artifactPath, scheduler.NewWorkInQueueScheduler(8))

	assertReplayWorkInQueueAdvancesInitializedTracesEarlierThanFIFO(t, fifoDispatches, workInQueueDispatches)
	assertInitializedTraceProgressIsPrioritized(t, workInQueueDispatches)
	assertReplayFinishedWithoutCompletedOrActiveDispatches(t, fifoHarness)
	assertReplayFinishedWithoutCompletedOrActiveDispatches(t, workInQueueHarness)
}
