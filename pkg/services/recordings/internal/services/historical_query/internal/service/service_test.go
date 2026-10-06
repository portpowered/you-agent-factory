package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	projectionquerywire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/projection_query/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestQueryHistoricalRecordingReadsAndProjectsPortableArtifact(t *testing.T) {
	t.Parallel()

	identity := recordings.HistoricalRecordingIdentity{
		RecordingID: "recording-history-001",
		Artifact:    "artifact-history-001",
		Scope:       recordings.CanonicalEventScope{FactorySessionID: "dur-sess-history-001"},
	}
	artifact := recordings.PortableArtifact{
		SchemaVersion: recordings.PortableArtifactSchemaV1,
		Summary: recordings.PortableArtifactSummary{
			RecordingID: identity.RecordingID,
			Reference:   identity.Artifact,
			Scope:       identity.Scope,
			State:       recordings.RecordingFinalized,
			Available:   true,
		},
		Integrity: recordings.PortableArtifactIntegrity{
			Algorithm: recordings.PortableArtifactIntegritySHA256,
		},
	}
	digest, err := portableArtifactDigest(artifact)
	if err != nil {
		t.Fatalf("portableArtifactDigest: %v", err)
	}
	artifact.Integrity.Digest = digest
	payload := portableArtifactWithFutureFields(t, artifact)

	var readReference string
	query := New(func(reference string) ([]byte, error) {
		readReference = reference
		return payload, nil
	}, projectionquerywire.NewService())
	result, err := query.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{Recording: identity})
	if err != nil {
		t.Fatalf("QueryHistoricalRecording: %v", err)
	}
	if readReference != string(identity.Artifact) {
		t.Fatalf("read reference = %q, want %q", readReference, identity.Artifact)
	}
	if result.Recording != identity {
		t.Fatalf("recording identity = %#v, want %#v", result.Recording, identity)
	}
	if result.Status.State != recordings.RecordingFinalized || result.Status.AcceptedEvents != 0 {
		t.Fatalf("status = %#v, want finalized empty history", result.Status)
	}
	assertIgnoredJSONPaths(t, result.IgnoredJSONPaths, []string{"$.futureTopLevel", "$.summary.futureSummary"})
	if result.WorldState.SchemaVersion != recordings.WorldStateViewSchemaV1 {
		t.Fatalf("world-state schema = %q, want %q", result.WorldState.SchemaVersion, recordings.WorldStateViewSchemaV1)
	}
	if len(result.Events) != 0 || len(result.Dispatches) != 0 {
		t.Fatalf("history result = %#v, want no events or dispatches", result)
	}
	var worldState map[string]any
	if err := json.Unmarshal([]byte(result.WorldState.Payload), &worldState); err != nil {
		t.Fatalf("world-state payload is not JSON: %v", err)
	}
}

// Historical query owns decoding and response construction; projection is a
// controlled peer so these cells do not also test the world-state reducer.
func TestHistoricalQueryRetainsTypedStateAndDetachedView(t *testing.T) {
	t.Parallel()
	for _, empty := range []bool{false, true} {
		t.Run(fmt.Sprint("empty=", empty), func(t *testing.T) {
			t.Parallel()
			identity, payload := historicalLoaderFixture(t, empty)
			state := recordings.FactoryWorldState{Tick: 2, WorkItemsByID: map[string]work.FactoryWorkItem{
				"work": {ID: "work", State: "waiting", Tags: map[string]string{"label": "§ —"}, Payload: json.RawMessage(`{"text":"§ —"}`)},
			}}
			peer := &historicalProjectionPeer{state: state}
			query := New(func(string) ([]byte, error) { return payload, nil }, peer)
			result, err := query.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{Recording: identity})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(peer.projected, state) {
				t.Fatalf("derived projection received %+v; want %+v", peer.projected, state)
			}
			assertHistoricalProjectionMetadata(t, empty, identity, peer, result)
			wantPayload, err := json.Marshal(state)
			if err != nil || result.WorldState.Payload != string(wantPayload) {
				t.Fatalf("view changed: %s, %v", result.WorldState.Payload, err)
			}
			state.WorkItemsByID["work"].Tags["label"] = "mutated"
			if result.WorldState.Payload != string(wantPayload) {
				t.Fatal("returned view aliases projection state")
			}
			if !empty {
				peer.events[0].Payload[0] = 'x'
				if result.Events[0].Payload != `{}` {
					t.Fatal("returned canonical events alias projection input")
				}
			}
		})
	}
}

func assertHistoricalProjectionMetadata(t *testing.T, empty bool, identity recordings.HistoricalRecordingIdentity, peer *historicalProjectionPeer, result recordings.HistoricalRecordingQueryResult) {
	t.Helper()
	wantTick, wantEvents := 2, 1
	if empty {
		wantTick, wantEvents = 0, 0
	}
	if len(peer.events) != wantEvents || peer.tick != wantTick || result.WorldState.SelectedTick != wantTick ||
		result.WorldState.Scope != identity.Scope || result.WorldState.SchemaVersion != recordings.WorldStateViewSchemaV1 {
		t.Fatalf("projection input/view metadata = %+v, %+v", peer, result.WorldState)
	}
	if !empty && (peer.events[0].Context.SessionID == nil || *peer.events[0].Context.SessionID != "session" ||
		result.WorldState.Through != result.Events[0].Cursor) {
		t.Fatal("session scope or cursor lost")
	}
	if empty && result.WorldState.Through != (recordings.CanonicalEventCursor{}) {
		t.Fatal("empty recording acquired a cursor")
	}
}

func TestHistoricalQueryPreservesReadAndProjectionFailureIdentity(t *testing.T) {
	t.Parallel()
	for _, readFailure := range []bool{true, false} {
		t.Run(fmt.Sprint("read=", readFailure), func(t *testing.T) {
			t.Parallel()
			identity, payload := historicalLoaderFixture(t, false)
			peer := &historicalProjectionPeer{}
			read := func(string) ([]byte, error) { return payload, nil }
			wantKind := recordings.HistoricalRecordingQueryErrorCorruptHistory
			if readFailure {
				read = func(string) ([]byte, error) { return nil, context.Canceled }
				wantKind = recordings.HistoricalRecordingQueryErrorUnavailable
			} else {
				peer.failure = context.Canceled
			}
			_, err := New(read, peer).QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{Recording: identity})
			assertHistoricalQueryKind(t, err, wantKind)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation identity lost: %v", err)
			}
		})
	}
}

func TestHistoricalQueryRejectsInvalidHistoryBeforeProjection(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name   string
		mutate func(*legacyArtifactDocument)
	}{
		{"schema", func(a *legacyArtifactDocument) { a.SchemaVersion = "future" }},
		{"timestamp", func(a *legacyArtifactDocument) { a.RecordedAt = time.Time{} }},
		{"event schema", func(a *legacyArtifactDocument) { a.Events[0].SchemaVersion = "future" }},
		{"sequence", func(a *legacyArtifactDocument) { a.Events[0].Context.Sequence = 1 }},
		{"tick", func(a *legacyArtifactDocument) { a.Events[0].Context.Tick = -1 }},
		{"scope", func(a *legacyArtifactDocument) { foreign := "foreign"; a.Events[0].Context.SessionID = &foreign }},
		{"identity", func(a *legacyArtifactDocument) { a.Events[0].Id = "" }},
		{"kind", func(a *legacyArtifactDocument) { a.Events[0].Type = "unknown" }},
		{"order", func(a *legacyArtifactDocument) { a.Events = append(a.Events, a.Events[0]) }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			identity, payload := historicalLoaderFixture(t, false)
			var artifact legacyArtifactDocument
			if err := json.Unmarshal(payload, &artifact); err != nil {
				t.Fatal(err)
			}
			scenario.mutate(&artifact)
			payload, err := json.Marshal(artifact)
			if err != nil {
				t.Fatal(err)
			}
			peer := &historicalProjectionPeer{}
			_, err = New(func(string) ([]byte, error) { return payload, nil }, peer).
				QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{Recording: identity})
			assertHistoricalQueryKind(t, err, recordings.HistoricalRecordingQueryErrorCorruptHistory)
			if peer.events != nil {
				t.Fatal("invalid history reached projection")
			}
		})
	}
}

func TestHistoricalQueryRetainsSerializationFailureCause(t *testing.T) {
	t.Parallel()
	identity, payload := historicalLoaderFixture(t, true)
	peer := &historicalProjectionPeer{state: recordings.FactoryWorldState{
		WorkItemsByID: map[string]work.FactoryWorkItem{"work": {StructuredResult: make(chan int)}},
	}}
	_, err := New(func(string) ([]byte, error) { return payload, nil }, peer).
		QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{Recording: identity})
	assertHistoricalQueryKind(t, err, recordings.HistoricalRecordingQueryErrorCorruptHistory)
	var unsupported *json.UnsupportedTypeError
	if !errors.As(err, &unsupported) {
		t.Fatalf("serialization cause lost: %v", err)
	}
}

func historicalLoaderFixture(t *testing.T, empty bool) (recordings.HistoricalRecordingIdentity, []byte) {
	t.Helper()
	identity := recordings.HistoricalRecordingIdentity{
		RecordingID: "recording", Artifact: "artifact", Scope: recordings.CanonicalEventScope{FactorySessionID: "session"},
	}
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	session := identity.Scope.FactorySessionID
	artifact := legacyArtifactDocument{SchemaVersion: factorydefinitions.ReplayV1SourceFormat, RecordedAt: now}
	if !empty {
		artifact.Events = []factorydefinitions.FactoryEvent{{
			SchemaVersion: factorydefinitions.FactoryEventSchemaVersionV1, Id: "event",
			Type: factorydefinitions.FactoryEventTypeRunRequest, Payload: json.RawMessage(`{}`),
			Context: factorydefinitions.FactoryEventContext{SessionID: &session, Tick: 2, EventTime: now},
		}}
	}
	payload, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	return identity, payload
}

type historicalProjectionPeer struct {
	recordings.ProjectionService
	state, projected recordings.FactoryWorldState
	events           []recordings.FactoryEvent
	tick             int
	failure          error
}

func (peer *historicalProjectionPeer) ReconstructFactoryWorldState(events []recordings.FactoryEvent, tick int) (recordings.FactoryWorldState, error) {
	peer.events, peer.tick = events, tick
	return peer.state, peer.failure
}

func (peer *historicalProjectionPeer) ProjectWorkstationRequests(state recordings.FactoryWorldState) recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice {
	peer.projected = state
	return recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice{}
}

func TestQueryHistoricalRecordingClassifiesMissingAndCorruptHistory(t *testing.T) {
	t.Parallel()

	identity := recordings.HistoricalRecordingIdentity{
		RecordingID: "recording-history-errors",
		Artifact:    "artifact-history-errors",
		Scope:       recordings.CanonicalEventScope{FactorySessionID: "dur-sess-history-errors"},
	}
	t.Run("missing", func(t *testing.T) {
		query := New(func(string) ([]byte, error) { return nil, os.ErrNotExist }, projectionquerywire.NewService())
		_, err := query.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{Recording: identity})
		assertHistoricalQueryKind(t, err, recordings.HistoricalRecordingQueryErrorMissingHistory)
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("error = %v, want os.ErrNotExist cause", err)
		}
	})
	t.Run("corrupt", func(t *testing.T) {
		query := New(func(string) ([]byte, error) {
			return []byte(`{"schemaVersion":"recordings.portable-artifact.v1","summary":{}}`), nil
		}, projectionquerywire.NewService())
		_, err := query.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{Recording: identity})
		assertHistoricalQueryKind(t, err, recordings.HistoricalRecordingQueryErrorCorruptHistory)
	})
	t.Run("unreadable", func(t *testing.T) {
		query := New(func(string) ([]byte, error) { return nil, os.ErrPermission }, projectionquerywire.NewService())
		_, err := query.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{Recording: identity})
		assertHistoricalQueryKind(t, err, recordings.HistoricalRecordingQueryErrorUnavailable)
		if errors.Is(err, os.ErrNotExist) {
			t.Fatal("unreadable recording was classified as missing history")
		}
	})
}

func assertHistoricalQueryKind(
	t *testing.T,
	err error,
	want recordings.HistoricalRecordingQueryErrorKind,
) {
	t.Helper()
	var typed *recordings.HistoricalRecordingQueryError
	if !errors.As(err, &typed) {
		t.Fatalf("error = %v, want HistoricalRecordingQueryError", err)
	}
	if typed.Kind != want {
		t.Fatalf("error kind = %q, want %q", typed.Kind, want)
	}
}

func portableArtifactWithFutureFields(t *testing.T, artifact recordings.PortableArtifact) []byte {
	t.Helper()
	payload, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	document["futureTopLevel"] = json.RawMessage(`true`)
	var summary map[string]json.RawMessage
	if err := json.Unmarshal(document["summary"], &summary); err != nil {
		t.Fatal(err)
	}
	summary["futureSummary"] = json.RawMessage(`"ignored"`)
	document["summary"], err = json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	payload, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func assertIgnoredJSONPaths(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ignored paths = %#v, want %#v", got, want)
	}
}
