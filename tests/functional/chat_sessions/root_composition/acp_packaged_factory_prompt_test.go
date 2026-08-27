package root_composition_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// packagedFactoryPromptCase is one packaged Factory driven end to end over the
// real "you server acp" command.
type packagedFactoryPromptCase struct {
	name string
	// target is the ACP Factory target reference selected for the session.
	target string
	// factory is the packaged Factory name installed into the test home.
	factory string
	// prompt is the single free-text turn, which is all an ACP client can send.
	prompt string
	// providerStdout is the queued provider stdout, one entry per expected
	// provider call, shaped the way that Factory's own prompts instruct the
	// model to answer.
	providerStdout []string
	// wantText must appear in the streamed assistant text.
	wantText string
	// wantProviderCalls is the characterized number of provider invocations
	// needed for this packaged Factory's one ACP turn.
	wantProviderCalls int
}

// packagedFactoryCohort is a fixed-home/profile root for one packaged Factory
// prompt cell. The target profile and catalog are written before construction
// and never changed afterward. A prompt cell that activates a Factory remains
// its own cohort until the production activation lifetime can be released;
// catalog-only cells use the shared cohort in acp_server_composition_test.go.
type packagedFactoryCohort struct {
	home    string
	process support.ApplicationProcess
}

func newPackagedFactoryCohort(
	t *testing.T,
	factory, target string,
	runner process.CommandRunner,
) *packagedFactoryCohort {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	seedInstalledPackagedFactory(t, home, factory)
	support.SeedACPAgentProfile(t, home, target, []string{target})
	buildProcess, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		ProviderCommandRunner: runner,
	})
	if err != nil {
		t.Fatalf("build packaged Factory %s cohort: %v", factory, err)
	}
	cohort := &packagedFactoryCohort{home: home, process: buildProcess}
	closeProcessCleanly(t, buildProcess)
	return cohort
}

// packagedFactoryPromptRunner routes by the provider request shape instead of
// consuming a result queue. This keeps the fixture valid if the workflow
// schedules compatible calls in a different order.
type packagedFactoryPromptRunner struct {
	factory string
	outputs []string

	mu       sync.Mutex
	requests []process.CommandRequest
}

func newPackagedFactoryPromptRunner(testCase packagedFactoryPromptCase) *packagedFactoryPromptRunner {
	return &packagedFactoryPromptRunner{
		factory: testCase.factory,
		outputs: append([]string(nil), testCase.providerStdout...),
	}
}

func (runner *packagedFactoryPromptRunner) Run(
	_ context.Context,
	request process.CommandRequest,
) (process.CommandResult, error) {
	runner.mu.Lock()
	runner.requests = append(runner.requests, request)
	runner.mu.Unlock()

	prompt := strings.ToLower(string(request.Stdin))
	var output string
	switch runner.factory {
	case "@you/goal", "@you/loop":
		output = runner.outputs[0]
	case "@you/classify":
		if strings.Contains(prompt, "return exactly one plain-text label") ||
			strings.Contains(prompt, "return only one lowercase label") {
			output = runner.outputs[0]
		} else {
			output = runner.outputs[1]
		}
	default:
		return process.CommandResult{}, fmt.Errorf("no packaged prompt route for Factory %q", runner.factory)
	}
	return process.CommandResult{Stdout: support.CodexSuccessStdout(output)}, nil
}

func (runner *packagedFactoryPromptRunner) callCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.requests)
}

// TestPackagedFactoriesCompleteOneACPPromptTurn proves the packaged Factories
// problems.md names are reachable and terminal over ACP, using only what an
// ACP client can actually supply: one free-text prompt and no named arguments.
//
// @you/loop is the load-bearing case. It previously could not complete here at
// all: `every` was a required NAMED-only parameter that free text cannot fill,
// and the Factory declared no invocationReturn, so its always-active
// controller Work meant a prompt that supplies no timeout -- which is every
// ACP prompt -- waited forever rather than returning its first execution.
func TestPackagedFactoriesCompleteOneACPPromptTurn(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving root.BuildProcess through the you server acp CLI command")
	}

	cases := []packagedFactoryPromptCase{
		{
			name:    "goal",
			target:  "factory:@you/goal",
			factory: "@you/goal",
			prompt:  "please pursue this goal",
			providerStdout: []string{
				`{"decision":"accepted","feedback":"","output":"goal reached over ACP"}`,
			},
			wantText:          "goal reached over ACP",
			wantProviderCalls: 1,
		},
		{
			name:    "classify",
			target:  "factory:@you/classify",
			factory: "@you/classify",
			prompt:  "classify and answer this request",
			providerStdout: []string{
				"small",
				"classified answer over ACP",
			},
			wantText:          "classified answer over ACP",
			wantProviderCalls: 2,
		},
		{
			name:    "loop",
			target:  "factory:@you/loop",
			factory: "@you/loop",
			prompt:  "check something on an interval",
			providerStdout: []string{
				"loop execution over ACP",
				"loop execution over ACP",
			},
			wantText:          "loop execution over ACP",
			wantProviderCalls: 1,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			runner := newPackagedFactoryPromptRunner(testCase)
			cohort := newPackagedFactoryCohort(t, testCase.factory, testCase.target, runner)

			cwd := t.TempDir()
			stdin, stdout := startServeACPProcess(t, cohort.process, cohort.home, cwd)

			sessionID := driveServeACPSessionNew(t, stdin, stdout, cwd)
			if sessionID == "" {
				t.Fatal("session/new returned a blank sessionId")
			}

			promptResp, notifications := driveServeACPSessionPrompt(t, stdin, stdout, sessionID, testCase.prompt)
			if promptResp.Error != nil {
				t.Fatalf("session/prompt response error = %+v, want a successful final result", promptResp.Error)
			}
			var decoded acpsdk.PromptResponse
			if err := json.Unmarshal(promptResp.Result, &decoded); err != nil {
				t.Fatalf("unmarshal PromptResponse: %v", err)
			}
			if decoded.StopReason != acpsdk.StopReasonEndTurn {
				t.Fatalf("stopReason = %q, want %q", decoded.StopReason, acpsdk.StopReasonEndTurn)
			}

			assistantText := agentMessageText(t, notifications)
			if assistantText == "" {
				t.Fatal("no agent_message_chunk text was streamed for the turn")
			}
			if !strings.Contains(assistantText, testCase.wantText) {
				t.Fatalf("streamed assistant text = %q, want it to contain %q", assistantText, testCase.wantText)
			}
			if got := runner.callCount(); got != testCase.wantProviderCalls {
				t.Fatalf("provider call count = %d, want %d for the request-shaped fixture", got, testCase.wantProviderCalls)
			}
		})
	}
}

// agentMessageText concatenates every agent_message_chunk text delivered for
// the turn.
func agentMessageText(t *testing.T, notifications []acpsdk.SessionNotification) string {
	t.Helper()
	text := ""
	for _, notification := range notifications {
		chunk := notification.Update.AgentMessageChunk
		if chunk == nil || chunk.Content.Text == nil {
			continue
		}
		text += chunk.Content.Text.Text
	}
	return text
}

// planParallelACPRunner answers each of @you/plan-parallel's three prompt
// shapes -- planner, per-task executor, and merge -- because that Factory
// dispatches a different worker per stage rather than a fixed call sequence.
type planParallelACPRunner struct {
	mu       sync.Mutex
	requests int
}

func (runner *planParallelACPRunner) Run(
	_ context.Context,
	request process.CommandRequest,
) (process.CommandResult, error) {
	runner.mu.Lock()
	runner.requests++
	runner.mu.Unlock()

	prompt := string(request.Stdin)
	switch {
	case strings.Contains(prompt, "Plan an executable Work DAG"):
		return process.CommandResult{Stdout: support.CodexSuccessStdout(
			`{"request":{"type":"FACTORY_REQUEST_BATCH","works":[` +
				`{"name":"task-01","workTypeName":"planned-task"}]}}`,
		)}, nil
	case strings.Contains(prompt, "Treat the original request and all completed generated Work inputs"):
		return process.CommandResult{Stdout: support.CodexSuccessStdout("merged plan-parallel result over ACP")}, nil
	default:
		return process.CommandResult{Stdout: support.CodexSuccessStdout("planned task completed")}, nil
	}
}

func (runner *planParallelACPRunner) callCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.requests
}

// TestPackagedPlanParallelCompletesOneACPPromptTurn covers the multi-stage
// packaged Factory problems.md names first. It needs its own cell because
// plan-parallel dispatches planner, executor, and merge workers whose prompts
// must each be answered by shape rather than by call order.
func TestPackagedPlanParallelCompletesOneACPPromptTurn(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving root.BuildProcess through the you server acp CLI command")
	}

	runner := &planParallelACPRunner{}
	cohort := newPackagedFactoryCohort(
		t,
		"@you/plan-parallel",
		"factory:@you/plan-parallel",
		runner,
	)

	cwd := t.TempDir()
	stdin, stdout := startServeACPProcess(t, cohort.process, cohort.home, cwd)

	sessionID := driveServeACPSessionNew(t, stdin, stdout, cwd)
	if sessionID == "" {
		t.Fatal("session/new returned a blank sessionId")
	}

	promptResp, notifications := driveServeACPSessionPrompt(
		t, stdin, stdout, sessionID, "implement and verify the requested change",
	)
	if promptResp.Error != nil {
		t.Fatalf("session/prompt response error = %+v, want a successful final result", promptResp.Error)
	}
	var decoded acpsdk.PromptResponse
	if err := json.Unmarshal(promptResp.Result, &decoded); err != nil {
		t.Fatalf("unmarshal PromptResponse: %v", err)
	}
	if decoded.StopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("stopReason = %q, want %q", decoded.StopReason, acpsdk.StopReasonEndTurn)
	}
	if text := agentMessageText(t, notifications); !strings.Contains(text, "merged plan-parallel result over ACP") {
		t.Fatalf("streamed assistant text = %q, want the merged result", text)
	}
	if got := runner.callCount(); got != 3 {
		t.Fatalf("provider call count = %d, want planner, task, and merge calls", got)
	}
}

// builderGreetingACPRunner routes every turn to Factory Builder's help branch
// and records whether the build workstation ever ran.
type builderGreetingACPRunner struct {
	mu     sync.Mutex
	builds int
}

func (runner *builderGreetingACPRunner) Run(
	_ context.Context,
	request process.CommandRequest,
) (process.CommandResult, error) {
	prompt := string(request.Stdin)
	switch {
	case strings.Contains(prompt, "return exactly one lowercase label: `build` or `help`"):
		return process.CommandResult{Stdout: support.CodexSuccessStdout("help")}, nil
	case strings.Contains(prompt, "You are Factory Builder."):
		runner.mu.Lock()
		runner.builds++
		runner.mu.Unlock()
		return process.CommandResult{Stdout: support.CodexSuccessStdout("should not have built")}, nil
	default:
		return process.CommandResult{Stdout: support.CodexSuccessStdout(
			"Factory Builder creates one reusable Factory from a description of what you want it to do.",
		)}, nil
	}
}

func (runner *builderGreetingACPRunner) buildCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.builds
}

// TestFactoryBuilderGreetsOnAVagueFirstACPTurn is problems.md 4.1 proven at the
// surface where it actually bites: @you/factory-builder is the ACP default
// target, so "hi" used to go straight into authoring and installing a Factory.
func TestFactoryBuilderGreetsOnAVagueFirstACPTurn(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving root.BuildProcess through the you server acp CLI command")
	}

	runner := &builderGreetingACPRunner{}
	cohort := newPackagedFactoryCohort(
		t,
		"@you/factory-builder",
		"factory:@you/factory-builder",
		runner,
	)
	cwd := t.TempDir()
	stdin, stdout := startServeACPProcess(t, cohort.process, cohort.home, cwd)

	sessionID := driveServeACPSessionNew(t, stdin, stdout, cwd)
	promptResp, notifications := driveServeACPSessionPrompt(t, stdin, stdout, sessionID, "hi")
	if promptResp.Error != nil {
		t.Fatalf("session/prompt response error = %+v, want a successful result", promptResp.Error)
	}
	var decoded acpsdk.PromptResponse
	if err := json.Unmarshal(promptResp.Result, &decoded); err != nil {
		t.Fatalf("unmarshal PromptResponse: %v", err)
	}
	if decoded.StopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("stopReason = %q, want %q", decoded.StopReason, acpsdk.StopReasonEndTurn)
	}
	if got := runner.buildCount(); got != 0 {
		t.Fatalf("build workstation ran %d times for a greeting, want 0", got)
	}
	if text := agentMessageText(t, notifications); !strings.Contains(text, "reusable Factory") {
		t.Fatalf("streamed assistant text = %q, want Factory Builder usage guidance", text)
	}
}

// spawnACPRunner answers @you/spawn's three agent roles by prompt shape.
// Order-based queuing cannot work here: spawn runs its task agents through
// the workflow's own parallel() call, so the task prompts arrive concurrently
// and in no fixed order.
type spawnACPRunner struct {
	mu    sync.Mutex
	calls int
}

func (runner *spawnACPRunner) Run(
	_ context.Context,
	request process.CommandRequest,
) (process.CommandResult, error) {
	runner.mu.Lock()
	runner.calls++
	runner.mu.Unlock()

	prompt := string(request.Stdin)
	switch {
	case strings.Contains(prompt, "You are a task planner"):
		return process.CommandResult{Stdout: support.CodexSuccessStdout(
			`{"tasks":["first independent task","second independent task","third independent task"]}`,
		)}, nil
	case strings.Contains(prompt, "You are the final merger"):
		return process.CommandResult{Stdout: support.CodexSuccessStdout(
			`{"answer":"merged spawn result over ACP"}`,
		)}, nil
	default:
		return process.CommandResult{Stdout: support.CodexSuccessStdout(
			`{"result":"independent task finding"}`,
		)}, nil
	}
}

func (runner *spawnACPRunner) callCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.calls
}

// TestPackagedJavaScriptFactoryCompletesOneACPPromptTurn proves a
// JavaScript-orchestrator packaged Factory is invocable over ACP.
//
// @you/spawn declares no workTypes at all -- its whole workflow is the
// JavaScript program in factory.js. The ACP dispatch path activates its
// runtime through factory_sessions' on-demand target activation, which used
// to invoke every Factory through the Work-submission path alone. That path
// begins by resolving the single work type carrying handlingBehavior DEFAULT,
// so a Factory with no work types failed with "expected exactly one work type
// with handlingBehavior DEFAULT for simplified prompt runs" before a single
// Worker ran -- surfacing to the client as a bare dependency_unavailable.
//
// The CLI never had this defect: its one-shot invocation operation branches on
// factorydefinitions.IsJavaScriptOrchestratorFactory first and runs the
// workflow through durable execution. So `you run --named @you/spawn` worked
// while the identical Factory over ACP could not start, breaking the CLI/API
// invocation equivalence this repository requires.
//
// This cell asserts the whole turn, not just the absence of that error: the
// workflow's planner, its three parallel task agents, and its merger all run,
// and the merged result reaches the client as streamed assistant text.
func TestPackagedJavaScriptFactoryCompletesOneACPPromptTurn(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving root.BuildProcess through the you server acp CLI command")
	}

	// A JavaScript Factory's agent.run children carry no provider of their
	// own -- @you/spawn passes an empty executorProvider/modelProvider -- so
	// the operator default is what selects their runner. An ACP client cannot
	// pass `--provider`, which is how the CLI supplies one.
	t.Setenv(operatorsettings.EnvDefaultWorkerModelProvider, "codex")
	t.Setenv(operatorsettings.EnvDefaultWorkerModel, "gpt-5")

	runner := &spawnACPRunner{}
	cohort := newPackagedFactoryCohort(t, "@you/spawn", "factory:@you/spawn", runner)
	cwd := t.TempDir()
	stdin, stdout := startServeACPProcess(t, cohort.process, cohort.home, cwd)

	sessionID := driveServeACPSessionNew(t, stdin, stdout, cwd)
	if sessionID == "" {
		t.Fatal("session/new returned a blank sessionId")
	}

	promptResp, notifications := driveServeACPSessionPrompt(
		t, stdin, stdout, sessionID, "research three independent angles on this question",
	)
	if promptResp.Error != nil {
		t.Fatalf("session/prompt response error = %+v (provider calls = %d), want a successful final result",
			promptResp.Error, runner.callCount())
	}
	var decoded acpsdk.PromptResponse
	if err := json.Unmarshal(promptResp.Result, &decoded); err != nil {
		t.Fatalf("unmarshal PromptResponse: %v", err)
	}
	if decoded.StopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("stopReason = %q, want %q", decoded.StopReason, acpsdk.StopReasonEndTurn)
	}
	if text := agentMessageText(t, notifications); !strings.Contains(text, "merged spawn result over ACP") {
		t.Fatalf("streamed assistant text = %q, want the merged workflow result", text)
	}
	// One planner, three parallel tasks (spawn's default count), one merger.
	// Asserting the exact count proves the workflow actually executed rather
	// than short-circuiting to a result some other way.
	if got := runner.callCount(); got != 5 {
		t.Fatalf("provider call count = %d, want 5 (planner, three tasks, merger)", got)
	}
}

// deepResearchACPRunner answers @you/deep-research's lead and specialist
// prompts. Every role gets ordinary prose: this Factory's own workflow, not
// its Workers, is what assembles the structured result this cell is about.
type deepResearchACPRunner struct {
	mu    sync.Mutex
	calls int
}

func (runner *deepResearchACPRunner) Run(
	_ context.Context,
	request process.CommandRequest,
) (process.CommandResult, error) {
	runner.mu.Lock()
	runner.calls++
	runner.mu.Unlock()
	prompt := string(request.Stdin)
	if strings.Contains(prompt, "in its evidence field") {
		return process.CommandResult{Stdout: support.CodexSuccessStdout(
			`{"evidence":"deep research finding over ACP"}`,
		)}, nil
	}
	return process.CommandResult{Stdout: support.CodexSuccessStdout(
		`{"answer":"deep research finding over ACP"}`,
	)}, nil
}

func (runner *deepResearchACPRunner) callCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.calls
}

// TestPackagedJavaScriptFactoryWithStructuredResultStreamsItsResult proves a
// Factory that succeeds with a structured (JSON) primary result still delivers
// that result to an ACP client.
//
// @you/deep-research returns its synthesis as a JSON primary-result part
// rather than a text one. The prompt outcome mapper projected only `text`
// parts, so this Factory produced a completed turn -- stopReason end_turn, no
// error -- carrying no assistant message whatsoever. On the wire that is
// indistinguishable from a Factory that ran and had nothing to say, and it is
// strictly worse than the CLI, which at least reports "invocation primary
// result is not plain text; use --json" rather than rendering silence.
//
// A successful invocation must return its result. The cell asserts the
// Factory's own output reaches the client, not merely that some text did.
func TestPackagedJavaScriptFactoryWithStructuredResultStreamsItsResult(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving root.BuildProcess through the you server acp CLI command")
	}

	t.Setenv(operatorsettings.EnvDefaultWorkerModelProvider, "codex")
	t.Setenv(operatorsettings.EnvDefaultWorkerModel, "gpt-5")

	runner := &deepResearchACPRunner{}
	cohort := newPackagedFactoryCohort(
		t,
		"@you/deep-research",
		"factory:@you/deep-research",
		runner,
	)
	cwd := t.TempDir()
	stdin, stdout := startServeACPProcess(t, cohort.process, cohort.home, cwd)

	sessionID := driveServeACPSessionNew(t, stdin, stdout, cwd)
	if sessionID == "" {
		t.Fatal("session/new returned a blank sessionId")
	}

	promptResp, notifications := driveServeACPSessionPrompt(
		t, stdin, stdout, sessionID, "research what this repository does and synthesize it",
	)
	if promptResp.Error != nil {
		t.Fatalf("session/prompt response error = %+v (provider calls = %d), want a successful final result",
			promptResp.Error, runner.callCount())
	}
	var decoded acpsdk.PromptResponse
	if err := json.Unmarshal(promptResp.Result, &decoded); err != nil {
		t.Fatalf("unmarshal PromptResponse: %v", err)
	}
	if decoded.StopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("stopReason = %q, want %q", decoded.StopReason, acpsdk.StopReasonEndTurn)
	}
	if runner.callCount() == 0 {
		t.Fatal("provider call count = 0, want the workflow's own lead and specialist agents to have run")
	}

	assistantText := agentMessageText(t, notifications)
	if strings.TrimSpace(assistantText) == "" {
		t.Fatalf("streamed assistant text is empty for a succeeded invocation (provider calls = %d); "+
			"a completed turn must carry the Factory's result, structured or not", runner.callCount())
	}
	// The delivered text must be this Factory's own synthesis, not a
	// placeholder standing in for it.
	if !strings.Contains(assistantText, "deep research finding over ACP") {
		t.Fatalf("streamed assistant text = %q, want it to carry the workflow's own synthesized findings", assistantText)
	}
}
