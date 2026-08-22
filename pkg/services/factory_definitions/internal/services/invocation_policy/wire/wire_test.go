package wire_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorylifecycle "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/lifecycle"
	invocationpolicyservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy"
	invocationpolicywire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestNewService_ConstructsPublishedRootPolicyContracts(t *testing.T) {
	t.Parallel()

	svc, err := invocationpolicywire.NewService(invocationpolicyservice.Dependencies{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if svc == nil {
		t.Fatal("NewService returned nil service")
	}
	var _ invocationpolicyservice.Service = svc

	if svc.DecisionEnvelope() == nil {
		t.Fatal("DecisionEnvelope() returned nil")
	}
	if svc.InvocationInterpolation() == nil {
		t.Fatal("InvocationInterpolation() returned nil")
	}
	if svc.InvocationOutput() == nil {
		t.Fatal("InvocationOutput() returned nil")
	}
	if svc.InvocationWorkType() == nil {
		t.Fatal("InvocationWorkType() returned nil")
	}
	if svc.QuorumPolicy() == nil {
		t.Fatal("QuorumPolicy() returned nil")
	}
	if svc.WorkPropagation() == nil {
		t.Fatal("WorkPropagation() returned nil")
	}
	if svc.WorkstationExecution() == nil {
		t.Fatal("WorkstationExecution() returned nil")
	}
	if svc.TTSObservability() == nil {
		t.Fatal("TTSObservability() returned nil")
	}
}

func TestNewService_DecisionEnvelopeContractThroughNestedOwner(t *testing.T) {
	t.Parallel()

	svc, err := invocationpolicywire.NewService(invocationpolicyservice.Dependencies{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	decisionEnvelopes := svc.DecisionEnvelope()
	workstation := &factorydefinitions.FactoryWorkstationConfig{
		OutcomeFormat: factorydefinitions.DecisionEnvelopeOutcomeFormat,
	}
	if !decisionEnvelopes.UsesDecisionEnvelopeOutcome(workstation) {
		t.Fatal("UsesDecisionEnvelopeOutcome() = false, want true for decision-envelope workstation")
	}

	raw := `{"decision":"ACCEPTED","feedback":"Ship it.","output":"done"}`
	result := decisionEnvelopes.WorkResultFromDecisionEnvelopeJSONOrFailed(
		"dispatch-1",
		"transition-1",
		raw,
	)
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("WorkResultFromDecisionEnvelopeJSONOrFailed() outcome = %q, want %q", result.Outcome, workerexecution.OutcomeAccepted)
	}
	if result.Feedback != "Ship it." {
		t.Fatalf("WorkResultFromDecisionEnvelopeJSONOrFailed() feedback = %q, want %q", result.Feedback, "Ship it.")
	}

	malformed := decisionEnvelopes.WorkResultFromDecisionEnvelopeJSONOrFailed(
		"dispatch-incomplete",
		"transition-incomplete",
		"<COMPLETE>",
	)
	if malformed.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("incomplete envelope outcome = %q, want FAILED", malformed.Outcome)
	}
	if malformed.FailureMetadata == nil || malformed.FailureMetadata.Type != workerexecution.WorkFailureTypeUnknown {
		t.Fatalf("incomplete envelope failure metadata = %#v, want terminal unknown", malformed.FailureMetadata)
	}
	if malformed.Diagnostics == nil || malformed.Diagnostics.Provider == nil {
		t.Fatalf("incomplete envelope diagnostics = %#v, want structured completion diagnostics", malformed.Diagnostics)
	}
	metadata := malformed.Diagnostics.Provider.ResponseMetadata
	if metadata[workerexecution.ProviderResponseMetadataFailureOperation] != "completion_validation" ||
		metadata[workerexecution.ProviderResponseMetadataFailureClassification] != "missing_required_output" {
		t.Fatalf("incomplete envelope diagnostics = %#v, want bounded completion-validation facts", metadata)
	}
}

func TestNewService_GoalRoutingDecisionEnvelopeThroughNestedOwner(t *testing.T) {
	t.Parallel()

	svc, err := invocationpolicywire.NewService(invocationpolicyservice.Dependencies{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	decisionEnvelopes := svc.DecisionEnvelope()
	workstation := &factorydefinitions.FactoryWorkstationConfig{
		OutcomeFormat: factorydefinitions.DecisionEnvelopeOutcomeFormat,
		ClassificationRoutes: []factorydefinitions.ClassificationRouteConfig{
			{Label: "accepted", Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "goal", StateName: "complete"}}},
		},
	}
	if !decisionEnvelopes.UsesGoalRoutingDecisionEnvelope(workstation) {
		t.Fatal("UsesGoalRoutingDecisionEnvelope() = false, want true for goal routing workstation")
	}

	acceptedRaw := `{"decision":"accepted","feedback":"Approved.","output":"done"}`
	accepted := decisionEnvelopes.WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed(
		"dispatch-goal",
		"review-goal",
		acceptedRaw,
	)
	if accepted.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("accepted envelope outcome = %q, want %q", accepted.Outcome, workerexecution.OutcomeAccepted)
	}
	if accepted.SelectedClassificationLabel != factorydefinitions.GoalRoutingDecisionAccepted {
		t.Fatalf("accepted envelope label = %q, want %q", accepted.SelectedClassificationLabel, factorydefinitions.GoalRoutingDecisionAccepted)
	}

	needsChangesRaw := `{"decision":"needs-changes","feedback":"Rework required."}`
	needsChanges := decisionEnvelopes.WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed(
		"dispatch-goal",
		"review-goal",
		needsChangesRaw,
	)
	if needsChanges.SelectedClassificationLabel != factorydefinitions.GoalRoutingDecisionNeedsChanges {
		t.Fatalf("needs-changes envelope label = %q, want %q", needsChanges.SelectedClassificationLabel, factorydefinitions.GoalRoutingDecisionNeedsChanges)
	}

	malformed := decisionEnvelopes.WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed(
		"dispatch-goal",
		"review-goal",
		`not-json`,
	)
	if malformed.Outcome != factorydefinitions.MalformedEnvelopeFailureOutcome {
		t.Fatalf("malformed envelope outcome = %q, want %q", malformed.Outcome, factorydefinitions.MalformedEnvelopeFailureOutcome)
	}
	if malformed.Error == "" {
		t.Fatal("malformed envelope error is empty, want actionable failure text")
	}
}

func TestNewService_InvocationInterpolationThroughNestedOwner(t *testing.T) {
	t.Parallel()

	svc, err := invocationpolicywire.NewService(invocationpolicyservice.Dependencies{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	interpolation := svc.InvocationInterpolation()
	workstation, err := interpolation.InterpolateWorkstationConfig(
		factorydefinitions.FactoryWorkstationConfig{
			PromptTemplate: "Use ${input} now",
		},
		&work.InvocationArguments{
			Arguments: map[string]work.InvocationArgument{
				"input": {Values: []string{"draft"}},
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("InterpolateWorkstationConfig: %v", err)
	}
	if workstation.PromptTemplate != "Use draft now" {
		t.Fatalf("PromptTemplate = %q, want interpolated draft", workstation.PromptTemplate)
	}

	_, err = interpolation.InterpolateWorkstationConfig(
		factorydefinitions.FactoryWorkstationConfig{
			PromptTemplate: "Use ${missing} now",
		},
		&work.InvocationArguments{},
		nil,
	)
	if err == nil {
		t.Fatal("InterpolateWorkstationConfig error = nil, want invalid interpolation")
	}
}

func TestNewService_InvocationOutputThroughNestedOwner(t *testing.T) {
	t.Parallel()

	svc, err := invocationpolicywire.NewService(invocationpolicyservice.Dependencies{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	output := svc.InvocationOutput()
	workstation := &factorydefinitions.FactoryWorkstationConfig{
		Name: "execute-goal",
		Type: factorydefinitions.WorkstationTypeModel,
	}
	if !output.ShouldFormatInvocationSummary(workstation) {
		t.Fatal("ShouldFormatInvocationSummary() = false, want true for goal workstation")
	}

	content, err := output.SummaryContentFromWorkerOutput("Final goal summary.\nCOMPLETE", "COMPLETE")
	if err != nil {
		t.Fatalf("SummaryContentFromWorkerOutput: %v", err)
	}
	if len(content) != 1 || content[0].Text != "Final goal summary." {
		t.Fatalf("summary content = %#v, want normalized goal summary text", content)
	}
}

func TestNewService_InvocationWorkTypeThroughNestedOwner(t *testing.T) {
	t.Parallel()

	svc, err := invocationpolicywire.NewService(invocationpolicyservice.Dependencies{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	workType, err := svc.InvocationWorkType().DefaultWorkType(&factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{Name: "task"},
			{Name: "story", HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault}},
		},
	})
	if err != nil {
		t.Fatalf("DefaultWorkType: %v", err)
	}
	if workType != "story" {
		t.Fatalf("DefaultWorkType = %q, want story", workType)
	}
}

func TestNewService_QuorumPolicyThroughNestedOwner(t *testing.T) {
	t.Parallel()

	svc, err := invocationpolicywire.NewService(invocationpolicyservice.Dependencies{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	quorum := svc.QuorumPolicy()
	if !quorum.IsPackagedQuorumFactory(&factorydefinitions.FactoryConfig{
		Name: factorydefinitions.PackagedQuorumFactoryName,
	}) {
		t.Fatal("IsPackagedQuorumFactory() = false, want true for packaged quorum factory")
	}

	relations := quorum.WorkRelations(
		factorydefinitions.PackagedQuorumSplitWorkstationName,
		"task-1",
		"quorum-branch-a",
		nil,
	)
	if len(relations) != 1 || relations[0].Type != work.RelationParentChild || relations[0].TargetWorkID != "task-1" {
		t.Fatalf("split WorkRelations = %#v, want parent-child to task-1", relations)
	}

	branches := []factorydefinitions.QuorumLineageInput{
		{WorkID: "branch-a", WorkTypeID: "quorum-branch-a"},
		{WorkID: "branch-b", WorkTypeID: "quorum-branch-b"},
	}
	mergeRelations := quorum.WorkRelations(
		factorydefinitions.PackagedQuorumMergeWorkstationName,
		"",
		"quorum-merge",
		branches,
	)
	if len(mergeRelations) != 2 {
		t.Fatalf("merge WorkRelations = %#v, want two branch dependencies", mergeRelations)
	}
}

func TestNewService_WorkPropagationThroughNestedOwner(t *testing.T) {
	t.Parallel()

	svc, err := invocationpolicywire.NewService(invocationpolicyservice.Dependencies{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	propagation := svc.WorkPropagation()
	if got := propagation.Mode(nil); got != factorydefinitions.WorkPropagationModeOutputAsPayload {
		t.Fatalf("Mode(nil) = %q, want output_as_payload default", got)
	}

	workstation := &factorydefinitions.FactoryWorkstationConfig{
		WorkPropagation: &factorydefinitions.WorkPropagationConfig{
			Mode: factorydefinitions.WorkPropagationModePreserveInput,
		},
	}
	if got := propagation.Mode(workstation); got != factorydefinitions.WorkPropagationModePreserveInput {
		t.Fatalf("Mode() = %q, want preserve_input", got)
	}
}

func TestNewService_WorkstationExecutionThroughNestedOwner(t *testing.T) {
	t.Parallel()

	svc, err := invocationpolicywire.NewService(invocationpolicyservice.Dependencies{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	execution := svc.WorkstationExecution()
	cfg := &factorydefinitions.FactoryWorkstationConfig{
		Limits: factorydefinitions.WorkstationLimits{MaxExecutionTime: "30s"},
	}
	timeout, err := execution.ExecutionTimeout(cfg)
	if err != nil {
		t.Fatalf("ExecutionTimeout: %v", err)
	}
	if timeout != 30*time.Second {
		t.Fatalf("ExecutionTimeout = %v, want 30s", timeout)
	}

	_, err = execution.ExecutionTimeout(&factorydefinitions.FactoryWorkstationConfig{
		Limits: factorydefinitions.WorkstationLimits{MaxExecutionTime: "not-a-duration"},
	})
	if err == nil {
		t.Fatal("ExecutionTimeout error = nil, want invalid duration parse failure")
	}
}

func TestNewService_TTSObservabilityThroughNestedOwner(t *testing.T) {
	t.Parallel()

	svc, err := invocationpolicywire.NewService(invocationpolicyservice.Dependencies{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	tts := svc.TTSObservability()
	if !tts.IsPackagedTTSFactory(&factorydefinitions.FactoryConfig{
		Name: factorydefinitions.PackagedTTSFactoryName,
	}) {
		t.Fatal("IsPackagedTTSFactory() = false, want true for packaged TTS factory")
	}

	wantLabel := factorydefinitions.DefaultTTSModelName + "/" + factorydefinitions.DefaultTTSBackendName
	if got := tts.TTSBackendRuntimeLabel(); got != wantLabel {
		t.Fatalf("TTSBackendRuntimeLabel() = %q, want %q", got, wantLabel)
	}

	outcome, failure := tts.ClassifyTTSInvocationWait(factorydefinitions.FactoryWorldState{}, "req-1", true)
	if outcome != factorydefinitions.TTSInvocationWaitOutcomeLoading {
		t.Fatalf("ClassifyTTSInvocationWait loading outcome = %q, want loading", outcome)
	}
	if failure != nil {
		t.Fatalf("ClassifyTTSInvocationWait loading failure = %#v, want nil", failure)
	}

	state := packagedTTSFailureWorldState(
		"req-tts",
		"work-tts",
		"model not available: required assets missing in managed cache",
	)
	outcome, failure = tts.ClassifyTTSInvocationWait(state, "req-tts", false)
	if outcome != factorydefinitions.TTSInvocationWaitOutcomeModelNotReady {
		t.Fatalf("ClassifyTTSInvocationWait outcome = %q, want model_not_ready", outcome)
	}
	if failure == nil || failure.ErrorCode != factorydefinitions.TTSInvocationErrorCodeModelNotReady {
		t.Fatalf("ClassifyTTSInvocationWait failure = %#v, want model-not-ready code", failure)
	}

	if !tts.IsTTSModelNotReadyFailure("model not available: required assets missing") {
		t.Fatal("IsTTSModelNotReadyFailure() = false, want true for model-not-ready evidence")
	}
}

func packagedTTSFailureWorldState(requestID, workID, failureMessage string) factorydefinitions.FactoryWorldState {
	submitted := work.FactoryWorkItem{
		ID:         workID,
		WorkTypeID: "task",
		State:      "init",
		TraceID:    requestID,
	}
	failed := submitted
	failed.State = "failed"

	state := factorydefinitions.FactoryWorldState{
		WorkRequestsByID:       make(map[string]factorydefinitions.WorkRequestPayload),
		FailedWorkItemsByID:    make(map[string]work.FactoryWorkItem),
		FailureDetailsByWorkID: make(map[string]factorydefinitions.FactoryWorldFailureDetail),
	}
	state.WorkRequestsByID[requestID] = factorydefinitions.WorkRequestPayload{
		RequestID: requestID,
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		WorkItems: []work.FactoryWorkItem{submitted},
	}
	state.FailedWorkItemsByID[workID] = failed
	state.FailureDetailsByWorkID[workID] = factorydefinitions.FactoryWorldFailureDetail{
		WorkstationName: factorydefinitions.PackagedTTSInvokeWorkstationName,
		WorkItem:        failed,
		FailureDetail:   &workerexecution.FailureDetail{Reason: workerexecution.WorkFailureTypeUnknown, Message: failureMessage},
	}
	return state
}

func TestResolveExecutionCatalog_DetachesInterpolatedPolicy(t *testing.T) {
	t.Parallel()
	resolver := factorylifecycle.New(nil, nil)
	request := detachedExecutionCatalogRequest()

	first := mustResolveExecutionCatalog(t, resolver, request)
	assertDetachedExecutionCatalog(t, first)
	mutateDetachedExecutionCatalog(first)

	second := mustResolveExecutionCatalog(t, resolver, request)
	assertDetachedExecutionCatalog(t, second)
	if reflect.DeepEqual(first, second) {
		t.Fatal("mutating the first detached result unexpectedly left it equal to a fresh result")
	}
}

func TestResolveExecutionCatalog_AppliesDetachedWorkerDefaults(t *testing.T) {
	t.Parallel()
	resolver := factorylifecycle.New(nil, nil)
	request := detachedExecutionCatalogRequest()
	worker := &request.EffectiveDefinition.Workers[0]
	worker.RuntimeDefaultModelProvider = "provider-a"
	worker.RuntimeDefaultModel = "model-a"
	delete(request.Invocation.Arguments.Arguments, "provider")
	delete(request.Invocation.Arguments.Arguments, "model")

	result := mustResolveExecutionCatalog(t, resolver, request)
	resolved := result.Workers["worker"]
	if resolved.ModelProvider != "provider-a" || resolved.Model != "model-a" {
		t.Fatalf("resolved worker defaults = %#v, want provider-a/model-a", resolved)
	}
}

func detachedExecutionCatalogRequest() factorydefinitions.ResolveExecutionCatalogRequest {
	return factorydefinitions.ResolveExecutionCatalogRequest{
		EffectiveDefinition: &factorydefinitions.FactoryConfig{
			Name:   "detached-factory",
			Runner: "codex",
			Version: &factorydefinitions.FactoryVersion{
				Logical:  7,
				Physical: time.Date(2026, 8, 12, 10, 11, 12, 0, time.UTC),
			},
			Workers: []factorydefinitions.FactoryWorkerConfig{{
				ID:              "worker-id",
				Name:            "worker",
				Type:            factorydefinitions.WorkerTypeModel,
				Provider:        "${provider}",
				Model:           "${model}",
				ModelProvider:   "${provider}",
				ReasoningEffort: "${effort}",
				Args:            []string{"--prompt=${prompt}"},
				Timeout:         "${timeout}",
				Body:            "worker body ${prompt}",
				SkipPermissions: true,
				AgentTools:      &factorydefinitions.AgentToolsConfig{Policy: "READ_ONLY"},
				Operations: []factorydefinitions.ModelOperation{{
					Name: "answer",
					Inputs: []factorydefinitions.ModelOperationSlot{{
						Name: "prompt", ContentTypes: []string{"TEXT"}, Required: true,
					}},
				}},
			}},
			Workstations: []factorydefinitions.FactoryWorkstationConfig{{
				ID:               "workstation-id",
				Name:             "run",
				Type:             factorydefinitions.WorkstationTypeModel,
				WorkerTypeName:   "worker",
				Runner:           "${runner}",
				PromptTemplate:   "run ${prompt}",
				Body:             "workstation body ${prompt}",
				WorkingDirectory: "${directory}",
				Worktree:         "${worktree}",
				Env:              map[string]string{"GREETING": "hello ${prompt}"},
				Timeout:          "${timeout}",
				Limits: factorydefinitions.WorkstationLimits{
					MaxExecutionTime: "${timeout}",
				},
				WorkPropagation: &factorydefinitions.WorkPropagationConfig{
					Mode: factorydefinitions.WorkPropagationModePreserveInput,
				},
				OutcomeFormat: factorydefinitions.DecisionEnvelopeOutcomeFormat,
				OperationBindings: []factorydefinitions.ModelOperationBinding{{
					Slot:     "prompt",
					Selector: &factorydefinitions.ModelOperationBindingSelector{Label: "${label}"},
					Config: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText, Text: "config ${prompt}",
					}},
				}},
				Inputs: []factorydefinitions.IOConfig{{
					WorkTypeName: "work", StateName: "ready",
					Guard: &factorydefinitions.InputGuardConfig{MatchInput: "${label}"},
				}},
				ClassificationRoutes: []factorydefinitions.ClassificationRouteConfig{{
					Label:   "accepted",
					Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "work", StateName: "done"}},
				}},
			}},
		},
		Invocation: factorydefinitions.InvocationDefinitionContext{
			Arguments: &work.InvocationArguments{Arguments: map[string]work.InvocationArgument{
				"provider":  {Values: []string{"provider-a"}},
				"model":     {Values: []string{"model-a"}},
				"effort":    {Values: []string{"high"}},
				"prompt":    {Values: []string{"hello"}},
				"timeout":   {Values: []string{"2s"}},
				"runner":    {Values: []string{"codex"}},
				"directory": {Values: []string{"workspace"}},
				"worktree":  {Values: []string{"tree"}},
				"label":     {Values: []string{"input-label"}},
			}},
		},
		References: factorydefinitions.ExecutionCatalogReferenceCatalog{
			Runners:   map[string]struct{}{"codex": {}},
			Providers: map[string]struct{}{"provider-a": {}},
			Models:    map[string]struct{}{"model-a": {}},
		},
	}
}

func mustResolveExecutionCatalog(
	t *testing.T,
	resolver *factorylifecycle.Service,
	request factorydefinitions.ResolveExecutionCatalogRequest,
) factorydefinitions.ResolveExecutionCatalogResult {
	t.Helper()
	result, err := resolver.ResolveExecutionCatalog(context.Background(), request)
	if err != nil {
		t.Fatalf("ResolveExecutionCatalog: %v", err)
	}
	return result
}

func assertDetachedExecutionCatalog(
	t *testing.T,
	result factorydefinitions.ResolveExecutionCatalogResult,
) {
	t.Helper()
	if result.DefinitionVersion != "7@2026-08-12T10:11:12Z" ||
		result.Workers["worker"].Model != "model-a" ||
		result.Workstations["run"].PromptTemplate != "run hello" ||
		result.Workstations["run"].Limits.MaxExecutionTime != "2s" ||
		!result.Workstations["run"].DecisionEnvelope ||
		result.Workstations["run"].WorkPropagation != factorydefinitions.WorkPropagationModePreserveInput {
		t.Fatalf("resolved catalog has unexpected policy: %#v", result)
	}
	if result.Workers["worker"].Args[0] != "--prompt=hello" ||
		result.Workers["worker"].Operations[0].Inputs[0].ContentTypes[0] != "TEXT" ||
		result.Workstations["run"].Environment["GREETING"] != "hello hello" ||
		result.Workstations["run"].OperationBindings[0].Config[0].Text != "config hello" ||
		result.Workstations["run"].Inputs[0].Guard.MatchInput != "input-label" ||
		len(result.Diagnostics) != 0 {
		t.Fatalf("resolved catalog lost detached values: %#v", result)
	}
}

func mutateDetachedExecutionCatalog(result factorydefinitions.ResolveExecutionCatalogResult) {
	result.Workers["worker"].Args[0] = "mutated"
	result.Workers["worker"].Operations[0].Inputs[0].ContentTypes[0] = "MUTATED"
	result.Workstations["run"].Environment["GREETING"] = "mutated"
	result.Workstations["run"].OperationBindings[0].Config[0].Text = "mutated"
	result.Workstations["run"].Inputs[0].Guard.MatchInput = "mutated"
	result.Diagnostics = append(result.Diagnostics, factorydefinitions.ExecutionCatalogDiagnostic{
		Code: factorydefinitions.ExecutionCatalogDiagnosticInvalidDefinition,
	})
}

func TestResolveExecutionCatalog_ReportsTypedDetachedReferenceDiagnostics(t *testing.T) {
	t.Parallel()
	resolver := factorylifecycle.New(nil, nil)

	result, err := resolver.ResolveExecutionCatalog(context.Background(), factorydefinitions.ResolveExecutionCatalogRequest{
		EffectiveDefinition: &factorydefinitions.FactoryConfig{
			Runner: "missing-runner",
			Workers: []factorydefinitions.FactoryWorkerConfig{{
				Name: "worker", Type: factorydefinitions.WorkerTypeModel,
				Provider: "missing-provider", Model: "missing-model",
			}},
			Workstations: []factorydefinitions.FactoryWorkstationConfig{{
				Name: "run", Type: factorydefinitions.WorkstationTypeModel,
				WorkerTypeName: "missing-worker", Runner: "missing-runner",
			}},
		},
		References: factorydefinitions.ExecutionCatalogReferenceCatalog{
			Runners:   map[string]struct{}{"codex": {}},
			Providers: map[string]struct{}{"provider-a": {}},
			Models:    map[string]struct{}{"model-a": {}},
		},
	})
	if err == nil {
		t.Fatal("ResolveExecutionCatalog error = nil, want typed diagnostics")
	}
	var catalogErr *factorydefinitions.ExecutionCatalogError
	if !errors.As(err, &catalogErr) {
		t.Fatalf("error = %T %v, want *ExecutionCatalogError", err, err)
	}
	if len(result.Diagnostics) == 0 || len(result.Diagnostics) != len(catalogErr.Diagnostics) {
		t.Fatalf("result diagnostics = %#v, error diagnostics = %#v", result.Diagnostics, catalogErr.Diagnostics)
	}
	wanted := map[factorydefinitions.ExecutionCatalogDiagnosticCode]bool{
		factorydefinitions.ExecutionCatalogDiagnosticUnknownRunner:   false,
		factorydefinitions.ExecutionCatalogDiagnosticUnknownProvider: false,
		factorydefinitions.ExecutionCatalogDiagnosticUnknownModel:    false,
		factorydefinitions.ExecutionCatalogDiagnosticUnknownWorker:   false,
	}
	for _, diagnostic := range result.Diagnostics {
		if _, ok := wanted[diagnostic.Code]; ok {
			wanted[diagnostic.Code] = true
		}
		if diagnostic.Message == "" || strings.Contains(diagnostic.Message, "missing-provider") {
			// The reference is available as a safe identity field; messages remain
			// stable diagnostic text rather than copied provider payloads.
			if diagnostic.Message == "" {
				t.Fatalf("diagnostic has empty message: %#v", diagnostic)
			}
		}
	}
	for code, found := range wanted {
		if !found {
			t.Fatalf("diagnostic code %q missing from %#v", code, result.Diagnostics)
		}
	}
}
