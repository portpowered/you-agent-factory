package service

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

const (
	modulePrefix       = "github.com/portpowered/infinite-you/"
	factoryRuntimeRoot = modulePrefix + "pkg/services/factory_runtime"
	lifecycleRecorder  = modulePrefix + "pkg/services/recordings/service"
)

// TestLifecycleRuntimeRecorderImportsRuntimeRootOnly seals CUT-REC-RUN story 002:
// the lifecycle recorder edge may depend on Factory Runtime only through the
// service root contract.
func TestLifecycleRuntimeRecorderImportsRuntimeRootOnly(t *testing.T) {
	t.Parallel()
	assertProductionImportsUseRuntimeRootOnly(t, lifecycleRecorder)
}

// TestLifecycleRuntimeRecorderAcceptsRuntimeRootFinishedEvent proves the
// recorder path accepts Runtime-facing event vocabulary from the Runtime root
// and preserves observable identity, kind, and payload fields.
func TestLifecycleRuntimeRecorderAcceptsRuntimeRootFinishedEvent(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 7, 27, 17, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(2 * time.Minute)
	root := NewServiceWithLifecycleEffects(
		NewRuntimeLedger(nil, func() time.Time { return startedAt }, "generation", nil),
		NewProjectionService(),
		nil,
		nil,
		nil,
		nil,
		runtimeRecorderTestClock{now: startedAt},
	)
	recorder := newLifecycleRecorderForTest(t, startedAt, "runtime-root-finished.json")
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-runtime-root"}
	if err := recorder.BindRecordingService(root, scope); err != nil {
		t.Fatalf("BindRecordingService: %v", err)
	}

	runtimeEvent := factoryruntime.FactoryEvent{
		Id:   "runtime-root-work-event",
		Type: factoryruntime.FactoryEventTypeWorkRequest,
		Context: factoryruntime.FactoryEventContext{
			EventTime: startedAt.Add(time.Second),
		},
		Payload: []byte(`{"workId":"work-runtime-root"}`),
	}
	recorder.RecordEvent(runtimeEvent)
	if err := recorder.Finalize(finishedAt); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	status, err := root.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: recorder.recordingID,
	})
	if err != nil {
		t.Fatalf("QueryRecordingStatus: %v", err)
	}
	if status.Status.AcceptedEvents < 3 {
		t.Fatalf("accepted events = %d, want run-started, work request, and run-finished", status.Status.AcceptedEvents)
	}

	built, err := root.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: recorder.recordingID,
	})
	if err != nil {
		t.Fatalf("BuildPortableArtifact: %v", err)
	}
	if len(built.Artifact.Events) < 3 {
		t.Fatalf("portable events = %d, want at least three", len(built.Artifact.Events))
	}

	recordedWorkEvent := built.Artifact.Events[len(built.Artifact.Events)-2]
	if recordedWorkEvent.ID != recordings.CanonicalEventID(runtimeEvent.Id) {
		t.Fatalf("recorded work event id = %q, want %q", recordedWorkEvent.ID, runtimeEvent.Id)
	}
	if recordedWorkEvent.Kind != recordings.CanonicalEventKind(runtimeEvent.Type) {
		t.Fatalf("recorded work event kind = %q, want %q", recordedWorkEvent.Kind, runtimeEvent.Type)
	}
	if !strings.Contains(recordedWorkEvent.Payload, "work-runtime-root") {
		t.Fatalf("recorded work event payload = %q, want runtime-root work id", recordedWorkEvent.Payload)
	}

	finishedEvent := built.Artifact.Events[len(built.Artifact.Events)-1]
	if finishedEvent.ID != recordings.CanonicalEventID(factoryruntime.RunFinishedFactoryEventID) {
		t.Fatalf("finished event id = %q, want %q", finishedEvent.ID, factoryruntime.RunFinishedFactoryEventID)
	}
	if finishedEvent.Kind != recordings.CanonicalEventKind(factoryruntime.FactoryEventTypeRunResponse) {
		t.Fatalf("finished event kind = %q, want %q", finishedEvent.Kind, factoryruntime.FactoryEventTypeRunResponse)
	}
	if !strings.Contains(finishedEvent.Payload, string(factoryruntime.FactoryStateCompleted)) {
		t.Fatalf("finished event payload = %q, want completed state", finishedEvent.Payload)
	}
}

func assertProductionImportsUseRuntimeRootOnly(t *testing.T, packagePath string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	for _, importPath := range strings.Fields(string(output)) {
		if isForbiddenLifecycleRuntimeImport(importPath) {
			t.Fatalf(
				"%s production import %s is forbidden; use %s for Factory Runtime surfaces",
				packagePath,
				importPath,
				factoryRuntimeRoot,
			)
		}
	}
}

func isForbiddenLifecycleRuntimeImport(importPath string) bool {
	if importPath == factoryRuntimeRoot {
		return false
	}
	if strings.HasPrefix(importPath, factoryRuntimeRoot+"/") {
		return true
	}
	if importPath == modulePrefix+"pkg/factory" ||
		strings.HasPrefix(importPath, modulePrefix+"pkg/factory/") {
		return true
	}
	if strings.HasPrefix(importPath, modulePrefix+"pkg/transports/mapping/factory") {
		return true
	}
	return false
}
