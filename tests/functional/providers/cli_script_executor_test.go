package providers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Fixture returns the root process that owns this scenario.
func (scenario *Scenario) Fixture() *ProcessFixture {
	if scenario == nil {
		return nil
	}
	return scenario.fixture
}

// RootDir returns the temporary root containing the shared process artifacts.
func (fixture *ProcessFixture) RootDir() string {
	if fixture == nil {
		return ""
	}
	return fixture.rootDir
}

type baseCaptureCommandRunner struct {
	mu    sync.Mutex
	calls int
}

func (runner *baseCaptureCommandRunner) Run(_ context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	runner.calls++
	runner.mu.Unlock()
	return platformprocess.CommandResult{Stdout: []byte("script-output-ok")}, nil
}

func (runner *baseCaptureCommandRunner) CallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.calls
}

type baseFailureCommandRunner struct{ message string }

func (runner baseFailureCommandRunner) Run(_ context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{Stderr: []byte(runner.message), ExitCode: 1}, nil
}

type baseTimeoutThenSuccessCommandRunner struct {
	mu    sync.Mutex
	calls int
}

func (runner *baseTimeoutThenSuccessCommandRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	runner.calls++
	call := runner.calls
	runner.mu.Unlock()
	if call == 1 {
		<-ctx.Done()
		return platformprocess.CommandResult{}, ctx.Err()
	}
	return platformprocess.CommandResult{Stdout: []byte("script-output-after-retry")}, nil
}

func (runner *baseTimeoutThenSuccessCommandRunner) CallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.calls
}

type baseCanceledCommandRunner struct{}

func (baseCanceledCommandRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, context.Canceled
}

func assertBaseSessionPlaces(t testing.TB, listed factoryapi.ListWorkResponse, wants map[string]int) {
	t.Helper()
	for placeID, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
}

func assertBaseDispatchOutput(t testing.TB, events []factoryapi.FactoryEvent, want string) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		if payload.Output == nil || *payload.Output != want {
			t.Fatalf("dispatch output = %#v, want %q", payload.Output, want)
		}
		return
	}
	t.Fatalf("Factory Event history has no dispatch response: %#v", events)
}

func assertBaseDispatchErrorContains(t testing.TB, events []factoryapi.FactoryEvent, want string) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		if payload.Error == nil || !strings.Contains(*payload.Error, want) {
			t.Fatalf("dispatch error = %#v, want substring %q", payload.Error, want)
		}
		return
	}
	t.Fatalf("Factory Event history has no dispatch response: %#v", events)
}

func TestProvidersSharedProcessAdverseRecovery(t *testing.T) {
	t.Parallel()
	fixture := FixtureFor(t)

	t.Run("invalid_template", func(t *testing.T) {
		dir := testutilCopySharedFixture(t, "script_executor_dir")
		writeBaseFixtureFile(t, dir, []string{"workstations", "run-script", "AGENTS.md"}, "---\ntype: MODEL_WORKSTATION\n---\n{{")
		testutil.WriteSeedFile(t, dir, "task", []byte("invalid-template-payload"))
		runner := &baseCaptureCommandRunner{}
		scenario, listed := RunFactory(t, dir, dir, runner, 5*time.Second)
		assertBaseSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:done": 0})
		if got := runner.CallCount(); got != 0 {
			t.Fatalf("invalid-template provider calls = %d, want zero", got)
		}
		assertBaseDispatchErrorContains(t, scenario.FactoryEvents(t), "prompt render failed")
		scenario.Stop(t)
	})

	t.Run("dependency_failure", func(t *testing.T) {
		dir := testutilCopySharedFixture(t, "script_executor_dir")
		testutil.WriteSeedFile(t, dir, "task", []byte("dependency-failure-payload"))
		scenario, listed := RunFactory(t, dir, dir, baseFailureCommandRunner{"adverse dependency failure"}, 5*time.Second)
		assertBaseSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:done": 0})
		assertBaseDispatchErrorContains(t, scenario.FactoryEvents(t), "adverse dependency failure")
		scenario.Stop(t)
	})

	t.Run("timeout", func(t *testing.T) {
		dir := testutilCopySharedFixture(t, "script_executor_dir")
		support.WriteWorkstationConfig(t, dir, "run-script", "---\ntype: MODEL_WORKSTATION\nlimits:\n  maxExecutionTime: 10ms\n---\nExecute the script.\n")
		testutil.WriteSeedFile(t, dir, "task", []byte("timeout-payload"))
		runner := &baseTimeoutThenSuccessCommandRunner{}
		scenario, listed := RunFactory(t, dir, dir, runner, 10*time.Second)
		assertBaseSessionPlaces(t, listed, map[string]int{"task:done": 1, "task:init": 0, "task:failed": 0})
		if got := runner.CallCount(); got < 2 {
			t.Fatalf("timeout recovery provider calls = %d, want at least two", got)
		}
		scenario.Stop(t)
	})

	t.Run("cancellation", func(t *testing.T) {
		dir := testutilCopySharedFixture(t, "script_executor_dir")
		testutil.WriteSeedFile(t, dir, "task", []byte("cancellation-payload"))
		scenario, listed := RunFactory(t, dir, dir, baseCanceledCommandRunner{}, 5*time.Second)
		assertBaseSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:done": 0})
		assertBaseDispatchErrorContains(t, scenario.FactoryEvents(t), "execution cancelled: context canceled")
		scenario.Stop(t)
	})

	t.Run("unknown_route", func(t *testing.T) {
		dir := testutilCopySharedFixture(t, "script_executor_dir")
		testutil.WriteSeedFile(t, dir, "task", []byte("unknown-route-payload"))
		scenario := fixture.OpenScenario(t, dir, "", nil)
		scenario.WaitForTerminal(t, 5*time.Second)
		listed := scenario.ListWork(t)
		assertBaseSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:done": 0})
		assertBaseDispatchErrorContains(t, scenario.FactoryEvents(t), "script command execution failed")
		scenario.Stop(t)
	})

	t.Run("known_good_after_adverse_cases", func(t *testing.T) {
		dir := testutilCopySharedFixture(t, "script_executor_dir")
		testutil.WriteSeedFile(t, dir, "task", []byte("known-good-payload"))
		scenario, listed := RunFactory(t, dir, dir, support.NewStaticSuccessCommandRunner("known-good-output"), 5*time.Second)
		assertBaseSessionPlaces(t, listed, map[string]int{"task:done": 1, "task:init": 0, "task:failed": 0})
		assertBaseDispatchOutput(t, scenario.FactoryEvents(t), "known-good-output")
		scenario.Stop(t)
	})

}

func testutilCopySharedFixture(t *testing.T, name string) string {
	t.Helper()
	source := support.LegacyFixtureDir(t, name)
	return testutil.CopyFixtureDir(t, source)
}

func writeBaseFixtureFile(t *testing.T, dir string, pathParts []string, content string) {
	t.Helper()
	path := filepath.Join(append([]string{dir}, pathParts...)...)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestScriptExecutor_Success(t *testing.T) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("input-payload"))

	server, listed := runScriptFactory(t, dir, support.NewStaticSuccessCommandRunner("script-output-ok"), 5*time.Second)
	assertSessionPlaces(t, listed, map[string]int{"task:done": 1, "task:init": 0, "task:failed": 0})
	assertDispatchOutput(t, server.factoryEvents(t), "script-output-ok")
	server.stop(t)
}

func TestScriptExecutor_Failure(t *testing.T) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("input-payload"))

	server, listed := runScriptFactory(t, dir, failureRunner("script broke"), 5*time.Second)
	assertSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:done": 0})
	server.stop(t)
}

// TestScriptExecutor_CommandCancellationIsReported proves provider command
// cancellation reaches the customer-visible execution result.
func TestScriptExecutor_CommandCancellationIsReported(t *testing.T) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("input-payload"))

	server, listed := runScriptFactory(t, dir, canceledCommandRunner{}, 5*time.Second)
	assertSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:done": 0})
	assertDispatchErrorContains(t, server.factoryEvents(t), "execution cancelled: context canceled")
	server.stop(t)
}

// TestScriptExecutor_MissingCommandFailsStartup is isolated because it proves
// malformed worker configuration is rejected during invocation startup, before
// a healthy shared host could have activated runtime state.
func TestScriptExecutor_MissingCommandFailsStartup(t *testing.T) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("input-payload"))
	agentsPath := filepath.Join(dir, "workers", "script-worker", "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte("---\ntype: SCRIPT_WORKER\n---\n"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", dir, "--quiet", "--no-record",
	})
	home := t.TempDir()
	inputs.Input.Env = []string{"HOME=" + home, "USERPROFILE=" + home}
	inputs.Input.WorkingDirectory = dir
	process := support.BuildProcess(t, serviceedges.Edges{
		ScriptCommandRunner: support.NewStaticSuccessCommandRunner("unused"),
	})
	support.CleanupProcess(t, process)
	err := process.Execute(inputs.Input)
	if err == nil ||
		!strings.Contains(err.Error(), "construct script worker") ||
		!strings.Contains(err.Error(), "misconfigured") {
		t.Fatalf("Process.Execute() error = %v, want misconfigured script worker", err)
	}
}

// TestScriptExecutor_InvalidWorkstationTemplateFailsBeforeCommand proves an
// invalid workstation template cannot invoke the provider command.
func TestScriptExecutor_InvalidWorkstationTemplateFailsBeforeCommand(t *testing.T) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	writeFixtureFile(
		t,
		dir,
		[]string{"workstations", "run-script", "AGENTS.md"},
		"---\ntype: MODEL_WORKSTATION\n---\n{{",
	)
	testutil.WriteSeedFile(t, dir, "task", []byte("input-payload"))

	runner := &captureCommandRunner{}
	server, listed := runScriptFactory(t, dir, runner, 5*time.Second)
	assertSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:done": 0})
	if calls := runner.CallCount(); calls != 0 {
		t.Fatalf("script command calls = %d, want none after template rejection", calls)
	}
	assertDispatchErrorContains(t, server.factoryEvents(t), "prompt render failed")
	server.stop(t)
}

func TestScriptExecutor_PreservesTokenColor(t *testing.T) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("original-payload"))

	server, listed := runScriptFactory(t, dir, support.NewStaticSuccessCommandRunner("new-payload"), 5*time.Second)
	assertSessionPlaces(t, listed, map[string]int{"task:done": 1, "task:init": 0})
	assertDispatchOutput(t, server.factoryEvents(t), "new-payload")
	assertListedWorkIdentity(t, listed, "done", "", "task", "", nil)
	server.stop(t)
}

func TestScriptExecutor_SuccessWithColorMetadata(t *testing.T) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     "work-seed-001",
		WorkTypeID: "task",
		TraceID:    "trace-seed-001",
		Payload:    []byte("seed-payload"),
		Tags:       map[string]string{"env": "test", "team": "platform"},
	})

	server, listed := runScriptFactory(t, dir, support.NewStaticSuccessCommandRunner("success-output"), 5*time.Second)
	assertSessionPlaces(t, listed, map[string]int{"task:done": 1, "task:init": 0, "task:failed": 0})
	assertDispatchOutput(t, server.factoryEvents(t), "success-output")
	assertListedWorkIdentity(t, listed, "done", "work-seed-001", "task", "trace-seed-001", map[string]string{
		"env": "test", "team": "platform",
	})
	server.stop(t)
}

func TestScriptExecutor_FailureRoutesToFailedPlace(t *testing.T) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("input-payload"))

	server, listed := runScriptFactory(t, dir, failureRunner("script-error-output"), 5*time.Second)
	assertSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:done": 0})
	assertDispatchErrorContains(t, server.factoryEvents(t), "script-error-output")
	server.stop(t)
}

func TestScriptExecutor_ArgTemplating(t *testing.T) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))

	agentsMD := "---\ntype: SCRIPT_WORKER\ncommand: echo\nargs:\n  - \"{{ (index .Inputs 0).Name }}\"\n  - \"{{ (index .Inputs 0).WorkID }}\"\n---\n"
	agentsPath := filepath.Join(dir, "workers", "script-worker", "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "prd-my-feature",
		WorkID:     "work-abc-123",
		WorkTypeID: "task",
		TraceID:    "trace-tmpl-test",
		Payload:    []byte("template-test-payload"),
	})

	server, listed := runScriptFactory(t, dir, &echoArgsRunner{}, 5*time.Second)
	assertSessionPlaces(t, listed, map[string]int{"task:done": 1, "task:init": 0, "task:failed": 0})
	assertDispatchOutput(t, server.factoryEvents(t), "prd-my-feature\nwork-abc-123")
	server.stop(t)
}

func TestScriptExecutor_WorkTypeIDFromTargetPlace(t *testing.T) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "type-stamp-test",
		WorkID:     "work-type-stamp",
		WorkTypeID: "task",
		TraceID:    "trace-type-stamp",
		Payload:    []byte("payload"),
	})

	server, listed := runScriptFactory(t, dir, support.NewStaticSuccessCommandRunner("output"), 5*time.Second)
	assertSessionPlaces(t, listed, map[string]int{"task:done": 1})
	assertListedWorkIdentity(t, listed, "done", "work-type-stamp", "task", "trace-type-stamp", nil)
	server.stop(t)
}

func TestScriptExecutor_ArgTemplatingWithTags(t *testing.T) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))

	agentsMD := "---\ntype: SCRIPT_WORKER\ncommand: echo\nargs:\n  - '{{ index (index .Inputs 0).Tags \"env\" }}'\n  - '{{ index (index .Inputs 0).Tags \"team\" }}'\n---\n"
	agentsPath := filepath.Join(dir, "workers", "script-worker", "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     "work-tag-test",
		WorkTypeID: "task",
		TraceID:    "trace-tag-test",
		Payload:    []byte("tag-test"),
		Tags:       map[string]string{"env": "staging", "team": "infra"},
	})

	server, listed := runScriptFactory(t, dir, &echoArgsRunner{}, 5*time.Second)
	assertSessionPlaces(t, listed, map[string]int{"task:done": 1})
	assertDispatchOutput(t, server.factoryEvents(t), "staging\ninfra")
	server.stop(t)
}

func TestScriptExecutor_RuntimeWorkstationConfigResolvesWorkingDirectoryAndEnv(t *testing.T) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))

	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
		workstations := cfg["workstations"].([]any)
		workstation := workstations[0].(map[string]any)
		workstation["workingDirectory"] = `/tmp/{{ index (index .Inputs 0).Tags "branch" }}`
		workstation["env"] = map[string]any{
			"TEAM":   `{{ index (index .Inputs 0).Tags "team" }}`,
			"BRANCH": `{{ index (index .Inputs 0).Tags "branch" }}`,
		}
	})

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     "script-runtime-fields",
		WorkTypeID: "task",
		TraceID:    "trace-script-runtime-fields",
		Payload:    []byte("input-payload"),
		Tags: map[string]string{
			"branch": "feature-script",
			"team":   "platform",
		},
	})

	runner := &captureCommandRunner{}
	server, listed := runScriptFactoryAt(t, dir, support.ResolvedRuntimePath(dir, "/tmp/feature-script"), runner, 5*time.Second)
	assertSessionPlaces(t, listed, map[string]int{"task:done": 1, "task:failed": 0})

	if got := runner.LastWorkDir(); got != support.ResolvedRuntimePath(dir, "/tmp/feature-script") {
		t.Fatalf("expected script runner work dir %q, got %q", support.ResolvedRuntimePath(dir, "/tmp/feature-script"), got)
	}

	env := runner.LastEnv()
	if !containsEnv(env, "TEAM=platform") {
		t.Fatalf("expected TEAM env in %v", env)
	}
	if !containsEnv(env, "BRANCH=feature-script") {
		t.Fatalf("expected BRANCH env in %v", env)
	}
	server.stop(t)
}

func TestScriptExecutor_RuntimeConfigMergePreservesCanonicalTopologyAndPromptTemplates(t *testing.T) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))

	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
		cfg["workTypes"] = []any{
			map[string]any{
				"name": "task",
				"states": []any{
					map[string]any{"name": "init", "type": "INITIAL"},
					map[string]any{"name": "canonical-done", "type": "TERMINAL"},
					map[string]any{"name": "runtime-done", "type": "TERMINAL"},
					map[string]any{"name": "failed", "type": "FAILED"},
				},
			},
		}

		workstations := cfg["workstations"].([]any)
		workstation := workstations[0].(map[string]any)
		workstation["type"] = "MODEL_WORKSTATION"
		workstation["body"] = "inline prompt {{ (index .Inputs 0).Name }}"
		workstation["workingDirectory"] = `/inline/{{ (index .Inputs 0).Name }}`
		workstation["env"] = map[string]any{
			"INLINE_ONLY":    "true",
			"RUNTIME_BRANCH": "inline-branch",
		}
		workstation["outputs"] = []any{
			map[string]any{"workType": "task", "state": "canonical-done"},
		}
	})
	writeRuntimeMergeWorkstationConfig(t, dir)
	writeScriptWorkerArgs(t, dir, []string{
		`name={{ (index .Inputs 0).Name }}`,
		`work={{ (index .Inputs 0).WorkID }}`,
		`payload={{ (index .Inputs 0).Payload }}`,
		`workdir={{ .Context.WorkDir }}`,
		`env_branch={{ index .Context.Env "RUNTIME_BRANCH" }}`,
	})

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "runtime-template-name",
		WorkID:     "work-runtime-config",
		WorkTypeID: "task",
		TraceID:    "trace-runtime-config",
		Payload:    []byte("runtime-payload"),
		Tags: map[string]string{
			"branch": "feature-runtime-config",
		},
	})

	runner := &templateCaptureCommandRunner{}
	server, listed := runScriptFactoryAt(t, dir, support.ResolvedRuntimePath(dir, "/runtime/runtime-template-name/feature-runtime-config"), runner, 10*time.Second)
	assertSessionPlaces(t, listed, map[string]int{
		"task:runtime-done":   1,
		"task:init":           0,
		"task:canonical-done": 0,
		"task:failed":         0,
	})

	req := runner.LastRequest()
	wantArgs := []string{
		"name=runtime-template-name",
		"work=work-runtime-config",
		"payload=runtime-payload",
		"workdir=" + support.ResolvedRuntimePath(dir, "/runtime/runtime-template-name/feature-runtime-config"),
		"env_branch=feature-runtime-config",
	}
	assertCommandArgs(t, req, wantArgs)
	assertRuntimeMergeCommandRequest(t, dir, req)
	assertDispatchOutput(t, server.factoryEvents(t), strings.Join(wantArgs, "\n"))
	server.stop(t)
}

func TestScriptExecutor_RuntimeWorkstationTimeoutRequeuesAndRetriesOnLaterTick(t *testing.T) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))

	workstationAgentsPath := filepath.Join(dir, "workstations", "run-script", "AGENTS.md")
	agentsMD := "---\ntype: MODEL_WORKSTATION\nlimits:\n  maxExecutionTime: 10ms\n---\nExecute the script.\n"
	if err := os.WriteFile(workstationAgentsPath, []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}

	testutil.WriteSeedFile(t, dir, "task", []byte("input-payload"))

	runner := newTimeoutThenSuccessCommandRunner()
	server, listed := runScriptFactory(t, dir, runner, 5*time.Second)
	assertSessionPlaces(t, listed, map[string]int{"task:done": 1, "task:init": 0, "task:failed": 0})

	if runner.CallCount() < 2 {
		t.Fatalf("expected script runner to be called at least twice, got %d", runner.CallCount())
	}

	assertDispatchTimeoutEventuallyAccepted(t, server.factoryEvents(t))
	server.stop(t)
}

func TestScriptExecutor_AsyncWorkerPoolTemplateFallbackScenarios(t *testing.T) {
	t.Parallel()
	support.SkipLongFunctional(t, "slow async worker-pool template fallback sweep")

	t.Run("SingleFileInputWithTemplateAndPayload_Completes", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
		writeWorkstationPromptTemplate(t, dir, "payload: {{ (index .Inputs 0).Payload }}")
		testutil.WriteSeedFile(t, dir, "task", []byte("template-input"))

		server, listed := runScriptFactory(t, dir, support.NewStaticSuccessCommandRunner("template-case-ok"), 10*time.Second)
		assertSessionPlaces(t, listed, map[string]int{"task:done": 1, "task:init": 0, "task:failed": 0})
		assertDispatchOutput(t, server.factoryEvents(t), "template-case-ok")
		server.stop(t)
	})

	t.Run("NoTemplateWithPayload_Completes", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
		testutil.WriteSeedFile(t, dir, "task", []byte("payload-only"))

		server, listed := runScriptFactory(t, dir, support.NewStaticSuccessCommandRunner("payload-only-ok"), 10*time.Second)
		assertSessionPlaces(t, listed, map[string]int{"task:done": 1, "task:init": 0, "task:failed": 0})
		assertDispatchOutput(t, server.factoryEvents(t), "payload-only-ok")
		server.stop(t)
	})

	t.Run("NoTemplateAndNoPayload_Completes", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_no_template"))
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkID:     "work-no-template-no-payload",
			WorkTypeID: "task",
			TraceID:    "trace-no-template-no-payload",
			Payload:    nil,
		})
		server, listed := runScriptFactory(t, dir, support.NewStaticSuccessCommandRunner("empty-input-ok"), 10*time.Second)
		assertSessionPlaces(t, listed, map[string]int{"task:done": 1, "task:init": 0, "task:failed": 0})
		assertDispatchOutput(t, server.factoryEvents(t), "empty-input-ok")
		server.stop(t)
	})
}

func runScriptFactory(
	t *testing.T,
	dir string,
	runner platformprocess.CommandRunner,
	timeout time.Duration,
) (*sharedProviderScenario, factoryapi.ListWorkResponse) {
	t.Helper()
	return runScriptFactoryAt(t, dir, dir, runner, timeout)
}

func runScriptFactoryAt(
	t *testing.T,
	dir, workDir string,
	runner platformprocess.CommandRunner,
	timeout time.Duration,
) (*sharedProviderScenario, factoryapi.ListWorkResponse) {
	t.Helper()
	scenario, listed := RunFactory(t, dir, workDir, runner, timeout)
	return &sharedProviderScenario{Scenario: scenario}, listed
}

func assertListedWorkIdentity(
	t *testing.T,
	response factoryapi.ListWorkResponse,
	stateName, workID, workType, traceID string,
	tags map[string]string,
) {
	t.Helper()
	for _, item := range response.Results {
		if item.State == nil || item.State.Name != stateName {
			continue
		}
		if workID != "" && (item.WorkId == nil || *item.WorkId != workID) {
			t.Errorf("listed Work ID = %#v, want %q", item.WorkId, workID)
		}
		if item.WorkTypeName == nil || *item.WorkTypeName != workType {
			t.Errorf("listed Work type = %#v, want %q", item.WorkTypeName, workType)
		}
		if traceID != "" && (item.TraceId == nil || *item.TraceId != traceID) {
			t.Errorf("listed Work trace ID = %#v, want %q", item.TraceId, traceID)
		}
		for key, want := range tags {
			if item.Tags == nil || (*item.Tags)[key] != want {
				t.Errorf("listed Work tag %q = %#v, want %q", key, item.Tags, want)
			}
		}
		return
	}
	t.Fatalf("listed Work has no item in state %q: %#v", stateName, response.Results)
}

func assertDispatchErrorContains(t *testing.T, events []factoryapi.FactoryEvent, want string) {
	t.Helper()
	for _, payload := range dispatchResponses(t, events) {
		if payload.Error != nil && strings.Contains(*payload.Error, want) {
			return
		}
	}
	t.Fatalf("dispatch responses do not contain error %q", want)
}

func assertDispatchOutcomeSequence(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	wants []factoryapi.WorkOutcome,
	firstError string,
) {
	t.Helper()
	responses := dispatchResponses(t, events)
	if len(responses) < len(wants) {
		t.Fatalf("dispatch response count = %d, want at least %d", len(responses), len(wants))
	}
	for index, want := range wants {
		if responses[index].Outcome != want {
			t.Errorf("dispatch response %d outcome = %s, want %s", index, responses[index].Outcome, want)
		}
	}
	if firstError != "" && (responses[0].Error == nil || !strings.Contains(*responses[0].Error, firstError)) {
		t.Errorf("first dispatch error = %#v, want text %q", responses[0].Error, firstError)
	}
}

func assertDispatchTimeoutEventuallyAccepted(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	responses := dispatchResponses(t, events)
	if len(responses) < 2 {
		t.Fatalf("dispatch response count = %d, want a timeout and a later accepted retry", len(responses))
	}
	first := responses[0]
	if first.Outcome != factoryapi.WorkOutcomeFailed {
		t.Errorf("first dispatch outcome = %s, want %s", first.Outcome, factoryapi.WorkOutcomeFailed)
	}
	if first.Error == nil || !strings.Contains(*first.Error, "execution timeout") {
		t.Errorf("first dispatch error = %#v, want execution timeout", first.Error)
	}
	// The 10ms production deadline is deliberately real: the command-runner
	// edge cannot replace workstation timeout policy. Under scheduler contention,
	// more than one retry may legitimately time out before the eventual success.
	if last := responses[len(responses)-1]; last.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Errorf("last dispatch outcome = %s after %d attempts, want %s", last.Outcome, len(responses), factoryapi.WorkOutcomeAccepted)
	}
}

func dispatchResponses(t *testing.T, events []factoryapi.FactoryEvent) []factoryapi.DispatchResponseEventPayload {
	t.Helper()
	var responses []factoryapi.DispatchResponseEventPayload
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		responses = append(responses, payload)
	}
	return responses
}
