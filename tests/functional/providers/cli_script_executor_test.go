package providers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestScriptExecutor_Success(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("input-payload"))

	server, listed := runScriptFactory(t, dir, support.NewStaticSuccessCommandRunner("script-output-ok"), 5*time.Second)
	assertSessionPlaces(t, listed, map[string]int{"task:done": 1, "task:init": 0, "task:failed": 0})
	assertDispatchOutput(t, server.factoryEvents(t), "script-output-ok")
	server.stop(t)
}

func TestScriptExecutor_Failure(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("input-payload"))

	server, listed := runScriptFactory(t, dir, failureRunner("script broke"), 5*time.Second)
	assertSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:done": 0})
	server.stop(t)
}

// TestScriptExecutor_CommandCancellationIsReported proves provider command
// cancellation reaches the customer-visible execution result.
func TestScriptExecutor_CommandCancellationIsReported(t *testing.T) {
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
	inputs.Input.Env = append(inputs.Input.Env, "HOME="+home, "USERPROFILE="+home)
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
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("original-payload"))

	server, listed := runScriptFactory(t, dir, support.NewStaticSuccessCommandRunner("new-payload"), 5*time.Second)
	assertSessionPlaces(t, listed, map[string]int{"task:done": 1, "task:init": 0})
	assertDispatchOutput(t, server.factoryEvents(t), "new-payload")
	assertListedWorkIdentity(t, listed, "done", "", "task", "", nil)
	server.stop(t)
}

func TestScriptExecutor_SuccessWithColorMetadata(t *testing.T) {
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
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("input-payload"))

	server, listed := runScriptFactory(t, dir, failureRunner("script-error-output"), 5*time.Second)
	assertSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:done": 0})
	assertDispatchErrorContains(t, server.factoryEvents(t), "script-error-output")
	server.stop(t)
}

func TestScriptExecutor_ArgTemplating(t *testing.T) {
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

	assertDispatchOutcomeSequence(t, server.factoryEvents(t), []factoryapi.WorkOutcome{
		factoryapi.WorkOutcomeFailed,
		factoryapi.WorkOutcomeAccepted,
	}, "execution timeout")
	server.stop(t)
}

func TestScriptExecutor_AsyncWorkerPoolTemplateFallbackScenarios(t *testing.T) {
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
	scenario := sharedProviderFixtureFor(t).openScenario(t, dir, workDir, runner)
	scenario.waitForTerminal(t, timeout)
	return scenario, scenario.listWork(t)
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
