package recordings_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
)

// The root adapter is the only real component. Unexpected operations fail
// through the nil embedded owner; no runtime, writer or owner graph is built.
type lifecycleAdapterOwner struct {
	recordingswire.RecordingLifecycleOwner
	request any
	status  recordings.RecordingStatusFacts
	err     error
	enabled bool
}

func lifecycleAdapter(owner *lifecycleAdapterOwner) recordings.RecordingLifecycle {
	return recordingswire.NewService(nil, nil, owner, nil, nil, nil, nil, platformclock.Real{}, logging.NoopLogger{}, nil, nil, nil, nil, nil).(recordings.RecordingLifecycle)
}

func (owner *lifecycleAdapterOwner) StartRecording(request recordings.StartRecordingRequest) (recordings.StartRecordingResult, error) {
	owner.request = request
	return recordings.StartRecordingResult{Enabled: owner.enabled, Status: owner.status}, owner.err
}

func (owner *lifecycleAdapterOwner) BindRecording(request recordings.BindRecordingRequest) (recordings.BindRecordingResult, error) {
	owner.request = request
	return recordings.BindRecordingResult{Status: owner.status}, owner.err
}

func (owner *lifecycleAdapterOwner) RecordRecordingEvent(request recordings.RecordRecordingEventRequest) (recordings.RecordRecordingEventResult, error) {
	owner.request = request
	return recordings.RecordRecordingEventResult{Status: owner.status}, owner.err
}

func (owner *lifecycleAdapterOwner) RecordRecordingError(request recordings.RecordRecordingErrorRequest) (recordings.RecordRecordingErrorResult, error) {
	owner.request = request
	return recordings.RecordRecordingErrorResult{Status: owner.status}, owner.err
}

func (owner *lifecycleAdapterOwner) FlushRecording(request recordings.FlushRecordingRequest) (recordings.FlushRecordingResult, error) {
	owner.request = request
	return recordings.FlushRecordingResult{Status: owner.status}, owner.err
}

func (owner *lifecycleAdapterOwner) StopRecording(request recordings.StopRecordingRequest) (recordings.StopRecordingResult, error) {
	owner.request = request
	return recordings.StopRecordingResult{Status: owner.status}, owner.err
}

func (owner *lifecycleAdapterOwner) FinishRecording(request recordings.FinishRecordingRequest) (recordings.FinishRecordingResult, error) {
	owner.request = request
	return recordings.FinishRecordingResult{Status: owner.status}, owner.err
}

func (owner *lifecycleAdapterOwner) QueryRecordingStatus(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
	owner.request = request
	return recordings.RecordingStatusResult{Status: owner.status}, owner.err
}

func TestRecordingLifecycleMapsRequestsAndDetachedStatus(t *testing.T) {
	t.Parallel()
	at := time.Unix(1700000000, 0).UTC()
	cause := errors.New("producer failed")
	scope := recordings.CanonicalEventScope{FactorySessionID: "session"}
	publicScope := recordings.LifecycleScope{FactorySessionID: "session"}
	status := recordings.RecordingStatusFacts{RecordingID: "recording", Artifact: "artifact", Scope: scope, State: recordings.RecordingFailed, AcceptedEvents: 3, LastEvent: &recordings.CanonicalEventCursor{StreamGenerationID: "generation", Sequence: 2}, FlushedThrough: &recordings.CanonicalEventCursor{StreamGenerationID: "generation", Sequence: 1}, Failures: []recordings.RecordingFailure{{Code: "failed", Message: "detail", RecordedAt: at}}, FinalizedAt: &at}
	want := recordings.RecordingLifecycleResult{Status: recordings.LifecycleStatus{RecordingID: "recording", Artifact: "artifact", Scope: publicScope, State: recordings.LifecycleStateFailed, AcceptedEvents: 3, LastEvent: &recordings.LifecycleEventCursor{StreamGenerationID: "generation", Sequence: 2}, FlushedThrough: &recordings.LifecycleEventCursor{StreamGenerationID: "generation", Sequence: 1}, Failures: []recordings.LifecycleFailure{{Code: "failed", Message: "detail", RecordedAt: at}}, FinalizedAt: &at}}
	cases := []struct {
		name    string
		request any
		call    func(recordings.RecordingLifecycle) (recordings.RecordingLifecycleResult, error)
	}{
		{"Begin", recordings.StartRecordingRequest{Enabled: true, RecordingID: "recording", Scope: scope, Target: recordings.RecordingTargetRequest{Artifact: "artifact", HomeDir: "home", CanonicalSessionID: "canonical", ReportedSessionID: "reported"}, FlushInterval: time.Second}, func(cap recordings.RecordingLifecycle) (recordings.RecordingLifecycleResult, error) {
			return cap.Begin(recordings.BeginRecordingRequest{Enabled: true, RecordingID: "recording", Scope: publicScope, Artifact: "artifact", HomeDir: "home", CanonicalSessionID: "canonical", ReportedSessionID: "reported", FlushInterval: time.Second})
		}},
		{"Bind", recordings.BindRecordingRequest{RecordingID: "recording", Artifact: "artifact", Scope: scope}, func(cap recordings.RecordingLifecycle) (recordings.RecordingLifecycleResult, error) {
			return cap.Bind(recordings.BindLifecycleRequest{RecordingID: "recording", Artifact: "artifact", Scope: publicScope})
		}},
		{"AppendEvent", recordings.RecordRecordingEventRequest{RecordingID: "recording", Event: recordings.CanonicalEvent{ID: "event", Sequence: 2, FactoryTick: 3, Scope: scope, Cursor: recordings.CanonicalEventCursor{StreamGenerationID: "generation", Sequence: 2}, RecordedAt: at, Kind: "WORK_REQUEST", Payload: "{}", SourceContext: "source"}}, func(cap recordings.RecordingLifecycle) (recordings.RecordingLifecycleResult, error) {
			return cap.AppendEvent(recordings.AppendLifecycleEventRequest{RecordingID: "recording", Event: recordings.LifecycleEvent{ID: "event", Sequence: 2, FactoryTick: 3, Scope: publicScope, Cursor: recordings.LifecycleEventCursor{StreamGenerationID: "generation", Sequence: 2}, RecordedAt: at, Kind: "WORK_REQUEST", Payload: "{}", SourceContext: "source"}})
		}},
		{"RecordFailure", recordings.RecordRecordingErrorRequest{RecordingID: "recording", Failure: recordings.RecordingFailure{Code: "failed", Message: "detail", RecordedAt: at}, Cause: cause}, func(cap recordings.RecordingLifecycle) (recordings.RecordingLifecycleResult, error) {
			return cap.RecordFailure(recordings.RecordLifecycleFailureRequest{RecordingID: "recording", Failure: recordings.LifecycleFailure{Code: "failed", Message: "detail", RecordedAt: at}, Cause: cause})
		}},
		{"Flush", recordings.FlushRecordingRequest{RecordingID: "recording"}, func(cap recordings.RecordingLifecycle) (recordings.RecordingLifecycleResult, error) {
			return cap.Flush(recordings.FlushLifecycleRequest{RecordingID: "recording"})
		}},
		{"Finish", recordings.FinishRecordingRequest{RecordingID: "recording", FinishedAt: at}, func(cap recordings.RecordingLifecycle) (recordings.RecordingLifecycleResult, error) {
			return cap.Finish(recordings.FinishLifecycleRequest{RecordingID: "recording", FinishedAt: at})
		}},
		{"Status", recordings.RecordingStatusRequest{RecordingID: "recording"}, func(cap recordings.RecordingLifecycle) (recordings.RecordingLifecycleResult, error) {
			return cap.Status(recordings.LifecycleStatusRequest{RecordingID: "recording"})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			owner := &lifecycleAdapterOwner{status: status, enabled: true}
			cap := lifecycleAdapter(owner)
			got, err := tc.call(cap)
			if err != nil || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(owner.request, tc.request) {
				t.Fatalf("result/request = (%#v,%v)/%#v, want %#v/%#v", got, err, owner.request, want, tc.request)
			}
			got.Status.Failures[0].Code = "mutated"
			got.Status.LastEvent.Sequence = 99
			if status.Failures[0].Code != "failed" || status.LastEvent.Sequence != 2 {
				t.Fatal("status aliases owner facts")
			}
			owner.err = cause
			got, err = tc.call(cap)
			if !errors.Is(err, cause) {
				t.Fatalf("cause lost: %v", err)
			}
			if tc.name == "Finish" && !reflect.DeepEqual(got, want) {
				t.Fatalf("terminal status lost with cause: %#v", got)
			}
		})
	}
}

func TestRecordingLifecycleDisabledBeginAndStop(t *testing.T) {
	t.Parallel()
	owner := &lifecycleAdapterOwner{}
	cap := lifecycleAdapter(owner)
	got, err := cap.Begin(recordings.BeginRecordingRequest{})
	if err != nil || !reflect.DeepEqual(got, recordings.RecordingLifecycleResult{}) {
		t.Fatalf("disabled Begin = %#v, %v", got, err)
	}
	if !reflect.DeepEqual(owner.request, recordings.StartRecordingRequest{}) {
		t.Fatalf("disabled request = %#v", owner.request)
	}
	if err := cap.Stop(recordings.StopLifecycleRequest{RecordingID: "recording"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(owner.request, recordings.StopRecordingRequest{RecordingID: "recording"}) {
		t.Fatalf("Stop request = %#v", owner.request)
	}
	cause := errors.New("stop failed")
	owner.err = cause
	if err := cap.Stop(recordings.StopLifecycleRequest{RecordingID: "recording"}); !errors.Is(err, cause) {
		t.Fatalf("stop cause lost: %v", err)
	}
}

func TestRecordingLifecycleTranslatesOwnerClassifications(t *testing.T) {
	t.Parallel()
	cases := []struct {
		cause error
		kind  recordings.LifecycleErrorKind
	}{
		{recordings.ErrMissingRecordingTarget, recordings.LifecycleErrorInvalidTarget},
		{recordings.ErrInvalidRecordingScope, recordings.LifecycleErrorInvalidScope},
		{recordings.ErrRecordingBindingConflict, recordings.LifecycleErrorBindingConflict},
		{recordings.ErrInvalidRecordingEvent, recordings.LifecycleErrorInvalidEvent},
		{recordings.ErrInvalidRecordingFailure, recordings.LifecycleErrorInvalidFailure},
		{recordings.ErrRecordingWriteRejected, recordings.LifecycleErrorTerminal},
		{recordings.ErrInvalidRecordingTerminalMetadata, recordings.LifecycleErrorInvalidTerminalMetadata},
		{errors.New("writer failed"), recordings.LifecycleErrorWriteFailed},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			t.Parallel()
			owner := &lifecycleAdapterOwner{err: tc.cause}
			cap := lifecycleAdapter(owner)
			_, err := cap.Finish(recordings.FinishLifecycleRequest{RecordingID: "recording"})
			var typed *recordings.LifecycleError
			if !errors.Is(err, tc.cause) || !errors.As(err, &typed) || typed.Kind != tc.kind {
				t.Fatalf("classification/cause lost: %v", err)
			}
		})
	}
}

// narrowLifecycleFake implements only recordings.RecordingLifecycle, proving
// peers can fake the capability without implementing replay, artifact
// export, projection query, or event subscription behavior from the broader
// recordings.Service surface.
type narrowLifecycleFake struct {
	status recordings.LifecycleStatus
}

var _ recordings.RecordingLifecycle = (*narrowLifecycleFake)(nil)

func (fake *narrowLifecycleFake) Begin(request recordings.BeginRecordingRequest) (recordings.RecordingLifecycleResult, error) {
	if !request.Enabled {
		return recordings.RecordingLifecycleResult{}, nil
	}
	fake.status = recordings.LifecycleStatus{RecordingID: request.RecordingID, State: recordings.LifecycleStateActive}
	return recordings.RecordingLifecycleResult{Status: fake.status}, nil
}

func (fake *narrowLifecycleFake) Bind(recordings.BindLifecycleRequest) (recordings.RecordingLifecycleResult, error) {
	return recordings.RecordingLifecycleResult{Status: fake.status}, nil
}

func (fake *narrowLifecycleFake) AppendEvent(recordings.AppendLifecycleEventRequest) (recordings.RecordingLifecycleResult, error) {
	fake.status.AcceptedEvents++
	return recordings.RecordingLifecycleResult{Status: fake.status}, nil
}

func (fake *narrowLifecycleFake) RecordFailure(recordings.RecordLifecycleFailureRequest) (recordings.RecordingLifecycleResult, error) {
	fake.status.State = recordings.LifecycleStateFailed
	return recordings.RecordingLifecycleResult{Status: fake.status}, nil
}

func (fake *narrowLifecycleFake) Flush(recordings.FlushLifecycleRequest) (recordings.RecordingLifecycleResult, error) {
	return recordings.RecordingLifecycleResult{Status: fake.status}, nil
}

func (fake *narrowLifecycleFake) Stop(recordings.StopLifecycleRequest) error { return nil }

func (fake *narrowLifecycleFake) Finish(recordings.FinishLifecycleRequest) (recordings.RecordingLifecycleResult, error) {
	fake.status.State = recordings.LifecycleStateFinalized
	return recordings.RecordingLifecycleResult{Status: fake.status}, nil
}

func (fake *narrowLifecycleFake) Status(recordings.LifecycleStatusRequest) (recordings.RecordingLifecycleResult, error) {
	return recordings.RecordingLifecycleResult{Status: fake.status}, nil
}

func TestRecordingLifecycle_NarrowFakeConsumption(t *testing.T) {
	t.Parallel()
	var lifecycle recordings.RecordingLifecycle = &narrowLifecycleFake{}

	if _, err := lifecycle.Begin(recordings.BeginRecordingRequest{Enabled: true, RecordingID: "narrow-fake"}); err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if _, err := lifecycle.AppendEvent(recordings.AppendLifecycleEventRequest{RecordingID: "narrow-fake"}); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}
	result, err := lifecycle.Finish(recordings.FinishLifecycleRequest{RecordingID: "narrow-fake", FinishedAt: time.Now()})
	if err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if result.Status.State != recordings.LifecycleStateFinalized {
		t.Fatalf("Finish() State = %q, want FINALIZED", result.Status.State)
	}
}
