//go:build functionallong

package replay_contracts

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestLegacyUnaryRetirementSmoke_ReplaySubmitsCanonicalBatchWorkRequests(t *testing.T) {
	support.SkipLongFunctional(t, "slow replay legacy-unary retirement smoke")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	artifactPath := filepath.Join(t.TempDir(), "retired-unary-smoke.replay.json")
	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "step one COMPLETE"},
		workerexecution.InferenceResponse{Content: "step two COMPLETE"},
	)
	request := factoryapi.WorkRequest{
		RequestId: "request-retired-unary-replay",
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{{
			Name:         "replayed",
			WorkId:       strPtr("work-retired-unary-replay"),
			WorkTypeName: strPtr("task"),
			Payload:      []byte(`{"title":"record replay canonical submit"}`),
		}},
	}
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--record", artifactPath},
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})
	support.UpsertDefaultSessionWorkRequest(t, server.URL(), request)
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	server.Stop(t)

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	assertReplayWorkRequestRecorded(t, artifact, request.RequestId, "external-submit", 1, 0)
	replayServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: t.TempDir(),
		Args:       []string{"--replay", artifactPath},
	})
	support.WaitForTerminalStatus(t, replayServer.URL(), 10*time.Second)
	events := replayServer.GetFactoryEvents(t)
	replayServer.Stop(t)
	support.AssertSingleWorkRequestEvent(t, events, request.RequestId, "work-retired-unary-replay", "task")
}
