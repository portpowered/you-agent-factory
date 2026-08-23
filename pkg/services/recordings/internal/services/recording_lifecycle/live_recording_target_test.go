package recordinglifecycle_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
	recordinglifecyclewire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle/wire"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
)

const characterizedLiveRecordingSessionToken = "__factory_session_id__"

func TestLiveRecordingTargetPlannerOwnsDefaultLayoutAndReportedSessionPath(t *testing.T) {
	t.Parallel()

	clock := platformclock.NewDeterministic(time.Date(2026, 2, 3, 18, 45, 12, 0, time.UTC), time.Second)
	planner := recordingswire.NewLiveRecordingTargetPlanner(clock, func() string { return "uuid-1" }, filepath.Join)
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
		"clock":              {recordingswire.NewLiveRecordingTargetPlanner(nil, func() string { return "id" }, filepath.Join), "Recordings live target clock is required"},
		"identity generator": {recordingswire.NewLiveRecordingTargetPlanner(validClock, nil, filepath.Join), "Recordings live target identity generator is required"},
		"path joiner":        {recordingswire.NewLiveRecordingTargetPlanner(validClock, func() string { return "id" }, nil), "Recordings live target path joiner is required"},
		"empty identity":     {recordingswire.NewLiveRecordingTargetPlanner(validClock, func() string { return "" }, filepath.Join), "Recordings live target identity generator returned an empty identity"},
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
	planner := recordingswire.NewLiveRecordingTargetPlanner(
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

func TestLiveRecordingLifecycleCharacterizesDistinctSameDayDefaultTargets(t *testing.T) {
	t.Parallel()

	clock := platformclock.NewDeterministic(time.Date(2026, 2, 3, 18, 45, 12, 0, time.UTC), time.Second)
	identities := []string{"uuid-1", "uuid-2"}
	planner := recordingswire.NewLiveRecordingTargetPlanner(
		clock,
		func() string {
			identity := identities[0]
			identities = identities[1:]
			return identity
		},
		filepath.Join,
	)
	var serviceTargets []string
	service := recordinglifecyclewire.NewService(
		planner,
		func(target string, _ recordings.RecordingSnapshot) error {
			serviceTargets = append(serviceTargets, target)
			return nil
		},
		nil,
		clock,
	)
	home := filepath.Join("home", "operator")
	scope := recordings.CanonicalEventScope{FactorySessionID: "~default"}

	var reportedTargets []string
	for index := range 2 {
		reportedTargets = append(reportedTargets, recordSameDayLifecycleRun(t, service, clock, scope, home, index+1))
	}

	wantDir := filepath.Join(home, ".you-agent-factory", "recordings", "2026-02", "2026-02-03")
	wantServiceTargets := []string{
		filepath.Join(wantDir, "factory-session-"+characterizedLiveRecordingSessionToken+"-184512-uuid-1.json"),
		filepath.Join(wantDir, "factory-session-"+characterizedLiveRecordingSessionToken+"-184512-uuid-2.json"),
	}
	wantReportedTargets := []string{
		filepath.Join(wantDir, "factory-session-~default-184512-uuid-1.json"),
		filepath.Join(wantDir, "factory-session-~default-184512-uuid-2.json"),
	}
	assertSameDayTargetPaths(t, serviceTargets, reportedTargets, wantServiceTargets, wantReportedTargets)
	t.Logf("current behavior: same-day default-session recording targets are distinct: service=%q reported=%q", serviceTargets, reportedTargets)
}

func recordSameDayLifecycleRun(
	t *testing.T,
	service recordinglifecycle.Service,
	clock recordings.RecordingClock,
	scope recordings.CanonicalEventScope,
	home string,
	run int,
) string {
	t.Helper()
	started, err := service.StartRecording(recordings.StartRecordingRequest{
		Enabled: true,
		Scope:   scope,
		Target: recordings.RecordingTargetRequest{
			HomeDir:           home,
			ReportedSessionID: "~default",
		},
	})
	if err != nil {
		t.Fatalf("StartRecording run %d: %v", run, err)
	}
	recordingID := started.Status.RecordingID
	if _, err := service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: recordingID,
		Event: recordings.CanonicalEvent{
			ID:         recordings.CanonicalEventID("event-" + string(recordingID)),
			Sequence:   0,
			Scope:      scope,
			Kind:       "WORK_REQUEST",
			RecordedAt: clock.Now(),
			Cursor: recordings.CanonicalEventCursor{
				StreamGenerationID: string(recordingID),
				Sequence:           0,
			},
			Payload: "{}",
		},
	}); err != nil {
		t.Fatalf("RecordRecordingEvent run %d: %v", run, err)
	}
	if _, err := service.FlushRecording(recordings.FlushRecordingRequest{RecordingID: recordingID}); err != nil {
		t.Fatalf("FlushRecording run %d: %v", run, err)
	}
	if _, err := service.StopRecording(recordings.StopRecordingRequest{RecordingID: recordingID}); err != nil {
		t.Fatalf("StopRecording run %d: %v", run, err)
	}
	return string(started.Status.Artifact)
}

func assertSameDayTargetPaths(
	t *testing.T,
	serviceTargets, reportedTargets, wantServiceTargets, wantReportedTargets []string,
) {
	t.Helper()
	if got := strings.Join(serviceTargets, "\n"); got != strings.Join(wantServiceTargets, "\n") {
		t.Fatalf("observed service recording targets = %q, want literal targets %q", got, strings.Join(wantServiceTargets, "\n"))
	}
	if got := strings.Join(reportedTargets, "\n"); got != strings.Join(wantReportedTargets, "\n") {
		t.Fatalf("observed reported recording targets = %q, want literal targets %q", got, strings.Join(wantReportedTargets, "\n"))
	}
	for index := range wantServiceTargets {
		if strings.Contains(serviceTargets[index], "~default") || !strings.Contains(serviceTargets[index], characterizedLiveRecordingSessionToken) {
			t.Fatalf("service target %q did not retain the placeholder token", serviceTargets[index])
		}
		if strings.Contains(reportedTargets[index], characterizedLiveRecordingSessionToken) || !strings.Contains(reportedTargets[index], "~default") {
			t.Fatalf("reported target %q did not substitute ReportedSessionID only", reportedTargets[index])
		}
	}
	if serviceTargets[0] == serviceTargets[1] || reportedTargets[0] == reportedTargets[1] {
		t.Fatalf("current same-day default-session recording targets collide: service=%q reported=%q", serviceTargets, reportedTargets)
	}
}
