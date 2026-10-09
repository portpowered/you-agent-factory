package service

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
)

func TestWorkerWorkAttributionInfersOnlyValidatedArtifactScope(t *testing.T) {
	t.Parallel()
	for _, format := range []string{"legacy", "portable", "replay-v2"} {
		for _, scope := range []string{"selected", "~default"} {
			t.Run(format+"/"+scope, func(t *testing.T) {
				t.Parallel()
				identity, _ := historicalLoaderFixture(t, false)
				identity.Scope.FactorySessionID = scope
				events := scopeFixtureEvents(identity.Scope.FactorySessionID)
				payload := scopeFixtureArtifact(t, format, identity, events)
				query := New(nil, nil)
				expected, err := query.DecodeHistoricalEvents(recordings.HistoricalRecordingQueryRequest{Recording: identity}, payload)
				if err != nil {
					t.Fatal(err)
				}
				requested := identity
				requested.Scope.FactorySessionID = "requested"
				inferred, err := query.DecodeHistoricalEvents(recordings.HistoricalRecordingQueryRequest{Recording: requested, InferFactorySessionScope: true}, payload)
				if err != nil || !reflect.DeepEqual(inferred, expected) {
					t.Fatalf("inferred facts = %+v, %v; want %+v", inferred, err, expected)
				}
				_, err = query.DecodeHistoricalEvents(recordings.HistoricalRecordingQueryRequest{Recording: requested}, payload)
				assertHistoricalQueryKind(t, err, recordings.HistoricalRecordingQueryErrorCorruptHistory)
			})
		}
	}
}

func TestInferredArtifactScopeStillValidatesCompleteStream(t *testing.T) {
	t.Parallel()
	for _, format := range []string{"legacy", "portable", "replay-v2"} {
		t.Run(format, func(t *testing.T) {
			t.Parallel()
			for _, fault := range []string{"foreign-later-event", "duplicate-sequence", "missing-scope", "malformed", "empty"} {
				t.Run(fault, func(t *testing.T) {
					t.Parallel()
					identity, _ := historicalLoaderFixture(t, false)
					events := scopeFixtureEvents(identity.Scope.FactorySessionID)
					switch fault {
					case "foreign-later-event":
						foreign := "foreign"
						events[1].Context.SessionID = &foreign
					case "duplicate-sequence":
						events[1].Context.Sequence = 0
					case "missing-scope":
						events[0].Context.SessionID = nil
						identity.Scope.FactorySessionID = ""
					case "empty":
						events = nil
					}
					payload := scopeFixtureArtifact(t, format, identity, events)
					if fault == "malformed" {
						payload = []byte("{")
					}
					identity.Scope.FactorySessionID = "requested"
					got, err := New(nil, nil).DecodeHistoricalEvents(recordings.HistoricalRecordingQueryRequest{Recording: identity, InferFactorySessionScope: true}, payload)
					if fault == "empty" && format == "portable" {
						if err != nil || len(got.Events) != 0 || got.Recording.Scope.FactorySessionID != "session" {
							t.Fatalf("empty portable = %+v, %v", got, err)
						}
						return
					}
					assertHistoricalQueryKind(t, err, recordings.HistoricalRecordingQueryErrorCorruptHistory)
				})
			}
		})
	}
}

func scopeFixtureEvents(session string) []factorydefinitions.FactoryEvent {
	at := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	return []factorydefinitions.FactoryEvent{
		{SchemaVersion: factorydefinitions.FactoryEventSchemaVersionV1, Id: "first", Type: factorydefinitions.FactoryEventTypeRunRequest, Payload: json.RawMessage(`{}`), Context: factorydefinitions.FactoryEventContext{SessionID: &session, Sequence: 0, Tick: 1, EventTime: at}},
		{SchemaVersion: factorydefinitions.FactoryEventSchemaVersionV1, Id: "second", Type: factorydefinitions.FactoryEventTypeRunRequest, Payload: json.RawMessage(`{}`), Context: factorydefinitions.FactoryEventContext{SessionID: &session, Sequence: 1, Tick: 2, EventTime: at}},
	}
}

func scopeFixtureArtifact(t *testing.T, format string, identity recordings.HistoricalRecordingIdentity, events []factorydefinitions.FactoryEvent) []byte {
	t.Helper()
	at := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	if format == "replay-v2" {
		// Header identity is a logical UUID; canonical events retain source-native scope.
		payload := []byte(`{"recordType":"header","schemaVersion":"agent-factory.replay.v2","recordedAt":"2026-10-06T00:00:00Z","sessionId":"2f23e4f2-3939-4f85-9ecc-c1c23db73d75","factoryIdentity":{"id":"factory","name":"Factory","factoryDirectory":"factory","sourceDirectory":"source"},"hashes":{"factory_hash":"hash","workers_hash":"hash","workstations_hash":"hash","runtime_config_hash":"hash"}}` + "\n")
		for _, event := range events {
			line, err := replayimpl.MarshalReplayV2Event(event)
			if err != nil {
				t.Fatal(err)
			}
			payload = append(payload, line...)
		}
		return payload
	}
	var document any = legacyArtifactDocument{SchemaVersion: factorydefinitions.ReplayV1SourceFormat, RecordedAt: at, Events: events}
	if format == "portable" {
		document = scopePortableArtifact(t, identity, events)
	}
	payload, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func scopePortableArtifact(t *testing.T, identity recordings.HistoricalRecordingIdentity, events []factorydefinitions.FactoryEvent) recordings.PortableArtifact {
	t.Helper()
	artifact := recordings.PortableArtifact{
		SchemaVersion: recordings.PortableArtifactSchemaV1,
		Summary:       recordings.PortableArtifactSummary{RecordingID: identity.RecordingID, Reference: identity.Artifact, Scope: identity.Scope, State: recordings.RecordingFinalized, Available: true, EventCount: len(events)},
		Integrity:     recordings.PortableArtifactIntegrity{Algorithm: recordings.PortableArtifactIntegritySHA256},
	}
	for _, event := range events {
		artifact.Events = append(artifact.Events, canonical.CanonicalEventFromFactory(event, "historical-recording/"+string(identity.RecordingID)))
	}
	if len(artifact.Events) > 0 {
		first, last := artifact.Events[0].Cursor, artifact.Events[len(artifact.Events)-1].Cursor
		artifact.Summary.FirstCursor, artifact.Summary.LastCursor = &first, &last
	}
	digest, err := portableArtifactDigest(artifact)
	if err != nil {
		t.Fatal(err)
	}
	artifact.Integrity.Digest = digest
	return artifact
}
