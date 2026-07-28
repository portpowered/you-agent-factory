//go:build functionallong

package submission_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	legacyUnaryRetirementRequestID = "request-retired-unary-replay"
	legacyUnaryRetirementWorkID    = "work-retired-unary-replay"
	legacyUnaryRetirementWorkName    = "replayed"
)

// TestLegacyUnaryRetirementReplaySubmitsCanonicalBatchWorkRequests proves
// recorded canonical FACTORY_REQUEST_BATCH submissions replay with stable public
// Work Request and Work identities through the public HTTP batch ingress path.
func TestLegacyUnaryRetirementReplaySubmitsCanonicalBatchWorkRequests(t *testing.T) {
	support.SkipLongFunctional(t, "slow legacy unary retirement batch replay smoke")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	artifactPath := filepath.Join(t.TempDir(), "retired-unary-smoke.replay.json")
	runner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("step one COMPLETE")},
		platformprocess.CommandResult{Stdout: []byte("step two COMPLETE")},
	)

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--record", artifactPath},
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
		},
	})

	workTypeName := batchInputsWorkType
	submitted := support.UpsertDefaultSessionWorkRequest(t, server.URL(), factoryapi.WorkRequest{
		RequestId: legacyUnaryRetirementRequestID,
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{{
			Name:         legacyUnaryRetirementWorkName,
			WorkId:       stringPtr(legacyUnaryRetirementWorkID),
			WorkTypeName: &workTypeName,
			Payload:      map[string]string{"title": "record replay canonical submit"},
		}},
	})
	if submitted.RequestId != legacyUnaryRetirementRequestID {
		t.Fatalf("PUT /work-requests requestId = %q, want %q", submitted.RequestId, legacyUnaryRetirementRequestID)
	}
	if len(submitted.Works) != 1 || submitted.Works[0].WorkId != legacyUnaryRetirementWorkID {
		t.Fatalf(
			"PUT /work-requests works = %#v, want one work with id %q",
			submitted.Works,
			legacyUnaryRetirementWorkID,
		)
	}

	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	server.Stop(t)

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	assertRecordedBatchWorkRequest(t, artifact, legacyUnaryRetirementRequestID, "external-submit", 1, 0)

	replayServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: t.TempDir(),
		Args:       []string{"--replay", artifactPath},
	})
	support.WaitForTerminalStatus(t, replayServer.URL(), 10*time.Second)
	support.AssertSingleWorkRequestEvent(
		t,
		replayServer.GetFactoryEvents(t),
		legacyUnaryRetirementRequestID,
		legacyUnaryRetirementWorkID,
		batchInputsWorkType,
	)
	replayServer.Stop(t)
}

func assertRecordedBatchWorkRequest(
	t *testing.T,
	artifact *interfaces.ReplayArtifact,
	requestID, source string,
	workItems, relations int,
) {
	t.Helper()

	for _, record := range recordedBatchWorkRequestEvents(t, artifact) {
		if record.RequestID != requestID {
			continue
		}
		if record.Source != source {
			t.Fatalf("work request %s source = %q, want %q", requestID, record.Source, source)
		}
		if got := len(support.FactoryWorksValue(record.Payload.Works)); got != workItems {
			t.Fatalf("work request %s work items = %d, want %d", requestID, got, workItems)
		}
		if got := len(support.FactoryRelationsValue(record.Payload.Relations)); got != relations {
			t.Fatalf("work request %s relations = %d, want %d", requestID, got, relations)
		}
		return
	}
	t.Fatalf("replay artifact missing work request %s: %#v", requestID, recordedBatchWorkRequestEvents(t, artifact))
}

type recordedBatchWorkRequestEvent struct {
	RequestID string
	Source    string
	Payload   factoryapi.WorkRequestEventPayload
}

func recordedBatchWorkRequestEvents(t *testing.T, artifact *interfaces.ReplayArtifact) []recordedBatchWorkRequestEvent {
	t.Helper()

	events := testutil.GeneratedFactoryEvents(t, artifact.Events)
	var out []recordedBatchWorkRequestEvent
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeWorkRequest {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil {
			t.Fatalf("decode work request event %q: %v", event.Id, err)
		}
		source := support.StringPointerValue(payload.Source)
		if source == "" {
			source = support.StringPointerValue(event.Context.Source)
		}
		out = append(out, recordedBatchWorkRequestEvent{
			RequestID: support.StringPointerValue(event.Context.RequestId),
			Source:    source,
			Payload:   payload,
		})
	}
	return out
}
