package service

import (
	"errors"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

type peerTargetClock struct{ at time.Time }

func (clock peerTargetClock) Now() time.Time { return clock.at }

type peerTargetReserver struct{ path string }

func (reserver peerTargetReserver) ReserveNamed(string, time.Time, string, string) (string, error) {
	return reserver.path, nil
}

func TestPeerRecordingTargetWritesReservedCanonicalPath(t *testing.T) {
	t.Parallel()
	const id = "5ca047d8-b32c-4fda-9e45-727b0fe7e3b2"
	at := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	path := filepath.Join("profile", ".you-agent-factory", "recordings", "2026", "10", "11", id+".json")
	planner := NewTargetPlanner(peerTargetClock{at}, peerTargetReserver{path}, filepath.Join)
	target, err := planner.PlanLiveRecordingTarget(recordings.LiveRecordingTargetRequest{
		HomeDir: "profile", CanonicalSessionID: id, ReportedSessionID: id,
	})
	if err != nil {
		t.Fatal(err)
	}
	if target.ReportedPath != path || factoryruntime.RecordingPath(target.ServicePath).ForSession(id) != path {
		t.Fatalf("peer writer differs from reserved/reported target: %+v", target)
	}
	defaultTarget, err := planner.PlanLiveRecordingTarget(recordings.LiveRecordingTargetRequest{
		HomeDir: "profile", CanonicalSessionID: id, ReportedSessionID: "~default",
	})
	if err != nil || defaultTarget.ServicePath != path || defaultTarget.ReportedPath != path {
		t.Fatalf("default recording policy changed: %+v error=%v", defaultTarget, err)
	}
}

// Fixture creation obtains an opaque identity with controlled file effects.
// The planner and lifecycle tests below use only the narrow owner spy.
type reservationIdentityFile struct{ fs.FileInfo }

func (reservationIdentityFile) Write(data []byte) (int, error)  { return len(data), nil }
func (reservationIdentityFile) Close() error                    { return nil }
func (file reservationIdentityFile) Stat() (fs.FileInfo, error) { return file.FileInfo, nil }

type reservationIdentityFS struct{ fs.FileInfo }

func (reservationIdentityFS) MkdirAll(string, fs.FileMode) error { return nil }
func (filesystem reservationIdentityFS) OpenFile(string, int, fs.FileMode) (io.WriteCloser, error) {
	return reservationIdentityFile(filesystem), nil
}
func (reservationIdentityFS) RemoveReserved(string, fs.FileInfo) error { return nil }
func peerReservationIdentity(t *testing.T) runtimeartifact.Reservation {
	t.Helper()
	identity, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	paths, err := runtimeartifact.NewReserver(reservationIdentityFS{identity})
	if err != nil {
		t.Fatal(err)
	}
	_, reservation, err := paths.(runtimeartifact.OwnedNamedReserver).ReserveNamedOwned("fixture", time.Unix(0, 0), "peer", ".json")
	if err != nil || reservation.IsZero() {
		t.Fatalf("opaque fixture: %+v %v", reservation, err)
	}
	return reservation
}

type peerOwnedTargetSpy struct {
	path                   string
	reservation            runtimeartifact.Reservation
	reserveErr, releaseErr error
	released, committed    []runtimeartifact.Reservation
	calls                  int
	root, name, ext        string
	at                     time.Time
}

func (owner *peerOwnedTargetSpy) ReserveNamed(string, time.Time, string, string) (string, error) {
	panic("path-only reservation used for rollback")
}
func (owner *peerOwnedTargetSpy) ReserveNamedOwned(root string, at time.Time, name, ext string) (string, runtimeartifact.Reservation, error) {
	owner.calls++
	owner.root, owner.at, owner.name, owner.ext = root, at, name, ext
	return owner.path, owner.reservation, owner.reserveErr
}
func (owner *peerOwnedTargetSpy) ReleaseReservation(reservation runtimeartifact.Reservation) error {
	owner.released = append(owner.released, reservation)
	return owner.releaseErr
}
func (owner *peerOwnedTargetSpy) CommitReservation(reservation runtimeartifact.Reservation) {
	owner.committed = append(owner.committed, reservation)
}

func TestPeerOwnedTargetPlanningRetainsRollbackAuthority(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"success", "reservation failure", "unexpected path"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			const id = "5ca047d8-b32c-4fda-9e45-727b0fe7e3b2"
			at := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
			path := filepath.Join("profile", ".you-agent-factory", "recordings", "2026", "10", "11", id+".json")
			cause := errors.New("close failed after reservation")
			owner := &peerOwnedTargetSpy{path: path, reservation: peerReservationIdentity(t)}
			if mode == "reservation failure" {
				owner.reserveErr = cause
			}
			if mode == "unexpected path" {
				owner.path = "foreign.json"
			}
			planner := NewTargetPlanner(peerTargetClock{at}, owner, filepath.Join)
			target, err := planner.PlanLiveRecordingTarget(recordings.LiveRecordingTargetRequest{HomeDir: "profile", CanonicalSessionID: id, ReportedSessionID: id, RollbackOnFailure: true})
			if (err != nil) != (mode != "success") || target.Reservation != owner.reservation {
				t.Fatalf("lost rollback authority: %+v %v", target, err)
			}
			if mode == "reservation failure" && !errors.Is(err, cause) {
				t.Fatalf("lost reserve error: %v", err)
			}
			if owner.calls != 1 || owner.root != filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(path)))) || owner.name != id || owner.ext != ".json" || !owner.at.Equal(at) {
				t.Fatalf("owned reservation request: %+v", owner)
			}
			assertOwnedPeerTargetPaths(t, mode, target, path, id)
		})
	}
}
func assertOwnedPeerTargetPaths(t *testing.T, mode string, target recordings.LiveRecordingTarget, path, id string) {
	t.Helper()
	if mode == "success" {
		if target.ReportedPath != path || factoryruntime.RecordingPath(target.ServicePath).ForSession(id) != path {
			t.Fatalf("owned target differs from reservation: %+v", target)
		}
	} else if target.ServicePath != "" || target.ReportedPath != "" {
		t.Fatalf("failed plan published destination: %+v", target)
	}
}

func TestPeerReservationCompletionDelegatesOnlyNonzeroAuthority(t *testing.T) {
	t.Parallel()
	for _, component := range []string{"planner", "lifecycle"} {
		t.Run(component, func(t *testing.T) {
			t.Parallel()
			cause := errors.New("release unavailable")
			owner := &peerOwnedTargetSpy{releaseErr: cause}
			var subject recordings.LiveRecordingReservationOwner = &liveRecordingTargetPlanner{reserve: owner}
			if component == "lifecycle" {
				subject = &Service{targets: &peerLifecycleTargetSpy{peerOwnedTargetSpy: owner}}
			}
			subject.CommitReservation(runtimeartifact.Reservation{})
			if err := subject.ReleaseReservation(runtimeartifact.Reservation{}); err != nil || len(owner.released) != 0 || len(owner.committed) != 0 {
				t.Fatalf("zero authority delegated: %+v %v", owner, err)
			}
			reservation := peerReservationIdentity(t)
			if err := subject.ReleaseReservation(reservation); !errors.Is(err, cause) {
				t.Fatalf("lost release error: %v", err)
			}
			owner.releaseErr = nil
			if err := subject.ReleaseReservation(reservation); err != nil {
				t.Fatal(err)
			}
			subject.CommitReservation(reservation)
			if len(owner.released) != 2 || owner.released[0] != reservation || owner.released[1] != reservation || len(owner.committed) != 1 || owner.committed[0] != reservation {
				t.Fatalf("owner identity or retry lost: %+v", owner)
			}
		})
	}
}

type peerLifecycleTargetSpy struct{ *peerOwnedTargetSpy }

func (*peerLifecycleTargetSpy) PlanLiveRecordingTarget(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
	panic("unexpected planning")
}

func TestPeerReservationMissingOwnerReportsFailure(t *testing.T) {
	t.Parallel()
	reservation := peerReservationIdentity(t)
	planner := &liveRecordingTargetPlanner{reserve: peerTargetReserver{}}
	if err := planner.ReleaseReservation(reservation); err == nil || !strings.Contains(err.Error(), "capability") {
		t.Fatalf("missing release owner: %v", err)
	}
	lifecycle := &Service{}
	if err := lifecycle.ReleaseReservation(reservation); err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("missing lifecycle owner: %v", err)
	}
	_, err := (&liveRecordingTargetPlanner{clock: peerTargetClock{}, reserve: peerTargetReserver{}, join: filepath.Join}).PlanLiveRecordingTarget(recordings.LiveRecordingTargetRequest{HomeDir: "profile", CanonicalSessionID: "5ca047d8-b32c-4fda-9e45-727b0fe7e3b2", RollbackOnFailure: true})
	if err == nil || !strings.Contains(err.Error(), "capability") {
		t.Fatalf("missing rollback owner: %v", err)
	}
}

func TestPeerTargetRejectsInvalidInputsBeforeReservation(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"nil planner", "clock", "reserver", "joiner", "home", "UUID"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			planner := &liveRecordingTargetPlanner{clock: peerTargetClock{}, reserve: peerTargetReserver{}, join: filepath.Join}
			request := recordings.LiveRecordingTargetRequest{HomeDir: "profile", CanonicalSessionID: "5ca047d8-b32c-4fda-9e45-727b0fe7e3b2"}
			switch mode {
			case "nil planner":
				planner = nil
			case "clock":
				planner.clock = nil
			case "reserver":
				planner.reserve = nil
			case "joiner":
				planner.join = nil
			case "home":
				request.HomeDir = " "
			case "UUID":
				request.CanonicalSessionID = "not-a-session-UUID"
			}
			target, err := planner.PlanLiveRecordingTarget(request)
			if err == nil || target != (recordings.LiveRecordingTarget{}) {
				t.Fatalf("invalid plan acquired target: %+v %v", target, err)
			}
		})
	}
}

func TestPeerRecordingBindingGeneratedIdentityAndIdempotence(t *testing.T) {
	t.Parallel()
	lifecycle := New(nil, nil, nil, peerTargetClock{})
	first, err := lifecycle.BindRecording(recordings.BindRecordingRequest{RecordingID: "recording-1", Artifact: "first.json"})
	if err != nil {
		t.Fatal(err)
	}
	request := recordings.BindRecordingRequest{Artifact: "peer.json", Scope: recordings.CanonicalEventScope{FactorySessionID: "peer"}}
	peer, err := lifecycle.BindRecording(request)
	if err != nil || peer.Status.RecordingID == first.Status.RecordingID || peer.Status.RecordingID == "" {
		t.Fatalf("generated ownership identity: %+v %v", peer, err)
	}
	request.RecordingID = peer.Status.RecordingID
	repeated, err := lifecycle.BindRecording(request)
	if err != nil || repeated.Status.RecordingID != peer.Status.RecordingID {
		t.Fatalf("idempotent binding: %+v %v", repeated, err)
	}
	request.Scope.FactorySessionID = "foreign"
	if _, err := lifecycle.BindRecording(request); !errors.Is(err, recordings.ErrRecordingBindingConflict) {
		t.Fatalf("foreign scope stole identity: %v", err)
	}
}

func TestPeerRecordingPlanningFailureDoesNotBindWriter(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"missing planner", "plan failure", "empty service path", "empty reported path"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			cause := errors.New("reservation failed")
			target := recordings.LiveRecordingTarget{ServicePath: "peer.json", ReportedPath: "peer.json"}
			var planErr error
			if mode == "plan failure" {
				planErr = cause
			}
			if mode == "empty service path" {
				target.ServicePath = ""
			}
			if mode == "empty reported path" {
				target.ReportedPath = ""
			}
			var planner recordings.LiveRecordingTargetPlanner = recordings.LiveRecordingTargetPlannerFunc(func(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
				return target, planErr
			})
			if mode == "missing planner" {
				planner = nil
			}
			lifecycle := New(planner, nil, nil, peerTargetClock{})
			result, err := lifecycle.StartRecording(recordings.StartRecordingRequest{Enabled: true, RecordingID: "peer", Target: recordings.RecordingTargetRequest{HomeDir: "profile"}})
			if err == nil || result.Enabled {
				t.Fatalf("failed target bound writer: %+v %v", result, err)
			}
			if mode == "plan failure" && !errors.Is(err, cause) {
				t.Fatalf("lost planning cause: %v", err)
			}
			if _, err := lifecycle.BindRecording(recordings.BindRecordingRequest{RecordingID: "peer", Artifact: "peer.json"}); err != nil {
				t.Fatalf("failed plan retained writer claim: %v", err)
			}
		})
	}
}
