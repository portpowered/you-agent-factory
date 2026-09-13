package replay

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestDecodeReplayV2StructuralCorruptionIsTypedAndAtomic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		build     func(t *testing.T) []byte
		wantCode  recordings.ReplayArtifactDiagnosticCode
		wantPath  string
		wantEvent string
		wantKind  recordings.ReplayArtifactErrorKind
	}{
		{
			name: "malformed syntax",
			build: func(t *testing.T) []byte {
				artifact := testReplayArtifact(t)
				header := mustReplayV2Header(t, artifact, replayV2TestSessionID)
				return append(header, []byte(`{"recordType":"event","event":`+"\n")...)
			},
			wantCode:  recordings.ReplayArtifactDiagnosticMalformed,
			wantPath:  "events[0]",
			wantEvent: "unknown",
			wantKind:  recordings.ReplayArtifactErrorCorruptInput,
		},
		{
			name: "duplicate event identity",
			build: func(t *testing.T) []byte {
				artifact := legacyIntegrityArtifact(t)
				artifact.Events[1].Id = artifact.Events[0].Id
				return replayV2FixtureData(t, artifact, artifact.RecordedAt)
			},
			wantCode:  recordings.ReplayArtifactDiagnosticInvalidIdentity,
			wantPath:  "events[1]",
			wantEvent: replayRunStartedEventID,
			wantKind:  recordings.ReplayArtifactErrorCorruptInput,
		},
		{
			name: "event order",
			build: func(t *testing.T) []byte {
				artifact := legacyIntegrityArtifact(t)
				artifact.Events[1].Context.Sequence = 17
				return replayV2FixtureData(t, artifact, artifact.RecordedAt)
			},
			wantCode:  recordings.ReplayArtifactDiagnosticInvalidOrder,
			wantPath:  "events[1]",
			wantEvent: "factory-event/work-request/request-1",
			wantKind:  recordings.ReplayArtifactErrorInvalidOrder,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			data := test.build(t)
			artifact, stream, err := DecodeReplayV2(data, testFactorySnapshotDecoder)
			if artifact != nil || stream != nil {
				t.Fatalf("DecodeReplayV2() result = artifact=%#v stream=%#v, want no partial result", artifact, stream)
			}
			assertReplayStructuralDiagnostic(t, err, test.wantCode, test.wantPath, test.wantEvent, test.wantKind)
		})
	}
}

func TestLoadReplayV2ForeignReferenceReturnsSafeFirstCorruptDiagnostic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(t *testing.T, artifact *interfaces.ReplayArtifact)
		wantCode  recordings.ReplayArtifactDiagnosticCode
		wantEvent string
		wantKind  recordings.ReplayArtifactErrorKind
	}{
		{
			name: "foreign dispatch work",
			mutate: func(t *testing.T, artifact *interfaces.ReplayArtifact) {
				var payload interfaces.DispatchRequestEventPayload
				if err := artifact.Events[2].DecodePayload(&payload); err != nil {
					t.Fatalf("decode dispatch payload: %v", err)
				}
				payload.Inputs = []interfaces.DispatchConsumedWorkRef{{WorkID: "foreign-work"}}
				artifact.Events[2].Payload, _ = json.Marshal(payload)
			},
			wantCode:  recordings.ReplayArtifactDiagnosticForeignReference,
			wantEvent: "factory-event/dispatch-created/dispatch-1",
			wantKind:  recordings.ReplayArtifactErrorForeign,
		},
		{
			name: "missing dispatch work",
			mutate: func(t *testing.T, artifact *interfaces.ReplayArtifact) {
				var payload interfaces.DispatchRequestEventPayload
				if err := artifact.Events[2].DecodePayload(&payload); err != nil {
					t.Fatalf("decode dispatch payload: %v", err)
				}
				payload.Inputs = []interfaces.DispatchConsumedWorkRef{{}}
				artifact.Events[2].Context.WorkIDs = nil
				artifact.Events[2].Payload, _ = json.Marshal(payload)
			},
			wantCode:  recordings.ReplayArtifactDiagnosticMissingReference,
			wantEvent: "factory-event/dispatch-created/dispatch-1",
			wantKind:  recordings.ReplayArtifactErrorCorruptInput,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			artifact := legacyIntegrityArtifact(t)
			test.mutate(t, artifact)
			data := replayV2FixtureData(t, artifact, artifact.RecordedAt)
			path := filepath.Join(t.TempDir(), "corrupt.jsonl")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatalf("write replay input: %v", err)
			}

			loaded, metadata, err := LoadWithMetadata(testReplayStorage(), path, testFactorySnapshotDecoder)
			if loaded != nil || metadata.V2 != nil {
				t.Fatalf("LoadWithMetadata() result = artifact=%#v metadata=%#v, want empty result", loaded, metadata)
			}
			assertReplayStructuralDiagnostic(
				t,
				err,
				test.wantCode,
				"events[2]",
				test.wantEvent,
				test.wantKind,
			)
			if strings.Contains(err.Error(), path) || strings.Contains(err.Error(), "foreign-work") {
				t.Fatalf("LoadWithMetadata() leaked private or payload data: %v", err)
			}
		})
	}
}

func TestReduceReplayEventsPreservesRecordedFailureAndLaterFacts(t *testing.T) {
	t.Parallel()

	artifact := testReplayArtifact(
		t,
		replayDispatchCompletedEvent(t, "completion-failed", workerexecution.WorkResult{
			DispatchID: "dispatch-failed",
			Outcome:    workerexecution.OutcomeFailed,
			Error:      `{"decision":"not-a-live-decision-envelope"}`,
		}, 2),
		replayWorkRequestEvent(t, "later-request", 3, "api", []factoryapi.Work{{
			Name:         "later-work",
			WorkId:       stringPtrIfNotEmpty("work-later"),
			WorkTypeName: stringPtrIfNotEmpty("task"),
			TraceId:      stringPtrIfNotEmpty("trace-later"),
		}}, nil),
	)

	reduced, err := reduceReplayEvents(artifact, testFactorySnapshotDecoder, testRuntimeConfigDecoder)
	if err != nil {
		t.Fatalf("reduceReplayEvents() error = %v", err)
	}
	if len(reduced.Completions) != 1 {
		t.Fatalf("completion count = %d, want one recorded failure", len(reduced.Completions))
	}
	if reduced.Completions[0].result.Outcome != workerexecution.OutcomeFailed ||
		reduced.Completions[0].result.Error != `{"decision":"not-a-live-decision-envelope"}` {
		t.Fatalf("recorded completion = %#v, want failed historical result", reduced.Completions[0].result)
	}
	if len(reduced.Submissions) != 1 || len(reduced.Submissions[0].request.Works) != 1 ||
		reduced.Submissions[0].request.Works[0].WorkID != "work-later" {
		t.Fatalf("later submission = %#v, want visible later Work", reduced.Submissions)
	}
}

func legacyIntegrityArtifact(t *testing.T) *interfaces.ReplayArtifact {
	t.Helper()
	return testReplayArtifact(
		t,
		replayWorkRequestEvent(t, "request-1", 1, "api", []factoryapi.Work{{
			Name:         "task-1",
			WorkId:       stringPtrIfNotEmpty("work-1"),
			WorkTypeName: stringPtrIfNotEmpty("task"),
			TraceId:      stringPtrIfNotEmpty("trace-1"),
		}}, nil),
		replayDispatchCreatedEvent(t, work.WorkDispatch{
			DispatchID:   "dispatch-1",
			TransitionID: "process",
			Execution: work.ExecutionMetadata{
				WorkIDs: []string{"work-1"},
			},
		}, 2),
	)
}

func assertReplayStructuralDiagnostic(
	t *testing.T,
	err error,
	wantCode recordings.ReplayArtifactDiagnosticCode,
	wantPath string,
	wantEvent string,
	wantKind recordings.ReplayArtifactErrorKind,
) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want typed structural failure")
	}
	var replayErr *recordings.ReplayArtifactError
	if !errors.As(err, &replayErr) {
		t.Fatalf("error = %v, want ReplayArtifactError", err)
	}
	if replayErr.Kind != wantKind {
		t.Fatalf("error kind = %q, want %q", replayErr.Kind, wantKind)
	}
	diagnostic := replayErr.Diagnostic
	if diagnostic.Code != wantCode || diagnostic.Area != "events" || diagnostic.Path != wantPath || diagnostic.Action != recordings.ReplayArtifactStructuralRepairAction {
		t.Fatalf("diagnostic = %#v, want code=%q area=events path=%q action=%q", diagnostic, wantCode, wantPath, recordings.ReplayArtifactStructuralRepairAction)
	}
	if !strings.Contains(diagnostic.Message, `event "`+wantEvent+`"`) {
		t.Fatalf("diagnostic message = %q, want safe event %q", diagnostic.Message, wantEvent)
	}
	if !errors.Is(err, recordings.ErrCorruptReplayInput) {
		t.Fatalf("error = %v, want ErrCorruptReplayInput", err)
	}
}
