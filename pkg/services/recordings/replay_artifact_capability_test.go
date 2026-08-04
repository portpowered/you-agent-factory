package recordings_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
)

type replayArtifactTestLedger struct{}

func (replayArtifactTestLedger) CanonicalEvents() []factorydefinitions.FactoryEvent { return nil }

func (replayArtifactTestLedger) Subscribe(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (factorydefinitions.FactoryEventStream, error) {
	return factorydefinitions.FactoryEventStream{}, nil
}

func (replayArtifactTestLedger) StreamGenerationID() string { return "replay-artifact-capability-test" }

func (replayArtifactTestLedger) AddEventRecorder(func(factorydefinitions.FactoryEvent)) {}

func (replayArtifactTestLedger) AddEventTypeRecorder(func(factorydefinitions.FactoryEventType)) {}

func (replayArtifactTestLedger) AppendRecordedEvent(factorydefinitions.FactoryEvent) {}

// newTestRecordingReplayArtifacts constructs the real composed Recordings
// root from ordinary os filesystem effects and narrows it down to the
// RecordingReplayArtifacts capability, proving construction alone (with no
// prior Bind/Finish calls) never touches those effects.
func newTestRecordingReplayArtifacts(t *testing.T) (recordings.Service, recordings.RecordingReplayArtifacts) {
	t.Helper()
	service, err := recordingswire.NewServiceWithProjectionAndEffects(
		replayArtifactTestLedger{},
		recordingswire.NewProjectionService(),
		nil,
		func(string, []byte) error { return nil },
		os.MkdirAll,
		func(dir, pattern string) (recordings.RecordingTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		os.Remove,
		os.Rename,
		os.ReadFile,
	)
	if err != nil {
		t.Fatalf("NewServiceWithProjectionAndEffects() error = %v", err)
	}
	replayArtifacts, ok := service.(recordings.RecordingReplayArtifacts)
	if !ok {
		t.Fatal("recordings.Service does not implement recordings.RecordingReplayArtifacts")
	}
	return service, replayArtifacts
}

// TestRecordingReplayArtifacts_ConstructionIsInert proves constructing the
// capability performs no I/O: every injected filesystem effect panics if
// invoked, yet construction alone succeeds.
func TestRecordingReplayArtifacts_ConstructionIsInert(t *testing.T) {
	t.Parallel()
	panicEffect := func(string, ...any) { panic("construction must not perform I/O") }
	service, err := recordingswire.NewServiceWithProjectionAndEffects(
		replayArtifactTestLedger{},
		recordingswire.NewProjectionService(),
		nil,
		func(string, []byte) error { panicEffect("writeFile"); return nil },
		func(string, os.FileMode) error { panicEffect("makeDirectories"); return nil },
		func(string, string) (recordings.RecordingTemporaryFile, error) {
			panicEffect("createTemporaryFile")
			return nil, nil
		},
		func(string) error { panicEffect("removePath"); return nil },
		func(string, string) error { panicEffect("renamePath"); return nil },
		func(string) ([]byte, error) { panicEffect("readFile"); return nil, nil },
	)
	if err != nil {
		t.Fatalf("NewServiceWithProjectionAndEffects() error = %v", err)
	}
	if _, ok := service.(recordings.RecordingReplayArtifacts); !ok {
		t.Fatal("recordings.Service does not implement recordings.RecordingReplayArtifacts")
	}
}

func finalizedReplayArtifactRecording(
	t *testing.T, service recordings.Service, recordingID string,
) recordings.ReplayRecordingID {
	t.Helper()
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-" + recordingID}
	bound, err := service.BindRecording(recordings.BindRecordingRequest{
		RecordingID: recordings.RecordingID(recordingID),
		Artifact:    recordings.RecordingArtifactReference(filepath.Join(t.TempDir(), recordingID+".json")),
		Scope:       scope,
	})
	if err != nil {
		t.Fatalf("BindRecording: %v", err)
	}
	recordedAt := time.Unix(1_700_000_000, 0).UTC()
	// The durable JSONL replay-artifact writer exercised by FinishRecording's
	// final flush requires the first recorded event to carry decodable
	// Factory snapshot config.
	payload := `{"factory":{"id":"` + recordingID + `"},"recordedAt":"` + recordedAt.Format(time.RFC3339Nano) + `"}`
	event := recordings.CanonicalEvent{
		ID:         recordings.CanonicalEventID(recordingID + "-event"),
		Kind:       recordings.CanonicalEventKind(recordings.FactoryEventTypeRunRequest),
		Scope:      scope,
		RecordedAt: recordedAt,
		Payload:    payload,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-" + recordingID,
		},
	}
	if _, err := service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event:       event,
	}); err != nil {
		t.Fatalf("RecordRecordingEvent: %v", err)
	}
	if _, err := service.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_001, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording: %v", err)
	}
	return recordings.ReplayRecordingID(bound.Status.RecordingID)
}

// TestRecordingReplayArtifacts_UnchangedBehaviorThroughComposedImplementation
// proves the narrow capability's LoadReplay and full artifact build/
// validate/encode/decode/summarize/export/read chain preserve identity,
// scope, canonical order, and summary through the real composed
// implementation, unchanged from the broader Service surface it adapts.
func TestRecordingReplayArtifacts_UnchangedBehaviorThroughComposedImplementation(t *testing.T) {
	t.Parallel()
	service, replayArtifacts := newTestRecordingReplayArtifacts(t)
	recordingID := finalizedReplayArtifactRecording(t, service, "unchanged-behavior")

	loaded, err := replayArtifacts.LoadReplay(recordings.LoadReplayRequest{RecordingID: recordingID})
	if err != nil {
		t.Fatalf("LoadReplay: %v", err)
	}
	if loaded.Replay.RecordingID != recordingID || len(loaded.Replay.Events) != 1 {
		t.Fatalf("LoadReplay() = %#v, want one event for %q", loaded.Replay, recordingID)
	}
	if loaded.Replay.Events[0].ID != "unchanged-behavior-event" {
		t.Fatalf("LoadReplay() Events[0].ID = %q, want unchanged-behavior-event", loaded.Replay.Events[0].ID)
	}

	built, err := replayArtifacts.BuildArtifact(recordings.BuildArtifactRequest{RecordingID: recordingID})
	if err != nil {
		t.Fatalf("BuildArtifact: %v", err)
	}
	if built.Artifact.SchemaVersion != recordings.ArtifactSchemaV1 ||
		built.Artifact.Summary.RecordingID != recordingID ||
		built.Artifact.Summary.EventCount != 1 ||
		!built.Artifact.Summary.Available {
		t.Fatalf("BuildArtifact() Artifact = %#v", built.Artifact)
	}

	validated, err := replayArtifacts.ValidateArtifact(recordings.ValidateArtifactRequest{Artifact: built.Artifact})
	if err != nil {
		t.Fatalf("ValidateArtifact: %v", err)
	}
	if !reflect.DeepEqual(validated.Summary, built.Artifact.Summary) {
		t.Fatalf("ValidateArtifact() Summary = %#v, want %#v", validated.Summary, built.Artifact.Summary)
	}

	encoded, err := replayArtifacts.EncodeArtifact(recordings.EncodeArtifactRequest{Artifact: built.Artifact})
	if err != nil || len(encoded.Payload) == 0 {
		t.Fatalf("EncodeArtifact = (%d bytes, %v)", len(encoded.Payload), err)
	}
	decoded, err := replayArtifacts.DecodeArtifact(recordings.DecodeArtifactRequest{Payload: encoded.Payload})
	if err != nil {
		t.Fatalf("DecodeArtifact: %v", err)
	}
	if decoded.Artifact.Integrity != built.Artifact.Integrity {
		t.Fatalf("DecodeArtifact() Integrity = %#v, want %#v", decoded.Artifact.Integrity, built.Artifact.Integrity)
	}
	summarized, err := replayArtifacts.SummarizeArtifact(recordings.SummarizeArtifactRequest{Artifact: decoded.Artifact})
	if err != nil {
		t.Fatalf("SummarizeArtifact: %v", err)
	}
	if !reflect.DeepEqual(summarized.Summary, built.Artifact.Summary) {
		t.Fatalf("SummarizeArtifact() Summary = %#v, want %#v", summarized.Summary, built.Artifact.Summary)
	}

	exported, err := replayArtifacts.ExportArtifact(context.Background(), recordings.ExportArtifactRequest{
		RecordingID: recordingID,
	})
	if err != nil {
		t.Fatalf("ExportArtifact: %v", err)
	}
	read, err := replayArtifacts.ReadArtifact(context.Background(), recordings.ReadArtifactRequest{
		RecordingID: recordingID,
		Reference:   exported.Reference,
	})
	if err != nil {
		t.Fatalf("ReadArtifact: %v", err)
	}
	if read.Artifact.Integrity != exported.Artifact.Integrity ||
		read.Artifact.Summary.EventCount != exported.Artifact.Summary.EventCount {
		t.Fatalf("ReadArtifact() = %#v, want %#v", read.Artifact, exported.Artifact)
	}
}

// TestRecordingReplayArtifacts_TypedFailures proves missing, not-yet-
// finalized, malformed, and foreign-handle inputs return the capability's own
// matchable ReplayArtifactError kinds while still unwrapping to the
// underlying Recordings sentinel errors.
func TestRecordingReplayArtifacts_TypedFailures(t *testing.T) {
	t.Parallel()
	service, replayArtifacts := newTestRecordingReplayArtifacts(t)

	_, err := replayArtifacts.LoadReplay(recordings.LoadReplayRequest{RecordingID: "missing"})
	assertReplayArtifactErrorKind(t, err, recordings.ReplayArtifactErrorNotFound, recordings.ErrReplayRecordingNotFound)

	active, err := service.BindRecording(recordings.BindRecordingRequest{Artifact: "artifact:active"})
	if err != nil {
		t.Fatalf("BindRecording active: %v", err)
	}
	_, err = replayArtifacts.LoadReplay(recordings.LoadReplayRequest{
		RecordingID: recordings.ReplayRecordingID(active.Status.RecordingID),
	})
	assertReplayArtifactErrorKind(
		t, err, recordings.ReplayArtifactErrorNotFinalized, recordings.ErrReplayRecordingNotFinalized,
	)

	_, err = replayArtifacts.DecodeArtifact(recordings.DecodeArtifactRequest{Payload: []byte("{")})
	assertReplayArtifactErrorKind(t, err, recordings.ReplayArtifactErrorInvalid, recordings.ErrInvalidPortableArtifact)

	owner := finalizedReplayArtifactRecording(t, service, "typed-failures-owner")
	other := finalizedReplayArtifactRecording(t, service, "typed-failures-other")
	ownerExport, err := replayArtifacts.ExportArtifact(context.Background(), recordings.ExportArtifactRequest{
		RecordingID: owner,
	})
	if err != nil {
		t.Fatalf("ExportArtifact owner: %v", err)
	}
	_, err = replayArtifacts.ReadArtifact(context.Background(), recordings.ReadArtifactRequest{
		RecordingID: other,
		Reference:   ownerExport.Reference,
	})
	assertReplayArtifactErrorKind(t, err, recordings.ReplayArtifactErrorForeign, recordings.ErrForeignPortableArtifact)

	cancelCause := errors.New("operator stopped export")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cancelCause)
	unexported := finalizedReplayArtifactRecording(t, service, "typed-failures-cancelled")
	_, err = replayArtifacts.ExportArtifact(ctx, recordings.ExportArtifactRequest{RecordingID: unexported})
	assertReplayArtifactErrorKind(t, err, recordings.ReplayArtifactErrorCancelled, recordings.ErrPortableArtifactCancelled)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, cancelCause) {
		t.Fatalf("ExportArtifact(cancelled) error = %v, want to unwrap context.Canceled and cause", err)
	}
}

// TestRecordingReplayArtifacts_UnsupportedSchemaVersion proves an artifact
// carrying a schema version other than the one this capability publishes
// returns the distinct unsupported-schema classification rather than a
// generic invalid classification.
func TestRecordingReplayArtifacts_UnsupportedSchemaVersion(t *testing.T) {
	t.Parallel()
	service, replayArtifacts := newTestRecordingReplayArtifacts(t)
	recordingID := finalizedReplayArtifactRecording(t, service, "unsupported-schema")
	built, err := replayArtifacts.BuildArtifact(recordings.BuildArtifactRequest{RecordingID: recordingID})
	if err != nil {
		t.Fatalf("BuildArtifact: %v", err)
	}
	built.Artifact.SchemaVersion = "recordings.portable-artifact.v999"

	_, err = replayArtifacts.ValidateArtifact(recordings.ValidateArtifactRequest{Artifact: built.Artifact})
	assertReplayArtifactErrorKind(
		t, err, recordings.ReplayArtifactErrorUnsupportedSchema, recordings.ErrUnsupportedPortableArtifactSchema,
	)
	diagnostic := replayArtifactDiagnostic(t, err)
	if diagnostic.Code != recordings.ArtifactDiagnosticUnsupportedSchema ||
		diagnostic.Area != "schemaVersion" ||
		diagnostic.Path != "schemaVersion" ||
		diagnostic.Message == "" ||
		len(diagnostic.SupportedVersions) != 1 || diagnostic.SupportedVersions[0] != string(recordings.ArtifactSchemaV1) {
		t.Fatalf("unsupported-schema diagnostic = %#v, want structured retry facts", diagnostic)
	}

	diagnostic.SupportedVersions[0] = "mutated"
	_, err = replayArtifacts.ValidateArtifact(recordings.ValidateArtifactRequest{Artifact: built.Artifact})
	second := replayArtifactDiagnostic(t, err)
	if len(second.SupportedVersions) != 1 || second.SupportedVersions[0] != string(recordings.ArtifactSchemaV1) {
		t.Fatalf("supported versions after mutating earlier diagnostic = %#v, want detached values", second.SupportedVersions)
	}
}

// TestRecordingReplayArtifacts_InvalidOrder proves an artifact whose summary
// cursors no longer match its canonical event order returns the distinct
// invalid-order classification.
func TestRecordingReplayArtifacts_InvalidOrder(t *testing.T) {
	t.Parallel()
	service, replayArtifacts := newTestRecordingReplayArtifacts(t)
	recordingID := finalizedReplayArtifactRecording(t, service, "invalid-order")
	built, err := replayArtifacts.BuildArtifact(recordings.BuildArtifactRequest{RecordingID: recordingID})
	if err != nil {
		t.Fatalf("BuildArtifact: %v", err)
	}
	if built.Artifact.Summary.FirstCursor == nil {
		t.Fatal("BuildArtifact() Summary.FirstCursor = nil, want a cursor to corrupt")
	}
	built.Artifact.Summary.FirstCursor.Sequence++

	_, err = replayArtifacts.ValidateArtifact(recordings.ValidateArtifactRequest{Artifact: built.Artifact})
	assertReplayArtifactErrorKind(
		t, err, recordings.ReplayArtifactErrorInvalidOrder, recordings.ErrInvalidPortableArtifactOrder,
	)
	diagnostic := replayArtifactDiagnostic(t, err)
	if diagnostic.Code != recordings.ArtifactDiagnosticInvalidOrder || diagnostic.Area != "events" || diagnostic.Path != "events" || diagnostic.Message == "" {
		t.Fatalf("invalid-order diagnostic = %#v, want structured order facts", diagnostic)
	}
}

// TestRecordingReplayArtifacts_InvalidIntegrity proves an artifact whose
// digest no longer matches its own content returns the distinct
// invalid-integrity classification.
func TestRecordingReplayArtifacts_InvalidIntegrity(t *testing.T) {
	t.Parallel()
	service, replayArtifacts := newTestRecordingReplayArtifacts(t)
	recordingID := finalizedReplayArtifactRecording(t, service, "invalid-integrity")
	built, err := replayArtifacts.BuildArtifact(recordings.BuildArtifactRequest{RecordingID: recordingID})
	if err != nil {
		t.Fatalf("BuildArtifact: %v", err)
	}
	built.Artifact.Integrity.Digest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

	_, err = replayArtifacts.ValidateArtifact(recordings.ValidateArtifactRequest{Artifact: built.Artifact})
	assertReplayArtifactErrorKind(
		t, err, recordings.ReplayArtifactErrorInvalidIntegrity, recordings.ErrInvalidPortableArtifactIntegrity,
	)
	diagnostic := replayArtifactDiagnostic(t, err)
	if diagnostic.Code != recordings.ArtifactDiagnosticInvalidIntegrity || diagnostic.Area != "integrity" || diagnostic.Path != "integrity.digest" || diagnostic.Message == "" {
		t.Fatalf("invalid-integrity diagnostic = %#v, want structured integrity facts", diagnostic)
	}
}

// TestRecordingReplayArtifacts_MalformedDecode proves empty, truncated,
// trailing-document, and unknown-field payloads all return the capability's
// invalid classification with no partial artifact result.
func TestRecordingReplayArtifacts_MalformedDecode(t *testing.T) {
	t.Parallel()
	service, replayArtifacts := newTestRecordingReplayArtifacts(t)
	recordingID := finalizedReplayArtifactRecording(t, service, "malformed-decode")
	built, err := replayArtifacts.BuildArtifact(recordings.BuildArtifactRequest{RecordingID: recordingID})
	if err != nil {
		t.Fatalf("BuildArtifact: %v", err)
	}
	encoded, err := replayArtifacts.EncodeArtifact(recordings.EncodeArtifactRequest{Artifact: built.Artifact})
	if err != nil {
		t.Fatalf("EncodeArtifact: %v", err)
	}

	cases := map[string][]byte{
		"empty":             nil,
		"truncated":         []byte(`{"schemaVersion":"recordings.portable-artifact.v1"`),
		"trailing document": append(append([]byte{}, encoded.Payload...), []byte(`{}`)...),
		"unknown field":     []byte(`{"schemaVersion":"recordings.portable-artifact.v1","unexpected":true}`),
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			result, err := replayArtifacts.DecodeArtifact(recordings.DecodeArtifactRequest{Payload: payload})
			assertReplayArtifactErrorKind(t, err, recordings.ReplayArtifactErrorInvalid, recordings.ErrInvalidPortableArtifact)
			diagnostic := replayArtifactDiagnostic(t, err)
			if diagnostic.Code != recordings.ArtifactDiagnosticMalformed || diagnostic.Area != "artifact" || diagnostic.Path != "artifact" || diagnostic.Message == "" {
				t.Fatalf("DecodeArtifact(%s) diagnostic = %#v, want structured malformed facts", name, diagnostic)
			}
			if result.Artifact.SchemaVersion != "" || len(result.Artifact.Events) != 0 {
				t.Fatalf("DecodeArtifact(%s) result = %#v, want zero value on failure", name, result.Artifact)
			}
		})
	}
}

// TestRecordingReplayArtifacts_ExportFailureLeavesNoPartialArtifact proves a
// failed atomic publication (writing to a destination that is itself a
// directory) reports the export-failed classification and leaves no
// partially readable public artifact behind.
func TestRecordingReplayArtifacts_ExportFailureLeavesNoPartialArtifact(t *testing.T) {
	t.Parallel()
	service, replayArtifacts := newTestRecordingReplayArtifacts(t)
	destination := filepath.Join(t.TempDir(), "destination-is-directory")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-export-failure"}
	bound, err := service.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "recording-export-failure",
		Artifact:    recordings.RecordingArtifactReference(destination),
		Scope:       scope,
	})
	if err != nil {
		t.Fatalf("BindRecording: %v", err)
	}
	recordedAt := time.Unix(1_700_000_000, 0).UTC()
	// The durable JSONL replay-artifact writer exercised by FinishRecording's
	// final flush requires the first recorded event to carry decodable
	// Factory snapshot config.
	event := recordings.CanonicalEvent{
		ID:         "recording-export-failure-event",
		Kind:       recordings.CanonicalEventKind(recordings.FactoryEventTypeRunRequest),
		Scope:      scope,
		RecordedAt: recordedAt,
		Payload:    `{"factory":{"id":"recording-export-failure"},"recordedAt":"` + recordedAt.Format(time.RFC3339Nano) + `"}`,
		Cursor:     recordings.CanonicalEventCursor{StreamGenerationID: "generation-export-failure"},
	}
	if _, err := service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event:       event,
	}); err != nil {
		t.Fatalf("RecordRecordingEvent: %v", err)
	}
	if _, err := service.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_001, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording: %v", err)
	}
	recordingID := recordings.ReplayRecordingID(bound.Status.RecordingID)

	_, err = replayArtifacts.ExportArtifact(context.Background(), recordings.ExportArtifactRequest{RecordingID: recordingID})
	assertReplayArtifactErrorKind(
		t, err, recordings.ReplayArtifactErrorExportFailed, recordings.ErrPortableArtifactExportFailed,
	)

	_, err = replayArtifacts.ReadArtifact(context.Background(), recordings.ReadArtifactRequest{
		RecordingID: recordingID,
		Reference:   recordings.ArtifactReference(destination),
	})
	var replayArtifactErr *recordings.ReplayArtifactError
	if !errors.As(err, &replayArtifactErr) ||
		(replayArtifactErr.Kind != recordings.ReplayArtifactErrorUnavailable &&
			replayArtifactErr.Kind != recordings.ReplayArtifactErrorInvalid) {
		t.Fatalf(
			"ReadArtifact() after failed export error = %v, want ReplayArtifactError with kind %q or %q",
			err, recordings.ReplayArtifactErrorUnavailable, recordings.ReplayArtifactErrorInvalid,
		)
	}
}

func assertReplayArtifactErrorKind(
	t *testing.T, err error, wantKind recordings.ReplayArtifactErrorKind, wantSentinel error,
) {
	t.Helper()
	var replayArtifactErr *recordings.ReplayArtifactError
	if !errors.As(err, &replayArtifactErr) || replayArtifactErr.Kind != wantKind {
		t.Fatalf("error = %v, want ReplayArtifactError with kind %q", err, wantKind)
	}
	if !errors.Is(err, wantSentinel) {
		t.Fatalf("error does not unwrap to %v: %v", wantSentinel, err)
	}
}

func replayArtifactDiagnostic(t *testing.T, err error) *recordings.ArtifactDiagnostic {
	t.Helper()
	var replayArtifactErr *recordings.ReplayArtifactError
	if !errors.As(err, &replayArtifactErr) || replayArtifactErr.Diagnostic == nil {
		t.Fatalf("error = %v, want ReplayArtifactError with a structured diagnostic", err)
	}
	return replayArtifactErr.Diagnostic
}

// TestRecordingReplayArtifacts_DetachedResults proves mutating a returned
// replay-fact or artifact-summary slice cannot mutate a later read.
func TestRecordingReplayArtifacts_DetachedResults(t *testing.T) {
	t.Parallel()
	service, replayArtifacts := newTestRecordingReplayArtifacts(t)
	recordingID := finalizedReplayArtifactRecording(t, service, "detached-results")

	first, err := replayArtifacts.LoadReplay(recordings.LoadReplayRequest{RecordingID: recordingID})
	if err != nil {
		t.Fatalf("LoadReplay: %v", err)
	}
	first.Replay.Events[0].Payload = "mutated"
	first.Replay.Events[0].ID = "mutated-id"

	second, err := replayArtifacts.LoadReplay(recordings.LoadReplayRequest{RecordingID: recordingID})
	if err != nil {
		t.Fatalf("LoadReplay: %v", err)
	}
	if second.Replay.Events[0].Payload == "mutated" || second.Replay.Events[0].ID == "mutated-id" {
		t.Fatalf("LoadReplay() second call observed mutation from first result: %#v", second.Replay.Events[0])
	}

	built, err := replayArtifacts.BuildArtifact(recordings.BuildArtifactRequest{RecordingID: recordingID})
	if err != nil {
		t.Fatalf("BuildArtifact: %v", err)
	}
	built.Artifact.Events[0].Payload = "mutated"
	built.Artifact.Summary.Failures = append(built.Artifact.Summary.Failures, recordings.ArtifactFailure{Code: "injected"})

	rebuilt, err := replayArtifacts.BuildArtifact(recordings.BuildArtifactRequest{RecordingID: recordingID})
	if err != nil {
		t.Fatalf("BuildArtifact (rebuild): %v", err)
	}
	if rebuilt.Artifact.Events[0].Payload == "mutated" || len(rebuilt.Artifact.Summary.Failures) != 0 {
		t.Fatalf("BuildArtifact() rebuild observed mutation from earlier result: %#v", rebuilt.Artifact)
	}
}

// narrowReplayArtifactsFake implements only recordings.RecordingReplayArtifacts,
// proving peers can fake the capability without implementing recording
// lifecycle, event streaming, projection query, or runtime execution
// behavior from the broader recordings.Service surface.
type narrowReplayArtifactsFake struct {
	replay   recordings.ReplayFacts
	artifact recordings.ArtifactEnvelope
}

var _ recordings.RecordingReplayArtifacts = (*narrowReplayArtifactsFake)(nil)

func (fake *narrowReplayArtifactsFake) LoadReplayInput(
	recordings.LoadReplayInputRequest,
) (recordings.LoadReplayInputResult, error) {
	return recordings.LoadReplayInputResult{Portable: &recordings.ReplayInputPortableRecording{
		RecordingKind: "narrow-fake",
		Session:       recordings.ReplayInputSessionSummary{ID: "narrow-fake-session"},
	}}, nil
}

func (fake *narrowReplayArtifactsFake) LoadReplay(
	request recordings.LoadReplayRequest,
) (recordings.LoadReplayResult, error) {
	if request.RecordingID != fake.replay.RecordingID {
		return recordings.LoadReplayResult{}, recordings.ErrReplayRecordingNotFound
	}
	return recordings.LoadReplayResult{Replay: fake.replay}, nil
}

func (fake *narrowReplayArtifactsFake) BuildArtifact(
	recordings.BuildArtifactRequest,
) (recordings.BuildArtifactResult, error) {
	return recordings.BuildArtifactResult{Artifact: fake.artifact}, nil
}

func (fake *narrowReplayArtifactsFake) ValidateArtifact(
	recordings.ValidateArtifactRequest,
) (recordings.ValidateArtifactResult, error) {
	return recordings.ValidateArtifactResult{Summary: fake.artifact.Summary}, nil
}

func (fake *narrowReplayArtifactsFake) EncodeArtifact(
	recordings.EncodeArtifactRequest,
) (recordings.EncodeArtifactResult, error) {
	return recordings.EncodeArtifactResult{Payload: []byte("encoded")}, nil
}

func (fake *narrowReplayArtifactsFake) DecodeArtifact(
	recordings.DecodeArtifactRequest,
) (recordings.DecodeArtifactResult, error) {
	return recordings.DecodeArtifactResult{Artifact: fake.artifact}, nil
}

func (fake *narrowReplayArtifactsFake) SummarizeArtifact(
	recordings.SummarizeArtifactRequest,
) (recordings.SummarizeArtifactResult, error) {
	return recordings.SummarizeArtifactResult{Summary: fake.artifact.Summary}, nil
}

func (fake *narrowReplayArtifactsFake) ExportArtifact(
	context.Context, recordings.ExportArtifactRequest,
) (recordings.ExportArtifactResult, error) {
	return recordings.ExportArtifactResult{Reference: fake.artifact.Summary.Reference, Artifact: fake.artifact}, nil
}

func (fake *narrowReplayArtifactsFake) ReadArtifact(
	context.Context, recordings.ReadArtifactRequest,
) (recordings.ReadArtifactResult, error) {
	return recordings.ReadArtifactResult{Artifact: fake.artifact}, nil
}

func TestRecordingReplayArtifacts_NarrowFakeConsumption(t *testing.T) {
	t.Parallel()
	fake := &narrowReplayArtifactsFake{
		replay: recordings.ReplayFacts{
			RecordingID: "narrow-fake",
			Events:      []recordings.ReplayEvent{{ID: "narrow-fake-event"}},
		},
		artifact: recordings.ArtifactEnvelope{
			SchemaVersion: recordings.ArtifactSchemaV1,
			Summary:       recordings.ArtifactSummary{RecordingID: "narrow-fake", EventCount: 1, Available: true},
		},
	}
	var replayArtifacts recordings.RecordingReplayArtifacts = fake
	input, err := replayArtifacts.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "narrow-fake"})
	if err != nil {
		t.Fatalf("LoadReplayInput() error = %v", err)
	}
	if input.Portable == nil || input.Portable.Session.ID != "narrow-fake-session" {
		t.Fatalf("LoadReplayInput() = %#v, want directly owned portable replay input", input)
	}

	loaded, err := replayArtifacts.LoadReplay(recordings.LoadReplayRequest{RecordingID: "narrow-fake"})
	if err != nil {
		t.Fatalf("LoadReplay() error = %v", err)
	}
	if len(loaded.Replay.Events) != 1 || loaded.Replay.Events[0].ID != "narrow-fake-event" {
		t.Fatalf("LoadReplay() = %#v", loaded.Replay)
	}

	built, err := replayArtifacts.BuildArtifact(recordings.BuildArtifactRequest{RecordingID: "narrow-fake"})
	if err != nil {
		t.Fatalf("BuildArtifact() error = %v", err)
	}
	exported, err := replayArtifacts.ExportArtifact(context.Background(), recordings.ExportArtifactRequest{
		RecordingID: "narrow-fake",
	})
	if err != nil {
		t.Fatalf("ExportArtifact() error = %v", err)
	}
	if !reflect.DeepEqual(exported.Artifact.Summary, built.Artifact.Summary) {
		t.Fatalf("ExportArtifact() Summary = %#v, want %#v", exported.Artifact.Summary, built.Artifact.Summary)
	}
}
