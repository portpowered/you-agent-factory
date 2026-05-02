//go:build functionallong

package replay_contracts

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestLegacyUnaryRetirementSmoke_ReplaySubmitsCanonicalBatchWorkRequests(t *testing.T) {
	support.SkipLongFunctional(t, "slow replay legacy-unary retirement smoke")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	artifactPath := filepath.Join(t.TempDir(), "retired-unary-smoke.replay.json")
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "step one COMPLETE"},
		interfaces.InferenceResponse{Content: "step two COMPLETE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithRecordPath(artifactPath),
	)
	request := interfaces.WorkRequest{
		RequestID: "request-retired-unary-replay",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "replayed",
			WorkID:     "work-retired-unary-replay",
			WorkTypeID: "task",
			Payload:    []byte(`{"title":"record replay canonical submit"}`),
		}},
	}
	h.SubmitWorkRequest(context.Background(), request)
	h.RunUntilComplete(t, 10*time.Second)

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	assertReplayWorkRequestRecorded(t, artifact, request.RequestID, "external-submit", 1, 0)
	replayHarness := testutil.AssertReplaySucceeds(t, artifactPath, 10*time.Second)
	events, err := replayHarness.Service.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents after replay: %v", err)
	}
	support.AssertSingleWorkRequestEvent(t, events, request.RequestID, "work-retired-unary-replay", "task")
}
