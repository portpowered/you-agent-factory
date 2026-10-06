package workersessions_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Captured fixtures enter at the recording external-effect boundary. Recovery
// observations use only customer CLI/HTTP/MCP reads; no provider files exist.
// Direct sessions have no Factory Session. Sequential hosts prove joined-store
// recovery here; the delivered OS-process restart belongs to T3-RUNTIME.
func runCapturedMetadataRecovery(t *testing.T, process support.Process) {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "captured-metadata-recovery")
	native := &deniedMetadataProviderFiles{}
	home := t.TempDir()
	var store recordings.WorkerRecordingStore
	config := support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderCommandRunner:               rejectLocalProvider{t: t},
			FactorySessionsWorkingDirectory:     historyWorkingDirectory(dir),
			ProviderSessionResolveHomeDirectory: func() (string, error) { return home, nil },
			ProviderSessionFileSystem:           native,
			WorkerRecordingStoreObserver:        func(value recordings.WorkerRecordingStore) { store = value },
		},
	}
	host := support.StartFunctionalAPIServer(t, config)
	for _, id := range []string{"updated", "zero", "absent"} {
		for _, record := range metadataCaptureRecords(t, id) {
			if err := store.PersistWorkerRecord(t.Context(), recordings.WorkerRecordingRecord{
				RecordingID: id + "-recording", WorkerSessionID: id, Record: record,
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	host.Close(t)
	writeLegacyMetadataCapture(t, dir)
	config.Edges.WorkerRecordingStoreObserver = nil
	reopened := support.StartFunctionalAPIServer(t, config)
	session, ctx := startMCP(t, process, reopened.URL())
	page := historyParityPage(t, ctx, session, reopened, "archived", "direct", "")
	rows := page["sessions"].([]any)
	if len(rows) != 4 {
		t.Fatalf("captured metadata membership: %v", page)
	}
	for _, value := range rows {
		observation := value.(map[string]any)
		id := observation["workerSessionId"].(string)
		assertMetadataCaptureFacts(t, id, observation)
		selected := getHost(t, reopened.URL()+"/worker-sessions/"+id)
		assertJSONEqual(t, observation, selected)
		assertRuntimeObservationParity(t, selected, callWorker(t, ctx, session, "read", map[string]any{"workerSessionId": id})["result"].(map[string]any)["session"])
		assertFactoryCLIParity(t, reopened, id, selected)
		if id != "legacy" {
			assertArchivedHostTiming(t, reopened, id)
		}
	}
	if native.calls.Load() != 0 {
		t.Fatal("captured metadata recovery attempted provider-native content reads")
	}
}

func assertMetadataCaptureFacts(t *testing.T, id string, observation map[string]any) {
	t.Helper()
	if observation["state"] != "COMPLETED" || observation["providerSessionAvailable"] != false || observation["recordingHealth"] != "COMPLETE" {
		t.Fatalf("captured metadata invented ownership/association or lost health: %v", observation)
	}
	for _, key := range []string{"workName", "workId", "factorySessionId"} {
		if observation[key] != nil {
			t.Fatalf("direct/legacy capture invented Work attribution: %v", observation)
		}
	}
	if id == "legacy" {
		for _, key := range []string{"provider", "model", "reasoningEffort", "tokenUsage", "startedAt", "endedAt", "durationMillis"} {
			if value := observation[key]; value != nil && value != "" {
				t.Fatalf("legacy capture invented %s: %v", key, observation)
			}
		}
		if observation["durationBasis"] != "UNAVAILABLE" {
			t.Fatalf("legacy timing basis: %v", observation)
		}
		return
	}
	if observation["provider"] != "codex" || observation["model"] != "reported-model" || observation["reasoningEffort"] != "high" {
		t.Fatalf("captured binding/model-only update lost: %v", observation)
	}
	if id == "absent" {
		if observation["tokenUsage"] != nil {
			t.Fatalf("model-only capture invented token counters: %v", observation)
		}
		return
	}
	want := map[string]any{"inputTokens": float64(8), "outputTokens": float64(12)}
	if id == "zero" {
		want = map[string]any{"inputTokens": float64(0), "outputTokens": float64(0), "totalTokens": float64(0)}
	}
	assertJSONEqual(t, observation["tokenUsage"], want)
}

func metadataCaptureRecords(t *testing.T, id string) []events.Record {
	t.Helper()
	opening := map[string]any{"status": "STARTING", "workerSessionId": id, "attemptId": id + "-attempt", "recordingId": id + "-recording"}
	if id != "legacy" {
		opening["startedAt"], opening["model"], opening["reasoningEffort"] = "2026-10-01T00:00:00Z", "selected-model", "high"
	}
	payloads := []map[string]any{opening}
	kinds, phases := []string{"SESSION"}, []string{"STARTED"}
	if id == "updated" || id == "zero" {
		usage := map[string]any{"provider": "codex", "model": "counter-model", "inputTokens": 8, "outputTokens": 12}
		if id == "zero" {
			usage["inputTokens"], usage["outputTokens"], usage["totalTokens"] = 0, 0, 0
		}
		payloads = append(payloads, usage)
		kinds, phases = append(kinds, "USAGE"), append(phases, "UPDATED")
	}
	if id != "legacy" {
		payloads = append(payloads, map[string]any{"provider": "codex", "model": "reported-model"})
		kinds, phases = append(kinds, "USAGE"), append(phases, "UPDATED")
	}
	payloads = append(payloads, map[string]any{"status": "COMPLETED", "workerSessionId": id, "attemptId": id + "-attempt"})
	kinds, phases = append(kinds, "SESSION"), append(phases, "COMPLETED")
	var records []events.Record
	for index, payload := range payloads {
		provenance := map[string]any{"delivery": "SYNTHESIZED", "fidelity": "LIFECYCLE_ONLY", "nativeEventType": "worker_session_lifecycle", "representation": "NOTIFICATION"}
		if id != "legacy" {
			provenance["provider"] = "codex"
		}
		data, err := json.Marshal(map[string]any{"kind": kinds[index], "phase": phases[index], "dispatchId": id + "-attempt", "provenance": provenance, "payload": payload})
		if err != nil {
			t.Fatal(err)
		}
		position := events.AggregateSequence(index + 1)
		sourceSequence := events.SourceSequence(position)
		eventID := events.SourceEventID(id + "-" + phases[index])
		if index == 0 {
			eventID = "started"
		}
		if phases[index] == "COMPLETED" {
			eventID, sourceSequence = "terminal", 2
		}
		records = append(records, events.Record{
			ID:         events.RecordID{Topic: events.Topic("worker-session/" + id + "/events"), Position: position},
			SourceType: "worker_session_lifecycle", SourceID: events.SourceID(id), SourceSequence: sourceSequence,
			SourceEventID: eventID, SchemaID: "workers.draft.v1", Payload: data,
		})
	}
	return records
}

func writeLegacyMetadataCapture(t *testing.T, dir string) {
	t.Helper()
	data, err := json.Marshal(recordings.WorkerRecordingSnapshot{
		RecordingID: "legacy-recording", Sessions: []recordings.WorkerSessionRecordingSnapshot{
			{WorkerSessionID: "legacy", Records: metadataCaptureRecords(t, "legacy")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("legacy-recording"))
	path := filepath.Join(dir, ".you-agent-factory", "worker-recordings", hex.EncodeToString(digest[:])+".worker.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
