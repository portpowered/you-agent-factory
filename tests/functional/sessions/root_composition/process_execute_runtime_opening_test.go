package root_composition_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestProcessExecuteRuntimeOpeningThroughReusableRootProcess preserves the
// success and failure opening witnesses through one immutable root process.
// Each subtest retains its own Factory, home, and public observation boundary;
// only compatible process wiring is reused.
func TestProcessExecuteRuntimeOpeningThroughReusableRootProcess(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)

	router := &reusableRootAPIServerStarter{}
	identities := &processExecuteOpeningIdentities{}
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		APIServerStarter: router.start,
		FactorySessionIDGenerator: func() string {
			return fmt.Sprintf("process-execute-session-%d", identities.session.Add(1))
		},
		FactorySessionRuntimeInstanceIDGenerator: func() string {
			return fmt.Sprintf("process-execute-runtime-%d", identities.runtime.Add(1))
		},
		FactorySessionReplayRecordingReader: func(path string) ([]byte, error) {
			identities.replayPath.Store(path)
			return nil, errors.New("replay fixture unavailable")
		},
	})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	support.CleanupProcess(t, process)

	t.Run("opens requested Factory Session", func(t *testing.T) {
		testProcessExecuteOpensRequestedFactorySessionThroughRoot(t, process, router)
	})
	t.Run("corrupt current-board recording stops opening", func(t *testing.T) {
		testProcessExecuteCorruptCurrentBoardRecordingStopsOpening(t, process)
	})
	t.Run("unavailable Factory does not register session", func(t *testing.T) {
		testProcessExecuteUnavailableFactoryDoesNotRegisterSession(t, process, identities)
	})
	t.Run("replay loader failure stops before live activation", func(t *testing.T) {
		testProcessExecuteReplayLoaderFailureStopsBeforeLiveActivation(t, process, identities)
	})
}

// testProcessExecuteOpensRequestedFactorySessionThroughRoot proves that an
// ordinary customer process invocation opens the selected Factory source and
// publishes one canonical Factory Session with observable lifecycle controls.
// The API reads and controls below are API-owned session observations; the
// opening itself is performed by Process.Execute on the root-built process.
func testProcessExecuteOpensRequestedFactorySessionThroughRoot(
	t *testing.T,
	process support.Process,
	router *reusableRootAPIServerStarter,
) {
	t.Helper()
	factoryDir := support.ScaffoldFactory(t, processExecuteRuntimeOpeningFactoryConfig())
	api := support.NewProcessAPIServer()
	router.setCurrent(api)

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--factory", filepath.Join(factoryDir, "factory.json"),
		"--continuously", "--with-server", "--quiet", "--no-record",
	})
	home := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = factoryDir
	command := support.StartProcessCommand(t, process, inputs.Input)

	baseURL := api.WaitForURL(t)
	session := support.GetDefaultSession(t, baseURL)
	if session.Id == "" || !session.IsDefault {
		t.Fatalf("opened Factory Session = %#v, want a canonical default identity", session)
	}
	if filepath.Clean(session.FactoryDir) != filepath.Clean(factoryDir) ||
		filepath.Clean(session.FolderPath) != filepath.Clean(factoryDir) {
		t.Fatalf(
			"opened Factory Session paths = factoryDir:%q folderPath:%q, want %q",
			session.FactoryDir, session.FolderPath, factoryDir,
		)
	}
	if session.Runtime.StreamIdentity == nil {
		t.Fatalf("opened Factory Session runtime = %#v, want stream identity", session.Runtime)
	}
	identity := session.Runtime.StreamIdentity
	if identity.FactorySessionID != session.Id ||
		identity.LogicalSessionKeyID == "" || identity.StreamGenerationID == "" {
		t.Fatalf("opened Factory Session stream identity = %#v, want canonical session linkage", identity)
	}

	current := support.GetJSON[factoryapi.Factory](
		t,
		baseURL+"/factory-sessions/"+factorysessions.DefaultSessionID+"/factory",
	)
	if current.FactoryDirectory == nil || filepath.Clean(*current.FactoryDirectory) != filepath.Clean(factoryDir) {
		t.Fatalf("session current Factory directory = %#v, want %q", current.FactoryDirectory, factoryDir)
	}
	if current.WorkTypes == nil || len(*current.WorkTypes) != 1 || (*current.WorkTypes)[0].Name != "task" {
		t.Fatalf("session current Factory work types = %#v, want selected task definition", current.WorkTypes)
	}

	listed := support.GetJSON[factoryapi.ListFactorySessionsResponse](t, baseURL+"/factory-sessions")
	if !sessionSummaryContains(listed.Sessions, session.Id) {
		t.Fatalf("listed Factory Sessions = %#v, want opened session %q", listed.Sessions, session.Id)
	}

	pause := postSessionsLifecycleControl(
		t,
		baseURL,
		factorysessions.DefaultSessionID,
		factoryapi.FactorySessionLifecycleControlKindPause,
	)
	if pause.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("pause response = %#v, want accepted lifecycle control", pause)
	}
	paused := support.GetDefaultSession(t, baseURL)
	if paused.Runtime.LifecycleControlStatus == nil ||
		*paused.Runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("paused Factory Session runtime = %#v, want PAUSED", paused.Runtime)
	}

	resume := postSessionsLifecycleControl(
		t,
		baseURL,
		factorysessions.DefaultSessionID,
		factoryapi.FactorySessionLifecycleControlKindResume,
	)
	if resume.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume response = %#v, want accepted lifecycle control", resume)
	}
	running := support.GetDefaultSession(t, baseURL)
	if running.Runtime.LifecycleControlStatus == nil ||
		*running.Runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("resumed Factory Session runtime = %#v, want RUNNING", running.Runtime)
	}

	if command.Err() != nil {
		t.Fatalf("Process.Execute() returned before lifecycle observations: %v", command.Err())
	}
	command.Stop(t)
}

// TestProcessExecuteCorruptCurrentBoardRecordingStopsOpening proves that a
// corrupt current-board artifact is rejected at the Process.Execute opening
// boundary and remains available for investigation.
func testProcessExecuteCorruptCurrentBoardRecordingStopsOpening(t *testing.T, process support.Process) {
	t.Helper()

	factoryDir := support.ScaffoldFactory(t, processExecuteRuntimeOpeningFactoryConfig())
	recordPath := filepath.Join(factoryDir, "current-board.json")
	corruptPayload := []byte(`{"schemaVersion":"recordings.portable-artifact.v1","summary":{}}`)
	if err := os.WriteFile(recordPath, corruptPayload, 0o600); err != nil {
		t.Fatalf("write corrupt current-board recording: %v", err)
	}

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", factoryDir, "--record", recordPath, "--quiet",
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+t.TempDir(), "USERPROFILE="+t.TempDir())
	inputs.Input.WorkingDirectory = factoryDir

	err := process.Execute(inputs.Input)
	if err == nil || !strings.Contains(err.Error(), "CORRUPT_HISTORY") ||
		!strings.Contains(err.Error(), filepath.Base(recordPath)) {
		t.Fatalf("Process.Execute() error = %v, want corrupt current-board diagnostic", err)
	}
	contents, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("read corrupt current-board recording after failed startup: %v", err)
	}
	if string(contents) != string(corruptPayload) {
		t.Fatal("failed startup changed the corrupt current-board recording")
	}
}

// TestProcessExecuteUnavailableFactoryDoesNotRegisterSession proves that an
// unavailable Factory definition fails at the customer process boundary before
// Factory Session or runtime identity allocation can publish partial state.
func testProcessExecuteUnavailableFactoryDoesNotRegisterSession(
	t *testing.T,
	process support.Process,
	identities *processExecuteOpeningIdentities,
) {
	t.Helper()

	missingFactory := filepath.Join(t.TempDir(), "missing", "factory.json")
	sessionBefore := identities.session.Load()
	runtimeBefore := identities.runtime.Load()

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--factory", missingFactory, "--quiet", "--no-record",
	})
	home := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = filepath.Dir(missingFactory)

	err := process.Execute(inputs.Input)
	if err == nil || !strings.Contains(err.Error(), filepath.Base(missingFactory)) {
		t.Fatalf("Process.Execute() error = %v, want unavailable Factory diagnostic", err)
	}
	if got := identities.session.Load() - sessionBefore; got != 0 {
		t.Fatalf("Factory Session identity allocations after unavailable definition = %d, want 0", got)
	}
	if got := identities.runtime.Load() - runtimeBefore; got != 0 {
		t.Fatalf("runtime identity allocations after unavailable definition = %d, want 0", got)
	}
}

// TestProcessExecuteReplayLoaderFailureStopsBeforeLiveActivation proves that
// a replay-source failure is returned from the canonical runtime-opening
// boundary without attempting to assemble a live Factory Session.
func testProcessExecuteReplayLoaderFailureStopsBeforeLiveActivation(
	t *testing.T,
	process support.Process,
	identities *processExecuteOpeningIdentities,
) {
	t.Helper()

	workingDirectory := t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", workingDirectory,
		"--replay", "recording.json", "--no-record", "--quiet",
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+t.TempDir(), "USERPROFILE="+t.TempDir())
	inputs.Input.WorkingDirectory = workingDirectory

	err := process.Execute(inputs.Input)
	if err == nil || !strings.Contains(err.Error(), "failed to load --replay input") {
		t.Fatalf("Process.Execute() error = %v, want replay loader failure", err)
	}
	if got, _ := identities.replayPath.Load().(string); got != "recording.json" {
		t.Fatalf("replay input path = %q, want recording.json", got)
	}
}

type processExecuteOpeningIdentities struct {
	session    atomic.Int32
	runtime    atomic.Int32
	replayPath atomic.Value
}

func processExecuteRuntimeOpeningFactoryConfig() map[string]any {
	return map[string]any{
		"name": "process-execute-runtime-opening",
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
