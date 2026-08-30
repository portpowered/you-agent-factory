package root_composition_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	recoveryActivationWorkID    = "fun-work-recovery-activation-id"
	recoveryActivationTraceID   = "fun-work-recovery-activation-trace"
	recoveryActivationRequestID = "fun-work-recovery-activation-request"
	recoveryActivationWorkName  = "fun-work-recovery-activation-task"
	recoveryActivationWorkType  = "task"

	recordingsActivationWorkType = "task"

	visualizationActivationRequestID = "fun-work-visualization-activation"
)

// TestWorkRecoveryActivatesThroughRootBuildProcessAfterLifecycle proves public
// recovery/manual-move advances failed Work to a terminal customer-visible state
// after runtime lifecycle on a process constructed only through root.BuildProcess
// with edges.Edges effect replacement. Detailed recovery coverage remains under
// tests/functional/work/recovery; this test closes the explicit public-process
// activation gap.
func TestWorkRecoveryActivatesThroughRootBuildProcessAfterLifecycle(t *testing.T) {
	t.Parallel()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "happy_path"))
	provider := testutil.NewMockWorkerMapProviderWithDefault(map[string][]testutil.WorkResponse{
		"worker-a": {
			{Error: errors.New("initial recoverable failure")},
			{Content: "COMPLETE"},
		},
		"worker-b": {
			{Content: "COMPLETE"},
		},
	})
	edges := serviceedges.Edges{ProviderOverride: provider}

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            false,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
	})
	t.Cleanup(func() { server.Stop(t) })

	baseURL := server.URL()
	workTypeName := recoveryActivationWorkType
	terminalObservation := support.OpenDefaultSessionTerminalFactoryEventObservation(t, baseURL)
	support.UpsertDefaultSessionWorkRequest(t, baseURL, factoryapi.WorkRequest{
		RequestId: recoveryActivationRequestID,
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{{
			Name:         recoveryActivationWorkName,
			WorkId:       stringPtr(recoveryActivationWorkID),
			WorkTypeName: &workTypeName,
			TraceId:      stringPtr(recoveryActivationTraceID),
			Payload:      map[string]string{"title": "FUN Work recovery activation"},
		}},
	})

	waitForRecoveryActivationWorkAtState(t, baseURL, recoveryActivationWorkID, "failed", 15*time.Second)

	process := support.BuildProcess(t, edges)
	moveOutput := executeRecoveryActivationWorkMoveCLI(
		t,
		process,
		baseURL,
		recoveryActivationWorkID,
		"init",
	)
	assertRecoveryActivationMoveHumanOutput(
		t,
		moveOutput,
		recoveryActivationWorkID,
		"failed",
		"init",
	)

	waitForRecoveryActivationWorkAtState(t, baseURL, recoveryActivationWorkID, "complete", 15*time.Second)

	listed := support.ListDefaultSessionWork(t, baseURL)
	completeLocation := support.WorkCustomerLocation(recoveryActivationWorkType, "complete")
	if !support.HasWorkAtCustomerState(listed, recoveryActivationWorkID, completeLocation) {
		t.Fatalf(
			"GET /work list = %#v, want %s after public recovery move",
			listed.Results,
			completeLocation,
		)
	}
	server.Stop(t)
	terminalObservation.Wait(15 * time.Second)
}

// TestWorkRecordingsReadActivatesThroughRootBuildProcessAfterLifecycle proves
// public Work list reads activate after runtime lifecycle on a process constructed
// only through root.BuildProcess with edges.Edges effect replacement. Recordings-
// backed read contract coverage remains under tests/functional/work/recordings;
// this test closes the explicit public-process activation gap for the read surface.
func TestWorkRecordingsReadActivatesThroughRootBuildProcessAfterLifecycle(t *testing.T) {
	t.Parallel()

	dir := support.ScaffoldFactory(t, recordingsActivationFactoryConfig())
	edges := serviceedges.Edges{}

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
	})
	t.Cleanup(func() { server.Stop(t) })

	baseURL := server.URL()
	workName := "fun-work-recordings-activation-task"
	workTypeName := recordingsActivationWorkType
	terminalObservation := support.OpenDefaultSessionTerminalFactoryEventObservation(t, baseURL)
	support.UpsertDefaultSessionWorkRequest(t, baseURL, factoryapi.WorkRequest{
		RequestId: "fun-work-recordings-activation-request",
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{{
			Name:         workName,
			WorkTypeName: &workTypeName,
			Payload:      map[string]string{"title": "FUN Work recordings-read activation"},
		}},
	})
	support.WaitForSessionWorkTerminalFromFactoryEvents(t, baseURL, "~default", 15*time.Second)

	process := support.BuildProcess(t, edges)
	listOutput := executeRecordingsActivationWorkListCLI(t, process, baseURL, "")
	listed := decodeRecordingsActivationWorkListJSON(t, listOutput)

	found := false
	for _, item := range listed.Results {
		if item.Name != workName {
			continue
		}
		if support.StringPointerValue(item.WorkTypeName) != recordingsActivationWorkType {
			t.Fatalf(
				"work list workTypeName = %q, want %q",
				support.StringPointerValue(item.WorkTypeName),
				recordingsActivationWorkType,
			)
		}
		if item.State == nil || item.State.Name != "complete" {
			t.Fatalf("work list state = %#v, want complete", item.State)
		}
		found = true
		break
	}
	if !found {
		t.Fatalf("work list missing %q at complete: %#v", workName, listed.Results)
	}
	server.Stop(t)
	terminalObservation.Wait(15 * time.Second)
}

func recordingsActivationFactoryConfig() map[string]any {
	return map[string]any{
		"name": "work-recordings-activation",
		"workTypes": []map[string]any{
			{
				"name": recordingsActivationWorkType,
				"states": []map[string]any{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "mock-worker"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process-task",
				"worker":    "mock-worker",
				"inputs":    []map[string]string{{"workType": recordingsActivationWorkType, "state": "init"}},
				"outputs":   []map[string]string{{"workType": recordingsActivationWorkType, "state": "complete"}},
				"onFailure": []map[string]string{{"workType": recordingsActivationWorkType, "state": "failed"}},
			},
		},
	}
}

// TestWorkVisualizationActivatesThroughRootBuildProcessAfterLifecycle proves
// dependency-graph visualization activates through the public Work CLI after
// runtime lifecycle on a process constructed only through root.BuildProcess with
// edges.Edges effect replacement. Detailed visualization coverage remains under
// tests/functional/work/visualization; this test closes the explicit
// public-process activation gap.
func TestWorkVisualizationActivatesThroughRootBuildProcessAfterLifecycle(t *testing.T) {
	t.Parallel()

	process := support.BuildProcess(t, serviceedges.Edges{})
	batchPath := writeVisualizationActivationBatchFile(t)

	output := executeVisualizationActivationCLI(t, process, batchPath)
	assertVisualizationActivationGraphOutput(t, output)
}

func waitForRecoveryActivationWorkAtState(
	t *testing.T,
	baseURL, workID, stateName string,
	timeout time.Duration,
) {
	t.Helper()
	support.WaitForSessionWorkIDsAtStateFromFactoryEvents(
		t,
		baseURL,
		"~default",
		[]string{workID},
		stateName,
		timeout,
	)
}

func executeRecoveryActivationWorkMoveCLI(
	t *testing.T,
	process support.Process,
	serverURL, workID, stateName string,
) string {
	t.Helper()

	home := t.TempDir()
	args := []string{
		"you", "--server", serverURL,
		"work", "move", workID, stateName,
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = recoveryActivationHomeEnvironment(home)
	inputs.Input.WorkingDirectory = home
	stdinIsTTY := true
	inputs.Input.StdinIsTTY = &stdinIsTTY
	inputs.Input.Stdin = strings.NewReader("")
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(work move) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	return inputs.Stdout()
}

func assertRecoveryActivationMoveHumanOutput(
	t *testing.T,
	output, workID, previousState, newState string,
) {
	t.Helper()
	for _, marker := range []string{
		"Work ID:\t" + workID,
		"Previous state:\t" + previousState,
		"New state:\t" + newState,
	} {
		if !strings.Contains(output, marker) {
			t.Fatalf("work move output missing %q:\n%s", marker, output)
		}
	}
}

func executeRecordingsActivationWorkListCLI(
	t *testing.T,
	process support.Process,
	serverURL, sessionID string,
) string {
	t.Helper()

	home := t.TempDir()
	args := []string{
		"you", "--server", serverURL, "--json",
		"work", "list",
		"--name", "fun-work-recordings-activation-task",
	}
	if sessionID != "" {
		args = append(args, "--session", sessionID)
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = recoveryActivationHomeEnvironment(home)
	inputs.Input.WorkingDirectory = home
	stdinIsTTY := true
	inputs.Input.StdinIsTTY = &stdinIsTTY
	inputs.Input.Stdin = strings.NewReader("")
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(work list) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	return inputs.Stdout()
}

func decodeRecordingsActivationWorkListJSON(t *testing.T, output string) factoryapi.ListWorkResponse {
	t.Helper()

	var listed factoryapi.ListWorkResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &listed); err != nil {
		t.Fatalf("decode work list JSON: %v\noutput:\n%s", err, output)
	}
	return listed
}

func writeVisualizationActivationBatchFile(t *testing.T) string {
	t.Helper()

	batchPath := filepath.Join(t.TempDir(), "fun-work-visualize-activation.json")
	content := fmt.Sprintf(`{
  "requestId": %q,
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "plan", "workTypeName": "task"},
    {"name": "ship-release", "workTypeName": "task"}
  ],
  "relations": [
    {"type": "DEPENDS_ON", "sourceWorkName": "ship-release", "targetWorkName": "plan"}
  ]
}`, visualizationActivationRequestID)
	if err := os.WriteFile(batchPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write visualize batch file: %v", err)
	}
	return batchPath
}

func executeVisualizationActivationCLI(t *testing.T, process support.Process, batchPath string) string {
	t.Helper()

	home := t.TempDir()
	args := []string{"you", "work", "render", batchPath}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = recoveryActivationHomeEnvironment(home)
	inputs.Input.WorkingDirectory = home
	stdinIsTTY := true
	inputs.Input.StdinIsTTY = &stdinIsTTY
	inputs.Input.Stdin = strings.NewReader("")
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(work render) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	return inputs.Stdout()
}

func assertVisualizationActivationGraphOutput(t *testing.T, output string) {
	t.Helper()

	if !strings.HasPrefix(output, "flowchart TD\n") {
		t.Fatalf("visualize output missing flowchart header:\n%s", output)
	}
	for _, want := range []string{
		`plan["plan"]`,
		`"ship-release"["ship-release"]`,
		`"ship-release" --> plan`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("visualize output missing %q:\n%s", want, output)
		}
	}
}

func recoveryActivationHomeEnvironment(home string) []string {
	if runtime.GOOS == "windows" {
		return []string{"USERPROFILE=" + home}
	}
	if runtime.GOOS == "plan9" {
		return []string{"home=" + home}
	}
	return []string{"HOME=" + home}
}
