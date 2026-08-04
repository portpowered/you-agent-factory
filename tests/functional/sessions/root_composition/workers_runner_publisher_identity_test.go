package root_composition_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	runnerIdentityGoldenCase   = "success"
	runnerIdentityScriptOutput = "runner-identity-script-tagged-output"

	runnerIdentityScriptWorkerAgentConfig = "---\n" +
		"type: SCRIPT_WORKER\n" +
		"command: echo\n" +
		"args:\n" +
		"    - default-output\n" +
		"---\n"

	runnerIdentityFailureModel          = "runner-identity-failure-model"
	runnerIdentityProviderFailureStderr = "runner-identity-provider-failure"
	runnerIdentityScriptFailureStderr   = "runner-identity-script-failure"
)

// TestRootBuildProcessRoutesProviderAndScriptWorkThroughInjectedRunnerInstances
// proves a single Factory Runtime build constructed only through
// root.BuildProcess (support.RunFactoryToCompletionWithEdgesAndResponseEvents
// composes the process through exactly one such call) uses the exact injected
// provider command runner, script command runner, and session progress
// publisher for the full lifetime of the built runtime, with no
// post-construction mutation, derived Workers view, or second Workers-root
// construction.
//
// A provider-backed dispatch and a script-backed dispatch both run against
// the SAME root-built process. Provider identity is proven directly: the
// injected providerRunner is a distinguishable Go instance, and its CallCount
// only increments when Workers execution actually invokes that exact pointer
// -- a derived/second Workers view built with its own default runner would
// never touch it, and no real "codex" binary exists in the test sandbox for
// a default runner to fall back to, so a passing provider-task completion
// with CallCount() == 1 is only possible through this exact instance.
// Script identity is proven by exact output-text equality: "echo" is a real
// executable that WOULD succeed if a derived/second Workers view fell back to
// an un-injected default script runner, so CallCount alone would not rule
// that out, but the dispatch's public primary result is asserted to be the
// literal tagged output only the injected scriptRunner instance returns
// (which is different from the AGENTS.md-configured real echo argument).
// Progress publisher identity is proven behaviorally: the provider fixture
// stdout streams a multi-record partial turn, and the resulting PROGRESS
// response events are asserted to reach the SAME session's public
// response-event stream that support.RunFactoryToCompletionWithEdgesAndResponseEvents
// subscribes to before dispatch, which is only possible if the progress
// publisher supplied to this exact Workers construction was the one that
// received progress fragments during execution.
func TestRootBuildProcessRoutesProviderAndScriptWorkThroughInjectedRunnerInstances(t *testing.T) {
	t.Parallel()

	loaded := loadRunnerIdentityCodexGoldenCase(t)
	dir := support.ScaffoldFactory(t, runnerIdentityFactoryConfig())
	support.WriteAgentConfig(
		t,
		dir,
		"provider-worker",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, loaded.Process.Model),
	)
	support.WriteAgentConfig(t, dir, "script-worker", runnerIdentityScriptWorkerAgentConfig)
	testutil.WriteSeedFile(t, dir, "provider-task", []byte("runner-identity-provider-seed"))
	testutil.WriteSeedFile(t, dir, "script-task", []byte("runner-identity-script-seed"))

	exitCode := 0
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	providerRunner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
		Stderr:   []byte(loaded.Stderr),
		ExitCode: exitCode,
	})
	scriptRunner := support.NewRecordingCommandRunner(runnerIdentityScriptOutput)

	_, listed, factoryEvents, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t,
		dir,
		serviceedges.Edges{
			ProviderCommandRunner: providerRunner,
			ScriptCommandRunner:   scriptRunner,
		},
		20*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "provider-task:complete"); got != 1 {
		t.Fatalf("provider-task completed work tokens = %d, want 1", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "provider-task:failed"); got != 0 {
		t.Fatalf("provider-task failed work tokens = %d, want 0", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "script-task:complete"); got != 1 {
		t.Fatalf("script-task completed work tokens = %d, want 1", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "script-task:failed"); got != 0 {
		t.Fatalf("script-task failed work tokens = %d, want 0", got)
	}

	if got := providerRunner.CallCount(); got != 1 {
		t.Fatalf(
			"provider command runner calls on the injected instance = %d, want exactly 1 "+
				"(a derived or second Workers root would leave this exact instance uncalled)",
			got,
		)
	}
	if got := scriptRunner.CallCount(); got != 1 {
		t.Fatalf(
			"script command runner calls on the injected instance = %d, want exactly 1 "+
				"(a derived or second Workers root would leave this exact instance uncalled)",
			got,
		)
	}

	assertRunnerIdentityScriptDispatchOutput(t, factoryEvents, runnerIdentityScriptOutput)
	assertRunnerIdentityResponseEventsIncludeProgress(t, responseEvents)
}

// TestRootBuildProcessRunnerFailureRoutesToFailedDispatchThroughInjectedInstance
// proves the same single-Workers-root construction path used by
// TestRootBuildProcessRoutesProviderAndScriptWorkThroughInjectedRunnerInstances
// preserves existing failed-dispatch semantics after the cutover: a
// representative provider and script command runner failure each remain a
// failed dispatch with a non-empty public error, not a successful
// construction or execution, and each failure is observed on the exact
// injected runner instance (CallCount() == 1), ruling out a derived or
// second Workers view handling the failure path instead.
func TestRootBuildProcessRunnerFailureRoutesToFailedDispatchThroughInjectedInstance(t *testing.T) {
	t.Parallel()

	dir := support.ScaffoldFactory(t, runnerIdentityFactoryConfig())
	support.WriteAgentConfig(
		t,
		dir,
		"provider-worker",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, runnerIdentityFailureModel),
	)
	support.WriteAgentConfig(t, dir, "script-worker", runnerIdentityScriptWorkerAgentConfig)
	testutil.WriteSeedFile(t, dir, "provider-task", []byte("runner-identity-provider-failure-seed"))
	testutil.WriteSeedFile(t, dir, "script-task", []byte("runner-identity-script-failure-seed"))

	providerRunner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stderr:   []byte(runnerIdentityProviderFailureStderr),
		ExitCode: 1,
	})
	scriptRunner := &runnerIdentityFailingScriptCommandRunner{
		stderr:   runnerIdentityScriptFailureStderr,
		exitCode: 1,
	}

	_, listed, factoryEvents, _ := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t,
		dir,
		serviceedges.Edges{
			ProviderCommandRunner: providerRunner,
			ScriptCommandRunner:   scriptRunner,
		},
		20*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "provider-task:failed"); got != 1 {
		t.Fatalf("provider-task failed work tokens = %d, want 1", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "provider-task:complete"); got != 0 {
		t.Fatalf("provider-task completed work tokens = %d, want 0", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "script-task:failed"); got != 1 {
		t.Fatalf("script-task failed work tokens = %d, want 1", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "script-task:complete"); got != 0 {
		t.Fatalf("script-task completed work tokens = %d, want 0", got)
	}

	if got := providerRunner.CallCount(); got != 1 {
		t.Fatalf(
			"provider command runner calls on the injected instance = %d, want exactly 1 "+
				"(a derived or second Workers root would leave this exact instance uncalled on the failure path too)",
			got,
		)
	}
	if got := scriptRunner.CallCount(); got != 1 {
		t.Fatalf(
			"script command runner calls on the injected instance = %d, want exactly 1 "+
				"(a derived or second Workers root would leave this exact instance uncalled on the failure path too)",
			got,
		)
	}

	assertRunnerIdentityFailedDispatchCount(t, factoryEvents, 2)
}

// runnerIdentityFailingScriptCommandRunner is a minimal script-worker
// platformprocess.CommandRunner that always fails, mirroring the established
// nonZeroExitScriptCommandRunner pattern used for petri dispatch
// terminal-routing tests.
type runnerIdentityFailingScriptCommandRunner struct {
	stderr   string
	exitCode int

	mu    sync.Mutex
	calls int
}

func (r *runnerIdentityFailingScriptCommandRunner) Run(
	_ context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()

	return platformprocess.CommandResult{
		Stderr:   []byte(r.stderr),
		ExitCode: r.exitCode,
	}, nil
}

func (r *runnerIdentityFailingScriptCommandRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func assertRunnerIdentityFailedDispatchCount(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	want int,
) {
	t.Helper()

	got := 0
	for _, observation := range support.ObserveDispatchEvents(t, events) {
		if observation.Response == nil {
			continue
		}
		if observation.Response.Outcome != factoryapi.WorkOutcomeFailed {
			continue
		}
		if observation.Response.Error == nil || *observation.Response.Error == "" {
			continue
		}
		got++
	}
	if got != want {
		t.Fatalf("failed dispatch responses with a public error = %d, want %d", got, want)
	}
}

func runnerIdentityFactoryConfig() map[string]any {
	return map[string]any{
		"name": "runner-publisher-identity",
		"workTypes": []map[string]any{
			{
				"name": "provider-task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
			{
				"name": "script-task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "provider-worker"},
			{"name": "script-worker"},
		},
		"workstations": []map[string]any{
			{
				"name":      "provider-station",
				"worker":    "provider-worker",
				"inputs":    []map[string]string{{"workType": "provider-task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "provider-task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "provider-task", "state": "failed"}},
			},
			{
				"name":      "script-station",
				"worker":    "script-worker",
				"inputs":    []map[string]string{{"workType": "script-task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "script-task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "script-task", "state": "failed"}},
			},
		},
	}
}

func loadRunnerIdentityCodexGoldenCase(t *testing.T) support.ProviderSessionCase {
	t.Helper()

	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(
		repoRoot,
		filepath.FromSlash(support.ProviderSessionFixturePath(
			string(modelprovider.ProviderCodex),
			runnerIdentityGoldenCase,
		)),
	)
	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase(%q): %v", runnerIdentityGoldenCase, err)
	}
	if loaded.Manifest.FidelityClass != support.ProviderSessionFidelityPartialStream {
		t.Fatalf(
			"manifest.fidelityClass = %q, want %q",
			loaded.Manifest.FidelityClass,
			support.ProviderSessionFidelityPartialStream,
		)
	}
	return loaded
}

func assertRunnerIdentityScriptDispatchOutput(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	want string,
) {
	t.Helper()

	for _, observation := range support.ObserveDispatchEvents(t, events) {
		if observation.Response == nil || observation.Response.Output == nil {
			continue
		}
		if *observation.Response.Output == want {
			return
		}
	}
	t.Fatalf("Factory Event history has no dispatch response with script-tagged output %q", want)
}

func assertRunnerIdentityResponseEventsIncludeProgress(
	t *testing.T,
	events []factoryapi.FactoryResponseEvent,
) {
	t.Helper()

	for _, event := range events {
		if event.Kind == factoryapi.FactoryResponseEventKindProgress {
			return
		}
	}
	t.Fatalf(
		"captured Response Events = %#v, want at least one PROGRESS event delivered through "+
			"the session progress publisher supplied to this Workers construction",
		events,
	)
}
