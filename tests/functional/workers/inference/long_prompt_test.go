package inference_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The customer submits Work to an explicit session of the reusable root process.
// Only the provider command edge is controlled; preparation and retry policy run
// through production wiring, with a private home owned by each scenario.
func TestLongWorkerPrompt(t *testing.T) {
	t.Parallel()
	for _, oversizedOptions := range []bool{false, true} {
		t.Run(fmt.Sprintf("fixed_options_%t", oversizedOptions), func(t *testing.T) {
			t.Parallel()
			dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
			home := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, ".git"), 0o700); err != nil {
				t.Fatal(err)
			}
			body := strings.Repeat("exact developer café 😀 ", 1800) + "\nquoted \"text\" C:\\workspace\\file\tend"
			config := sharedInferenceWithExecutorProvider(support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "long-prompt-model"), "CODEX")
			config = strings.Replace(config, "Process the input task.", body, 1)
			if oversizedOptions {
				config = strings.Replace(config, "stopToken: COMPLETE", "args:\n  - "+strings.Repeat("x", 33000)+"\nstopToken: COMPLETE", 1)
			}
			support.WriteAgentConfig(t, dir, "worker", config)
			support.WriteWorkstationConfig(t, dir, "process", "---\ntype: MODEL_WORKSTATION\nenv:\n  CODEX_HOME: "+fmt.Sprintf("%q", home)+"\n---\nuser café 😀 with quoted \"text\" and trailing whitespace  \n")
			testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"long-prompt-public-work"}`))
			runner := &longPromptRunner{delegate: testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("COMPLETE")})}
			result := runLongPromptFactory(t, dir, runner)
			assertLongPromptOutcome(t, result, runner, body, oversizedOptions)
			residue, err := filepath.Glob(filepath.Join(home, "you-prompt-*.config.toml"))
			if err != nil || len(residue) != 0 {
				t.Fatalf("own instruction profiles remain: %v (%v)", residue, err)
			}
		})
	}
}

func assertLongPromptOutcome(t *testing.T, result sharedInferenceFactoryResult, runner *longPromptRunner, body string, oversizedOptions bool) {
	t.Helper()
	session, listed, events := result.session, result.work, result.events
	wantDone, wantFailed, wantCalls := 1, 0, 1
	if oversizedOptions {
		wantDone, wantFailed, wantCalls = 0, 1, 0
		failure := terminalInferenceFailureObservation(t, events)
		if failure.FailureDetail == nil || failure.FailureDetail.Reason != "command_line_too_long" {
			t.Fatalf("failure detail = %#v", failure.FailureDetail)
		}
		encoded, err := json.Marshal(failure)
		if err != nil || strings.Contains(string(encoded), body) || !strings.Contains(string(encoded), "32767") {
			t.Fatalf("missing safe measured diagnostic: %s (%v)", encoded, err)
		}
	}
	if support.CountWorkAtCustomerState(listed, "task:done") != wantDone || support.CountWorkAtCustomerState(listed, "task:failed") != wantFailed || session.Runtime.Progress.Categories.Failed != wantFailed {
		encoded, _ := json.Marshal(terminalInferenceFailureObservation(t, events))
		t.Fatalf("unexpected Work outcome: %s", encoded)
	}
	if runner.delegate.CallCount() != wantCalls {
		t.Fatalf("provider attempts = %d, want %d", runner.delegate.CallCount(), wantCalls)
	}
	if !oversizedOptions {
		assertLongPromptDelivery(t, runner, body)
	}
}

func assertLongPromptDelivery(t *testing.T, runner *longPromptRunner, body string) {
	t.Helper()
	runner.mu.Lock()
	developer, err := runner.developer, runner.err
	runner.mu.Unlock()
	call := runner.delegate.LastRequest()
	if err != nil || !strings.Contains(developer, body) || string(call.Stdin) != "user café 😀 with quoted \"text\" and trailing whitespace" || platformprocess.ComposedCommandLineLength(call.Command, call.Args) >= 32767 {
		t.Fatalf("provider edge changed rendered prompt or exceeded budget: %v", err)
	}
}

func runLongPromptFactory(t *testing.T, dir string, runner platformprocess.CommandRunner) sharedInferenceFactoryResult {
	t.Helper()
	group := sharedInferenceGroup
	group.ensure(t)
	scenario := sharedInferenceScenario{commandRunner: runner}
	release := prepareSharedInferenceScenario(t, group, dir, scenario)
	t.Cleanup(func() { group.commands.clear(dir); group.scripts.clear(dir); release() })
	ctx, cancel := context.WithTimeout(t.Context(), sharedInferenceScenarioTimeout)
	t.Cleanup(cancel)
	inputs := support.FakeInputs(ctx, []string{"you", "--remote", "--server", group.baseURL, "--json", "session", "create", "--dir", dir})
	inputs.Input.Env = sharedInferenceProcessEnvironment(group.homeDir)
	inputs.Input.WorkingDirectory = dir
	if err := group.process.Execute(inputs.Input); err != nil {
		t.Fatalf("create explicit Factory Session: %v stdout=%s stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var opened factoryapi.OpenFactorySessionResponse
	if err := json.Unmarshal([]byte(inputs.Stdout()), &opened); err != nil || opened.Session == nil || opened.Session.Id == "" {
		t.Fatalf("session creation response=%s (%v)", inputs.Stdout(), err)
	}
	sessionID := opened.Session.Id
	t.Cleanup(func() { closeSharedInferenceSession(t, group, sessionID) })
	updateSharedInferenceRouteContext(t, group, dir, scenario, sessionID)
	release()
	support.WaitForSessionTerminalStatus(t, group.baseURL, sessionID, sharedInferenceScenarioTimeout)
	return collectSharedInferenceFactoryResult(t, group, sessionID, scenario, nil, sharedInferenceScenarioTimeout)
}

type longPromptRunner struct {
	delegate  *testutil.ProviderCommandRunner
	mu        sync.Mutex
	developer string
	err       error
}

func (r *longPromptRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	home := ""
	for _, entry := range request.Env {
		name, value, _ := strings.Cut(entry, "=")
		if strings.EqualFold(name, "CODEX_HOME") {
			home = value
		}
	}
	var developer string
	var readErr error
	for i, arg := range request.Args {
		if arg == "--profile" && i+1 < len(request.Args) {
			var raw []byte
			raw, readErr = os.ReadFile(filepath.Join(home, request.Args[i+1]+".config.toml"))
			if readErr == nil {
				readErr = json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(string(raw), "developer_instructions="))), &developer)
			}
		}
	}
	if developer == "" && readErr == nil {
		readErr = fmt.Errorf("provider did not receive a completed instruction profile")
	}
	r.mu.Lock()
	r.developer, r.err = developer, readErr
	r.mu.Unlock()
	return r.delegate.Run(ctx, request)
}
