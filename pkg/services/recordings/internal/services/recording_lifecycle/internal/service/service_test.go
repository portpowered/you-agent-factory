package service_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsinternal "github.com/portpowered/infinite-you/pkg/services/recordings/internal"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
)

type unusedLedger struct {
	recordings.Ledger
}

func TestRecordingsRootSelectsAndBindsOneStableGeneratedTarget(t *testing.T) {
	t.Parallel()

	identityCalls := 0
	planner := recordinglifecycle.NewLiveRecordingTargetPlanner(
		platformclock.NewDeterministic(
			time.Date(2026, 7, 27, 15, 4, 5, 0, time.UTC),
			time.Second,
		),
		func() string {
			identityCalls++
			return "identity-" + string(rune('0'+identityCalls))
		},
		filepath.Join,
	)
	root := recordingsinternal.NewService(
		&unusedLedger{},
		recordingsinternal.NewProjectionService(),
		planner,
	)
	request := recordings.StartRecordingRequest{
		Enabled:     true,
		RecordingID: "recording-explicit",
		Scope: recordings.CanonicalEventScope{
			FactorySessionID: "session-1",
		},
		Target: recordings.RecordingTargetRequest{
			HomeDir:           filepath.Join("home", "operator"),
			ReportedSessionID: "~default",
		},
	}

	first, err := root.StartRecording(request)
	if err != nil {
		t.Fatalf("StartRecording first: %v", err)
	}
	second, err := root.StartRecording(request)
	if err != nil {
		t.Fatalf("StartRecording repeated: %v", err)
	}
	if !reflect.DeepEqual(first, second) ||
		!first.Enabled ||
		first.Status.State != recordings.RecordingActive {
		t.Fatalf("repeated binding = %#v then %#v, want same active binding", first, second)
	}
	if identityCalls != 1 {
		t.Fatalf("identity calls = %d, want one target selection", identityCalls)
	}
	want := filepath.Join(
		"home",
		"operator",
		".you-agent-factory",
		"recordings",
		"2026-07",
		"2026-07-27",
		"factory-session-~default-150405-identity-1.json",
	)
	if first.Status.Artifact != recordings.RecordingArtifactReference(want) {
		t.Fatalf("reported target = %q, want %q", first.Status.Artifact, want)
	}

	conflict := request
	conflict.Target.ReportedSessionID = "session-other"
	if _, err := root.StartRecording(conflict); !errors.Is(
		err,
		recordings.ErrRecordingBindingConflict,
	) {
		t.Fatalf("conflicting rebind error = %v, want ErrRecordingBindingConflict", err)
	}
	if identityCalls != 1 {
		t.Fatalf("conflicting rebind selected another target: %d calls", identityCalls)
	}
}

func TestRecordingsRootDisabledAndInvalidStartsAreInert(t *testing.T) {
	t.Parallel()

	targetCalls := 0
	planner := recordings.LiveRecordingTargetPlannerFunc(
		func(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
			targetCalls++
			return recordings.LiveRecordingTarget{
				ServicePath:  "service-target",
				ReportedPath: "reported-target",
			}, nil
		},
	)
	root := recordingsinternal.NewService(
		&unusedLedger{},
		recordingsinternal.NewProjectionService(),
		planner,
	)

	disabled, err := root.StartRecording(recordings.StartRecordingRequest{
		Target: recordings.RecordingTargetRequest{HomeDir: "ignored"},
	})
	if err != nil || !reflect.DeepEqual(disabled, recordings.StartRecordingResult{}) {
		t.Fatalf("disabled start = (%#v, %v), want inert result", disabled, err)
	}
	if _, err := root.StartRecording(recordings.StartRecordingRequest{
		Enabled: true,
		Scope: recordings.CanonicalEventScope{
			FactorySessionID: " ",
		},
		Target: recordings.RecordingTargetRequest{HomeDir: "ignored"},
	}); !errors.Is(err, recordings.ErrInvalidRecordingScope) {
		t.Fatalf("invalid scope error = %v, want ErrInvalidRecordingScope", err)
	}
	if _, err := root.StartRecording(recordings.StartRecordingRequest{
		Enabled: true,
	}); !errors.Is(err, recordings.ErrMissingRecordingTarget) {
		t.Fatalf("missing target error = %v, want ErrMissingRecordingTarget", err)
	}
	if targetCalls != 0 {
		t.Fatalf("target planner called %d times for inert/invalid starts", targetCalls)
	}
}

func TestRecordingsRootExplicitTargetDoesNotInvokeGeneratedTargetEffects(t *testing.T) {
	t.Parallel()

	targetCalls := 0
	planner := recordings.LiveRecordingTargetPlannerFunc(
		func(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
			targetCalls++
			return recordings.LiveRecordingTarget{}, errors.New("unexpected target generation")
		},
	)
	root := recordingsinternal.NewService(
		&unusedLedger{},
		recordingsinternal.NewProjectionService(),
		planner,
	)
	started, err := root.StartRecording(recordings.StartRecordingRequest{
		Enabled:     true,
		RecordingID: "recording-explicit",
		Scope: recordings.CanonicalEventScope{
			FactorySessionID: "session-1",
		},
		Target: recordings.RecordingTargetRequest{
			Artifact: "artifact:explicit",
		},
	})
	if err != nil || started.Status.Artifact != "artifact:explicit" {
		t.Fatalf("explicit start = (%#v, %v)", started, err)
	}
	if targetCalls != 0 {
		t.Fatalf("generated target planner called %d times for explicit target", targetCalls)
	}
}
