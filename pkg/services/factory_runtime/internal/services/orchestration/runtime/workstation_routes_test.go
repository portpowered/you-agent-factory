package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type routeNamesTestExecutor struct{}

type invocationInterpolationTestService struct{}

func (invocationInterpolationTestService) ValidateInvocationInterpolation(
	*interfaces.FactoryConfig,
	*work.InvocationArguments,
	interfaces.FileReader,
) error {
	return nil
}

func (invocationInterpolationTestService) InterpolateWorkerConfig(
	worker interfaces.FactoryWorkerConfig,
	invocation *work.InvocationArguments,
	_ interfaces.FileReader,
) (interfaces.FactoryWorkerConfig, error) {
	if invocation != nil {
		for name, argument := range invocation.Arguments {
			if len(argument.Values) == 1 {
				worker.Body = strings.ReplaceAll(
					worker.Body,
					"${"+name+"}",
					argument.Values[0],
				)
			}
		}
	}
	return worker, nil
}

func (invocationInterpolationTestService) InterpolateWorkstationConfig(
	workstation interfaces.FactoryWorkstationConfig,
	invocation *work.InvocationArguments,
	_ interfaces.FileReader,
) (interfaces.FactoryWorkstationConfig, error) {
	for name, argument := range invocation.Arguments {
		if len(argument.Values) == 1 {
			workstation.PromptTemplate = strings.ReplaceAll(
				workstation.PromptTemplate,
				"${"+name+"}",
				argument.Values[0],
			)
		}
	}
	return workstation, nil
}

func (routeNamesTestExecutor) Execute(context.Context, work.WorkDispatch) (workerexecution.WorkResult, error) {
	return workerexecution.WorkResult{}, nil
}

func TestRuntimeWorkstationRouteNamesIncludeWorkerAndWorkstationKeys(t *testing.T) {
	net := &state.Net{
		Transitions: map[string]*petri.Transition{
			"tr-1": {ID: "tr-1", Name: "review", WorkerType: "swe"},
		},
	}
	names := runtimeWorkstationRouteNames(net, map[string]workers.WorkerExecutor{
		"swe": routeNamesTestExecutor{},
	})
	want := map[string]struct{}{"tr-1": {}, "review": {}, "swe": {}}
	if len(names) != len(want) {
		t.Fatalf("route names = %v, want %v", names, want)
	}
	for _, name := range names {
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected route name %q in %v", name, names)
		}
	}
}

func TestRuntimeExecutionSelectionMarksTopologyOnlyWorkerAsNoop(t *testing.T) {
	lookup := runtimefixtures.RuntimeConfigLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"step-one": {
				Name:           "step-one",
				WorkerTypeName: "worker-a",
			},
		},
	}
	selection := resolveRuntimeExecutionSelection(
		&runtimeConfig{runtimeConfig: lookup},
		workers.WorkstationDispatchRequest{
			WorkstationName: "step-one",
			Execution: workers.WorkstationExecutionRequest{
				WorkerType: "worker-a",
			},
		},
		nil,
		nil,
		nil,
	)

	if !selection.noop {
		t.Fatalf("selection = %#v, want topology-only no-op", selection)
	}
	if selection.workerName != "worker-a" || selection.runnerID != workers.RunnerIDCodex {
		t.Fatalf("selection identity = %#v, want worker-a with default codex runner", selection)
	}
}

func TestApplyRuntimeWorkstationSelectionMarksGoalRoutingEnvelope(t *testing.T) {
	selection := runtimeExecutionSelection{}
	applyRuntimeWorkstationSelection(nil, &selection, nil, &interfaces.FactoryWorkstationConfig{
		OutcomeFormat: interfaces.DecisionEnvelopeOutcomeFormat,
		ClassificationRoutes: []interfaces.ClassificationRouteConfig{{
			Label: "accepted",
		}},
	})

	if !selection.decisionEnvelope || !selection.goalRoutingDecisionEnvelope {
		t.Fatalf("selection output policy = %#v, want decision and goal-routing envelopes", selection)
	}
}

func TestFinalizeRuntimeExecutionSelectionUsesProviderRunner(t *testing.T) {
	tests := []struct {
		name          string
		providerID    string
		modelProvider string
		wantRunner    string
	}{
		{name: "authored claude model provider", modelProvider: "claude", wantRunner: workers.RunnerIDClaude},
		{name: "authored agy executor provider", providerID: "agy", modelProvider: "codex", wantRunner: workers.RunnerIDAntigravity},
		{name: "authored ACP integration", providerID: workers.ExecutorProviderACP, modelProvider: "copilot-acp", wantRunner: "copilot-acp"},
		{name: "unknown provider uses codex default", modelProvider: "operator-provider", wantRunner: workers.RunnerIDCodex},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selection := runtimeExecutionSelection{
				providerID:    test.providerID,
				modelProvider: test.modelProvider,
				model:         "model",
				workerType:    interfaces.WorkerTypeModel,
			}
			finalizeRuntimeExecutionSelection(nil, &selection, nil)
			if selection.runnerID != test.wantRunner {
				t.Fatalf("runnerID = %q, want %q; selection = %#v", selection.runnerID, test.wantRunner, selection)
			}
		})
	}
}

func TestFinalizeRuntimeExecutionSelectionCanonicalizesACPProvider(t *testing.T) {
	selection := runtimeExecutionSelection{
		providerID:    workers.ExecutorProviderACP,
		modelProvider: "cursor-acp",
		model:         "test-model",
		workerType:    interfaces.WorkerTypeModel,
	}

	finalizeRuntimeExecutionSelection(nil, &selection, nil)

	if selection.providerID != "cursor-acp" || selection.runnerID != "cursor-acp" {
		t.Fatalf("selection = %#v, want concrete ACP provider and runner", selection)
	}
}

func TestFinalizeRuntimeExecutionSelectionCanonicalizesScriptWrapProvider(t *testing.T) {
	selection := runtimeExecutionSelection{
		providerID:    "SCRIPT_WRAP",
		modelProvider: "codex",
		model:         "gpt-5-codex",
		workerType:    interfaces.WorkerTypeModel,
	}

	finalizeRuntimeExecutionSelection(nil, &selection, nil)

	if selection.providerID != "codex" || selection.runnerID != workers.RunnerIDCodex {
		t.Fatalf("selection = %#v, want canonical codex provider and runner", selection)
	}
}

func TestApplyRuntimeWorkerSelectionUsesWorkerBodyAsSystemPrompt(t *testing.T) {
	selection := runtimeExecutionSelection{}
	applyRuntimeWorkerSelection(nil, &selection, workers.WorkstationExecutionRequest{}, nil, &interfaces.FactoryWorkerConfig{
		Name: "worker",
		Type: interfaces.WorkerTypeModel,
		Body: "worker system prompt",
	})

	if selection.systemPrompt != "worker system prompt" {
		t.Fatalf("systemPrompt = %q, want worker body", selection.systemPrompt)
	}
}

func TestApplyRuntimeWorkerSelectionInterpolatesRefreshedWorkerPrompt(t *testing.T) {
	lookup := runtimePromptSourceLookupFixture{
		RuntimeDefinitionLookupFixture: runtimefixtures.RuntimeDefinitionLookupFixture{},
		worker:                         interfaces.PromptSource{Path: "worker.md"},
	}
	cfg := &runtimeConfig{
		runtimeConfig:           lookup,
		invocationInterpolation: invocationInterpolationTestService{},
		promptSourceReader: func(path string) ([]byte, error) {
			if path != "worker.md" {
				return nil, errors.New("missing source")
			}
			return []byte("---\nrole: worker\n---\nReasoning effort: ${effort}\nbody"), nil
		},
	}
	selection := runtimeExecutionSelection{}
	err := applyRuntimeWorkerSelection(
		cfg,
		&selection,
		workers.WorkstationExecutionRequest{},
		&work.InvocationArguments{Arguments: map[string]work.InvocationArgument{
			"effort": {Values: []string{"low"}},
		}},
		&interfaces.FactoryWorkerConfig{Name: "worker", Type: interfaces.WorkerTypeModel},
	)
	if err != nil {
		t.Fatalf("applyRuntimeWorkerSelection() error = %v", err)
	}
	if selection.systemPrompt != "Reasoning effort: low\nbody" {
		t.Fatalf("systemPrompt = %q, want interpolated refreshed prompt", selection.systemPrompt)
	}
}

func TestRuntimeAuthoredPromptBodyHandlesFrontMatterAndPlainPrompts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "plain", body: "plain prompt", want: "plain prompt"},
		{name: "front matter", body: "---\nrole: worker\n---\n\nbody", want: "body"},
		{name: "windows front matter", body: "---\r\nrole: worker\r\n---\r\nbody", want: "body"},
		{name: "front matter only", body: "---\nrole: worker\n---\n", want: ""},
		{name: "unterminated front matter", body: "---\nrole: worker\nbody", want: "---\nrole: worker\nbody"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runtimeAuthoredPromptBody(test.body); got != test.want {
				t.Fatalf("runtimeAuthoredPromptBody() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestApplyRuntimeWorkstationSelectionUsesAuthoredLimitsAndRoutingPolicy(t *testing.T) {
	t.Parallel()

	selection := runtimeExecutionSelection{workerType: interfaces.WorkerTypeScript}
	applyRuntimeWorkstationSelection(nil, &selection, nil, &interfaces.FactoryWorkstationConfig{
		Name:             "classify",
		Type:             interfaces.WorkstationTypeClassify,
		Body:             "system",
		PromptTemplate:   "prompt",
		WorkingDirectory: "workspace",
		OutcomeFormat:    interfaces.DecisionEnvelopeOutcomeFormat,
		ClassificationRoutes: []interfaces.ClassificationRouteConfig{{
			Label: "accepted",
		}},
		Limits:  interfaces.WorkstationLimits{MaxExecutionTime: "3s"},
		Timeout: "1s",
	})

	if selection.systemPrompt != "system" || selection.promptTemplate != "prompt" {
		t.Fatalf("selection prompts = %#v, want authored body/template", selection)
	}
	if selection.timeout != 3*time.Second {
		t.Fatalf("selection timeout = %s, want 3s", selection.timeout)
	}
	if !selection.scriptClassifier || !selection.decisionEnvelope || !selection.goalRoutingDecisionEnvelope {
		t.Fatalf("selection output policy = %#v, want classifier and decision envelopes", selection)
	}
	if !selection.workingDirectoryAuthored {
		t.Fatal("workingDirectoryAuthored = false, want true")
	}
}

func TestRenderRuntimePromptAndTemplateFieldsUsesDetachedCapabilities(t *testing.T) {
	t.Parallel()

	selection := &runtimeExecutionSelection{
		promptTemplate:   "authored prompt",
		workingDirectory: "{{.workdir}}",
		environment:      map[string]string{"TOKEN": "{{.token}}"},
		worktree:         "{{.worktree}}",
	}
	fieldResolver := runtimeTemplateFieldResolverFunc(func(
		workingDirectory string,
		environment map[string]string,
		_ []workers.Token,
		_ *workers.Context,
		worktree string,
	) (*workers.ResolvedTemplateFields, error) {
		if workingDirectory == "" || environment["TOKEN"] == "" || worktree == "" {
			return nil, errors.New("detached template inputs were lost")
		}
		return &workers.ResolvedTemplateFields{
			WorkingDirectory: "resolved-workdir",
			Worktree:         "resolved-worktree",
			Env:              map[string]string{"TOKEN": "resolved"},
		}, nil
	})
	cfg := &runtimeConfig{
		promptRenderer: runtimePromptRendererFunc(func(
			prompt string,
			_ []workers.Token,
			_ *workers.Context,
		) (string, error) {
			if prompt != "authored prompt" {
				return "", errors.New("unexpected prompt")
			}
			return "rendered prompt", nil
		}),
		templateFieldResolver: fieldResolver,
	}
	if err := renderRuntimePrompt(cfg, selection, nil, &workers.Context{}, nil, nil); err != nil {
		t.Fatalf("renderRuntimePrompt() error = %v", err)
	}
	if selection.userMessage != "rendered prompt" || selection.workingDirectory != "resolved-workdir" ||
		selection.worktree != "resolved-worktree" || selection.environment["TOKEN"] != "resolved" {
		t.Fatalf("rendered selection = %#v, want detached rendered fields", selection)
	}

	badRenderer := &runtimeConfig{
		promptRenderer: runtimePromptRendererFunc(func(string, []workers.Token, *workers.Context) (string, error) {
			return "", errors.New("prompt failed")
		}),
	}
	if err := renderRuntimePrompt(badRenderer, &runtimeExecutionSelection{promptTemplate: "bad"}, nil, nil, nil, nil); err == nil {
		t.Fatal("renderRuntimePrompt() error = nil, want prompt rendering error")
	}
}

func TestRenderRuntimePromptUsesResolvedContextAndWorkInputTokens(t *testing.T) {
	t.Parallel()

	selection := &runtimeExecutionSelection{
		promptTemplate:   "authored prompt",
		workingDirectory: "authored-workdir",
		environment:      map[string]string{"TOKEN": "authored"},
		worktree:         "authored-worktree",
	}
	tokens := []workers.Token{
		{ID: "resource", Color: workers.Color{DataType: workers.DataTypeResource}},
		{ID: "work", Color: workers.Color{DataType: workers.DataTypeWork}},
	}
	callOrder := make([]string, 0, 2)
	fieldResolver := runtimeTemplateFieldResolverFunc(func(
		string,
		map[string]string,
		[]workers.Token,
		*workers.Context,
		string,
	) (*workers.ResolvedTemplateFields, error) {
		callOrder = append(callOrder, "resolve")
		return &workers.ResolvedTemplateFields{
			WorkingDirectory: "resolved-workdir",
			Env:              map[string]string{"TOKEN": "resolved"},
		}, nil
	})
	promptRenderer := runtimePromptRendererFunc(func(
		prompt string,
		tokens []workers.Token,
		context *workers.Context,
	) (string, error) {
		callOrder = append(callOrder, "render")
		if prompt != "authored prompt" {
			return "", errors.New("unexpected prompt")
		}
		if len(tokens) != 1 || tokens[0].ID != "work" {
			return "", errors.New("prompt received non-work tokens")
		}
		if context.WorkDirectory != "resolved-workdir" ||
			context.EnvVars["TOKEN"] != "resolved" ||
			context.EnvVars["BASE"] != "base" {
			return "", errors.New("prompt received unresolved execution context")
		}
		return "resolved prompt", nil
	})
	baseContext := &workers.Context{
		WorkDirectory: "base-workdir",
		EnvVars:       map[string]string{"BASE": "base"},
	}
	cfg := &runtimeConfig{
		promptRenderer:        promptRenderer,
		templateFieldResolver: fieldResolver,
	}
	if err := renderRuntimePrompt(cfg, selection, tokens, baseContext, nil, nil); err != nil {
		t.Fatalf("renderRuntimePrompt() error = %v", err)
	}
	if strings.Join(callOrder, ",") != "resolve,render" {
		t.Fatalf("render call order = %q, want resolve,render", strings.Join(callOrder, ","))
	}
	if selection.userMessage != "resolved prompt" {
		t.Fatalf("userMessage = %q, want resolved prompt", selection.userMessage)
	}
	if baseContext.WorkDirectory != "base-workdir" || baseContext.EnvVars["TOKEN"] != "" {
		t.Fatalf("base prompt context mutated = %#v", baseContext)
	}
}

func TestRenderRuntimePromptInterpolatesInvocationArguments(t *testing.T) {
	t.Parallel()

	cfg := &runtimeConfig{
		invocationInterpolation: invocationInterpolationTestService{},
		promptRenderer: runtimePromptRendererFunc(func(
			prompt string,
			_ []workers.Token,
			_ *workers.Context,
		) (string, error) {
			if prompt != "clip=/tmp/clip.mp4\nshot=hero" {
				t.Fatalf("prompt = %q, want interpolated invocation values", prompt)
			}
			return prompt, nil
		}),
	}
	selection := &runtimeExecutionSelection{
		promptTemplate: "clip=${clipPath}\nshot=${shotSpecification}",
	}
	invocation := &work.InvocationArguments{Arguments: map[string]work.InvocationArgument{
		"clipPath":          {Values: []string{"/tmp/clip.mp4"}},
		"shotSpecification": {Values: []string{"hero"}},
	}}

	if err := renderRuntimePrompt(cfg, selection, nil, &workers.Context{}, nil, invocation); err != nil {
		t.Fatalf("renderRuntimePrompt() error = %v", err)
	}
	if selection.userMessage != "clip=/tmp/clip.mp4\nshot=hero" {
		t.Fatalf("userMessage = %q, want interpolated invocation values", selection.userMessage)
	}
}

func TestRuntimePromptSourceContentRefreshesAuthoredFiles(t *testing.T) {
	t.Parallel()

	lookup := runtimePromptSourceLookupFixture{
		RuntimeDefinitionLookupFixture: runtimefixtures.RuntimeDefinitionLookupFixture{},
		worker:                         interfaces.PromptSource{Path: "worker.md"},
		workstation:                    interfaces.PromptSource{Path: "workstation.md", IsTemplate: true},
	}
	cfg := &runtimeConfig{
		runtimeConfig: lookup,
		promptSourceReader: func(path string) ([]byte, error) {
			switch path {
			case "worker.md":
				return []byte("---\nrole: worker\n---\nworker body"), nil
			case "workstation.md":
				return []byte("{{.Inputs}}"), nil
			default:
				return nil, errors.New("missing source")
			}
		},
	}
	if got, ok := runtimePromptSourceContent(cfg, "worker", true, true); !ok || got != "worker body" {
		t.Fatalf("worker source = %q, %t, want exact authored body", got, ok)
	}
	if got, ok := runtimePromptSourceContent(cfg, "workstation", false, false); !ok || got != "{{.Inputs}}" {
		t.Fatalf("workstation source = %q, %t, want exact template", got, ok)
	}
	if _, ok := runtimePromptSourceContent(cfg, "missing", true, true); ok {
		t.Fatal("missing prompt source reported found")
	}
}

func TestNormalizeScriptClassifierResultKeepsFinalLabelOnly(t *testing.T) {
	t.Parallel()

	result := workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeAccepted,
		Output: workers.ProposedOutput{Primary: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "diagnostic line\naccepted",
		}}},
	}
	normalized := normalizeScriptClassifierResult(
		workers.ExecutionTarget{RunnerID: "script", Output: workers.OutputPolicy{ScriptClassifier: true}},
		result,
	)
	if normalized.Output.Primary[0].Text != "accepted" || normalized.Output.Classification != "accepted" {
		t.Fatalf("normalized classifier result = %#v, want final label", normalized)
	}
	unchanged := normalizeScriptClassifierResult(
		workers.ExecutionTarget{RunnerID: workers.RunnerIDCodex, Output: workers.OutputPolicy{ScriptClassifier: true}},
		result,
	)
	if unchanged.Output.Primary[0].Text != "diagnostic line\naccepted" {
		t.Fatalf("non-script result changed = %#v", unchanged)
	}
}

func TestIsLogicalWorkstationDispatchUsesRuntimeDefinitionLookup(t *testing.T) {
	t.Parallel()

	cfg := &runtimeConfig{runtimeConfig: runtimefixtures.RuntimeDefinitionLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"move":    {Type: interfaces.WorkstationTypeLogical},
			"execute": {Type: interfaces.WorkstationTypeModel},
		},
	}}
	logical := workers.WorkstationDispatchRequest{WorkstationName: "move"}
	if !isLogicalWorkstationDispatch(cfg, logical) {
		t.Fatal("logical workstation was not recognized")
	}
	if isLogicalWorkstationDispatch(cfg, workers.WorkstationDispatchRequest{WorkstationName: "execute"}) {
		t.Fatal("execute workstation was recognized as logical")
	}
}

func TestOrderedRuntimeWorkDispatchTokensUsesAuthoredInputAndResourceOrder(t *testing.T) {
	t.Parallel()

	token := func(id, place string) workers.Token {
		prefix, stateName := place, ""
		if index := strings.LastIndexByte(place, ':'); index >= 0 {
			prefix, stateName = place[:index], place[index+1:]
		}
		return workers.Token{ID: id, State: stateName, Color: workers.Color{WorkTypeID: prefix}}
	}
	dispatch := work.WorkDispatch{InputTokens: workers.InputTokens(
		token("unmatched", "other:state"),
		token("resource", "gpu:available"),
		token("input", "task:ready"),
	)}
	request := workers.WorkstationDispatchRequest{
		WorkstationName: "invoke",
		Execution:       workers.WorkstationExecutionRequest{Dispatch: dispatch},
	}
	workstation := &interfaces.FactoryWorkstationConfig{
		Type: interfaces.WorkstationTypeInference,
		Inputs: []interfaces.IOConfig{{
			WorkTypeName: "task",
			StateName:    "ready",
		}},
		Resources: []interfaces.ResourceConfig{{Name: "gpu"}},
	}
	cfg := &runtimeConfig{runtimeConfig: runtimefixtures.RuntimeDefinitionLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{"invoke": workstation},
	}}

	ordered, err := orderedRuntimeWorkDispatchTokens(cfg, request, nil)
	if err != nil {
		t.Fatalf("orderedRuntimeWorkDispatchTokens() error = %v", err)
	}
	if len(ordered) != 3 || ordered[0].ID != "input" || ordered[1].ID != "resource" || ordered[2].ID != "unmatched" {
		t.Fatalf("ordered tokens = %#v, want input/resource/unmatched order", ordered)
	}

	unchangedCases := []struct {
		name string
		cfg  *runtimeConfig
		req  workers.WorkstationDispatchRequest
	}{
		{name: "short dispatch", cfg: cfg, req: workers.WorkstationDispatchRequest{Execution: workers.WorkstationExecutionRequest{Dispatch: work.WorkDispatch{InputTokens: workers.InputTokens(token("only", "task:ready"))}}}},
		{name: "missing lookup", cfg: nil, req: request},
		{name: "missing workstation", cfg: &runtimeConfig{runtimeConfig: runtimefixtures.RuntimeDefinitionLookupFixture{}}, req: request},
	}
	for _, test := range unchangedCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			got, err := orderedRuntimeWorkDispatchTokens(test.cfg, test.req, nil)
			if err != nil {
				t.Fatalf("orderedRuntimeWorkDispatchTokens() error = %v", err)
			}
			want := workers.WorkDispatchInputTokens(test.req.Execution.Dispatch)
			if len(got) != len(want) {
				t.Fatalf("ordered tokens = %#v, want unchanged length %d", got, len(want))
			}
			for index := range want {
				if got[index].ID != want[index].ID {
					t.Fatalf("ordered token %d = %#v, want unchanged token %#v", index, got[index], want[index])
				}
			}
		})
	}
}

func TestResolveExecutionRequestUsesDirectInputTokensForAuthoredPrompt(t *testing.T) {
	t.Parallel()
	cfg, request, modelScope := directInputPromptRuntimeFixture(t)
	executeRequest, err := executeRequestFromWorkstationRequest(cfg, request)
	if err != nil {
		t.Fatalf("executeRequestFromWorkstationRequest() error = %v", err)
	}
	assertDirectInputPromptExecuteRequest(t, executeRequest, modelScope)
}

func TestAttemptLifecycleAllowsExplicitRetryAfterTerminal(t *testing.T) {
	t.Parallel()

	var calls int
	service := attemptExecuteFunc(func(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		calls++
		return workers.ExecuteResult{Correlation: request.Correlation, Outcome: workers.ExecutionOutcomeAccepted}, nil
	})
	lifecycle := newAttemptLifecycle(service, func() string { return "generated-attempt" }, 1)
	terminal := func(_ context.Context, _ workers.ExecuteRequest, result workers.ExecuteResult, err error) {
		if err != nil || result.Outcome != workers.ExecutionOutcomeAccepted {
			t.Errorf("terminal callback = result %#v, error %v; want accepted result", result, err)
		}
	}

	if err := lifecycle.start(context.Background(), attemptTestRequest("dispatch-retry", "attempt-1"), false, terminal); err != nil {
		t.Fatalf("start(first) error = %v", err)
	}
	if err := lifecycle.startRetry(context.Background(), attemptTestRequest("dispatch-retry", "attempt-2"), false, terminal); err != nil {
		t.Fatalf("startRetry(second) error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("Execute calls = %d, want two attempts", calls)
	}
	if got, ok := lifecycle.terminalAttemptID("dispatch-retry"); !ok || got != "attempt-2" {
		t.Fatalf("terminal attempt = %q, %v; want attempt-2, true", got, ok)
	}
}

func TestFailedWorkstationDispatchResultPreservesDispatchFailureIdentity(t *testing.T) {
	t.Parallel()

	dispatchErr := errors.New("provider failed")
	request := workers.WorkstationDispatchRequest{
		WorkstationName: "station",
		Execution: workers.WorkstationExecutionRequest{Dispatch: work.WorkDispatch{
			DispatchID:   "dispatch-failed",
			TransitionID: "transition-failed",
		}},
	}
	result, err := failedWorkstationDispatchResult(request, dispatchErr)
	if !errors.Is(err, dispatchErr) {
		t.Fatalf("failedWorkstationDispatchResult() error = %v, want original error", err)
	}
	if result.DispatchID != "dispatch-failed" || result.WorkstationName != "station" ||
		result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeFailed ||
		result.Result.DispatchID != "dispatch-failed" || result.Result.TransitionID != "transition-failed" ||
		result.Result.Outcome != workers.OutcomeFailed || result.Result.Error != dispatchErr.Error() {
		t.Fatalf("failed dispatch result = %#v, want dispatch failure identity", result)
	}
}

func TestWorkInputsFromDispatchFiltersResourcesAndBuildsAttemptFacts(t *testing.T) {
	t.Parallel()

	dispatch := workInputsFromDispatchFixture()
	inputs, gotInvocation, attempt := workInputsFromDispatch(dispatch)
	if len(inputs) != 2 || attempt != 5 {
		t.Fatalf("work inputs = %#v, attempt = %d; want two inputs and attempt 5", inputs, attempt)
	}
	if gotInvocation.Arguments["mode"].Values[0] != "fast" {
		t.Fatalf("invocation arguments = %#v, want cloned mode argument", gotInvocation)
	}
	assertLegacyWorkInput(t, inputs[0])
	assertContentWorkInput(t, inputs[1])
}

func TestDetachedResultMaterializationMapsTerminalOutcomes(t *testing.T) {
	t.Parallel()

	request := workers.WorkstationDispatchRequest{
		WorkstationName: "review",
		Execution: workers.WorkstationExecutionRequest{Dispatch: work.WorkDispatch{
			DispatchID:   "dispatch-outcomes",
			TransitionID: "transition-outcomes",
		}},
	}
	tests := []struct {
		name          string
		outcome       workers.ExecutionOutcome
		executeErr    error
		wantTerminal  workers.WorkstationDispatchTerminalOutcome
		wantWork      workers.WorkOutcome
		wantErrorText string
	}{
		{name: "accepted", outcome: workers.ExecutionOutcomeAccepted, wantTerminal: workers.WorkstationDispatchTerminalOutcomeCompleted, wantWork: workers.OutcomeAccepted},
		{name: "continue", outcome: workers.ExecutionOutcomeContinue, wantTerminal: workers.WorkstationDispatchTerminalOutcomeCompleted, wantWork: workers.OutcomeContinue},
		{name: "rejected", outcome: workers.ExecutionOutcomeRejected, wantTerminal: workers.WorkstationDispatchTerminalOutcomeCompleted, wantWork: workers.OutcomeRejected},
		{name: "canceled", outcome: workers.ExecutionOutcomeCanceled, wantTerminal: workers.WorkstationDispatchTerminalOutcomeCanceled, wantWork: workers.OutcomeCanceled, wantErrorText: workers.ErrWorkstationDispatchCanceled.Error()},
		{name: "canceled error", outcome: workers.ExecutionOutcomeFailed, executeErr: workers.ErrWorkstationDispatchCanceled, wantTerminal: workers.WorkstationDispatchTerminalOutcomeCanceled, wantWork: workers.OutcomeCanceled, wantErrorText: workers.ErrWorkstationDispatchCanceled.Error()},
		{name: "unknown", outcome: workers.ExecutionOutcome("unexpected"), wantTerminal: workers.WorkstationDispatchTerminalOutcomeFailed, wantWork: workers.OutcomeFailed},
		{name: "execution error", outcome: workers.ExecutionOutcomeAccepted, executeErr: errors.New("transport failed"), wantTerminal: workers.WorkstationDispatchTerminalOutcomeFailed, wantWork: workers.OutcomeFailed, wantErrorText: "transport failed"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			result, err := workstationDispatchResultFromExecute(request, workers.ExecuteResult{Outcome: test.outcome}, test.executeErr)
			if !errors.Is(err, test.executeErr) {
				t.Fatalf("workstationDispatchResultFromExecute() error = %v, want %v", err, test.executeErr)
			}
			if result.TerminalOutcome != test.wantTerminal || result.Result.Outcome != test.wantWork {
				t.Fatalf("result = %#v, want terminal %q and Work outcome %q", result, test.wantTerminal, test.wantWork)
			}
			if test.wantErrorText != "" && result.Result.Error != test.wantErrorText {
				t.Fatalf("result error = %q, want %q", result.Result.Error, test.wantErrorText)
			}
		})
	}
}

func TestDetachedResultMaterializationMapsProcessGoneReconciliation(t *testing.T) {
	t.Parallel()

	request := workers.WorkstationDispatchRequest{
		WorkstationName: "review",
		Execution: workers.WorkstationExecutionRequest{Dispatch: work.WorkDispatch{
			DispatchID:   "dispatch-process-gone",
			TransitionID: "transition-process-gone",
		}},
	}
	result, err := workstationDispatchResultFromExecute(
		request,
		workers.ExecuteResult{Outcome: workers.ExecutionOutcomeCanceled},
		workers.ErrWorkstationDispatchProcessGone,
	)
	if !errors.Is(err, workers.ErrWorkstationDispatchProcessGone) {
		t.Fatalf("workstationDispatchResultFromExecute() error = %v, want process-gone error", err)
	}
	if result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeFailed ||
		result.ReconciliationReason != workers.WorkstationDispatchReconciliationReasonProcessGone ||
		result.Result.Outcome != workers.OutcomeFailed ||
		result.Result.Error != workers.ErrWorkstationDispatchProcessGone.Error() {
		t.Fatalf("process-gone dispatch result = %#v, want failed reconciled result", result)
	}
	if result.Result.FailureMetadata == nil ||
		result.Result.FailureMetadata.Family != workers.WorkFailureFamilyRetryable ||
		result.Result.FailureMetadata.Type != workers.WorkFailureTypeUnknown {
		t.Fatalf("process-gone failure metadata = %#v, want retryable unknown failure", result.Result.FailureMetadata)
	}
	if result.Result.Diagnostics == nil || result.Result.Diagnostics.Metadata == nil ||
		result.Result.Diagnostics.Metadata[workers.ProviderResponseMetadataFailureOperation] != "worker_session_reconciliation" ||
		result.Result.Diagnostics.Metadata[workers.ProviderResponseMetadataFailureClassification] != "process_gone" ||
		result.Result.Diagnostics.Metadata[workers.ProviderResponseMetadataFailureStage] != "process" {
		t.Fatalf("process-gone diagnostics = %#v, want bounded reconciliation metadata", result.Result.Diagnostics)
	}
}

func TestProcessGoneScriptAttemptPreservesTerminalFailureReason(t *testing.T) {
	t.Parallel()

	result := processGoneAttemptResult(
		workers.ExecuteRequest{Target: workers.ExecutionTarget{RunnerID: "script"}},
		workers.ExecuteResult{
			Failure: &workers.ExecutionFailure{Message: "script worker exited non-zero"},
			Diagnostics: &workers.SafeDiagnostics{Command: &workers.SafeCommandDiagnostic{
				Stderr: "script worker exited non-zero",
			}},
		},
	)
	if result.Failure == nil || result.Failure.Message != "script worker exited non-zero" {
		t.Fatalf("process-gone script failure = %#v, want preserved script reason", result.Failure)
	}
}

func TestProviderSessionFromContinuationUsesAvailableIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		continuation *workers.ProviderContinuationRef
		wantNil      bool
		wantProvider string
		wantID       string
	}{
		{name: "nil", wantNil: true},
		{name: "session ID", continuation: &workers.ProviderContinuationRef{Provider: "codex", ProviderSessionID: "session-1"}, wantProvider: "codex", wantID: "session-1"},
		{name: "external reference", continuation: &workers.ProviderContinuationRef{Provider: "codex", ExternalRef: "external-1"}, wantProvider: "codex", wantID: "external-1"},
		{name: "provider only", continuation: &workers.ProviderContinuationRef{Provider: "codex"}, wantProvider: "codex"},
		{name: "empty", continuation: &workers.ProviderContinuationRef{}, wantNil: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			got := providerSessionFromContinuation(test.continuation)
			if test.wantNil {
				if got != nil {
					t.Fatalf("providerSessionFromContinuation() = %#v, want nil", got)
				}
				return
			}
			if got == nil || got.Provider != test.wantProvider || got.ID != test.wantID {
				t.Fatalf("providerSessionFromContinuation() = %#v, want provider %q and ID %q", got, test.wantProvider, test.wantID)
			}
		})
	}
}

func TestMapDispatchPlanningErrorPreservesPublicBoundaryTypes(t *testing.T) {
	t.Parallel()

	plain := errors.New("plain planning error")
	tests := []struct {
		name string
		err  error
		want error
	}{
		{name: "duplicate", err: dispatchplanning.ErrDuplicateDispatchIntent, want: factory.ErrDuplicateDispatchIntent},
		{name: "unknown correlation", err: dispatchplanning.ErrUnknownDispatchCorrelation, want: factory.ErrUnknownDispatchCorrelation},
		{name: "invalid result", err: dispatchplanning.ErrInvalidDispatchResultBoundary, want: factory.ErrInvalidDispatchResultBoundary},
		{name: "invalid decision", err: dispatchplanning.ErrInvalidRunnableDecision, want: factory.ErrInvalidDispatchResultBoundary},
		{name: "unrelated", err: plain, want: plain},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			got := mapDispatchPlanningError(test.err)
			if !errors.Is(got, test.want) {
				t.Fatalf("mapDispatchPlanningError(%v) = %v, want %v", test.err, got, test.want)
			}
			if test.name == "unrelated" && got != test.err {
				t.Fatalf("unrelated error = %v, want original error identity", got)
			}
		})
	}
}

type runtimePromptRendererFunc func(string, []workers.Token, *workers.Context) (string, error)

func (renderer runtimePromptRendererFunc) RenderPrompt(
	prompt string,
	tokens []workers.Token,
	context *workers.Context,
) (string, error) {
	return renderer(prompt, tokens, context)
}

type runtimeTemplateFieldResolverFunc func(
	string,
	map[string]string,
	[]workers.Token,
	*workers.Context,
	string,
) (*workers.ResolvedTemplateFields, error)

func (resolver runtimeTemplateFieldResolverFunc) ResolveTemplateFields(
	workingDirectory string,
	environment map[string]string,
	tokens []workers.Token,
	context *workers.Context,
	worktree string,
) (*workers.ResolvedTemplateFields, error) {
	return resolver(workingDirectory, environment, tokens, context, worktree)
}

type runtimePromptSourceLookupFixture struct {
	runtimefixtures.RuntimeDefinitionLookupFixture
	worker                interfaces.PromptSource
	workstation           interfaces.PromptSource
	workerProvenance      interfaces.RuntimePromptProvenance
	workstationProvenance interfaces.RuntimePromptProvenance
}

func (fixture runtimePromptSourceLookupFixture) WorkerPromptSource(name string) (interfaces.PromptSource, bool) {
	if name != "worker" {
		return interfaces.PromptSource{}, false
	}
	return fixture.worker, fixture.worker.Path != ""
}

func (fixture runtimePromptSourceLookupFixture) WorkstationPromptSource(name string) (interfaces.PromptSource, bool) {
	if name != "workstation" {
		return interfaces.PromptSource{}, false
	}
	return fixture.workstation, fixture.workstation.Path != ""
}

func (fixture runtimePromptSourceLookupFixture) WorkerPromptProvenance(
	name string,
) (interfaces.RuntimePromptProvenance, bool) {
	return fixture.workerProvenance, fixture.workerProvenance.Name == name
}

func (fixture runtimePromptSourceLookupFixture) WorkstationPromptProvenance(
	name string,
) (interfaces.RuntimePromptProvenance, bool) {
	return fixture.workstationProvenance, fixture.workstationProvenance.Name == name
}

func TestVerifyRuntimeExpectedArtifactDeclarationsClassifiesWorkspaceResults(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	reports := filepath.Join(workspace, "reports")
	if err := os.MkdirAll(reports, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(reports, "summary.md"), []byte("summary"), 0o600); err != nil {
		t.Fatalf("WriteFile(summary) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(reports, "empty.md"), nil, 0o600); err != nil {
		t.Fatalf("WriteFile(empty) error = %v", err)
	}

	declarations := []work.ExpectedArtifactDeclaration{
		{Name: "summary", Pattern: "reports/summary.md", NonEmpty: true},
		{Name: "empty", Pattern: "reports/empty.md", NonEmpty: true},
		{Name: "missing", Pattern: "reports/missing.md"},
		{Name: "glob", Pattern: "reports/summary*.md", NonEmpty: true},
		{Name: "unsafe", Pattern: "../outside.md"},
		{Name: "invalid", Pattern: "["},
		{Name: "template", Pattern: "{{.Missing}}"},
	}
	verification := verifyRuntimeExpectedArtifactDeclarations(
		workspace,
		work.WorkDispatch{ExpectedArtifactContext: &work.ExpectedArtifactTemplateContext{}},
		declarations,
		platformfilesystem.Local{},
	)
	if verification == nil || len(verification.Entries) != 5 {
		t.Fatalf("verification = %#v, want five unmet declarations", verification)
	}
	wantReasons := map[string]workers.ExpectedArtifactVerificationReason{
		"empty":    workers.ExpectedArtifactVerificationReasonEmpty,
		"missing":  workers.ExpectedArtifactVerificationReasonMissing,
		"unsafe":   workers.ExpectedArtifactVerificationReasonMissing,
		"invalid":  workers.ExpectedArtifactVerificationReasonMissing,
		"template": workers.ExpectedArtifactVerificationReasonMissing,
	}
	for _, entry := range verification.Entries {
		if want := wantReasons[entry.Name]; entry.Reason != want {
			t.Fatalf("entry %q reason = %q, want %q", entry.Name, entry.Reason, want)
		}
		if entry.Name == "unsafe" && entry.Pattern != unsafeExpectedArtifactPattern {
			t.Fatalf("unsafe pattern = %q, want redacted marker", entry.Pattern)
		}
	}
	message := expectedArtifactVerificationMessage(verification)
	if message == "" || !containsExpectedArtifactMessage(message, "EXPECTED_ARTIFACTS_UNSATISFIED") {
		t.Fatalf("verification message = %q, want failure code", message)
	}

	accepted := verifyExpectedArtifactsForDispatch(
		&runtimeConfig{net: &state.Net{}},
		workers.ExecuteRequest{Target: workers.ExecutionTarget{Environment: workers.EnvironmentPolicy{WorkingDirectory: workspace}}},
		workers.ExecuteResult{Outcome: workers.ExecutionOutcomeAccepted},
	)
	if accepted.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("dispatch without declarations outcome = %q, want accepted", accepted.Outcome)
	}
}

func TestExpectedArtifactDeclarationsForDispatchDeduplicatesWorkTypeAndTransitionDeclarations(t *testing.T) {
	t.Parallel()

	declaration := work.ExpectedArtifactDeclaration{Name: "summary", Pattern: "summary.md"}
	cfg := &runtimeConfig{net: &state.Net{
		WorkTypes: map[string]*state.WorkType{
			"report": {ExpectedArtifacts: []work.ExpectedArtifactDeclaration{declaration}},
		},
		Transitions: map[string]*petri.Transition{
			"review": {ExpectedArtifacts: []work.ExpectedArtifactDeclaration{
				declaration,
				{Name: "details", Pattern: "details.md"},
			}},
		},
	}}
	dispatch := work.WorkDispatch{
		TransitionID: "review",
		InputTokens:  workers.InputTokens(workers.Token{Color: workers.Color{WorkTypeID: "report"}}),
	}
	got := expectedArtifactDeclarationsForDispatch(cfg, dispatch)
	if len(got) != 2 || got[0] != declaration || got[1].Name != "details" {
		t.Fatalf("declarations = %#v, want stable deduplicated order", got)
	}
}

func TestSafeRuntimeExpectedArtifactPatternRejectsHostEscapes(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"", "/absolute/path", "../outside", `C:\\outside`, "["} {
		if normalized, ok := safeRuntimeExpectedArtifactPattern(value); ok || normalized != "" {
			t.Fatalf("safeRuntimeExpectedArtifactPattern(%q) = %q, %t, want rejection", value, normalized, ok)
		}
	}
	if normalized, ok := safeRuntimeExpectedArtifactPattern("reports/*.md"); !ok || normalized != "reports/*.md" {
		t.Fatalf("safeRuntimeExpectedArtifactPattern(valid) = %q, %t", normalized, ok)
	}
}

func TestPathExistsUsesTheInjectedArtifactFilesystem(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	existing := filepath.Join(workspace, "existing.txt")
	if err := os.WriteFile(existing, []byte("content"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	fileSystem := platformfilesystem.Local{}
	if pathExists(existing, fileSystem) != true {
		t.Fatal("pathExists(existing) = false, want true")
	}
	if pathExists(filepath.Join(workspace, "missing.txt"), fileSystem) {
		t.Fatal("pathExists(missing) = true, want false")
	}
	if pathExists(existing, nil) {
		t.Fatal("pathExists(existing, nil filesystem) = true, want false")
	}
}

func containsExpectedArtifactMessage(message, value string) bool {
	for start := 0; start+len(value) <= len(message); start++ {
		if message[start:start+len(value)] == value {
			return true
		}
	}
	return false
}
