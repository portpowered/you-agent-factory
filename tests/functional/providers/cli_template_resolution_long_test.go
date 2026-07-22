//go:build functionallong

package providers

import (
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
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

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "script-resource-name",
		WorkID:     "work-script-template-resource",
		WorkTypeID: "task",
		TraceID:    "trace-script-template-resource",
		Payload:    []byte("script-resource-payload"),
	})

	runner := &templateCaptureCommandRunner{}
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Edges: serviceedges.Edges{
			ScriptCommandRunner: runner,
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	session := support.GetDefaultSession(t, server.URL())
	assertSessionPlaces(t, session, map[string]int{
		"task:done": 1, "task:init": 0, "task:failed": 0,
	})

	wantArgs := []string{
		"name=script-resource-name",
		"work=work-script-template-resource",
		"payload=script-resource-payload",
		"inputs=1",
		"type=work",
	}
	assertCommandArgs(t, runner.LastRequest(), wantArgs)
	assertDispatchOutput(t, server.GetFactoryEvents(t), strings.Join(wantArgs, "\n"))
	server.Stop(t)
}

func TestTemplateTests_ScriptWrapDropsResourceTokensFromWorkstationTemplates(t *testing.T) {
	support.SkipLongFunctional(t, "slow provider prompt-template sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "simple_pipeline"))
	configureResourceGatedTemplateWorkstation(t, dir)
	writeNamedWorkstationPromptTemplate(t, dir, "process", strings.Join([]string{
		`name={{ (index .Inputs 0).Name }}`,
		`work={{ (index .Inputs 0).WorkID }}`,
		`payload={{ (index .Inputs 0).Payload }}`,
		`inputs={{ len .Inputs }}`,
		`type={{ (index .Inputs 0).DataType }}`,
	}, "\n"))

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "script-wrap-resource-name",
		WorkID:     "work-script-wrap-template-resource",
		WorkTypeID: "task",
		TraceID:    "trace-script-wrap-template-resource",
		Payload:    []byte("script-wrap-resource-payload"),
	})

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: []byte("Done. COMPLETE")})
	session := support.RunFactoryToCompletionWithEdges(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 10*time.Second)
	assertCursorProviderCompleted(t, session)

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
	session := support.RunFactoryToCompletionWithEdges(t, dir, serviceedges.Edges{
		ScriptCommandRunner: runner,
	}, 10*time.Second)
	assertSessionPlaces(t, session, map[string]int{
		"zeta-resource:done":  1,
		"alpha-resource:done": 1,
		"zeta-resource:init":  0,
		"alpha-resource:init": 0,
	})

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

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: []byte("Done. COMPLETE")})
	session := support.RunFactoryToCompletionWithEdges(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 10*time.Second)
	assertSessionPlaces(t, session, map[string]int{
		"zeta-resource:done":  1,
		"alpha-resource:done": 1,
		"zeta-resource:init":  0,
		"alpha-resource:init": 0,
	})

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
	support.SkipLongFunctional(t, "slow workstation execution-template provider smoke")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "simple_pipeline"))
	configureExecutionTemplateWorkstation(t, dir)
	writeNamedWorkerAgents(t, dir, "processor", buildModelWorkerConfig(modelprovider.ProviderClaude, "test-claude-model"))

	writeExecutionTemplateSeed(t, dir)

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: []byte("Done. COMPLETE")})
	session := support.RunFactoryToCompletionWithEdges(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 10*time.Second)
	assertCursorProviderCompleted(t, session)

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
	support.SkipLongFunctional(t, "slow codex execution-template provider smoke")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "simple_pipeline"))
	configureExecutionTemplateWorkstation(t, dir)
	writeNamedWorkerAgents(t, dir, "processor", buildModelWorkerConfig(modelprovider.ProviderCodex, "test-codex-model"))

	writeExecutionTemplateSeed(t, dir)

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: []byte("Done. COMPLETE")})
	session := support.RunFactoryToCompletionWithEdges(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 10*time.Second)
	assertCursorProviderCompleted(t, session)

	req := runner.LastRequest()
	assertCommandArgs(t, req, []string{"exec", "--model", "test-codex-model", "-"})
	assertProviderStdin(t, req, executionTemplateWantPrompt(dir))
	assertProviderExecutionFields(t, dir, req)
}

func TestTemplateTests_ScriptWrapCursorResolvesWorkstationExecutionTemplates(t *testing.T) {
	support.SkipLongFunctional(t, "slow cursor execution-template provider smoke")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "simple_pipeline"))
	configureCursorExecutionTemplateWorkstation(t, dir)
	writeNamedWorkerAgents(t, dir, "processor", buildModelWorkerConfig(modelprovider.ProviderCursor, "test-cursor-model"))

	writeExecutionTemplateSeed(t, dir)

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: support.CursorProviderSuccessStdout("Done. COMPLETE")})
	session := support.RunFactoryToCompletionWithEdges(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 10*time.Second)
	assertCursorProviderCompleted(t, session)

	req := runner.LastRequest()
	if req.Command != string(modelprovider.ProviderCursor) {
		t.Fatalf("command = %q, want %q", req.Command, modelprovider.ProviderCursor)
	}
	wantWorkDir := support.ResolvedRuntimePath(dir, "/workspace/execution-template-name/feature-token-branch")
	assertCommandArgs(t, req, []string{
		"-p",
		"--model", "test-cursor-model",
		"--workspace", wantWorkDir,
		cursorExecutionTemplateWantPrompt(dir),
	})
	assertProviderStdin(t, req, "")
	assertProviderExecutionFields(t, dir, req)
}
