package providers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestMockWorkers_EndToEndSmokeRunsMixedOutcomesWithoutLiveProviderCredentials
// is isolated because it intentionally crosses record and replay process
// modes and proves a real mock-script child filesystem side effect.
func TestMockWorkers_EndToEndSmokeRunsMixedOutcomesWithoutLiveProviderCredentials(t *testing.T) {
	if testing.Short() {
		t.Skip("slow mock-workers end-to-end smoke")
	}

	dir := support.ScaffoldFactory(t, mixedMockWorkersSmokeConfig())
	support.WriteAgentConfig(t, dir, "accept-agent", mockWorkersSmokeModelWorkerConfig())
	support.WriteAgentConfig(t, dir, "reject-agent", mockWorkersSmokeModelWorkerConfig())
	support.WriteAgentConfig(t, dir, "script-worker", `---
type: SCRIPT_WORKER
command: echo
args:
  - "unmocked-script-output"
---
`)

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     "mock-smoke-accept-work",
		WorkTypeID: "accept-task",
		TraceID:    "mock-smoke-accept-trace",
		Payload:    []byte(`{"title":"default accept"}`),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     "mock-smoke-reject-work",
		WorkTypeID: "reject-task",
		TraceID:    "mock-smoke-reject-trace",
		Payload:    []byte(`{"title":"configured reject"}`),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     "mock-smoke-script-work",
		WorkTypeID: "script-task",
		TraceID:    "mock-smoke-script-trace",
		Payload:    []byte(`{"title":"configured script"}`),
	})

	sideEffectPath := filepath.Join(t.TempDir(), "mixed-mock-script-side-effect.txt")
	mockWorkersPath := writeMixedMockWorkersSmokeConfig(t, sideEffectPath)
	artifactPath := filepath.Join(t.TempDir(), "mixed-mock-workers.replay.json")

	output, err := runRecordReplayCLIWithCapturedStdoutForProviders(t, dir, mockWorkersPath, artifactPath)
	if err != nil {
		t.Fatalf("mock-worker smoke run failed: %v", err)
	}
	if output != "" {
		t.Fatalf("mock-worker smoke stdout = %q, want empty output with dashboard rendering suppressed", output)
	}

	rawSideEffect, err := os.ReadFile(sideEffectPath)
	if err != nil {
		t.Fatalf("read mock script side effect: %v", err)
	}
	if string(rawSideEffect) != "mixed mock script side effect" {
		t.Fatalf("script side effect = %q, want %q", rawSideEffect, "mixed mock script side effect")
	}

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	assertMockWorkersSmokeRecordedOutcomes(t, artifact)

	replayServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: t.TempDir(),
		Args:       []string{"--replay", artifactPath},
	})
	defer replayServer.Stop(t)
	support.WaitForTerminalStatus(t, replayServer.URL(), 10*time.Second)
	listed := support.ListDefaultSessionWork(t, replayServer.URL())
	for placeID, want := range map[string]int{
		"accept-task:done": 1, "reject-task:failed": 1, "script-task:done": 1,
		"accept-task:init": 0, "reject-task:init": 0, "script-task:init": 0,
	} {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
}

func mixedMockWorkersSmokeConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{
			mockWorkersSmokeWorkType("accept-task"),
			mockWorkersSmokeWorkType("reject-task"),
			mockWorkersSmokeWorkType("script-task"),
		},
		"workers": []map[string]string{
			{"name": "accept-agent"},
			{"name": "reject-agent"},
			{"name": "script-worker"},
		},
		"workstations": []map[string]any{
			mockWorkersSmokeWorkstation("accept-process", "accept-agent", "accept-task"),
			mockWorkersSmokeWorkstation("reject-process", "reject-agent", "reject-task"),
			mockWorkersSmokeWorkstation("script-process", "script-worker", "script-task"),
		},
	}
}

func mockWorkersSmokeWorkType(name string) map[string]any {
	return map[string]any{
		"name": name,
		"states": []map[string]string{
			{"name": "init", "type": "INITIAL"},
			{"name": "done", "type": "TERMINAL"},
			{"name": "failed", "type": "FAILED"},
		},
	}
}

func mockWorkersSmokeWorkstation(name, workerName, workType string) map[string]any {
	return map[string]any{
		"name":      name,
		"worker":    workerName,
		"inputs":    []map[string]string{{"workType": workType, "state": "init"}},
		"outputs":   []map[string]string{{"workType": workType, "state": "done"}},
		"onFailure": []map[string]string{{"workType": workType, "state": "failed"}},
	}
}

func mockWorkersSmokeModelWorkerConfig() string {
	return `---
type: MODEL_WORKER
model: test-model
stopToken: COMPLETE
---
Mock-worker smoke model worker.
`
}

func writeMixedMockWorkersSmokeConfig(t *testing.T, sideEffectPath string) string {
	t.Helper()

	exitCode := 13
	cfg := workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{
			{
				ID:              "reject-agent-by-workstation",
				WorkerName:      "reject-agent",
				WorkstationName: "reject-process",
				RunType:         workers.MockWorkerRunTypeReject,
				RejectConfig: &workers.MockWorkerRejectConfig{
					Stdout:   "mixed reject stdout",
					Stderr:   "mixed reject stderr",
					ExitCode: &exitCode,
				},
			},
			{
				ID:              "script-worker-side-effect",
				WorkerName:      "script-worker",
				WorkstationName: "script-process",
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: os.Args[0],
					Args: []string{
						"-test.run=TestMockWorkers_ScriptHelper",
						"--",
						"write-file",
						sideEffectPath,
						"mixed mock script side effect",
					},
				},
			},
		},
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal mock-workers smoke config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "mock-workers.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write mock-workers smoke config: %v", err)
	}
	return path
}

func assertMockWorkersSmokeRecordedOutcomes(t *testing.T, artifact *interfaces.ReplayArtifact) {
	t.Helper()

	dispatchCount := 0
	for _, event := range testutil.GeneratedFactoryEvents(t, artifact.Events) {
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
			continue
		}
		if _, err := event.Payload.AsDispatchRequestEventPayload(); err != nil {
			t.Fatalf("decode dispatch created event %q: %v", event.Id, err)
		}
		dispatchCount++
	}
	if dispatchCount != 3 {
		t.Fatalf("recorded dispatch count = %d, want 3", dispatchCount)
	}
	completions := replayDispatchCompletedEvents(t, artifact)
	if len(completions) != 3 {
		t.Fatalf("recorded completion count = %d, want 3", len(completions))
	}

	outcomes := make(map[string]factoryapi.DispatchResponseEventPayload, len(completions))
	for _, completion := range completions {
		outcomes[completion.TransitionId] = completion
	}

	if got := outcomes["accept-process"].Outcome; got != factoryapi.WorkOutcome(workerexecution.OutcomeAccepted) {
		t.Fatalf("accept-process outcome = %s, want %s", got, workerexecution.OutcomeAccepted)
	}
	rejectResult := outcomes["reject-process"]
	if rejectResult.Outcome != factoryapi.WorkOutcome(workerexecution.OutcomeFailed) {
		t.Fatalf("reject-process outcome = %s, want %s", rejectResult.Outcome, workerexecution.OutcomeFailed)
	}
	if rejectResult.FailureDetail == nil || rejectResult.FailureDetail.Reason == "" {
		t.Fatal("reject-process result missing failure reason")
	}
	if string(rejectResult.FailureDetail.Reason) != string(workerexecution.WorkFailureTypePermanentBadRequest) {
		t.Fatalf("reject-process failure reason = %q, want neutral terminal refusal", rejectResult.FailureDetail.Reason)
	}
	if !strings.Contains(stringPointerValue(rejectResult.Error), "provider error: permanent_bad_request: provider rejected the execution request") {
		t.Fatalf("reject-process error = %q, want neutral terminal refusal", stringPointerValue(rejectResult.Error))
	}
	scriptResult := outcomes["script-process"]
	if scriptResult.Outcome != factoryapi.WorkOutcome(workerexecution.OutcomeAccepted) {
		t.Fatalf("script-process outcome = %s, want %s", scriptResult.Outcome, workerexecution.OutcomeAccepted)
	}
	if !strings.Contains(stringPointerValue(scriptResult.Output), "mock script helper wrote file") {
		t.Fatalf("script-process output = %q, want helper output", stringPointerValue(scriptResult.Output))
	}
}

func runRecordReplayCLIWithCapturedStdoutForProviders(
	t *testing.T,
	factoryDir string,
	mockWorkersPath string,
	artifactPath string,
) (string, error) {
	t.Helper()

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", factoryDir,
		"--with-mock-workers", mockWorkersPath,
		"--record", artifactPath,
		"--quiet",
	})
	inputs.WorkingDirectory = t.TempDir()
	err := process.Execute(inputs.Input)
	return inputs.Stdout(), err
}
