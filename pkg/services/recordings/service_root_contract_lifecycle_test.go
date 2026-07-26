package recordings_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestRecordingLifecycleRootContract_StatusAndDetachedFacts(t *testing.T) {
	t.Parallel()

	service := recordings.Service(&peerRootServiceFake{})
	assertLifecycleBindingIsNonDestructive(t, service)
	recordingID := assertLifecycleSuccessPath(t, service)
	assertLifecycleInputFailures(t, service)
	assertLifecycleFailureFactsAreDetached(t, service)
	assertLifecyclePostFinishRejection(t, service, recordingID)
}

func assertLifecycleSuccessPath(t *testing.T, service recordings.Service) recordings.RecordingID {
	t.Helper()
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-1"}
	bound, err := service.BindRecording(recordings.BindRecordingRequest{
		Artifact: "artifact:session-1",
		Scope:    scope,
	})
	if err != nil {
		t.Fatalf("BindRecording success path: %v", err)
	}
	if bound.Status.RecordingID == "" || bound.Status.State != recordings.RecordingActive {
		t.Fatalf("BindRecording status = %#v, want active bound recording", bound.Status)
	}
	event := lifecycleEvent("event-1", 4, scope)
	recorded, err := service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event:       event,
	})
	if err != nil || recorded.Status.AcceptedEvents != 1 {
		t.Fatalf("RecordRecordingEvent = (%#v, %v), want one accepted event", recorded, err)
	}
	flushed, err := service.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: bound.Status.RecordingID,
	})
	if err != nil || flushed.Status.FlushedThrough == nil ||
		*flushed.Status.FlushedThrough != event.Cursor {
		t.Fatalf("FlushRecording = (%#v, %v), want event cursor", flushed, err)
	}
	finishedAt := time.Unix(1_700_000_000, 0).UTC()
	finished, err := service.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  finishedAt,
	})
	if err != nil || finished.Status.State != recordings.RecordingFinalized ||
		finished.Status.FinalizedAt == nil || !finished.Status.FinalizedAt.Equal(finishedAt) {
		t.Fatalf("FinishRecording = (%#v, %v), want finalized status", finished, err)
	}
	return bound.Status.RecordingID
}

func assertLifecycleInputFailures(
	t *testing.T,
	service recordings.Service,
) {
	t.Helper()
	if _, err := service.BindRecording(recordings.BindRecordingRequest{}); !errors.Is(
		err,
		recordings.ErrMissingRecordingTarget,
	) {
		t.Fatalf("missing artifact error = %v, want ErrMissingRecordingTarget", err)
	}
	if _, err := service.BindRecording(recordings.BindRecordingRequest{
		Artifact: "artifact:bad-scope",
		Scope:    recordings.CanonicalEventScope{FactorySessionID: " "},
	}); !errors.Is(err, recordings.ErrInvalidRecordingScope) {
		t.Fatalf("invalid scope error = %v, want ErrInvalidRecordingScope", err)
	}
	active, err := service.BindRecording(recordings.BindRecordingRequest{
		Artifact: "artifact:association",
		Scope:    recordings.CanonicalEventScope{FactorySessionID: "session-expected"},
	})
	if err != nil {
		t.Fatalf("BindRecording association path: %v", err)
	}
	assertMalformedLifecycleEventsDoNotMutate(t, service, active.Status)
	wrongScope := recordings.CanonicalEventScope{FactorySessionID: "session-other"}
	if _, err := service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: active.Status.RecordingID,
		Event:       lifecycleEvent("wrong-session", 5, wrongScope),
	}); !errors.Is(err, recordings.ErrInvalidRecordingEvent) {
		t.Fatalf("wrong-scope event error = %v, want ErrInvalidRecordingEvent", err)
	}
	if _, err := service.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: "missing-id",
	}); !errors.Is(err, recordings.ErrMissingRecordingTarget) {
		t.Fatalf("missing recording error = %v, want ErrMissingRecordingTarget", err)
	}
	if _, err := service.RecordRecordingError(recordings.RecordRecordingErrorRequest{
		RecordingID: active.Status.RecordingID,
	}); !errors.Is(err, recordings.ErrInvalidRecordingFailure) {
		t.Fatalf("empty failure error = %v, want ErrInvalidRecordingFailure", err)
	}
}

func assertMalformedLifecycleEventsDoNotMutate(
	t *testing.T,
	service recordings.Service,
	status recordings.RecordingStatusFacts,
) {
	t.Helper()
	valid := lifecycleEvent("valid-event", 0, status.Scope)
	tests := map[string]func(*recordings.CanonicalEvent){
		"missing identity":    func(event *recordings.CanonicalEvent) { event.ID = "" },
		"whitespace identity": func(event *recordings.CanonicalEvent) { event.ID = " " },
		"missing kind":        func(event *recordings.CanonicalEvent) { event.Kind = "" },
		"whitespace kind":     func(event *recordings.CanonicalEvent) { event.Kind = " " },
		"missing timestamp": func(event *recordings.CanonicalEvent) {
			event.RecordedAt = time.Time{}
		},
		"invalid JSON": func(event *recordings.CanonicalEvent) { event.Payload = "{" },
	}
	for name, mutate := range tests {
		event := valid
		mutate(&event)
		if _, err := service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
			RecordingID: status.RecordingID,
			Event:       event,
		}); !errors.Is(err, recordings.ErrInvalidRecordingEvent) {
			t.Fatalf("%s error = %v, want ErrInvalidRecordingEvent", name, err)
		}
		got := lifecycleStatus(t, service, status.RecordingID)
		if got.AcceptedEvents != status.AcceptedEvents || got.LastEvent != nil {
			t.Fatalf("%s mutated recording status: %#v", name, got)
		}
	}
}

func assertLifecycleFailureFactsAreDetached(t *testing.T, service recordings.Service) {
	t.Helper()
	bound, err := service.BindRecording(recordings.BindRecordingRequest{
		Artifact: "artifact:failing",
		Scope:    recordings.CanonicalEventScope{FactorySessionID: "session-failing"},
	})
	if err != nil {
		t.Fatalf("BindRecording failing path: %v", err)
	}
	recorded, err := service.RecordRecordingError(recordings.RecordRecordingErrorRequest{
		RecordingID: bound.Status.RecordingID,
		Failure: recordings.RecordingFailure{
			Code: "producer_failed", Message: "producer boundary failure",
		},
	})
	if err != nil || recorded.Status.State != recordings.RecordingFailed ||
		len(recorded.Status.Failures) != 1 {
		t.Fatalf("RecordRecordingError = (%#v, %v), want failed status", recorded, err)
	}
	recorded.Status.Failures[0].Message = "caller mutation"
	status, err := service.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: bound.Status.RecordingID,
	})
	if err != nil || status.Status.Failures[0].Message != "producer boundary failure" {
		t.Fatalf("QueryRecordingStatus = (%#v, %v), want detached failure", status, err)
	}
}

func assertLifecycleBindingIsNonDestructive(t *testing.T, service recordings.Service) {
	t.Helper()
	assertGeneratedLifecycleIDDoesNotCollide(t, service)
	request := recordings.BindRecordingRequest{
		RecordingID: "recording-stable",
		Artifact:    "artifact:stable",
		Scope: recordings.CanonicalEventScope{
			FactorySessionID: "session-stable",
		},
	}
	bound, err := service.BindRecording(request)
	if err != nil {
		t.Fatalf("BindRecording stable: %v", err)
	}
	event := lifecycleEvent("event-stable", 7, request.Scope)
	if _, err := service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event:       event,
	}); err != nil {
		t.Fatalf("RecordRecordingEvent stable: %v", err)
	}
	if _, err := service.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: bound.Status.RecordingID,
	}); err != nil {
		t.Fatalf("FlushRecording stable: %v", err)
	}
	active := lifecycleStatus(t, service, bound.Status.RecordingID)
	assertIdempotentLifecycleBind(t, service, request, active, "active")
	assertConflictingLifecycleBinds(t, service, request, active, "active")

	if _, err := service.RecordRecordingError(recordings.RecordRecordingErrorRequest{
		RecordingID: bound.Status.RecordingID,
		Failure: recordings.RecordingFailure{
			Code: "producer_failed", Message: "preserve this failure",
		},
	}); err != nil {
		t.Fatalf("RecordRecordingError stable: %v", err)
	}
	if _, err := service.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_100, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording stable: %v", err)
	}
	terminal := lifecycleStatus(t, service, bound.Status.RecordingID)
	if terminal.State != recordings.RecordingFailed || terminal.FinalizedAt == nil {
		t.Fatalf("terminal status = %#v, want finalized failed recording", terminal)
	}
	assertIdempotentLifecycleBind(t, service, request, terminal, "terminal")
	assertConflictingLifecycleBinds(t, service, request, terminal, "terminal")
}

func assertGeneratedLifecycleIDDoesNotCollide(t *testing.T, service recordings.Service) {
	t.Helper()
	explicit, err := service.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "recording-1",
		Artifact:    "artifact:explicit",
	})
	if err != nil {
		t.Fatalf("BindRecording explicit generated-form ID: %v", err)
	}
	generated, err := service.BindRecording(recordings.BindRecordingRequest{
		Artifact: "artifact:generated",
	})
	if err != nil {
		t.Fatalf("BindRecording generated ID: %v", err)
	}
	if generated.Status.RecordingID == explicit.Status.RecordingID {
		t.Fatalf("generated RecordingID %q replaced an existing binding", generated.Status.RecordingID)
	}
	got := lifecycleStatus(t, service, explicit.Status.RecordingID)
	if got.Artifact != explicit.Status.Artifact {
		t.Fatalf("explicit binding after generated bind = %#v, want unchanged", got)
	}
}

func assertIdempotentLifecycleBind(
	t *testing.T,
	service recordings.Service,
	request recordings.BindRecordingRequest,
	want recordings.RecordingStatusFacts,
	phase string,
) {
	t.Helper()
	rebound, err := service.BindRecording(request)
	if err != nil || !reflect.DeepEqual(rebound.Status, want) {
		t.Fatalf(
			"BindRecording identical %s = (%#v, %v), want unchanged %#v",
			phase,
			rebound.Status,
			err,
			want,
		)
	}
}

func assertConflictingLifecycleBinds(
	t *testing.T,
	service recordings.Service,
	request recordings.BindRecordingRequest,
	want recordings.RecordingStatusFacts,
	phase string,
) {
	t.Helper()
	conflicts := []recordings.BindRecordingRequest{
		{RecordingID: request.RecordingID, Artifact: "artifact:other", Scope: request.Scope},
		{
			RecordingID: request.RecordingID,
			Artifact:    request.Artifact,
			Scope: recordings.CanonicalEventScope{
				FactorySessionID: "session-other",
			},
		},
	}
	for _, conflict := range conflicts {
		if _, err := service.BindRecording(conflict); !errors.Is(
			err,
			recordings.ErrRecordingBindingConflict,
		) {
			t.Fatalf(
				"BindRecording conflicting %s = %v, want ErrRecordingBindingConflict",
				phase,
				err,
			)
		}
	}
	got := lifecycleStatus(t, service, request.RecordingID)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf(
			"status after conflicting %s binds = %#v, want unchanged %#v",
			phase,
			got,
			want,
		)
	}
}

func lifecycleStatus(
	t *testing.T,
	service recordings.Service,
	recordingID recordings.RecordingID,
) recordings.RecordingStatusFacts {
	t.Helper()
	result, err := service.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: recordingID,
	})
	if err != nil {
		t.Fatalf("QueryRecordingStatus %q: %v", recordingID, err)
	}
	return result.Status
}

func assertLifecyclePostFinishRejection(
	t *testing.T,
	service recordings.Service,
	recordingID recordings.RecordingID,
) {
	t.Helper()
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-1"}
	_, err := service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: recordingID,
		Event:       lifecycleEvent("event-after-finish", 5, scope),
	})
	if !errors.Is(err, recordings.ErrRecordingWriteRejected) {
		t.Fatalf("post-finish write error = %v, want ErrRecordingWriteRejected", err)
	}
}

func lifecycleEvent(
	id recordings.CanonicalEventID,
	sequence recordings.CanonicalEventSequence,
	scope recordings.CanonicalEventScope,
) recordings.CanonicalEvent {
	return recordings.CanonicalEvent{
		ID: id, Sequence: sequence, Scope: scope, Kind: "WORK_REQUEST",
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-1",
			Sequence:           sequence,
		},
		RecordedAt: time.Unix(1_700_000_000+int64(sequence), 0).UTC(),
		Payload:    "{}",
	}
}

func TestPortableArtifactRootContract_RoundTripAndTypedFailures(t *testing.T) {
	t.Parallel()

	service := recordings.Service(&peerRootServiceFake{})
	recordingID := closePortableTestRecording(t, service)
	built := buildAndValidatePortableTestArtifact(t, service, recordingID)
	roundTripPortableTestArtifact(t, service, built)
	assertPortableTestArtifactFailures(t, service, built)
}

func closePortableTestRecording(
	t *testing.T,
	service recordings.Service,
) recordings.RecordingID {
	t.Helper()
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-export-1"}
	bound, err := service.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "recording-export-1",
		Artifact:    "artifact:recording-export-1",
		Scope:       scope,
	})
	if err != nil {
		t.Fatalf("BindRecording: %v", err)
	}
	for sequence := recordings.CanonicalEventSequence(4); sequence <= 5; sequence++ {
		event := lifecycleEvent("event-export", sequence, scope)
		event.ID = recordings.CanonicalEventID(string(event.ID) + "-" + string(rune('0'+sequence)))
		event.RecordedAt = time.Unix(1_700_000_000+int64(sequence), 0).UTC()
		if _, err := service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
			RecordingID: bound.Status.RecordingID,
			Event:       event,
		}); err != nil {
			t.Fatalf("RecordRecordingEvent sequence %d: %v", sequence, err)
		}
	}
	if _, err := service.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_010, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording: %v", err)
	}
	return bound.Status.RecordingID
}

func buildAndValidatePortableTestArtifact(
	t *testing.T,
	service recordings.Service,
	recordingID recordings.RecordingID,
) recordings.PortableArtifact {
	t.Helper()
	built, err := service.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: recordingID,
	})
	if err != nil {
		t.Fatalf("BuildPortableArtifact: %v", err)
	}
	artifact := built.Artifact
	if artifact.SchemaVersion != recordings.PortableArtifactSchemaV1 ||
		artifact.Summary.RecordingID != recordingID ||
		artifact.Summary.EventCount != 2 ||
		!artifact.Summary.Available {
		t.Fatalf("portable artifact facts = %#v", artifact)
	}
	if artifact.Events[0].Sequence != 4 || artifact.Events[1].Sequence != 5 {
		t.Fatalf("portable event order = %#v, want 4,5", artifact.Events)
	}
	validated, err := service.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Artifact: artifact,
	})
	if err != nil || validated.Summary.EventCount != 2 {
		t.Fatalf("ValidatePortableArtifact = (%#v, %v)", validated, err)
	}
	return artifact
}

func roundTripPortableTestArtifact(
	t *testing.T,
	service recordings.Service,
	artifact recordings.PortableArtifact,
) {
	t.Helper()
	encoded, err := service.EncodePortableArtifact(recordings.EncodePortableArtifactRequest{
		Artifact: artifact,
	})
	if err != nil || len(encoded.Payload) == 0 {
		t.Fatalf("EncodePortableArtifact = (%d bytes, %v)", len(encoded.Payload), err)
	}
	decoded, err := service.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: encoded.Payload,
	})
	if err != nil {
		t.Fatalf("DecodePortableArtifact: %v", err)
	}
	if decoded.Artifact.Integrity != artifact.Integrity ||
		decoded.Artifact.Events[0].Sequence != artifact.Events[0].Sequence ||
		decoded.Artifact.Summary.RecordingID != artifact.Summary.RecordingID ||
		decoded.Artifact.Summary.EventCount != artifact.Summary.EventCount {
		t.Fatalf("portable round trip changed public facts: %#v", decoded.Artifact)
	}
	summarized, err := service.SummarizePortableArtifact(
		recordings.SummarizePortableArtifactRequest{Artifact: decoded.Artifact},
	)
	if err != nil || summarized.Summary.EventCount != 2 {
		t.Fatalf("SummarizePortableArtifact = (%#v, %v)", summarized, err)
	}
}

func assertPortableTestArtifactFailures(
	t *testing.T,
	service recordings.Service,
	artifact recordings.PortableArtifact,
) {
	t.Helper()
	unsupported := artifact
	unsupported.SchemaVersion = "recordings.portable-artifact.v999"
	_, err := service.ValidatePortableArtifact(
		recordings.ValidatePortableArtifactRequest{Artifact: unsupported},
	)
	if !errors.Is(err, recordings.ErrUnsupportedPortableArtifactSchema) {
		t.Fatalf("unsupported schema error = %v", err)
	}

	tampered := artifact
	tampered.Events = append([]recordings.CanonicalEvent{}, artifact.Events...)
	tampered.Events[0].Payload = `{"tampered":true}`
	_, err = service.ValidatePortableArtifact(
		recordings.ValidatePortableArtifactRequest{Artifact: tampered},
	)
	if !errors.Is(err, recordings.ErrInvalidPortableArtifactIntegrity) {
		t.Fatalf("tampered integrity error = %v", err)
	}

	outOfOrder := artifact
	outOfOrder.Events = append([]recordings.CanonicalEvent{}, artifact.Events...)
	outOfOrder.Events[1].Sequence = outOfOrder.Events[0].Sequence
	outOfOrder.Events[1].Cursor.Sequence = outOfOrder.Events[1].Sequence
	_, err = service.ValidatePortableArtifact(
		recordings.ValidatePortableArtifactRequest{Artifact: outOfOrder},
	)
	if !errors.Is(err, recordings.ErrInvalidPortableArtifactOrder) {
		t.Fatalf("invalid order error = %v", err)
	}
}
