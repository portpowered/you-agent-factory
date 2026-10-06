package service_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	lifecycleservice "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle/internal/service"
)

type unusedLedger struct {
	recordings.Ledger
}

type namedTargetReserver struct {
	path  string
	calls int
}

func TestLifecyclePlanLiveRecordingTargetForwardsRequestAndResult(t *testing.T) {
	t.Parallel()
	request := recordings.LiveRecordingTargetRequest{
		HomeDir: "home/operator", CanonicalSessionID: "canonical-session", ReportedSessionID: "~default",
	}
	want := recordings.LiveRecordingTarget{ServicePath: "private-target", ReportedPath: "reported-target"}
	calls := 0
	planner := recordings.LiveRecordingTargetPlannerFunc(func(got recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
		calls++
		if got != request {
			t.Fatalf("planner request = %#v, want %#v", got, request)
		}
		return want, nil
	})
	owner := lifecycleservice.New(planner, nil, nil, fixedRecordingClock{})
	got, err := owner.PlanLiveRecordingTarget(request)
	if err != nil || got != want || calls != 1 {
		t.Fatalf("plan = (%#v, %v), calls=%d; want (%#v, nil), one call", got, err, calls, want)
	}
}

func TestLifecyclePlanLiveRecordingTargetPropagatesPlannerError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("target reservation unavailable")
	planner := recordings.LiveRecordingTargetPlannerFunc(func(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
		return recordings.LiveRecordingTarget{}, wantErr
	})
	owner := lifecycleservice.New(planner, nil, nil, fixedRecordingClock{})
	got, err := owner.PlanLiveRecordingTarget(recordings.LiveRecordingTargetRequest{HomeDir: "home/operator"})
	if !errors.Is(err, wantErr) || got != (recordings.LiveRecordingTarget{}) {
		t.Fatalf("plan = (%#v, %v), want empty target and planner error", got, err)
	}
}

func TestLifecyclePlanLiveRecordingTargetRejectsMissingPlanner(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		owner *lifecycleservice.Service
	}{
		{name: "nil service"},
		{name: "missing planner", owner: lifecycleservice.New(nil, nil, nil, fixedRecordingClock{})},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := test.owner.PlanLiveRecordingTarget(recordings.LiveRecordingTargetRequest{HomeDir: "home/operator"})
			if !errors.Is(err, recordings.ErrMissingRecordingTarget) || got != (recordings.LiveRecordingTarget{}) {
				t.Fatalf("plan = (%#v, %v), want empty target and ErrMissingRecordingTarget", got, err)
			}
		})
	}
}

func TestLifecycleSnapshotReportsPublicReferenceWhileWriterUsesPrivateTarget(t *testing.T) {
	t.Parallel()
	const privatePath = "/private/ledger/storage/recording-internal.json"
	const publicReference = "artifact:reported-export"
	planner := recordings.LiveRecordingTargetPlannerFunc(func(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
		return recordings.LiveRecordingTarget{ServicePath: privatePath, ReportedPath: publicReference}, nil
	})
	var writtenPath string
	owner := lifecycleservice.New(planner, func(path string, _ recordings.RecordingSnapshot) error {
		writtenPath = path
		return nil
	}, nil, fixedRecordingClock{})
	started, err := owner.StartRecording(recordings.StartRecordingRequest{
		Enabled: true, RecordingID: "recording-private-target",
		Scope:  recordings.CanonicalEventScope{FactorySessionID: "session-private-target"},
		Target: recordings.RecordingTargetRequest{HomeDir: "home/operator", ReportedSessionID: "session-private-target"},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := owner.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: started.Status.RecordingID, FinishedAt: time.Unix(1_700_000_001, 0).UTC(),
	}); err != nil {
		t.Fatalf("finish: %v", err)
	}
	snapshot, err := owner.Snapshot(started.Status.RecordingID)
	if err != nil || snapshot.Status.Artifact != publicReference || snapshot.Status.State != recordings.RecordingFinalized {
		t.Fatalf("export snapshot = %#v, %v; want finalized public reference", snapshot, err)
	}
	if writtenPath != privatePath {
		t.Fatalf("writer target = %q, want private storage target", writtenPath)
	}
}

func (reserver *namedTargetReserver) ReserveNamed(string, time.Time, string, string) (string, error) {
	reserver.calls++
	return reserver.path, nil
}

func TestRecordingsRootSelectsAndBindsOneStableGeneratedTarget(t *testing.T) {
	t.Parallel()

	canonicalID := "7d9d3fb4-6bc9-4df5-a67f-0f504f8ea3ba"
	reservedPath := filepath.Join(
		"home", "operator", ".you-agent-factory", "recordings", "2026", "07", "27", canonicalID+".json",
	)
	reserver := &namedTargetReserver{path: reservedPath}
	planner := lifecycleservice.NewTargetPlanner(
		platformclock.NewDeterministic(
			time.Date(2026, 7, 27, 15, 4, 5, 0, time.UTC),
			time.Second,
		),
		reserver,
		filepath.Join,
	)
	root := lifecycleservice.New(planner, nil, nil, fixedRecordingClock{})
	request := recordings.StartRecordingRequest{
		Enabled:     true,
		RecordingID: "recording-explicit",
		Scope: recordings.CanonicalEventScope{
			FactorySessionID: "session-1",
		},
		Target: recordings.RecordingTargetRequest{
			HomeDir:            filepath.Join("home", "operator"),
			CanonicalSessionID: canonicalID,
			ReportedSessionID:  "~default",
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
	if reserver.calls != 1 {
		t.Fatalf("reservation calls = %d, want one target selection", reserver.calls)
	}
	want := reservedPath
	if first.Status.Artifact != recordings.RecordingArtifactReference(want) {
		t.Fatalf("reported target = %q, want %q", first.Status.Artifact, want)
	}

	conflict := request
	conflict.Target.CanonicalSessionID = "550e8400-e29b-41d4-a716-446655440000"
	if _, err := root.StartRecording(conflict); !errors.Is(
		err,
		recordings.ErrRecordingBindingConflict,
	) {
		t.Fatalf("conflicting rebind error = %v, want ErrRecordingBindingConflict", err)
	}
	if reserver.calls != 1 {
		t.Fatalf("conflicting rebind selected another target: %d calls", reserver.calls)
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
	root := lifecycleservice.New(planner, nil, nil, fixedRecordingClock{})

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
	root := lifecycleservice.New(planner, nil, nil, fixedRecordingClock{})
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
