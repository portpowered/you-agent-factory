package artifacts_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/events"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recording "github.com/portpowered/infinite-you/pkg/services/recordings/internal/artifacts"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestContractFixtures(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"valid-v3-worker-history.json", "valid-v2.json", "valid-v2-checkpoint.json", "valid-v1.json"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := loadFixture(name)
			if err != nil {
				t.Fatalf("DecodeAndValidate() error = %v", err)
			}
			if got.Session.ID == "" {
				t.Fatal("validated fixture has empty session identity")
			}
		})
	}
}

func TestVersionPinnedSchemaV1FixturePreservesFactorySessionFacts(t *testing.T) {
	t.Parallel()
	assertVersionPinnedFixture(t, "valid-v1.json", versionPinnedFixtureExpectation{
		schemaVersion: "1", sessionID: "session-historical-001", sourceRef: "workflow/historical.js",
		eventIDs: []string{"event-historical-1"},
	})
}

func TestVersionPinnedSchemaV2FixturePreservesFactorySessionFacts(t *testing.T) {
	t.Parallel()
	assertVersionPinnedFixture(t, "valid-v2.json", versionPinnedFixtureExpectation{
		schemaVersion: "2", sessionID: "session-js-001", sourceRef: "workflow/example.js",
		eventIDs: []string{"event-1", "event-2"}, artifactCount: 1, result: true,
	})
}

func TestVersionPinnedSchemaV2CheckpointFixturePreservesFactorySessionFacts(t *testing.T) {
	t.Parallel()
	assertVersionPinnedFixture(t, "valid-v2-checkpoint.json", versionPinnedFixtureExpectation{
		schemaVersion: "2", sessionID: "session-js-checkpoint-001", sourceRef: "workflow/checkpoint.js",
		eventIDs: []string{"event-started", "event-checkpoint", "event-completed"}, artifactCount: 1, result: true, checkpoint: true,
	})
}

func TestVersionPinnedSchemaV3FixturePreservesFactorySessionAndWorkerFacts(t *testing.T) {
	t.Parallel()
	value, err := loadFixture("valid-v3-worker-history.json")
	if err != nil {
		t.Fatalf("DecodeAndValidate() error = %v", err)
	}
	assertCurrentFixtureFactoryFacts(t, value)
	assertCurrentFixtureWorkerFacts(t, value)
	assertVersionPinnedJSONRoundTrip(t, value)
}

func assertCurrentFixtureFactoryFacts(t *testing.T, value recordings.PortableRecording) {
	t.Helper()
	if value.SchemaVersion != recordings.PortableRecordingSchemaV3 ||
		value.ReplayCompatibilityVersion != recordings.PortableRecordingReplayCompatibilityV1 {
		t.Fatalf("compatibility = %q/%q", value.SchemaVersion, value.ReplayCompatibilityVersion)
	}
	if value.Session.ID != "session-current-worker-001" || value.Source.Ref != "workflow/current-worker.js" ||
		len(value.Events) != 2 || value.Events[0].Sequence != 0 || value.Events[1].Sequence != 1 {
		t.Fatalf("Factory Session facts = %#v", value)
	}
}

func assertCurrentFixtureWorkerFacts(t *testing.T, value recordings.PortableRecording) {
	t.Helper()
	history := value.WorkerHistory
	if history == nil || history.Availability != recordings.PortableRecordingWorkerHistoryAvailable ||
		history.WorkerPortableRecording == nil || len(history.Records) != 3 {
		t.Fatalf("Worker history = %#v, want available ordered history", history)
	}
	if history.Lifecycle.Terminal == nil || history.Lifecycle.Terminal.Status != "COMPLETED" ||
		history.Correlation.FactorySessionID != value.Session.ID || history.Correlation.DispatchID != "dispatch-current-001" {
		t.Fatalf("Worker lifecycle/correlation = %#v", history)
	}
	if history.Records[1].Provenance.Fidelity != workers.FidelityNormalized ||
		history.Records[1].Provenance.Delivery != workers.DeliveryNativeStream {
		t.Fatalf("Worker fidelity facts = %#v", history.Records[1])
	}
}

func TestPortableRecordingDecodeReportsWorkerHistoryFutureFields(t *testing.T) {
	value, err := loadFixture("valid-v3-worker-history.json")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	document["futureTopLevel"] = json.RawMessage(`true`)
	var history map[string]json.RawMessage
	if err := json.Unmarshal(document["workerHistory"], &history); err != nil {
		t.Fatal(err)
	}
	history["futureWorkerField"] = json.RawMessage(`"ignored"`)
	document["workerHistory"], err = json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	payload, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}

	decoded, diagnostics, err := recordings.DecodePortableRecordingWithDiagnostics(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("DecodePortableRecordingWithDiagnostics() error = %v", err)
	}
	if !reflect.DeepEqual(diagnostics.Paths(), []string{
		"$.futureTopLevel", "$.workerHistory.futureWorkerField",
	}) {
		t.Fatalf("ignored paths = %#v", diagnostics.Paths())
	}
	wantKnown, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	gotKnown, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotKnown, wantKnown) {
		t.Fatalf("known Factory and Worker facts changed after additive fields")
	}
}

func TestLegacyFixturesNormalizeWorkerHistoryAsUnavailable(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"valid-v1.json", "valid-v2.json", "valid-v2-checkpoint.json"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			value, err := loadFixture(name)
			if err != nil {
				t.Fatalf("DecodeAndValidate() error = %v", err)
			}
			got := recordings.NormalizePortableRecordingWorkerHistory(value)
			if got.Availability != recordings.PortableRecordingWorkerHistoryUnavailable ||
				got.Reason != recordings.PortableRecordingWorkerHistoryReasonLegacySchema {
				t.Fatalf("Worker history = %#v, want unavailable legacy outcome", got)
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("Marshal Worker history: %v", err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatalf("decode Worker history projection: %v", err)
			}
			if len(fields) != 2 {
				t.Fatalf("legacy Worker history fields = %#v, want availability and reason only", fields)
			}
		})
	}
}

func TestCurrentBuildDeclaresWorkerHistoryCapabilityAndOldReaderRejectsIt(t *testing.T) {
	t.Parallel()
	facts := recordings.PortableRecordingCanonicalFacts{
		SessionID:        "session-current-001",
		Status:           "COMPLETED",
		OrchestratorKind: "JAVASCRIPT",
		SourceRef:        "workflow/current.js",
		SourceHash:       digestForTest('1'),
		PolicyHash:       digestForTest('2'),
		Events: []json.RawMessage{
			json.RawMessage(`{"id":"event-started","type":"SESSION_STARTED","context":{"sequence":0,"eventTime":"2026-08-12T12:00:00Z"},"payload":{}}`),
			json.RawMessage(`{"id":"event-completed","type":"SESSION_COMPLETED","context":{"sequence":1,"eventTime":"2026-08-12T12:00:01Z"},"payload":{}}`),
		},
		Result: &recordings.PortableRecordingCanonicalResult{Status: "FINAL", Mode: "final"},
	}
	value, err := recordings.BuildPortableRecording(facts)
	if err != nil {
		t.Fatalf("BuildPortableRecording() error = %v", err)
	}
	assertCurrentUnavailableRecording(t, value)
	payload := marshalPortableRecording(t, value)
	assertCurrentPortableRoundTrip(t, value, payload)
	assertOldReaderRejectsCurrentRecording(t, payload)
}

func assertCurrentUnavailableRecording(t *testing.T, value recordings.PortableRecording) {
	t.Helper()
	if value.SchemaVersion != recordings.PortableRecordingSchemaV3 || value.ReplayCompatibilityVersion != recordings.PortableRecordingReplayCompatibilityV1 {
		t.Fatalf("compatibility = %q/%q", value.SchemaVersion, value.ReplayCompatibilityVersion)
	}
	if value.WorkerHistory == nil || value.WorkerHistory.Availability != recordings.PortableRecordingWorkerHistoryUnavailable || value.WorkerHistory.Reason != recordings.PortableRecordingWorkerHistoryReasonNotCaptured {
		t.Fatalf("Worker history = %#v, want explicit current unavailable outcome", value.WorkerHistory)
	}
}

func marshalPortableRecording(t *testing.T, value recordings.PortableRecording) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return payload
}

func assertCurrentPortableRoundTrip(t *testing.T, value recordings.PortableRecording, payload []byte) {
	t.Helper()
	decoded, err := recordings.DecodePortableRecording(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("current DecodePortableRecording() error = %v", err)
	}
	if !reflect.DeepEqual(decoded, value) {
		t.Fatalf("current round trip changed facts:\nwant=%#v\ngot=%#v", value, decoded)
	}
}

func assertOldReaderRejectsCurrentRecording(t *testing.T, payload []byte) {
	t.Helper()
	oldReader := recordings.NewPortableRecordingCodec(
		[]string{recordings.PortableRecordingSchemaV1, recordings.PortableRecordingSchemaV2},
		[]string{recordings.PortableRecordingReplayCompatibilityV1},
	)
	got, err := oldReader.Decode(bytes.NewReader(payload))
	if !reflect.DeepEqual(got, recordings.PortableRecording{}) {
		t.Fatalf("old-reader result = %#v, want no partial recording", got)
	}
	var diagnostic *recordings.PortableRecordingDiagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("old-reader error = %T %v, want typed diagnostic", err, err)
	}
	if diagnostic.Code != recordings.PortableRecordingCodeUnsupportedSchema || diagnostic.Path != "schemaVersion" || diagnostic.EncounteredVersion != recordings.PortableRecordingSchemaV3 || diagnostic.Action != recordings.PortableRecordingCompatibilityAction {
		t.Fatalf("old-reader diagnostic = %#v", diagnostic)
	}
	if !reflect.DeepEqual(diagnostic.SupportedVersions, []string{recordings.PortableRecordingSchemaV1, recordings.PortableRecordingSchemaV2}) {
		t.Fatalf("old-reader supported versions = %#v", diagnostic.SupportedVersions)
	}
}

func TestUnsupportedReplayCompatibilityHasSeparateTypedDiagnostic(t *testing.T) {
	t.Parallel()
	value, err := loadFixture("valid-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	document["replayCompatibilityVersion"] = json.RawMessage(`"99"`)
	payload, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	_, err = recordings.DecodePortableRecording(bytes.NewReader(payload))
	var diagnostic *recordings.PortableRecordingDiagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error = %T %v, want typed diagnostic", err, err)
	}
	if diagnostic.Code != recordings.PortableRecordingCodeUnsupportedVersion || diagnostic.Path != "replayCompatibilityVersion" || diagnostic.EncounteredVersion != "99" || diagnostic.Action != recordings.PortableRecordingCompatibilityAction {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestCurrentFixturePreservesAvailableWorkerHistoryFacts(t *testing.T) {
	t.Parallel()
	factorySessionID := "session-current-worker-001"
	history := availableWorkerHistory(t, factorySessionID)
	facts := recordings.PortableRecordingCanonicalFacts{
		SessionID:        factorySessionID,
		Status:           "COMPLETED",
		OrchestratorKind: "JAVASCRIPT",
		SourceRef:        "workflow/current-worker.js",
		SourceHash:       digestForTest('1'),
		PolicyHash:       digestForTest('2'),
		Events: []json.RawMessage{
			json.RawMessage(`{"id":"event-started","type":"SESSION_STARTED","context":{"sequence":0,"eventTime":"2026-08-12T13:00:00Z"},"payload":{}}`),
			json.RawMessage(`{"id":"event-completed","type":"SESSION_COMPLETED","context":{"sequence":1,"eventTime":"2026-08-12T13:00:01Z"},"payload":{}}`),
		},
		Result:        &recordings.PortableRecordingCanonicalResult{Status: "FINAL", Mode: "final"},
		WorkerHistory: history,
	}
	value, err := recordings.BuildPortableRecording(facts)
	if err != nil {
		t.Fatalf("BuildPortableRecording() error = %v", err)
	}
	if value.WorkerHistory == nil || value.WorkerHistory.Availability != recordings.PortableRecordingWorkerHistoryAvailable || value.WorkerHistory.WorkerPortableRecording == nil {
		t.Fatalf("Worker history = %#v, want available detached recording", value.WorkerHistory)
	}
	if len(value.WorkerHistory.Records) != 3 || value.WorkerHistory.Lifecycle.Terminal == nil || value.WorkerHistory.Correlation.FactorySessionID != factorySessionID {
		t.Fatalf("Worker history facts = %#v, want ordered records/lifecycle/correlation", value.WorkerHistory)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	decoded, err := recordings.DecodePortableRecording(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("DecodePortableRecording() error = %v", err)
	}
	if decoded.WorkerHistory == nil || len(decoded.WorkerHistory.Records) != 3 || decoded.WorkerHistory.Records[1].Provenance.Fidelity != workers.FidelityNormalized {
		t.Fatalf("decoded Worker history = %#v, want ordered fidelity facts", decoded.WorkerHistory)
	}
	if !reflect.DeepEqual(recordings.NormalizePortableRecordingWorkerHistory(value), recordings.NormalizePortableRecordingWorkerHistory(decoded)) {
		t.Fatalf("normalized Worker history changed across round trip")
	}
}

func availableWorkerHistory(t *testing.T, factorySessionID string) *recordings.PortableRecordingWorkerHistory {
	t.Helper()
	workerSessionID := "worker-session-current-001"
	topic := events.Topic("worker-session/" + workerSessionID + "/events")
	startedAt := time.Date(2026, 8, 12, 13, 0, 0, 0, time.UTC)
	openingPayload := workers.SessionPayload{
		Status: "STARTING", StartedAt: &startedAt, WorkerSessionID: workerSessionID,
		FactorySessionID: factorySessionID, RecordingID: "worker-recording-current-001",
		DispatchID: "dispatch-current-001", TurnID: "turn-current-001", TraceID: "trace-current-001",
		WorkIDs: []string{"work-current-001"}, AttemptID: "attempt-current-001", Attempt: 1,
		AttemptReason:     workers.AttemptReasonInitial,
		ProviderSelection: &workers.SessionProviderSelection{RunnerID: "codex"},
	}
	opening := workerHistoryRecord(t, topic, 1, "worker_session_lifecycle", workerSessionID, 1, "started", workers.Draft{
		Kind: workers.KindSession, Phase: workers.PhaseStarted,
		Provenance: workerLifecycleProvenance("codex"), Payload: mustJSON(t, openingPayload),
		DispatchID: openingPayload.DispatchID, TurnID: openingPayload.TurnID,
	})
	message := workerHistoryRecord(t, topic, 2, "worker_observation", workerSessionID, 1, "worker/1", workers.Draft{
		Kind: workers.KindMessage, Phase: workers.PhaseCompleted,
		Provenance: workers.Provenance{
			Delivery: workers.DeliveryNativeStream, Fidelity: workers.FidelityNormalized,
			NativeEventType: "message.completed", Provider: "codex", Representation: workers.RepresentationSnapshot,
		}, Payload: mustJSON(t, workers.MessagePayload{Role: "assistant", ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: "done"}}}),
		DispatchID: openingPayload.DispatchID, TurnID: openingPayload.TurnID, ItemID: "message-current-001",
	})
	terminal := workerHistoryRecord(t, topic, 3, "worker_session_lifecycle", workerSessionID, 2, "terminal", workers.Draft{
		Kind: workers.KindSession, Phase: workers.PhaseCompleted,
		Provenance: workerLifecycleProvenance("codex"), Payload: mustJSON(t, map[string]string{"status": "COMPLETED"}),
		DispatchID: openingPayload.DispatchID,
	})
	snapshot := recordings.WorkerRecordingSnapshot{
		RecordingID: "worker-recording-current-001",
		Sessions: []recordings.WorkerSessionRecordingSnapshot{{
			WorkerSessionID: workerSessionID, Topic: topic, Status: recordings.WorkerRecordingStatusCompleted,
			LastPosition: 3, Records: []events.Record{opening, message, terminal},
		}},
	}
	portable, err := (recordings.WorkerRecordingCodec{}).BuildWorkerPortableRecording(snapshot)
	if err != nil {
		t.Fatalf("BuildWorkerPortableRecording() error = %v", err)
	}
	return &recordings.PortableRecordingWorkerHistory{
		Availability:            recordings.PortableRecordingWorkerHistoryAvailable,
		WorkerPortableRecording: &portable,
	}
}

func workerHistoryRecord(t *testing.T, topic events.Topic, position events.AggregateSequence, sourceType, sourceID string, sourceSequence events.SourceSequence, sourceEventID string, draft workers.Draft) events.Record {
	t.Helper()
	return events.Record{
		ID: events.RecordID{Topic: topic, Position: position}, SourceType: events.SourceType(sourceType), SourceID: events.SourceID(sourceID),
		SourceSequence: sourceSequence, SourceEventID: events.SourceEventID(sourceEventID), SchemaID: "workers.draft.v1", Payload: mustJSON(t, draft),
	}
}

func workerLifecycleProvenance(provider string) workers.Provenance {
	return workers.Provenance{
		Delivery: workers.DeliverySynthesized, Fidelity: workers.FidelityLifecycleOnly,
		NativeEventType: "worker_session_lifecycle", Provider: provider, Representation: workers.RepresentationNotification,
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

type versionPinnedFixtureExpectation struct {
	schemaVersion string
	sessionID     string
	sourceRef     string
	eventIDs      []string
	artifactCount int
	result        bool
	checkpoint    bool
}

func assertVersionPinnedFixture(t *testing.T, name string, want versionPinnedFixtureExpectation) {
	t.Helper()
	value, err := loadFixture(name)
	if err != nil {
		t.Fatalf("DecodeAndValidate() error = %v", err)
	}
	if value.SchemaVersion != want.schemaVersion || value.ReplayCompatibilityVersion != recording.ReplayCompatibilityVersion {
		t.Fatalf("compatibility = %q/%q", value.SchemaVersion, value.ReplayCompatibilityVersion)
	}
	if value.Session.ID != want.sessionID || value.Source.Ref != want.sourceRef {
		t.Fatalf("identity/source = %#v/%#v", value.Session, value.Source)
	}
	if len(value.Events) != len(want.eventIDs) || len(value.Artifacts) != want.artifactCount {
		t.Fatalf("summary counts = events:%d artifacts:%d", len(value.Events), len(value.Artifacts))
	}
	for index, eventID := range want.eventIDs {
		if value.Events[index].ID != eventID || value.Events[index].Sequence != int64(index) {
			t.Fatalf("event[%d] = %#v, want %q at sequence %d", index, value.Events[index], eventID, index)
		}
	}
	if (value.Result != nil) != want.result || (value.Checkpoint != nil) != want.checkpoint {
		t.Fatalf("versioned optional facts = result:%t checkpoint:%t", value.Result != nil, value.Checkpoint != nil)
	}
	if value.Redaction != (recording.RedactionMetadata{
		RuntimeStateOmitted: true, CheckpointBodiesOmitted: true,
		ProviderTranscriptsOmitted: true, ChildDispatchesOmitted: true,
		SecretsRedacted: value.Redaction.SecretsRedacted,
	}) {
		t.Fatalf("redaction metadata = %#v", value.Redaction)
	}
	assertVersionPinnedRoundTrip(t, value)
}

func assertVersionPinnedRoundTrip(t *testing.T, value recording.Recording) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	decoded, err := recording.DecodeAndValidate(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("round-trip DecodeAndValidate() error = %v", err)
	}
	if !reflect.DeepEqual(value, decoded) {
		t.Fatalf("round-trip changed versioned facts:\nwant=%#v\ngot=%#v", value, decoded)
	}
}

func assertVersionPinnedJSONRoundTrip(t *testing.T, value recording.Recording) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	decoded, err := recording.DecodeAndValidate(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("round-trip DecodeAndValidate() error = %v", err)
	}
	reencoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("re-marshal() error = %v", err)
	}
	if !bytes.Equal(encoded, reencoded) {
		t.Fatalf("round-trip changed canonical JSON:\nwant=%s\ngot=%s", encoded, reencoded)
	}
}

func TestSupportedSchemaCorruptionKeepsItsOwnedDiagnostic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		code   recording.DiagnosticCode
		path   string
		mutate func(*recording.Recording)
	}{
		{name: "identity", code: recording.CodeInvalidIdentity, path: "session.id", mutate: func(value *recording.Recording) { value.Session.ID = "" }},
		{name: "digest", code: recording.CodeInvalidDigest, path: "source.hash", mutate: func(value *recording.Recording) { value.Source.Hash = "sha256:not-a-digest" }},
		{name: "order", code: recording.CodeInvalidOrder, path: "events[1].sequence", mutate: func(value *recording.Recording) { value.Events[1].Sequence = value.Events[0].Sequence }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			value, err := loadFixture("valid-v2.json")
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(&value)
			payload, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			_, err = recording.DecodeAndValidate(bytes.NewReader(payload))
			assertDiagnostic(t, err, test.code, test.path)
		})
	}

	value, err := loadFixture("valid-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	document["unsupportedField"] = json.RawMessage(`true`)
	payload, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	_, diagnostics, err := recordings.DecodePortableRecordingWithDiagnostics(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("DecodePortableRecordingWithDiagnostics() error = %v, want additive field accepted", err)
	}
	if !reflect.DeepEqual(diagnostics.Paths(), []string{"$.unsupportedField"}) {
		t.Fatalf("ignored paths = %#v, want unsupportedField path", diagnostics.Paths())
	}
}

func TestMalformedFixtureReturnsTypedAreaDiagnostic(t *testing.T) {
	_, err := loadFixture("malformed.json")
	var diagnostic *recording.Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error = %T %v, want *Diagnostic", err, err)
	}
	if diagnostic.Code != recording.CodeInvalidDigest || diagnostic.Area != "digest" || diagnostic.Path != "source.hash" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestUnsupportedCompatibilityIsRejectedBeforeMalformedBody(t *testing.T) {
	_, err := loadFixture("unsupported.json")
	var diagnostic *recording.Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error = %T %v, want *Diagnostic", err, err)
	}
	if diagnostic.Code != recording.CodeUnsupportedVersion || diagnostic.Area != "compatibility" || diagnostic.Path != "replayCompatibilityVersion" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	if len(diagnostic.SupportedVersions) != 1 || diagnostic.SupportedVersions[0] != recording.ReplayCompatibilityVersion {
		t.Fatalf("supported versions = %#v", diagnostic.SupportedVersions)
	}
}

func TestMissingVersionHeaderRemainsMalformed(t *testing.T) {
	t.Parallel()
	value, err := loadFixture("valid-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	delete(document, "schemaVersion")
	payload, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	_, err = recordings.DecodePortableRecording(bytes.NewReader(payload))
	assertDiagnostic(t, err, recording.CodeMalformedContract, "schemaVersion")
}

func TestValidationRejectsEventOrderingAndUnknownArtifactReferences(t *testing.T) {
	valid, err := loadFixture("valid-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	valid.Events[1].Sequence = valid.Events[0].Sequence
	assertDiagnostic(t, recording.Validate(valid), recording.CodeInvalidOrder, "events[1].sequence")
	valid, _ = loadFixture("valid-v2.json")
	valid.Events[1].ArtifactIDs[0] = "missing"
	assertDiagnostic(t, recording.Validate(valid), recording.CodeInvalidSummary, "events[1].artifactIds[0]")
}

func TestDecodeAcceptsUnknownDetailAndReportsItsPath(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "valid-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	data = append(data[:len(data)-2], []byte(",\n  \"runtimeState\": {\"secret\": true}\n}\n")...)
	_, diagnostics, err := recordings.DecodePortableRecordingWithDiagnostics(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("DecodePortableRecordingWithDiagnostics() error = %v, want additive field accepted", err)
	}
	if !reflect.DeepEqual(diagnostics.Paths(), []string{"$.runtimeState"}) {
		t.Fatalf("ignored paths = %#v, want runtimeState path", diagnostics.Paths())
	}
}

func TestValidationRejectsMalformedPublicSummaries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		code   recording.DiagnosticCode
		path   string
		mutate func(*recording.Recording)
	}{
		{"missing session id", recording.CodeInvalidIdentity, "session.id", func(r *recording.Recording) { r.Session.ID = "" }},
		{"unknown session status", recording.CodeInvalidSummary, "session.status", func(r *recording.Recording) { r.Session.Status = "UNKNOWN" }},
		{"wrong orchestrator", recording.CodeInvalidSummary, "session.orchestratorKind", func(r *recording.Recording) { r.Session.OrchestratorKind = "GO" }},
		{"missing source ref", recording.CodeInvalidSummary, "source.ref", func(r *recording.Recording) { r.Source.Ref = "" }},
		{"incomplete artifact", recording.CodeInvalidSummary, "artifacts[0]", func(r *recording.Recording) { r.Artifacts[0].Kind = "" }},
		{"duplicate artifact", recording.CodeInvalidIdentity, "artifacts[1].id", func(r *recording.Recording) { r.Artifacts = append(r.Artifacts, r.Artifacts[0]) }},
		{"artifact digest", recording.CodeInvalidDigest, "artifacts[0].contentHash", func(r *recording.Recording) { r.Artifacts[0].ContentHash = "invalid" }},
		{"negative artifact size", recording.CodeInvalidSummary, "artifacts[0].sizeBytes", func(r *recording.Recording) { r.Artifacts[0].SizeBytes = -1 }},
		{"incomplete event", recording.CodeInvalidSummary, "events[0]", func(r *recording.Recording) { r.Events[0].Type = "" }},
		{"duplicate event", recording.CodeInvalidIdentity, "events[1].id", func(r *recording.Recording) { r.Events[1].ID = r.Events[0].ID }},
		{"unsupported result status", recording.CodeInvalidSummary, "result.status", func(r *recording.Recording) { r.Result.Status = "UNKNOWN" }},
		{"unsupported result mode", recording.CodeInvalidSummary, "result.mode", func(r *recording.Recording) { r.Result.Mode = "unknown" }},
		{"unknown result artifact", recording.CodeInvalidSummary, "result.artifactIds[0]", func(r *recording.Recording) { r.Result.ArtifactIDs[0] = "missing" }},
		{"hash without result", recording.CodeInvalidDigest, "result.contentHash", func(r *recording.Recording) { r.Result.ContentHash = digestForTest('5') }},
		{"invalid inline result", recording.CodeInvalidSummary, "result.primaryResult", func(r *recording.Recording) { r.Result.PrimaryResult = []byte(`{`) }},
		{"failed final result", recording.CodeInvalidSummary, "result.status", func(r *recording.Recording) { r.Session.Status = "FAILED" }},
		{"omission flag false", recording.CodeInvalidSummary, "redaction", func(r *recording.Recording) { r.Redaction.RuntimeStateOmitted = false }},
		{"negative redaction count", recording.CodeInvalidSummary, "redaction.secretsRedacted", func(r *recording.Recording) { r.Redaction.SecretsRedacted = -1 }},
		{"redaction count above maximum", recording.CodeInvalidSummary, "redaction.secretsRedacted", func(r *recording.Recording) { r.Redaction.SecretsRedacted = recording.MaxSecretsRedacted + 1 }},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			value, err := loadFixture("valid-v2.json")
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(&value)
			assertDiagnostic(t, recording.Validate(value), tc.code, tc.path)
		})
	}
}

func TestValidationRejectsMalformedCheckpointSummaries(t *testing.T) {
	valid, err := loadFixture("valid-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	valid.Checkpoint = &recording.CheckpointSummary{ID: "checkpoint-1", Timestamp: valid.Events[0].Timestamp}
	assertDiagnostic(t, recording.Validate(valid), recording.CodeInvalidSummary, "checkpoint.id")

	valid.Events[0].CheckpointID = "checkpoint-1"
	valid.Checkpoint.ArtifactID = "missing"
	assertDiagnostic(t, recording.Validate(valid), recording.CodeInvalidSummary, "checkpoint.artifactId")

	valid.Checkpoint.ArtifactID = "artifact-1"
	valid.Checkpoint.ID = ""
	assertDiagnostic(t, recording.Validate(valid), recording.CodeInvalidSummary, "checkpoint")
}

func TestDiagnosticErrorIncludesPathWhenPresent(t *testing.T) {
	if got := (*recording.Diagnostic)(nil).Error(); got != "" {
		t.Fatalf("nil diagnostic error = %q", got)
	}
	withoutPath := &recording.Diagnostic{Message: "message"}
	if got := withoutPath.Error(); got != "message" {
		t.Fatalf("diagnostic error = %q", got)
	}
	withPath := &recording.Diagnostic{Path: "session.id", Message: "is required"}
	if got := withPath.Error(); got != "session.id: is required" {
		t.Fatalf("diagnostic error = %q", got)
	}
}

func digestForTest(character byte) string {
	return "sha256:" + string(bytes.Repeat([]byte{character}, 64))
}

func loadFixture(name string) (recording.Recording, error) {
	file, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		return recording.Recording{}, err
	}
	defer file.Close()
	return recording.DecodeAndValidate(file)
}

func assertDiagnostic(t *testing.T, err error, code recording.DiagnosticCode, path string) {
	t.Helper()
	var diagnostic *recording.Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error = %T %v, want *Diagnostic", err, err)
	}
	if diagnostic.Code != code || diagnostic.Path != path {
		t.Fatalf("diagnostic = %#v, want code %s path %q", diagnostic, code, path)
	}
}
