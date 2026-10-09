package inference_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const capturedProviderOutage = `unexpected status 503 Service Unavailable: {"detail":"Unable to verify Daybreak Blue access. Please try again."}`

func TestProviderOutageRecoversOriginalAndDependentWork(t *testing.T) {
	t.Parallel()
	structured, err := json.Marshal(map[string]any{"type": "turn.failed", "error": map[string]string{
		"type": "invalid_request_error", "message": capturedProviderOutage + " authentication forbidden; invalid request",
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		result platformprocess.CommandResult
	}{
		{"captured stderr", platformprocess.CommandResult{ExitCode: 1, Stderr: []byte(capturedProviderOutage)}},
		{"structured collision", platformprocess.CommandResult{ExitCode: 1, Stdout: structured}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
			support.ClearSeedInputs(t, dir)
			support.WriteAgentConfig(t, dir, "worker", sharedInferenceWithExecutorProvider(support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"), "CODEX"))
			support.WriteWorkstationConfig(t, dir, "process", "---\ntype: MODEL_WORKSTATION\nlimits:\n  maxRetries: 1\n---\nProcess the task.\n")
			loaded := loadOpeningRecordFixture(t, "codex", "success")
			success := platformprocess.CommandResult{Stdout: loaded.Stdout.Raw, Stderr: []byte(loaded.Stderr)}
			runner := testutil.NewProviderCommandRunner(tc.result, success, success)
			result := runProviderOutageBatch(t, dir, runner)
			if runner.CallCount() != 3 {
				t.Fatalf("provider attempts = %d, want two prerequisite attempts and one dependent attempt", runner.CallCount())
			}
			assertProviderOutageRecovery(t, result)
		})
	}
}

func runProviderOutageBatch(t *testing.T, dir string, runner platformprocess.CommandRunner) sharedInferenceFactoryResult {
	t.Helper()
	group := sharedInferenceGroup
	group.ensure(t)
	scenario := sharedInferenceScenario{commandRunner: runner}
	release := prepareSharedInferenceScenario(t, group, dir, scenario)
	defer release()
	defer group.commands.clear(dir)
	defer group.scripts.clear(dir)
	sessionID := openSharedInferenceSession(t, group, dir)
	defer closeSharedInferenceSession(t, group, sessionID)
	updateSharedInferenceRouteContext(t, group, dir, scenario, sessionID)
	batch := `{"requestId":"outage-recovery","type":"FACTORY_REQUEST_BATCH","works":[{"name":"prerequisite","workTypeName":"task","payload":{"title":"prerequisite"}},{"name":"dependent","workTypeName":"task","payload":{"title":"dependent"}}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"dependent","targetWorkName":"prerequisite","requiredState":"done"}]}`
	path := filepath.Join(dir, "outage-batch.json")
	if err := os.WriteFile(path, []byte(batch), 0600); err != nil {
		t.Fatal(err)
	}
	// Production's first outage backoff is 24-36 seconds. This scenario-owned
	// ceiling allows that behavior; it is not a readiness sleep or timing target.
	const ceiling = 2 * time.Minute
	ctx, cancel := context.WithTimeout(t.Context(), ceiling)
	defer cancel()
	inputs := support.FakeInputs(ctx, []string{"you", "submit", "batch", path, "--session", sessionID, "--server", group.baseURL, "--json"})
	inputs.Input.Env = sharedInferenceProcessEnvironment(group.homeDir)
	inputs.Input.WorkingDirectory = dir
	if err := group.process.Execute(inputs.Input); err != nil {
		t.Fatalf("submit batch: %v stdout=%s stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	support.WaitForSessionTerminalStatus(t, group.baseURL, sessionID, ceiling)
	return collectSharedInferenceFactoryResult(t, group, sessionID, scenario, nil, ceiling)
}

func assertProviderOutageRecovery(t *testing.T, result sharedInferenceFactoryResult) {
	t.Helper()
	if got := result.session.Runtime.Progress.Categories; got.Terminal != 2 || got.Failed != 0 {
		t.Fatalf("session progress = %+v, want two terminal, zero failed", got)
	}
	for _, id := range []string{"batch-outage-recovery-prerequisite", "batch-outage-recovery-dependent"} {
		if !support.HasWorkAtCustomerState(result.work, id, "task:done") {
			t.Fatalf("%s did not complete: %#v", id, result.work)
		}
	}
	dispatches := support.ObserveDispatchEvents(t, result.events)
	if len(dispatches) != 2 {
		t.Fatalf("dispatches = %#v, want only original and dependent dispatch (no escalation/cascade)", dispatches)
	}
	for _, dispatch := range dispatches {
		if dispatch.Request.TransitionId != "process" || dispatch.Response == nil || dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted {
			t.Fatalf("dispatch = %#v, want accepted process dispatch without breaker/failure", dispatch)
		}
	}
	if !support.DispatchObservationIncludesWork(dispatches[0], "batch-outage-recovery-prerequisite") || !support.DispatchObservationIncludesWork(dispatches[1], "batch-outage-recovery-dependent") || dispatches[1].StartedAt.Before(dispatches[0].CompletedAt) {
		t.Fatalf("dependent dispatched before accepted prerequisite: %#v", dispatches)
	}
}
