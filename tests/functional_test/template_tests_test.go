package functional_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
)

type templateCaptureCommandRunner struct {
	mu      sync.Mutex
	request workers.CommandRequest
}

func (r *templateCaptureCommandRunner) Run(_ context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	r.mu.Lock()
	r.request = req
	r.mu.Unlock()

	return workers.CommandResult{Stdout: []byte(strings.Join(req.Args, "\n"))}, nil
}

func (r *templateCaptureCommandRunner) LastRequest() workers.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.request
}

func TestTemplateTests_ScriptExecutorDropsResourceTokensFromArgTemplates(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "script_executor_dir"))
	configureResourceGatedTemplateWorkstation(t, dir)
	writeScriptWorkerArgs(t, dir, []string{
		`name={{ (index .Inputs 0).Name }}`,
		`work={{ (index .Inputs 0).WorkID }}`,
		`payload={{ (index .Inputs 0).Payload }}`,
		`inputs={{ len .Inputs }}`,
		`type={{ (index .Inputs 0).DataType }}`,
	})

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "script-resource-name",
		WorkID:     "work-script-template-resource",
		WorkTypeID: "task",
		TraceID:    "trace-script-template-resource",
		Payload:    []byte("script-resource-payload"),
	})

	runner := &templateCaptureCommandRunner{}
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(runner),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	wantArgs := []string{
		"name=script-resource-name",
		"work=work-script-template-resource",
		"payload=script-resource-payload",
		"inputs=1",
		"type=work",
	}
	assertCommandArgs(t, runner.LastRequest(), wantArgs)
	assertTokenPayload(t, h.Marking(), "task:done", strings.Join(wantArgs, "\n"))
}

func TestTemplateTests_ScriptWrapDropsResourceTokensFromWorkstationTemplates(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "simple_pipeline"))
	configureResourceGatedTemplateWorkstation(t, dir)
	writeNamedWorkstationPromptTemplate(t, dir, "process", strings.Join([]string{
		`name={{ (index .Inputs 0).Name }}`,
		`work={{ (index .Inputs 0).WorkID }}`,
		`payload={{ (index .Inputs 0).Payload }}`,
		`inputs={{ len .Inputs }}`,
		`type={{ (index .Inputs 0).DataType }}`,
	}, "\n"))

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "script-wrap-resource-name",
		WorkID:     "work-script-wrap-template-resource",
		WorkTypeID: "task",
		TraceID:    "trace-script-wrap-template-resource",
		Payload:    []byte("script-wrap-resource-payload"),
	})

	runner := testutil.NewProviderCommandRunner(workers.CommandResult{Stdout: []byte("Done. COMPLETE")})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithProviderCommandRunner(runner),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:complete", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	req := runner.LastRequest()
	wantPrompt := strings.Join([]string{
		"name=script-wrap-resource-name",
		"work=work-script-wrap-template-resource",
		"payload=script-wrap-resource-payload",
		"inputs=1",
		"type=work",
	}, "\n")
	assertProviderArgsPrompt(t, req, wantPrompt)
	assertProviderStdin(t, req, "")
}

func TestTemplateTests_ScriptExecutorOrdersMultipleInputsByWorkstationConfigWithResources(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "script_executor_dir"))
	configureTwoInputResourceGatedTemplateWorkstation(t, dir, "run-script", "script-worker")
	writeScriptWorkerArgs(t, dir, twoInputTemplateArgs())

	writeTwoInputResourceSeeds(t, dir)

	runner := &templateCaptureCommandRunner{}
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(runner),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("zeta-resource:done", 1).
		PlaceTokenCount("alpha-resource:done", 1).
		HasNoTokenInPlace("zeta-resource:init").
		HasNoTokenInPlace("alpha-resource:init")

	wantArgs := []string{
		"first_name=zeta-input-name",
		"first_payload=zeta-payload",
		"second_name=alpha-input-name",
		"second_payload=alpha-payload",
		"inputs=2",
	}
	assertCommandArgs(t, runner.LastRequest(), wantArgs)
}

func TestTemplateTests_ScriptWrapOrdersMultipleInputsByWorkstationConfigWithResources(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "simple_pipeline"))
	configureTwoInputResourceGatedTemplateWorkstation(t, dir, "process", "processor")
	writeNamedWorkerAgents(t, dir, "processor", "---\ntype: MODEL_WORKER\nmodelProvider: codex\nmodel: test-model\nstopToken: COMPLETE\n---\nYou are the processor.\n")
	writeNamedWorkstationPromptTemplate(t, dir, "process", strings.Join(twoInputTemplateArgs(), "\n"))

	writeTwoInputResourceSeeds(t, dir)

	runner := testutil.NewProviderCommandRunner(workers.CommandResult{Stdout: []byte("Done. COMPLETE")})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithProviderCommandRunner(runner),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("zeta-resource:done", 1).
		PlaceTokenCount("alpha-resource:done", 1).
		HasNoTokenInPlace("zeta-resource:init").
		HasNoTokenInPlace("alpha-resource:init")

	wantPrompt := strings.Join([]string{
		"first_name=zeta-input-name",
		"first_payload=zeta-payload",
		"second_name=alpha-input-name",
		"second_payload=alpha-payload",
		"inputs=2",
	}, "\n")
	assertCommandArgs(t, runner.LastRequest(), []string{"exec", "--model", "test-model", "-"})
	assertProviderStdin(t, runner.LastRequest(), wantPrompt)
}

func TestTemplateTests_ScriptWrapClaudeResolvesWorkstationExecutionTemplates(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "simple_pipeline"))
	setWorkingDirectory(t, dir)
	configureExecutionTemplateWorkstation(t, dir)
	writeNamedWorkerAgents(t, dir, "processor", buildModelWorkerConfig(workers.ModelProviderClaude, "test-claude-model"))

	writeExecutionTemplateSeed(t, dir)

	runner := testutil.NewProviderCommandRunner(workers.CommandResult{Stdout: []byte("Done. COMPLETE")})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithProviderCommandRunner(runner),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:complete", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	req := runner.LastRequest()
	assertCommandArgs(t, req, append([]string{
		"-p",
		"--worktree", "worktrees/feature-token-branch/work-execution-template",
		"--system-prompt", "Process the input task.",
		"--model", "test-claude-model",
	}, executionTemplateWantPrompt(dir)))
	assertProviderStdin(t, req, "")
	assertProviderExecutionFields(t, dir, req)
}

func TestTemplateTests_ScriptWrapCodexResolvesWorkstationExecutionTemplates(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "simple_pipeline"))
	setWorkingDirectory(t, dir)
	configureExecutionTemplateWorkstation(t, dir)
	writeNamedWorkerAgents(t, dir, "processor", buildModelWorkerConfig(workers.ModelProviderCodex, "test-codex-model"))

	writeExecutionTemplateSeed(t, dir)

	runner := testutil.NewProviderCommandRunner(workers.CommandResult{Stdout: []byte("Done. COMPLETE")})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithProviderCommandRunner(runner),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:complete", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	req := runner.LastRequest()
	assertCommandArgs(t, req, []string{"exec", "--model", "test-codex-model", "-"})
	assertProviderStdin(t, req, executionTemplateWantPrompt(dir))
	assertProviderExecutionFields(t, dir, req)
}

func configureResourceGatedTemplateWorkstation(t *testing.T, dir string) {
	t.Helper()

	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
		cfg["resources"] = []any{
			map[string]any{"name": "aaa-slot", "capacity": 1},
			map[string]any{"name": "zzz-slot", "capacity": 1},
		}

		workstations := cfg["workstations"].([]any)
		workstation := workstations[0].(map[string]any)
		workstation["resources"] = []any{
			map[string]any{"name": "aaa-slot", "capacity": 1},
			map[string]any{"name": "zzz-slot", "capacity": 1},
		}
	})
}

func configureExecutionTemplateWorkstation(t *testing.T, dir string) {
	t.Helper()

	workstationName := ""
	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
		cfg["resources"] = []any{
			map[string]any{"name": "template-slot", "capacity": 1},
		}

		workstations := cfg["workstations"].([]any)
		workstation := workstations[0].(map[string]any)
		workstationName = workstation["name"].(string)
		workstation["resources"] = []any{
			map[string]any{"name": "template-slot", "capacity": 1},
		}
	})
	writeExecutionTemplateWorkstationAgents(t, dir, workstationName)
}

func writeExecutionTemplateWorkstationAgents(t *testing.T, dir, workstationName string) {
	t.Helper()

	agentsMD := strings.Join([]string{
		"---",
		"type: MODEL_WORKSTATION",
		`workingDirectory: '/workspace/{{ (index .Inputs 0).Name }}/{{ index (index .Inputs 0).Tags "branch" }}'`,
		`worktree: 'worktrees/{{ index (index .Inputs 0).Tags "branch" }}/{{ (index .Inputs 0).WorkID }}'`,
		"env:",
		`  TEMPLATE_BRANCH: '{{ index (index .Inputs 0).Tags "branch" }}'`,
		`  TEMPLATE_NAME: '{{ (index .Inputs 0).Name }}'`,
		`  TEMPLATE_PAYLOAD: '{{ (index .Inputs 0).Payload }}'`,
		`  TEMPLATE_WORKID: '{{ (index .Inputs 0).WorkID }}'`,
		"---",
		executionTemplatePrompt(),
	}, "\n") + "\n"
	writeFixtureFile(t, dir, []string{"workstations", workstationName, "AGENTS.md"}, agentsMD)
}

func configureTwoInputResourceGatedTemplateWorkstation(t *testing.T, dir, workstationName, workerName string) {
	t.Helper()

	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
		cfg["workTypes"] = []any{
			map[string]any{
				"name": "zeta-resource",
				"states": []any{
					map[string]any{"name": "init", "type": "INITIAL"},
					map[string]any{"name": "done", "type": "TERMINAL"},
					map[string]any{"name": "failed", "type": "FAILED"},
				},
			},
			map[string]any{
				"name": "alpha-resource",
				"states": []any{
					map[string]any{"name": "init", "type": "INITIAL"},
					map[string]any{"name": "done", "type": "TERMINAL"},
					map[string]any{"name": "failed", "type": "FAILED"},
				},
			},
		}
		cfg["resources"] = []any{
			map[string]any{"name": "repo-slot", "capacity": 1},
			map[string]any{"name": "gpu-slot", "capacity": 1},
		}
		cfg["workers"] = []any{map[string]any{"name": workerName}}
		cfg["workstations"] = []any{map[string]any{
			"name":   workstationName,
			"worker": workerName,
			"inputs": []any{
				map[string]any{"workType": "zeta-resource", "state": "init"},
				map[string]any{"workType": "alpha-resource", "state": "init"},
			},
			"outputs": []any{
				map[string]any{"workType": "zeta-resource", "state": "done"},
				map[string]any{"workType": "alpha-resource", "state": "done"},
			},
			"onFailure": map[string]any{"workType": "zeta-resource", "state": "failed"},
			"resources": []any{map[string]any{"name": "repo-slot", "capacity": 1}, map[string]any{"name": "gpu-slot", "capacity": 1}},
		}}
	})
}

func twoInputTemplateArgs() []string {
	return []string{
		`first_name={{ (index .Inputs 0).Name }}`,
		`first_payload={{ (index .Inputs 0).Payload }}`,
		`second_name={{ (index .Inputs 1).Name }}`,
		`second_payload={{ (index .Inputs 1).Payload }}`,
		`inputs={{ len .Inputs }}`,
	}
}

func executionTemplatePrompt() string {
	return strings.Join([]string{
		`name={{ (index .Inputs 0).Name }}`,
		`payload={{ (index .Inputs 0).Payload }}`,
		`context_workdir={{ .Context.WorkDir }}`,
		`env_branch={{ index .Context.Env "TEMPLATE_BRANCH" }}`,
		`env_workid={{ index .Context.Env "TEMPLATE_WORKID" }}`,
		`inputs={{ len .Inputs }}`,
	}, "\n")
}

func executionTemplateWantPrompt(dir string) string {
	return strings.Join([]string{
		"name=execution-template-name",
		"payload=execution-template-payload",
		"context_workdir=" + resolvedRuntimePath(dir, "/workspace/execution-template-name/feature-token-branch"),
		"env_branch=feature-token-branch",
		"env_workid=work-execution-template",
		"inputs=1",
	}, "\n")
}

func writeTwoInputResourceSeeds(t *testing.T, dir string) {
	t.Helper()

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "zeta-input-name",
		WorkID:     "zeta-work",
		WorkTypeID: "zeta-resource",
		TraceID:    "trace-two-input-resources",
		Payload:    []byte("zeta-payload"),
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "alpha-input-name",
		WorkID:     "alpha-work",
		WorkTypeID: "alpha-resource",
		TraceID:    "trace-two-input-resources",
		Payload:    []byte("alpha-payload"),
	})
}

func writeExecutionTemplateSeed(t *testing.T, dir string) {
	t.Helper()

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "execution-template-name",
		WorkID:     "work-execution-template",
		WorkTypeID: "task",
		TraceID:    "trace-execution-template",
		Payload:    []byte("execution-template-payload"),
		Tags: map[string]string{
			"branch": "feature-token-branch",
		},
	})
}

func writeScriptWorkerArgs(t *testing.T, dir string, args []string) {
	t.Helper()

	lines := []string{"---", "type: SCRIPT_WORKER", "command: echo", "args:"}
	for _, arg := range args {
		lines = append(lines, "  - "+quoteYAMLString(arg))
	}
	lines = append(lines, "---", "Execute the script.")
	writeFixtureFile(t, dir, []string{"workers", "script-worker", "AGENTS.md"}, strings.Join(lines, "\n")+"\n")
}

func writeNamedWorkerAgents(t *testing.T, dir, workerName, content string) {
	t.Helper()

	writeFixtureFile(t, dir, []string{"workers", workerName, "AGENTS.md"}, content)
}

func writeFixtureFile(t *testing.T, dir string, pathParts []string, content string) {
	t.Helper()

	path := filepath.Join(append([]string{dir}, pathParts...)...)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func quoteYAMLString(value string) string {
	return strconv.Quote(value)
}

func assertCommandArgs(t *testing.T, req workers.CommandRequest, want []string) {
	t.Helper()

	if !reflect.DeepEqual(req.Args, want) {
		t.Fatalf("command args = %#v, want %#v", req.Args, want)
	}
}

func assertProviderArgsPrompt(t *testing.T, req workers.CommandRequest, want string) {
	t.Helper()

	if len(req.Args) == 0 {
		t.Fatal("provider args were empty")
	}
	if got := req.Args[len(req.Args)-1]; got != want {
		t.Fatalf("provider prompt arg = %q, want %q", got, want)
	}
}

func assertProviderStdin(t *testing.T, req workers.CommandRequest, want string) {
	t.Helper()

	if got := string(req.Stdin); got != want {
		t.Fatalf("provider stdin = %q, want %q", got, want)
	}
}

func assertProviderExecutionFields(t *testing.T, dir string, req workers.CommandRequest) {
	t.Helper()

	if req.WorkDir != resolvedRuntimePath(dir, "/workspace/execution-template-name/feature-token-branch") {
		t.Fatalf("provider work dir = %q, want resolved workstation working_directory", req.WorkDir)
	}
	for _, want := range []string{
		"TEMPLATE_BRANCH=feature-token-branch",
		"TEMPLATE_NAME=execution-template-name",
		"TEMPLATE_PAYLOAD=execution-template-payload",
		"TEMPLATE_WORKID=work-execution-template",
	} {
		if !containsEnv(req.Env, want) {
			t.Fatalf("provider env missing %s in %v", want, req.Env)
		}
	}
}
