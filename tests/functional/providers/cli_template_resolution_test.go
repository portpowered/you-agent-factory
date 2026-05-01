package providers

import (
	"strings"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/pkg/workers"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestTemplateTests_ScriptExecutorDropsResourceTokensFromArgTemplates(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
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
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "simple_pipeline"))
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
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
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
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "simple_pipeline"))
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
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "simple_pipeline"))
	support.SetWorkingDirectory(t, dir)
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
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "simple_pipeline"))
	support.SetWorkingDirectory(t, dir)
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
