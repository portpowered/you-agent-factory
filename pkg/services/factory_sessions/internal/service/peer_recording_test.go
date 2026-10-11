package service

import (
	"context"
	"errors"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

type peerRecordingAssembly struct {
	openingAssembly
	current *runtimebinding.SessionState
}

type peerRecordingPlanner struct{ currentBoardTargetPlanner }

func (*peerRecordingPlanner) ReleaseReservation(runtimeartifact.Reservation) error { return nil }

type peerRecordingRollbackOwner struct {
	recordings.RuntimeScopeService
	err   error
	calls int
}

func (owner *peerRecordingRollbackOwner) ReleaseReservation(runtimeartifact.Reservation) error {
	owner.calls++
	return owner.err
}

func TestPeerRecordingRollbackWaitsForConsumersAndRetriesRelease(t *testing.T) {
	t.Parallel()
	consumerErr := errors.New("writer still owns resource")
	releaseErr := errors.New("owned removal unavailable")
	owner := &peerRecordingRollbackOwner{err: releaseErr}
	cleanup := &runtimeOpeningCleanup{}
	cleanup.Add(func() error { return consumerErr })
	cleanup.OwnRecordingTarget(recordingReservationRollback{owner: owner})
	if err := cleanup.Close(); !errors.Is(err, consumerErr) || owner.calls != 0 {
		t.Fatalf("released before consumers joined: %v calls=%d", err, owner.calls)
	}
	consumerErr = nil
	if err := cleanup.Close(); !errors.Is(err, releaseErr) || owner.calls != 1 {
		t.Fatalf("failed removal was lost: %v calls=%d", err, owner.calls)
	}
	owner.err = nil
	if err := cleanup.Close(); err != nil || owner.calls != 2 {
		t.Fatalf("release retry: %v calls=%d", err, owner.calls)
	}
	if err := cleanup.Close(); err != nil || owner.calls != 2 {
		t.Fatalf("successful release repeated: %v calls=%d", err, owner.calls)
	}
}

func TestPeerRecordingFailedLifecycleRetainsJoinForRollback(t *testing.T) {
	t.Parallel()
	startErr := errors.New("startup failed")
	joinErr := errors.New("worker join failed")
	lifecycle := &startLifecycleStub{completeErr: startErr, workerStopErr: joinErr}
	activation, err := startSessionLifecycle(t.Context(), lifecycle, nil, false)
	if activation == nil || !errors.Is(err, startErr) || !errors.Is(err, joinErr) {
		t.Fatalf("failed startup lost cleanup ownership: %v", err)
	}
	owner := &peerRecordingRollbackOwner{}
	cleanup := &runtimeOpeningCleanup{}
	cleanup.Add(func() error { return activation.Abort(t.Context()) })
	cleanup.OwnRecordingTarget(recordingReservationRollback{owner: owner})
	if err := cleanup.Close(); !errors.Is(err, joinErr) || owner.calls != 0 {
		t.Fatalf("released before worker joined: %v calls=%d", err, owner.calls)
	}
	lifecycle.workerStopErr = nil
	if err := cleanup.Close(); err != nil || owner.calls != 1 {
		t.Fatalf("joined retry failed to release: %v calls=%d", err, owner.calls)
	}
}

func TestPeerRecordingOpeningRetainsProfile(t *testing.T) {
	t.Parallel()
	selection := &factorysessions.SessionRuntimeSelection{SystemConfigHome: "selected-profile"}
	opening := &sessionRuntimeOpening{
		sessionSelection: selection,
		configured:       preparedRuntime{Recordings: recordings.RuntimeSelection{RecordPath: "host.json"}},
	}
	state := &runtimebinding.SessionState{}
	opening.bindSelectedState(state)
	selection.SystemConfigHome = "changed"
	if state.RecordingHome != "selected-profile" || state.CurrentBoardRecordPath != "host.json" {
		t.Fatalf("retained profile=%q artifact=%q", state.RecordingHome, state.CurrentBoardRecordPath)
	}
	(&sessionRuntimeOpening{}).bindSelectedState(state)
	if state.RecordingHome != "" || state.CurrentBoardRecordPath != "" {
		t.Fatal("new opening retained old recording facts")
	}
}

func (a peerRecordingAssembly) Resolve(id string) *livesession.LiveSession {
	if id != factorysessions.DefaultSessionID || a.current == nil {
		return nil
	}
	return &livesession.LiveSession{Handle: a.current}
}

func TestPeerRecordingSelection(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"automatic", "explicit", "replay", "resume", "unrecorded host", "direct no-record", "default", "validation", "reservation failure", "empty target", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			const peer = "5ca047d8-b32c-4fda-9e45-727b0fe7e3b2"
			cause := errors.New("reservation unavailable")
			planner := &peerRecordingPlanner{currentBoardTargetPlanner: currentBoardTargetPlanner{target: recordings.LiveRecordingTarget{ServicePath: "peer.json"}}}
			current := &runtimebinding.SessionState{CurrentBoardRecordPath: "host.json", RecordingHome: "host-profile"}
			opening := &RuntimeOpening{assembly: peerRecordingAssembly{current: current}, recordingsRuntime: planner}
			selection := factorysessions.SessionRuntimeSelection{SystemConfigHome: "selected-profile", Recording: factorysessions.SessionRecordingSelection{FlushInterval: time.Second}}
			request := factorysessions.SessionStartRequest{ActivationOnly: true}
			id := peer
			ctx := context.Background()
			wantCalls := 0
			switch mode {
			case "automatic":
				wantCalls = 1
			case "explicit":
				selection.Recording.RecordPath = "explicit.json"
			case "replay":
				selection.Recording.ReplayPath = "replay.json"
			case "resume":
				selection.Recording.ResumePath = "resume.json"
			case "unrecorded host":
				current.CurrentBoardRecordPath = ""
			case "direct no-record":
				request.ActivationOnly = false
			case "default":
				id = factorysessions.DefaultSessionID
			case "validation":
				request.ValidateOnly = true
			case "reservation failure":
				planner.err, wantCalls = cause, 1
			case "empty target":
				planner.target.ServicePath, wantCalls = "", 1
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			before := selection
			err := opening.selectPeerRecording(ctx, request, id, &selection)
			wantError := mode == "reservation failure" || mode == "empty target" || mode == "canceled"
			if (err != nil) != wantError || planner.calls != wantCalls {
				t.Fatalf("error=%v, planner calls=%d; want error=%t calls=%d", err, planner.calls, wantError, wantCalls)
			}
			if mode == "reservation failure" && !errors.Is(err, cause) {
				t.Fatalf("reservation cause lost: %v", err)
			}
			if mode == "automatic" {
				before.Recording.RecordPath = "peer.json"
				if planner.request.HomeDir != "selected-profile" || planner.request.CanonicalSessionID != peer || planner.request.ReportedSessionID != peer {
					t.Fatalf("target request = %+v", planner.request)
				}
			}
			if selection.Recording != before.Recording || current.CurrentBoardRecordPath == "peer.json" {
				t.Fatalf("selection changed unrelated policy: %+v", selection)
			}
		})
	}
}

func TestPeerRecordingUsesSelectedHostHomeBeforeAmbientResolver(t *testing.T) {
	t.Parallel()
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "inherited", true: "explicit"}[explicit], func(t *testing.T) {
			t.Parallel()
			planner := &peerRecordingPlanner{currentBoardTargetPlanner: currentBoardTargetPlanner{target: recordings.LiveRecordingTarget{ServicePath: "peer.json"}}}
			opening := &RuntimeOpening{
				assembly:          peerRecordingAssembly{current: &runtimebinding.SessionState{RecordingHome: "host-profile", CurrentBoardRecordPath: "host.json"}},
				recordingsRuntime: planner,
				resolveHome:       func() (string, error) { t.Fatal("ambient home used"); return "", nil },
			}
			selection := factorysessions.SessionRuntimeSelection{}
			wantHome := "host-profile"
			if explicit {
				wantHome, selection.SystemConfigHome = "explicit-profile", "explicit-profile"
			}
			request := factorysessions.SessionStartRequest{ActivationOnly: true, FolderPath: "/factory", RuntimeSelection: &selection}
			selected, err := opening.prepareLiveStartRequest(t.Context(), request, "5ca047d8-b32c-4fda-9e45-727b0fe7e3b2")
			if err != nil || selected.RuntimeSelection.SystemConfigHome != wantHome || planner.request.HomeDir != wantHome {
				t.Fatalf("selected=%+v planner=%+v error=%v", selected, planner.request, err)
			}
			if selection.Recording.RecordPath != "" {
				t.Fatal("caller selection mutated")
			}
		})
	}
}
