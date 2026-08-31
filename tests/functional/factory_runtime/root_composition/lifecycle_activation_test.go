package root_composition_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestFactoryRuntimeControlObservationAndDispatchPlanActivateThroughRootBuildProcessAfterLifecycle
// proves Factory Runtime control, observation, and dispatch-plan activate through
// published public HTTP surfaces after runtime lifecycle on a process constructed
// only through root.BuildProcess with Factory Runtime effects replaced via edges.Edges.
// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
func TestFactoryRuntimeControlObservationAndDispatchPlanActivateThroughRootBuildProcessAfterLifecycle(
	t *testing.T,
) {
	t.Parallel()

	enterSharedRootCompositionScenario(t)
	fixture := sharedRootCompositionFixtureForTest(t)
	dir := support.ScaffoldFactory(t, factoryRuntimeLifecycleActivationFactoryConfig())
	session := openSharedRootCompositionLiveSession(t, dir)
	baseURL := fixture.baseURL
	recorder := fixture.recorder

	if got := recorder.totalControl(); got <= 0 {
		t.Fatalf("control effect calls after runtime lifecycle = %d, want > 0 via edges", got)
	}
	if got := recorder.totalObservation(); got <= 0 {
		t.Fatalf("observation effect calls after runtime lifecycle = %d, want > 0 via edges", got)
	}

	dispatchBefore := recorder.totalDispatchPlan()

	status := sharedRootCompositionSessionStatus(t, fixture, session.sessionID)
	if status.FactoryState != string(interfaces.FactoryStateRunning) {
		t.Fatalf("GET /status factoryState = %q, want RUNNING", status.FactoryState)
	}
	if status.RuntimeStatus != string(interfaces.RuntimeStatusIdle) {
		t.Fatalf("GET /status runtimeStatus = %q, want IDLE", status.RuntimeStatus)
	}
	if status.TotalTokens != 0 {
		t.Fatalf("GET /status totalTokens = %d, want 0 before work submission", status.TotalTokens)
	}

	pause := postFactoryRuntimeLifecycleControl(
		t,
		baseURL,
		session.sessionID,
		factoryapi.FactorySessionLifecycleControlKindPause,
	)
	if pause.Operation != factoryapi.FactorySessionLifecycleControlKindPause ||
		pause.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("pause response = %#v, want accepted pause", pause)
	}
	pausedSession := sharedRootCompositionSessionRead(t, fixture, session.sessionID)
	if pausedSession.Runtime.LifecycleControlStatus == nil ||
		*pausedSession.Runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf(
			"paused session lifecycleControlStatus = %#v, want %q",
			pausedSession.Runtime.LifecycleControlStatus,
			factoryapi.FactorySessionDurableLifecycleStatusPaused,
		)
	}

	resume := postFactoryRuntimeLifecycleControl(
		t,
		baseURL,
		session.sessionID,
		factoryapi.FactorySessionLifecycleControlKindResume,
	)
	if resume.Operation != factoryapi.FactorySessionLifecycleControlKindResume ||
		resume.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume response = %#v, want accepted resume", resume)
	}
	runningSession := sharedRootCompositionSessionRead(t, fixture, session.sessionID)
	if runningSession.Runtime.LifecycleControlStatus == nil ||
		*runningSession.Runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf(
			"resumed session lifecycleControlStatus = %#v, want %q",
			runningSession.Runtime.LifecycleControlStatus,
			factoryapi.FactorySessionDurableLifecycleStatusRunning,
		)
	}

	submitted := support.SubmitSessionWorkAt(t, baseURL, session.sessionID, factoryapi.SubmitWorkRequest{
		Name:         stringPointer("factory-runtime-lifecycle-activation"),
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "activate dispatch-plan through public process"},
	})
	workID := stringPointerValue(submitted.WorkId)
	if workID == "" {
		t.Fatalf("submit work missing work id: %#v", submitted)
	}

	support.WaitForSessionTerminalStatus(t, baseURL, session.sessionID, 15*time.Second)
	completedStatus := sharedRootCompositionSessionStatus(t, fixture, session.sessionID)
	if completedStatus.Categories.Terminal != 1 {
		t.Fatalf("GET /status terminal count = %d, want 1 after dispatch", completedStatus.Categories.Terminal)
	}
	if completedStatus.RuntimeStatus != string(interfaces.RuntimeStatusIdle) {
		t.Fatalf("GET /status runtimeStatus after completion = %q, want IDLE", completedStatus.RuntimeStatus)
	}
	if got := recorder.totalDispatchPlan() - dispatchBefore; got <= 0 {
		t.Fatalf("dispatch-plan effect calls after work submission = %d, want > 0 via edges", got)
	}

	listed := sharedRootCompositionSessionWork(t, fixture, session.sessionID)
	if !support.HasWorkAtCustomerState(listed, workID, support.WorkCustomerLocation("task", "complete")) {
		t.Fatalf("work %q did not reach task:complete: %#v", workID, listed.Results)
	}

	events := support.GetFactoryEventsForSessionAt(t, baseURL, session.sessionID)
	dispatches := support.ObserveDispatchEvents(t, events)
	foundCompleted := false
	for _, dispatch := range dispatches {
		if support.DispatchObservationIncludesWork(dispatch, workID) && dispatch.Response != nil {
			foundCompleted = true
			break
		}
	}
	if !foundCompleted {
		t.Fatalf("public Factory Events missing completed dispatch for work %q", workID)
	}
	session.close(t)
}

func factoryRuntimeLifecycleActivationFactoryConfig() map[string]any {
	return map[string]any{
		"name": "factory-runtime-lifecycle-activation",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

func postFactoryRuntimeLifecycleControl(
	t *testing.T,
	baseURL string,
	sessionID string,
	operation factoryapi.FactorySessionLifecycleControlKind,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()
	pathSegment := "pause"
	if operation == factoryapi.FactorySessionLifecycleControlKindResume {
		pathSegment = "resume"
	}
	return postFactoryRuntimeJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		baseURL+"/factory-sessions/"+sessionID+"/"+pathSegment,
		factoryapi.FactorySessionLifecycleControlRequest{},
		"apply Factory Runtime lifecycle control through public HTTP surface",
	)
}

func postFactoryRuntimeJSON[T any](t *testing.T, endpoint string, request any, failurePrefix string) T {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("%s: marshal request: %v", failurePrefix, err)
	}
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("%s: POST %s: %v", failurePrefix, endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("%s: POST %s status = %d, want success: %s", failurePrefix, endpoint, response.StatusCode, payload)
	}
	var decoded T
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("%s: decode %s response: %v", failurePrefix, endpoint, err)
	}
	return decoded
}

func stringPointer(value string) *string {
	return &value
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

type factoryRuntimeDelegatingRecorder struct {
	local             platformfilesystem.Local
	workflowHome      string
	idGeneratorCalls  atomic.Int32
	directoryMkdir    atomic.Int32
	directoryStat     atomic.Int32
	inputReadDir      atomic.Int32
	inputReadFile     atomic.Int32
	inputStat         atomic.Int32
	inputWalk         atomic.Int32
	dispatchRecord    atomic.Int32
	workflowReadDir   atomic.Int32
	workflowReadFile  atomic.Int32
	workflowStat      atomic.Int32
	workflowSymlink   atomic.Int32
	workflowHomeCalls atomic.Int32
	scriptCommand     atomic.Int32
}

func (recorder *factoryRuntimeDelegatingRecorder) edges() serviceedges.Edges {
	return serviceedges.Edges{
		FactoryRuntimeIDGenerator:                   recorder.generateID,
		FactoryRuntimeDirectories:                   &factoryRuntimeDelegatingDirectory{recorder: recorder},
		FactoryRuntimeInputs:                        &factoryRuntimeDelegatingInput{recorder: recorder},
		FactoryRuntimeInputDirectoryWalker:          recorder.walkInputs,
		DispatchRecorder:                            recorder.recordDispatch,
		FactoryRuntimeWorkflowSources:               &factoryRuntimeDelegatingWorkflowSource{recorder: recorder},
		FactoryRuntimeWorkflowSourceResolveSymlinks: recorder.resolveWorkflowSymlink,
		FactoryRuntimeWorkflowHome:                  recorder.resolveWorkflowHome,
		ScriptCommandRunner:                         &factoryRuntimeDelegatingScriptCommand{recorder: recorder},
	}
}

func (recorder *factoryRuntimeDelegatingRecorder) totalControl() int32 {
	return recorder.idGeneratorCalls.Load() +
		recorder.directoryMkdir.Load() +
		recorder.directoryStat.Load()
}

func (recorder *factoryRuntimeDelegatingRecorder) totalObservation() int32 {
	return recorder.inputReadDir.Load() +
		recorder.inputReadFile.Load() +
		recorder.inputStat.Load() +
		recorder.inputWalk.Load()
}

func (recorder *factoryRuntimeDelegatingRecorder) totalDispatchPlan() int32 {
	return recorder.dispatchRecord.Load()
}

func (recorder *factoryRuntimeDelegatingRecorder) totalJavaScriptOrchestration() int32 {
	return recorder.workflowReadDir.Load() +
		recorder.workflowReadFile.Load() +
		recorder.workflowStat.Load() +
		recorder.workflowSymlink.Load() +
		recorder.workflowHomeCalls.Load() +
		recorder.scriptCommand.Load()
}

func (recorder *factoryRuntimeDelegatingRecorder) generateID() string {
	n := recorder.idGeneratorCalls.Add(1)
	return fmt.Sprintf("factory-runtime-activation-edge-id-%d", n)
}

func (recorder *factoryRuntimeDelegatingRecorder) walkInputs(root string, walkFn fs.WalkDirFunc) error {
	recorder.inputWalk.Add(1)
	return filepath.WalkDir(root, walkFn)
}

func (recorder *factoryRuntimeDelegatingRecorder) recordDispatch(recordings.FactoryDispatchRecord) {
	recorder.dispatchRecord.Add(1)
}

func (recorder *factoryRuntimeDelegatingRecorder) resolveWorkflowSymlink(path string) (string, error) {
	recorder.workflowSymlink.Add(1)
	return filepath.EvalSymlinks(path)
}

func (recorder *factoryRuntimeDelegatingRecorder) resolveWorkflowHome() (string, error) {
	recorder.workflowHomeCalls.Add(1)
	return recorder.workflowHome, nil
}

type factoryRuntimeDelegatingDirectory struct {
	recorder *factoryRuntimeDelegatingRecorder
}

func (adapter *factoryRuntimeDelegatingDirectory) MkdirAll(path string, mode fs.FileMode) error {
	adapter.recorder.directoryMkdir.Add(1)
	return adapter.recorder.local.MkdirAll(path, mode)
}

func (adapter *factoryRuntimeDelegatingDirectory) Stat(path string) (fs.FileInfo, error) {
	adapter.recorder.directoryStat.Add(1)
	return adapter.recorder.local.Stat(path)
}

type factoryRuntimeDelegatingInput struct {
	recorder *factoryRuntimeDelegatingRecorder
}

func (adapter *factoryRuntimeDelegatingInput) ReadDir(path string) ([]fs.DirEntry, error) {
	adapter.recorder.inputReadDir.Add(1)
	return adapter.recorder.local.ReadDir(path)
}

func (adapter *factoryRuntimeDelegatingInput) ReadFile(path string) ([]byte, error) {
	adapter.recorder.inputReadFile.Add(1)
	return adapter.recorder.local.ReadFile(path)
}

func (adapter *factoryRuntimeDelegatingInput) Stat(path string) (fs.FileInfo, error) {
	adapter.recorder.inputStat.Add(1)
	return adapter.recorder.local.Stat(path)
}

type factoryRuntimeDelegatingWorkflowSource struct {
	recorder *factoryRuntimeDelegatingRecorder
}

func (adapter *factoryRuntimeDelegatingWorkflowSource) ReadDir(path string) ([]fs.DirEntry, error) {
	adapter.recorder.workflowReadDir.Add(1)
	return adapter.recorder.local.ReadDir(path)
}

func (adapter *factoryRuntimeDelegatingWorkflowSource) ReadFile(path string) ([]byte, error) {
	adapter.recorder.workflowReadFile.Add(1)
	return adapter.recorder.local.ReadFile(path)
}

func (adapter *factoryRuntimeDelegatingWorkflowSource) Stat(path string) (fs.FileInfo, error) {
	adapter.recorder.workflowStat.Add(1)
	return adapter.recorder.local.Stat(path)
}

type factoryRuntimeDelegatingScriptCommand struct {
	recorder *factoryRuntimeDelegatingRecorder
}

func (adapter *factoryRuntimeDelegatingScriptCommand) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	adapter.recorder.scriptCommand.Add(1)
	return platformprocess.CommandResult{ExitCode: 0}, nil
}
