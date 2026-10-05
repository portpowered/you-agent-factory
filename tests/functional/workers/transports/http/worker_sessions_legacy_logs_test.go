package http_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Prepare the documented legacy snapshot format from the captured public
// records, without commit metadata. Assertions use only public reads. Each
// host is stopped before the next root opens its store; no writer is shared.
func assertLegacyCapturedLogsRecovery(t *testing.T, config support.FunctionalAPIServerConfig, current factoryapi.WorkerSessionLogPage) {
	t.Helper()
	legacyRoot := t.TempDir()
	writeLegacyCapturedFixture(t, legacyRoot, current)
	config.Edges.FactorySessionsWorkingDirectory = capturedRecordingDirectory(legacyRoot)
	server := support.StartFunctionalAPIServer(t, config)
	legacy := assertLegacyLogsCLIHTTPParity(t, server, current.WorkerSessionId)
	assertLegacyCapturedEvents(t, legacy, current)
	assertCapturedSummaryUsage(t, server, current.WorkerSessionId)
	shown := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/"+current.WorkerSessionId)
	if shown.State != factoryapi.WorkerSessionObservationStateCompleted || shown.ProviderSession != nil {
		t.Fatalf("legacy history restored live ownership or provider association: %+v", shown)
	}
	server.Close(t)
	reopened := support.StartFunctionalAPIServer(t, config)
	again := assertLegacyLogsCLIHTTPParity(t, reopened, current.WorkerSessionId)
	if !reflect.DeepEqual(again, legacy) {
		t.Fatal("legacy recovery changed generation, payload, time omission or watermark")
	}
	assertCapturedSummaryUsage(t, reopened, current.WorkerSessionId)
}

func assertLegacyLogsCLIHTTPParity(t *testing.T, server *support.FunctionalAPIServer, id string) factoryapi.WorkerSessionLogPage {
	t.Helper()
	page := support.GetJSON[factoryapi.WorkerSessionLogPage](t, server.URL()+"/worker-sessions/"+url.PathEscape(id)+"/logs")
	inputs := support.FakeInputs(t.Context(), []string{"you", "worker-sessions", "read", "--worker-session-id", id, "--view", "logs", "--server", server.URL(), "--output", "json"})
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("CLI legacy logs: %v %s", err, inputs.Stderr())
	}
	var cli factoryapi.WorkerSessionLogPage
	if err := json.Unmarshal([]byte(inputs.Stdout()), &cli); err != nil || !reflect.DeepEqual(cli, page) {
		t.Fatalf("legacy CLI/HTTP page differs: error=%v", err)
	}
	return page
}

func assertLegacyCapturedEvents(t *testing.T, legacy, current factoryapi.WorkerSessionLogPage) {
	t.Helper()
	if legacy.RecordingGenerationId == "" || legacy.CommittedPosition != current.CommittedPosition || len(legacy.Events) != len(current.Events) {
		t.Fatal("legacy recovery lost the captured identity or ordered prefix")
	}
	for index, frame := range legacy.Events {
		want := current.Events[index]
		want.Event.CapturedAt = nil
		if frame.Event.CapturedAt != nil || !reflect.DeepEqual(frame, want) {
			t.Fatalf("legacy record %d changed payload or invented capture time", index)
		}
	}
}

func writeLegacyCapturedFixture(t *testing.T, root string, page factoryapi.WorkerSessionLogPage) {
	t.Helper()
	var records []events.Record
	for _, frame := range page.Events {
		payload, err := json.Marshal(frame.Event.Payload)
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, events.Record{
			ID:         events.RecordID{Topic: events.Topic("worker-session/" + page.WorkerSessionId + "/events"), Position: events.AggregateSequence(frame.Event.Position)},
			SourceType: events.SourceType(frame.Event.SourceType), SourceID: events.SourceID(frame.Event.SourceId),
			SourceSequence: events.SourceSequence(frame.Event.SourceSequence), SourceEventID: events.SourceEventID(frame.Event.SourceEventId),
			SchemaID: events.SchemaID(frame.Event.SchemaId), Payload: payload,
		})
	}
	var draft workers.Draft
	var opening workers.SessionPayload
	if json.Unmarshal(records[0].Payload, &draft) != nil || json.Unmarshal(draft.Payload, &opening) != nil || opening.RecordingID == "" {
		t.Fatal("captured opening has no recording identity")
	}
	snapshot := recordings.WorkerRecordingSnapshot{
		RecordingID: opening.RecordingID,
		Sessions:    []recordings.WorkerSessionRecordingSnapshot{{WorkerSessionID: page.WorkerSessionId, Records: records}},
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".you-agent-factory", "worker-recordings")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(opening.RecordingID))
	if err := os.WriteFile(filepath.Join(dir, hex.EncodeToString(digest[:])+".worker.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
