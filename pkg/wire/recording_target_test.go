package wire

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestCLIRunDefaultsRetainWireSelectedRecordingTargetPlanner(t *testing.T) {
	t.Parallel()

	planner := recordings.LiveRecordingTargetPlannerFunc(func(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
		return recordings.LiveRecordingTarget{}, nil
	})
	defaults := provideCLIRunDefaults(planner)
	if defaults.RecordingTargetPlanner == nil {
		t.Fatal("CLI run defaults dropped the Wire-selected recording target planner")
	}
}

func TestProductionLiveRecordingTargetPlannerIsUsable(t *testing.T) {
	t.Parallel()

	target, err := provideLiveRecordingTargetPlanner().PlanLiveRecordingTarget(recordings.LiveRecordingTargetRequest{
		HomeDir:           t.TempDir(),
		ReportedSessionID: "~default",
	})
	if err != nil {
		t.Fatalf("PlanLiveRecordingTarget: %v", err)
	}
	if target.ServicePath == "" || target.ReportedPath == "" || target.ServicePath == target.ReportedPath {
		t.Fatalf("target = %#v, want distinct runtime template and reported paths", target)
	}
}
