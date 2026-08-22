package service

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsinternal "github.com/portpowered/infinite-you/pkg/services/recordings/internal"
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
	}, recordingsinternal.NewProjectionService())
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

func TestQueryHistoricalRecordingClassifiesMissingAndCorruptHistory(t *testing.T) {
	t.Parallel()

	identity := recordings.HistoricalRecordingIdentity{
		RecordingID: "recording-history-errors",
		Artifact:    "artifact-history-errors",
		Scope:       recordings.CanonicalEventScope{FactorySessionID: "dur-sess-history-errors"},
	}
	t.Run("missing", func(t *testing.T) {
		query := New(func(string) ([]byte, error) { return nil, os.ErrNotExist }, recordingsinternal.NewProjectionService())
		_, err := query.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{Recording: identity})
		assertHistoricalQueryKind(t, err, recordings.HistoricalRecordingQueryErrorMissingHistory)
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("error = %v, want os.ErrNotExist cause", err)
		}
	})
	t.Run("corrupt", func(t *testing.T) {
		query := New(func(string) ([]byte, error) {
			return []byte(`{"schemaVersion":"recordings.portable-artifact.v1","summary":{}}`), nil
		}, recordingsinternal.NewProjectionService())
		_, err := query.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{Recording: identity})
		assertHistoricalQueryKind(t, err, recordings.HistoricalRecordingQueryErrorCorruptHistory)
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
