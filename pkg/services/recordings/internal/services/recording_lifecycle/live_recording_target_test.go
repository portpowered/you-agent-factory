package recordinglifecycle_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformruntimeartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
)

const canonicalRecordingSessionID = "7d9d3fb4-6bc9-4df5-a67f-0f504f8ea3ba"

type namedPathReserver struct {
	path string
	err  error

	root string
	at   time.Time
	name string
	ext  string
}

func (reserver *namedPathReserver) ReserveNamed(root string, at time.Time, name, ext string) (string, error) {
	reserver.root = root
	reserver.at = at
	reserver.name = name
	reserver.ext = ext
	return reserver.path, reserver.err
}

func newPlanner(clock recordings.RecordingClock, reserver recordings.RecordingNamedPathReserver) recordings.LiveRecordingTargetPlanner {
	return recordingswire.NewLiveRecordingTargetPlanner(clock, reserver, filepath.Join)
}

func TestLiveRecordingTargetPlannerReservesCanonicalUUIDInDatedLayout(t *testing.T) {
	t.Parallel()

	clock := platformclock.NewDeterministic(time.Date(2026, 8, 23, 18, 45, 12, 0, time.UTC), time.Second)
	home := filepath.Join("home", "operator")
	want := filepath.Join(home, ".you-agent-factory", "recordings", "2026", "08", "23", canonicalRecordingSessionID+".json")
	reserver := &namedPathReserver{path: want}

	target, err := newPlanner(clock, reserver).PlanLiveRecordingTarget(recordings.LiveRecordingTargetRequest{
		HomeDir:            home,
		CanonicalSessionID: canonicalRecordingSessionID,
		ReportedSessionID:  "~default",
	})
	if err != nil {
		t.Fatalf("PlanLiveRecordingTarget: %v", err)
	}
	if target.ServicePath != want || target.ReportedPath != want {
		t.Fatalf("target = %#v, want both paths %q", target, want)
	}
	if reserver.root != filepath.Join(home, ".you-agent-factory", "recordings") ||
		!reserver.at.Equal(clock.Now().UTC()) || reserver.name != canonicalRecordingSessionID || reserver.ext != ".json" {
		t.Fatalf("ReserveNamed request = root %q, at %s, name %q, ext %q; want dated root, controlled time, UUID, .json",
			reserver.root, reserver.at, reserver.name, reserver.ext)
	}
	if strings.Contains(filepath.Base(target.ServicePath), "factory-session-") ||
		strings.Contains(filepath.Base(target.ServicePath), "~default") ||
		strings.Contains(filepath.Base(target.ServicePath), "__factory_session_id__") {
		t.Fatalf("target basename %q contains a forbidden identity component", filepath.Base(target.ServicePath))
	}
}

func TestLiveRecordingTargetPlannerIgnoresReportedSessionAlias(t *testing.T) {
	t.Parallel()

	clock := platformclock.NewDeterministic(time.Date(2026, 8, 23, 18, 45, 12, 0, time.UTC), time.Second)
	home := t.TempDir()
	path := filepath.Join(home, ".you-agent-factory", "recordings", "2026", "08", "23", canonicalRecordingSessionID+".json")
	first, err := newPlanner(clock, &namedPathReserver{path: path}).PlanLiveRecordingTarget(recordings.LiveRecordingTargetRequest{
		HomeDir: home, CanonicalSessionID: canonicalRecordingSessionID, ReportedSessionID: "~default",
	})
	if err != nil {
		t.Fatalf("first target: %v", err)
	}
	second, err := newPlanner(clock, &namedPathReserver{path: path}).PlanLiveRecordingTarget(recordings.LiveRecordingTargetRequest{
		HomeDir: home, CanonicalSessionID: canonicalRecordingSessionID, ReportedSessionID: "operator-alias",
	})
	if err != nil {
		t.Fatalf("second target: %v", err)
	}
	if first != second {
		t.Fatalf("reported alias changed target: first %#v, second %#v", first, second)
	}
}

func TestLiveRecordingTargetPlannerRejectsCanonicalReuseWithoutCollisionArtifact(t *testing.T) {
	t.Parallel()

	clock := platformclock.NewDeterministic(time.Date(2026, 8, 23, 18, 45, 12, 0, time.UTC), time.Second)
	home := t.TempDir()
	recordingsRoot := filepath.Join(home, ".you-agent-factory", "recordings")
	canonicalPath := filepath.Join(recordingsRoot, "2026", "08", "23", canonicalRecordingSessionID+".json")
	if err := os.MkdirAll(filepath.Dir(canonicalPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	const existingContent = "existing recording"
	if err := os.WriteFile(canonicalPath, []byte(existingContent), 0o600); err != nil {
		t.Fatalf("WriteFile canonical recording: %v", err)
	}
	reserver, err := platformruntimeartifact.NewReserver(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewReserver: %v", err)
	}

	_, err = newPlanner(clock, reserver).PlanLiveRecordingTarget(recordings.LiveRecordingTargetRequest{
		HomeDir: home, CanonicalSessionID: canonicalRecordingSessionID,
	})
	if !errors.Is(err, platformruntimeartifact.ErrNamedReservationExists) {
		t.Fatalf("canonical reuse error = %v, want ErrNamedReservationExists", err)
	}
	collisionPath := filepath.Join(recordingsRoot, "2026", "08", "23", canonicalRecordingSessionID+"-2.json")
	if _, statErr := os.Stat(collisionPath); !os.IsNotExist(statErr) {
		t.Fatalf("collision artifact stat error = %v, want no %q", statErr, collisionPath)
	}
	content, readErr := os.ReadFile(canonicalPath)
	if readErr != nil || string(content) != existingContent {
		t.Fatalf("canonical artifact after reuse = %q, %v; want unchanged content", content, readErr)
	}
}

func TestLiveRecordingTargetPlannerRejectsCanonicalReuseWithoutCollisionArtifact(t *testing.T) {
	t.Parallel()

	clock := platformclock.NewDeterministic(time.Date(2026, 8, 23, 18, 45, 12, 0, time.UTC), time.Second)
	home := t.TempDir()
	recordingsRoot := filepath.Join(home, ".you-agent-factory", "recordings")
	canonicalPath := filepath.Join(
		recordingsRoot, "2026", "08", "23", canonicalRecordingSessionID+".json",
	)
	if err := os.MkdirAll(filepath.Dir(canonicalPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	const existingContent = "existing recording"
	if err := os.WriteFile(canonicalPath, []byte(existingContent), 0o600); err != nil {
		t.Fatalf("WriteFile canonical recording: %v", err)
	}
	reserver, err := platformruntimeartifact.NewReserver(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewReserver: %v", err)
	}

	_, err = newPlanner(clock, reserver).PlanLiveRecordingTarget(recordings.LiveRecordingTargetRequest{
		HomeDir:            home,
		CanonicalSessionID: canonicalRecordingSessionID,
	})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("canonical reuse error = %v, want named reservation failure", err)
	}
	collisionPath := filepath.Join(
		recordingsRoot, "2026", "08", "23", canonicalRecordingSessionID+"-2.json",
	)
	if _, statErr := os.Stat(collisionPath); !os.IsNotExist(statErr) {
		t.Fatalf("collision artifact stat error = %v, want no %q", statErr, collisionPath)
	}
	content, readErr := os.ReadFile(canonicalPath)
	if readErr != nil || string(content) != existingContent {
		t.Fatalf("canonical artifact after reuse = %q, %v; want unchanged content", content, readErr)
	}
}

func TestLiveRecordingTargetPlannerRequiresCanonicalUUIDAndExactReservation(t *testing.T) {
	t.Parallel()

	clock := platformclock.NewDeterministic(time.Unix(1, 0), time.Second)
	validPath := filepath.Join("home", ".you-agent-factory", "recordings", "1970", "01", "01", canonicalRecordingSessionID+".json")
	tests := map[string]struct {
		planner recordings.LiveRecordingTargetPlanner
		request recordings.LiveRecordingTargetRequest
		want    string
	}{
		"clock": {
			planner: newPlanner(nil, &namedPathReserver{path: validPath}),
			request: recordings.LiveRecordingTargetRequest{HomeDir: "home", CanonicalSessionID: canonicalRecordingSessionID},
			want:    "Recordings live target clock is required",
		},
		"named path reserver": {
			planner: newPlanner(clock, nil),
			request: recordings.LiveRecordingTargetRequest{HomeDir: "home", CanonicalSessionID: canonicalRecordingSessionID},
			want:    "Recordings live target named path reserver is required",
		},
		"path joiner": {
			planner: recordingswire.NewLiveRecordingTargetPlanner(clock, &namedPathReserver{path: validPath}, nil),
			request: recordings.LiveRecordingTargetRequest{HomeDir: "home", CanonicalSessionID: canonicalRecordingSessionID},
			want:    "Recordings live target path joiner is required",
		},
		"empty canonical identity": {
			planner: newPlanner(clock, &namedPathReserver{path: validPath}),
			request: recordings.LiveRecordingTargetRequest{HomeDir: "home"},
			want:    `canonical Factory Session ID "" must be a UUID`,
		},
		"routing alias": {
			planner: newPlanner(clock, &namedPathReserver{path: validPath}),
			request: recordings.LiveRecordingTargetRequest{HomeDir: "home", CanonicalSessionID: "~default"},
			want:    `canonical Factory Session ID "~default" must be a UUID`,
		},
		"non UUID": {
			planner: newPlanner(clock, &namedPathReserver{path: validPath}),
			request: recordings.LiveRecordingTargetRequest{HomeDir: "home", CanonicalSessionID: "recording-1"},
			want:    `canonical Factory Session ID "recording-1" must be a UUID`,
		},
		"collision suffix": {
			planner: newPlanner(clock, &namedPathReserver{path: validPath + "-2"}),
			request: recordings.LiveRecordingTargetRequest{HomeDir: "home", CanonicalSessionID: canonicalRecordingSessionID},
			want:    "want exact path",
		},
		"reservation error": {
			planner: newPlanner(clock, &namedPathReserver{err: errors.New("occupied")}),
			request: recordings.LiveRecordingTargetRequest{HomeDir: "home", CanonicalSessionID: canonicalRecordingSessionID},
			want:    "reserve recording target for canonical Factory Session ID",
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := test.planner.PlanLiveRecordingTarget(test.request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}
