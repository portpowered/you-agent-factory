package recordings_test

import (
	"path/filepath"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestLiveRecordingTargetPlannerOwnsDefaultLayoutAndReportedSessionPath(t *testing.T) {
	t.Parallel()

	clock := platformclock.NewDeterministic(time.Date(2026, 2, 3, 18, 45, 12, 0, time.UTC), time.Second)
	planner := recordings.NewLiveRecordingTargetPlanner(clock, func() string { return "uuid-1" }, filepath.Join)
	target, err := planner.PlanLiveRecordingTarget(recordings.LiveRecordingTargetRequest{
		HomeDir:           filepath.Join("home", "operator"),
		ReportedSessionID: "~default",
	})
	if err != nil {
		t.Fatalf("PlanLiveRecordingTarget: %v", err)
	}
	wantDir := filepath.Join("home", "operator", ".you-agent-factory", "recordings", "2026-02", "2026-02-03")
	if want := filepath.Join(wantDir, "factory-session-__factory_session_id__-184512-uuid-1.json"); target.ServicePath != want {
		t.Fatalf("service path = %q, want %q", target.ServicePath, want)
	}
	if want := filepath.Join(wantDir, "factory-session-~default-184512-uuid-1.json"); target.ReportedPath != want {
		t.Fatalf("reported path = %q, want %q", target.ReportedPath, want)
	}
}

func TestLiveRecordingTargetPlannerRequiresExactMechanics(t *testing.T) {
	t.Parallel()

	validClock := platformclock.NewDeterministic(time.Unix(1, 0), time.Second)
	tests := map[string]struct {
		planner recordings.LiveRecordingTargetPlanner
		want    string
	}{
		"clock":              {recordings.NewLiveRecordingTargetPlanner(nil, func() string { return "id" }, filepath.Join), "Recordings live target clock is required"},
		"identity generator": {recordings.NewLiveRecordingTargetPlanner(validClock, nil, filepath.Join), "Recordings live target identity generator is required"},
		"path joiner":        {recordings.NewLiveRecordingTargetPlanner(validClock, func() string { return "id" }, nil), "Recordings live target path joiner is required"},
		"empty identity":     {recordings.NewLiveRecordingTargetPlanner(validClock, func() string { return "" }, filepath.Join), "Recordings live target identity generator returned an empty identity"},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := test.planner.PlanLiveRecordingTarget(recordings.LiveRecordingTargetRequest{HomeDir: "home"})
			if err == nil || err.Error() != test.want {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLiveRecordingTargetPlannerUsesInjectedIdentityForUniqueNames(t *testing.T) {
	t.Parallel()

	identities := []string{"uuid-1", "uuid-2"}
	planner := recordings.NewLiveRecordingTargetPlanner(
		platformclock.NewDeterministic(time.Date(2026, 2, 3, 18, 45, 12, 0, time.UTC), time.Second),
		func() string {
			identity := identities[0]
			identities = identities[1:]
			return identity
		},
		filepath.Join,
	)
	first, err := planner.PlanLiveRecordingTarget(recordings.LiveRecordingTargetRequest{HomeDir: "home"})
	if err != nil {
		t.Fatalf("first target: %v", err)
	}
	second, err := planner.PlanLiveRecordingTarget(recordings.LiveRecordingTargetRequest{HomeDir: "home"})
	if err != nil {
		t.Fatalf("second target: %v", err)
	}
	if first.ServicePath == second.ServicePath {
		t.Fatalf("recording paths matched: %q", first.ServicePath)
	}
}
