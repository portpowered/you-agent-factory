package functional_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"go.uber.org/zap"
)

func TestMockWorkers_EndToEndSmokeRunsMixedOutcomesWithoutLiveProviderCredentials(t *testing.T) {
	skipSlowFunctionalSmokeInShort(t, "slow mock-workers end-to-end smoke")
	dir := scaffoldFactory(t, mixedMockWorkersSmokeConfig())
	writeAgentConfig(t, dir, "accept-agent", mockWorkersSmokeModelWorkerConfig())
	writeAgentConfig(t, dir, "reject-agent", mockWorkersSmokeModelWorkerConfig())
	writeAgentConfig(t, dir, "script-worker", `---
type: SCRIPT_WORKER
command: echo
args:
  - "unmocked-script-output"
---
`)

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkID:     "mock-smoke-accept-work",
		WorkTypeID: "accept-task",
		TraceID:    "mock-smoke-accept-trace",
		Payload:    []byte(`{"title":"default accept"}`),
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkID:     "mock-smoke-reject-work",
		WorkTypeID: "reject-task",
		TraceID:    "mock-smoke-reject-trace",
		Payload:    []byte(`{"title":"configured reject"}`),
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkID:     "mock-smoke-script-work",
		WorkTypeID: "script-task",
		TraceID:    "mock-smoke-script-trace",
		Payload:    []byte(`{"title":"configured script"}`),
	})

	sideEffectPath := filepath.Join(t.TempDir(), "mixed-mock-script-side-effect.txt")
	mockWorkersPath := writeMixedMockWorkersSmokeConfig(t, sideEffectPath)
	artifactPath := filepath.Join(t.TempDir(), "mixed-mock-workers.replay.json")

	output, err := runRecordReplayCLIWithCapturedStdout(t, runcli.RunConfig{
		Dir:                        dir,
		Port:                       0,
		MockWorkersEnabled:         true,
		MockWorkersConfigPath:      mockWorkersPath,
		RecordPath:                 artifactPath,
		SuppressDashboardRendering: true,
		Logger:                     zap.NewNop(),
	})
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

	replayHarness := testutil.AssertReplaySucceeds(t, artifactPath, 10*time.Second)
	replayHarness.Service.Assert().
		PlaceTokenCount("accept-task:done", 1).
		PlaceTokenCount("reject-task:failed", 1).
		PlaceTokenCount("script-task:done", 1).
		HasNoTokenInPlace("accept-task:init").
		HasNoTokenInPlace("reject-task:init").
		HasNoTokenInPlace("script-task:init")
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
		"onFailure": map[string]string{"workType": workType, "state": "failed"},
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
	cfg := factoryconfig.MockWorkersConfig{
		MockWorkers: []factoryconfig.MockWorkerConfig{
			{
				ID:              "reject-agent-by-workstation",
				WorkerName:      "reject-agent",
				WorkstationName: "reject-process",
				RunType:         factoryconfig.MockWorkerRunTypeReject,
				RejectConfig: &factoryconfig.MockWorkerRejectConfig{
					Stdout:   "mixed reject stdout",
					Stderr:   "mixed reject stderr",
					ExitCode: &exitCode,
				},
			},
			{
				ID:              "script-worker-side-effect",
				WorkerName:      "script-worker",
				WorkstationName: "script-process",
				RunType:         factoryconfig.MockWorkerRunTypeScript,
				ScriptConfig: &factoryconfig.MockWorkerScriptConfig{
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
	for _, event := range artifact.Events {
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

	if got := outcomes["accept-process"].Outcome; got != factoryapi.WorkOutcome(interfaces.OutcomeAccepted) {
		t.Fatalf("accept-process outcome = %s, want %s", got, interfaces.OutcomeAccepted)
	}
	rejectResult := outcomes["reject-process"]
	if rejectResult.Outcome != factoryapi.WorkOutcome(interfaces.OutcomeFailed) {
		t.Fatalf("reject-process outcome = %s, want %s", rejectResult.Outcome, interfaces.OutcomeFailed)
	}
	if rejectResult.FailureReason == nil || *rejectResult.FailureReason == "" {
		t.Fatal("reject-process result missing failure reason")
	}
	if !strings.Contains(stringPointerValue(rejectResult.Error), "exited with code 13") {
		t.Fatalf("reject-process error = %q, want exit-code failure", stringPointerValue(rejectResult.Error))
	}
	scriptResult := outcomes["script-process"]
	if scriptResult.Outcome != factoryapi.WorkOutcome(interfaces.OutcomeAccepted) {
		t.Fatalf("script-process outcome = %s, want %s", scriptResult.Outcome, interfaces.OutcomeAccepted)
	}
	if !strings.Contains(stringPointerValue(scriptResult.Output), "mock script helper wrote file") {
		t.Fatalf("script-process output = %q, want helper output", stringPointerValue(scriptResult.Output))
	}
}
