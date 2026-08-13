package worker_capture

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

var workerRecordingCodec = WorkerRecordingCodec{}

func TestWorkerPortableRecordingRoundTripPreservesFidelityMatrix(t *testing.T) {
	tests := []struct {
		name      string
		fidelity  string
		wantCount int
	}{
		{name: "streaming", fidelity: "streaming", wantCount: 4},
		{name: "snapshot", fidelity: "snapshot", wantCount: 3},
		{name: "final-only", fidelity: "final-only", wantCount: 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := portableSnapshot(t, test.fidelity, "codex", "codex")
			portable, err := workerRecordingCodec.BuildWorkerPortableRecording(snapshot)
			if err != nil {
				t.Fatalf("BuildWorkerPortableRecording() error = %v", err)
			}
			if err := workerRecordingCodec.ValidateWorkerPortableRecording(portable); err != nil {
				t.Fatalf("ValidateWorkerPortableRecording() error = %v", err)
			}
			if portable.Provider.Provider != "codex" || portable.Provider.ProviderSessionRef != "" {
				t.Fatalf("provider attribution = %#v, want codex without fabricated Provider Session reference", portable.Provider)
			}
			if len(portable.Records) != test.wantCount || portable.Lifecycle.Terminal == nil {
				t.Fatalf("portable record count/terminal = %d/%#v", len(portable.Records), portable.Lifecycle.Terminal)
			}

			encoded, err := workerRecordingCodec.EncodeWorkerPortableRecording(portable)
			if err != nil {
				t.Fatalf("EncodeWorkerPortableRecording() error = %v", err)
			}
			decoded, err := workerRecordingCodec.DecodeWorkerPortableRecording(encoded)
			if err != nil {
				t.Fatalf("DecodeWorkerPortableRecording() error = %v", err)
			}
			if !reflect.DeepEqual(portable, decoded) {
				t.Fatalf("decoded portable recording differs from exported value:\nwant=%#v\ngot=%#v", portable, decoded)
			}

			replayed, err := workerRecordingCodec.ReplayWorkerPortableRecording(decoded)
			if err != nil {
				t.Fatalf("ReplayWorkerPortableRecording() error = %v", err)
			}
			live, err := workerRecordingCodec.ReduceWorkerRecording(WorkerRecordingHistory{
				RecordingID:     snapshot.RecordingID,
				WorkerSessionID: snapshot.Sessions[0].WorkerSessionID,
				Topic:           snapshot.Sessions[0].Topic,
				Records:         snapshot.Sessions[0].Records,
			})
			if err != nil {
				t.Fatalf("ReduceWorkerRecording() error = %v", err)
			}
			if !reflect.DeepEqual(live, replayed.Projection) {
				t.Fatalf("portable replay differs from live reduction:\nlive=%#v\nreplay=%#v", live, replayed.Projection)
			}
			assertFidelityFacts(t, decoded, test.fidelity)
		})
	}
}

func TestWorkerPortableRecordingRejectsOrderingFidelityIntegrityAndUnknownFields(t *testing.T) {
	portable, err := workerRecordingCodec.BuildWorkerPortableRecording(portableSnapshot(t, "snapshot", "codex", "codex"))
	if err != nil {
		t.Fatal(err)
	}

	ordering := cloneWorkerPortableRecording(portable)
	ordering.Records[1].Position = 3
	if err := workerRecordingCodec.ValidateWorkerPortableRecording(ordering); !errors.Is(err, ErrWorkerPortableRecordingOrder) {
		t.Fatalf("ordering validation error = %v, want order classification", err)
	}

	fidelity := cloneWorkerPortableRecording(portable)
	var fidelityDraft workers.Draft
	if err := json.Unmarshal(fidelity.Records[1].Payload, &fidelityDraft); err != nil {
		t.Fatal(err)
	}
	fidelityDraft.Provenance.Fidelity = workers.FidelityFinalOnly
	fidelity.Records[1].Payload, _ = json.Marshal(fidelityDraft)
	fidelity.Records[1].Provenance = fidelityDraft.Provenance
	if err := workerRecordingCodec.ValidateWorkerPortableRecording(fidelity); !errors.Is(err, ErrWorkerPortableRecordingFidelity) {
		t.Fatalf("overstated fidelity error = %v, want fidelity classification", err)
	}

	integrity := cloneWorkerPortableRecording(portable)
	var changedDraft workers.Draft
	if err := json.Unmarshal(integrity.Records[1].Payload, &changedDraft); err != nil {
		t.Fatal(err)
	}
	var message workers.MessagePayload
	if err := json.Unmarshal(changedDraft.Payload, &message); err != nil {
		t.Fatal(err)
	}
	message.ContentBlocks[0].Text = "tampered"
	changedDraft.Payload, _ = json.Marshal(message)
	integrity.Records[1].Payload, _ = json.Marshal(changedDraft)
	if err := workerRecordingCodec.ValidateWorkerPortableRecording(integrity); !errors.Is(err, ErrWorkerPortableRecordingIntegrity) {
		t.Fatalf("integrity validation error = %v, want integrity classification", err)
	}

	encoded, err := workerRecordingCodec.EncodeWorkerPortableRecording(portable)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	document["unsupportedField"] = json.RawMessage(`true`)
	unknown, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workerRecordingCodec.DecodeWorkerPortableRecording(unknown); !errors.Is(err, ErrWorkerPortableRecording) {
		t.Fatalf("unknown field error = %v, want malformed portable recording", err)
	}
}

func TestWorkerPortableRecordingRejectsProviderOutputBeforeBinding(t *testing.T) {
	_, err := workerRecordingCodec.BuildWorkerPortableRecording(portableSnapshot(t, "snapshot", "", "codex"))
	if !errors.Is(err, ErrWorkerPortableRecordingProvenance) {
		t.Fatalf("provider-before-binding error = %v, want provenance classification", err)
	}
}

func TestWorkerRecordingRejectsInvalidAttemptLineageBeforeReplay(t *testing.T) {
	snapshot := portableSnapshot(t, "snapshot", "codex", "codex")
	session := snapshot.Sessions[0]
	terminal := session.Records[len(session.Records)-1]
	terminal.ID.Position = 4
	invalid := workerRecord(t, session.Topic, 3, "worker_session_attempt", session.WorkerSessionID, 1, "retry", workers.Draft{
		Kind:       workers.KindSession,
		Phase:      workers.PhaseUpdated,
		Provenance: lifecycleProvenanceForTest("codex"),
		Payload: mustJSON(t, workers.SessionPayload{
			Status:          "STARTING",
			WorkerSessionID: session.WorkerSessionID,
			DispatchID:      "dispatch-portable/attempt/2",
			AttemptID:       "dispatch-portable/attempt/2",
			Attempt:         2,
			AttemptReason:   workers.AttemptReasonRetry,
			Lineage: &workers.SessionLineage{
				PreviousDispatchID: "previous-dispatch",
				PreviousAttemptID:  "different-previous-attempt",
			},
		}),
		DispatchID: "dispatch-portable/attempt/2",
	})
	session.Records = append(append([]events.Record(nil), session.Records[:len(session.Records)-1]...), invalid, terminal)
	session.LastPosition = terminal.ID.Position
	snapshot.Sessions[0] = session

	_, err := workerRecordingCodec.ReduceWorkerRecording(WorkerRecordingHistory{
		RecordingID:     snapshot.RecordingID,
		WorkerSessionID: session.WorkerSessionID,
		Topic:           session.Topic,
		Records:         session.Records,
	})
	if !errors.Is(err, workers.ErrInvalidSessionLineage) {
		t.Fatalf("ReduceWorkerRecording() error = %v, want invalid lineage", err)
	}
}

func TestBuildWorkerPortableRecordingSelectsOneSessionFromMultiSessionSnapshot(t *testing.T) {
	first := portableSnapshot(t, "snapshot", "codex", "codex")
	second := first.Sessions[0]
	second.WorkerSessionID = "portable-worker-other"
	combined := first
	combined.Sessions = append([]WorkerSessionRecordingSnapshot(nil), first.Sessions[0], second)

	if _, err := workerRecordingCodec.BuildWorkerPortableRecording(combined); !errors.Is(err, ErrWorkerPortableRecordingIdentity) {
		t.Fatalf("multi-session export without selector = %v, want identity diagnostic", err)
	}
	portable, err := workerRecordingCodec.BuildWorkerPortableRecording(combined, first.Sessions[0].WorkerSessionID)
	if err != nil {
		t.Fatalf("multi-session export with selector error = %v", err)
	}
	if portable.Identity.WorkerSessionID != first.Sessions[0].WorkerSessionID {
		t.Fatalf("selected Worker Session ID = %q, want %q", portable.Identity.WorkerSessionID, first.Sessions[0].WorkerSessionID)
	}
}

func assertFidelityFacts(t *testing.T, recording WorkerPortableRecording, class string) {
	t.Helper()
	delta, snapshot, finalOnly := portableTestFidelityFacts(recording)
	switch class {
	case "streaming":
		if !delta || !snapshot || finalOnly {
			t.Fatalf("streaming facts = delta:%t snapshot:%t finalOnly:%t", delta, snapshot, finalOnly)
		}
	case "snapshot":
		if delta || !snapshot || finalOnly {
			t.Fatalf("snapshot facts = delta:%t snapshot:%t finalOnly:%t", delta, snapshot, finalOnly)
		}
	case "final-only":
		if delta || snapshot || !finalOnly {
			t.Fatalf("final-only facts = delta:%t snapshot:%t finalOnly:%t", delta, snapshot, finalOnly)
		}
	}
}

func portableTestFidelityFacts(recording WorkerPortableRecording) (delta, snapshot, finalOnly bool) {
	for _, record := range recording.Records {
		switch record.Provenance.Delivery {
		case workers.DeliveryNativeFinal:
			finalOnly = true
		case workers.DeliveryNativeStream:
			if record.Provenance.Representation == workers.RepresentationDelta {
				delta = true
			} else if record.Provenance.Representation == workers.RepresentationSnapshot {
				snapshot = true
			}
		}
	}
	return delta, snapshot, finalOnly
}

func portableSnapshot(t *testing.T, class, openingProvider, outputProvider string) WorkerRecordingSnapshot {
	t.Helper()
	const (
		recordingID = "portable-recording"
		sessionID   = "portable-worker"
	)
	topic := canonicalWorkerTopic(sessionID)
	startedAt := time.Date(2026, time.August, 11, 7, 0, 0, 0, time.UTC)
	openingPayload := workers.SessionPayload{
		Status:           "STARTING",
		StartedAt:        &startedAt,
		WorkerSessionID:  sessionID,
		FactorySessionID: "factory-portable",
		RecordingID:      recordingID,
		DispatchID:       "dispatch-portable",
		TurnID:           "turn-portable",
		TraceID:          "trace-portable",
		WorkIDs:          []string{"work-portable"},
		AttemptID:        "attempt-portable",
		Attempt:          1,
		AttemptReason:    workers.AttemptReasonInitial,
		ProviderSelection: &workers.SessionProviderSelection{
			RunnerID: outputProvider,
		},
	}
	opening := workerRecord(t, topic, 1, "worker_session_lifecycle", sessionID, 1, "started", workers.Draft{
		Kind:       workers.KindSession,
		Phase:      workers.PhaseStarted,
		Provenance: lifecycleProvenanceForTest(openingProvider),
		Payload:    mustJSON(t, openingPayload),
		DispatchID: openingPayload.DispatchID,
		TurnID:     openingPayload.TurnID,
	})

	records := []events.Record{opening}
	switch class {
	case "streaming":
		records = append(records,
			workerRecord(t, topic, 2, "worker_observation", sessionID, 1, "worker/1", workers.Draft{
				Kind: workers.KindMessage, Phase: workers.PhaseDelta,
				Provenance: workers.Provenance{
					Delivery: workers.DeliveryNativeStream, Fidelity: workers.FidelityNormalized,
					NativeEventType: "message.delta", Provider: outputProvider,
					Representation: workers.RepresentationDelta,
				}, DispatchID: openingPayload.DispatchID, TurnID: openingPayload.TurnID, ItemID: "message-1",
				Payload: mustJSON(t, workers.MessageDeltaPayload{ContentBlockIndex: 0, ContentBlockKind: workers.ContentBlockText, TextDelta: "hello"}),
			}),
			workerRecord(t, topic, 3, "worker_observation", sessionID, 2, "worker/2", workers.Draft{
				Kind: workers.KindMessage, Phase: workers.PhaseCompleted,
				Provenance: workers.Provenance{
					Delivery: workers.DeliveryNativeStream, Fidelity: workers.FidelityNormalized,
					NativeEventType: "message.completed", Provider: outputProvider,
					Representation: workers.RepresentationSnapshot,
				}, DispatchID: openingPayload.DispatchID, TurnID: openingPayload.TurnID, ItemID: "message-1",
				Payload: mustJSON(t, workers.MessagePayload{Role: "assistant", ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: "hello"}}}),
			}),
		)
	case "snapshot":
		records = append(records, workerRecord(t, topic, 2, "worker_observation", sessionID, 1, "worker/1", workers.Draft{
			Kind: workers.KindMessage, Phase: workers.PhaseCompleted,
			Provenance: workers.Provenance{
				Delivery: workers.DeliveryNativeStream, Fidelity: workers.FidelityNormalized,
				NativeEventType: "message.completed", Provider: outputProvider,
				Representation: workers.RepresentationSnapshot,
			}, DispatchID: openingPayload.DispatchID, TurnID: openingPayload.TurnID, ItemID: "message-1",
			Payload: mustJSON(t, workers.MessagePayload{Role: "assistant", ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: "snapshot"}}}),
		}))
	case "final-only":
		records = append(records, workerRecord(t, topic, 2, "worker_observation", sessionID, 1, "worker/1", workers.Draft{
			Kind: workers.KindMessage, Phase: workers.PhaseCompleted,
			Provenance: workers.Provenance{
				Delivery: workers.DeliveryNativeFinal, Fidelity: workers.FidelityFinalOnly,
				NativeEventType: "final_response", Provider: outputProvider,
				Representation: workers.RepresentationSnapshot,
			}, DispatchID: openingPayload.DispatchID, TurnID: openingPayload.TurnID, ItemID: "message-1",
			Payload: mustJSON(t, workers.MessagePayload{Role: "assistant", ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: "final"}}}),
		}))
	default:
		t.Fatalf("unknown portable fixture class %q", class)
	}
	terminalPosition := events.AggregateSequence(len(records) + 1)
	records = append(records, workerRecord(t, topic, terminalPosition, "worker_session_lifecycle", sessionID, 2, "terminal", workers.Draft{
		Kind: workers.KindSession, Phase: workers.PhaseCompleted,
		Provenance: lifecycleProvenanceForTest(outputProvider),
		Payload:    mustJSON(t, map[string]string{"status": "COMPLETED"}),
		DispatchID: openingPayload.DispatchID,
	}))
	return WorkerRecordingSnapshot{
		RecordingID: recordingID,
		Sessions: []WorkerSessionRecordingSnapshot{{
			WorkerSessionID: sessionID, Topic: topic, Status: WorkerRecordingStatusCompleted,
			LastPosition: terminalPosition, Records: records,
		}},
	}
}

func workerRecord(t *testing.T, topic events.Topic, position events.AggregateSequence, sourceType, sourceID string, sourceSequence events.SourceSequence, sourceEventID string, draft workers.Draft) events.Record {
	t.Helper()
	return events.Record{
		ID:             events.RecordID{Topic: topic, Position: position},
		SourceType:     events.SourceType(sourceType),
		SourceID:       events.SourceID(sourceID),
		SourceSequence: sourceSequence,
		SourceEventID:  events.SourceEventID(sourceEventID),
		SchemaID:       "workers.draft.v1",
		Payload:        mustJSON(t, draft),
	}
}

func lifecycleProvenanceForTest(provider string) workers.Provenance {
	return workers.Provenance{
		Delivery: workers.DeliverySynthesized, Fidelity: workers.FidelityLifecycleOnly,
		NativeEventType: "worker_session_lifecycle", Provider: provider,
		Representation: workers.RepresentationNotification,
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
