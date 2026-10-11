package service

import (
	"path/filepath"
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
