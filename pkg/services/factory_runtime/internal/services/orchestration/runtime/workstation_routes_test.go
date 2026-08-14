package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type routeNamesTestExecutor struct{}

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
			finalizeRuntimeExecutionSelection(&selection, nil)
			if selection.runnerID != test.wantRunner {
				t.Fatalf("runnerID = %q, want %q; selection = %#v", selection.runnerID, test.wantRunner, selection)
			}
		})
	}
}

func TestFinalizeRuntimeExecutionSelectionCanonicalizesScriptWrapProvider(t *testing.T) {
	selection := runtimeExecutionSelection{
		providerID:    "SCRIPT_WRAP",
		modelProvider: "codex",
		model:         "gpt-5-codex",
		workerType:    interfaces.WorkerTypeModel,
	}

	finalizeRuntimeExecutionSelection(&selection, nil)

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
	if err := renderRuntimePrompt(cfg, selection, nil, &workers.Context{}, nil); err != nil {
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
	if err := renderRuntimePrompt(badRenderer, &runtimeExecutionSelection{promptTemplate: "bad"}, nil, nil, nil); err == nil {
		t.Fatal("renderRuntimePrompt() error = nil, want prompt rendering error")
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
	worker      interfaces.PromptSource
	workstation interfaces.PromptSource
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

func containsExpectedArtifactMessage(message, value string) bool {
	for start := 0; start+len(value) <= len(message); start++ {
		if message[start:start+len(value)] == value {
			return true
		}
	}
	return false
}
