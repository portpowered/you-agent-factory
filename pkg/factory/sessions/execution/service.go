package factorysessionexecution

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/runtimepersist"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"github.com/portpowered/infinite-you/pkg/workers/providerexecution"
)

// ErrServiceNotConfigured reports an application composition graph that did
// not supply its required durable Factory Session execution collaborator.
var ErrServiceNotConfigured = errors.New("durable factory session execution service is not configured")

// Service is the shared durable factory-session execution contract consumed by
// API, CLI, MCP, and UI transports. Live-session open and invocation remain on
// the separate factorysessions compatibility surface. All methods are
// cancellation-aware; transports must not mutate runtime state directly.
type Service interface {
	StartAsync(ctx context.Context, req StartRequest) (AsyncStartResult, error)
	StartSync(ctx context.Context, req StartRequest) (SyncStartResult, error)
	ResumeInterruptedSession(ctx context.Context, sessionID string, req ResumeSessionRequest) (AsyncStartResult, error)
	GetSession(ctx context.Context, sessionID string) (SessionReadResult, error)
	Pause(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error)
	Resume(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error)
	Cancel(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error)
	Terminate(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error)
	Approve(ctx context.Context, sessionID string, req ApproveRequest) (LifecycleControlResult, error)
	RetryDispatch(ctx context.Context, sessionID string, req RetryDispatchRequest) (LifecycleControlResult, error)
	InterruptDispatch(ctx context.Context, sessionID string, req InterruptDispatchRequest) (LifecycleControlResult, error)
	GetResult(ctx context.Context, sessionID string, req ResultRequest) (ResultReadResult, error)
	ListDispatches(ctx context.Context, sessionID string) (ListDispatchesResult, error)
	GetDispatch(ctx context.Context, sessionID, dispatchID string) (DispatchDetail, error)
	ListArtifacts(ctx context.Context, sessionID string) (ListArtifactsResult, error)
	GetArtifact(ctx context.Context, sessionID, artifactID string) (ArtifactDetail, error)
	ReadEvents(ctx context.Context, sessionID string, req EventReconnectRequest) (EventReadResult, error)
	ListSessions(ctx context.Context, req ListSessionsRequest) (ListSessionsResult, error)
}

// recordCanonicalTerminalState is the sole publication boundary for terminal
// JavaScript session facts. It validates and persists the complete canonical
// event projection before making that projection visible to live readers.
// The caller must hold s.mu.
func (s *JavaScriptRuntimeService) recordCanonicalTerminalState(target *runtimeSessionState, candidate runtimeSessionState) error {
	events, err := MapCanonicalRuntimeSessionEvents(
		candidate.session,
		candidate.result,
		runtimeDispatchEventInputFromState(&candidate),
	)
	if err != nil {
		return err
	}
	candidate.events = mergePreservedDispatchInterruptedEvents(
		events,
		extractDispatchInterruptedEvents(candidate.events),
	)
	if err := s.persistTerminalSessionState(candidate); err != nil {
		return err
	}
	applyRuntimeSessionFields(target, candidate)
	target.runtimeRecords = cloneRuntimeRecords(candidate.runtimeRecords)
	target.checkpointSummary = cloneCheckpointSummary(candidate.checkpointSummary)
	target.startRequest = cloneStartRequestPtr(candidate.startRequest)
	target.resolvedSource = candidate.resolvedSource
	target.sourceContent = candidate.sourceContent
	target.runCancel = candidate.runCancel
	return nil
}

func (s *JavaScriptRuntimeService) buildTerminalRuntimeCandidate(
	state *runtimeSessionState,
	terminal runtimeSessionState,
	outcome workflowruntime.Outcome,
	startedAt time.Time,
) runtimeSessionState {
	candidate := cloneRuntimeSessionState(state)
	s.applyTerminalRuntimeState(&candidate, terminal, outcome, startedAt)
	candidate.runtimeRecords = mergeRuntimeRecords(state.runtimeRecords, outcome.Records)
	if candidate.session.Status == LifecycleStatusInterrupted {
		candidate.checkpointSummary = latestCheckpointSummaryFromRuntime(candidate.session.SessionID, &candidate, candidate.runtimeRecords)
	}
	return candidate
}

func (s *JavaScriptRuntimeService) publishAsyncTerminalCandidate(
	state *runtimeSessionState,
	candidate runtimeSessionState,
	normalized StartRequest,
	resolved ResolvedSource,
	policyResolution workflowpolicy.Resolution,
	startedAt time.Time,
) {
	if err := s.recordCanonicalTerminalState(state, candidate); err != nil {
		failureOutcome := workflowruntime.Outcome{Failure: workflowruntime.Failure{
			Code:    workflowruntime.CodeScriptError,
			Message: err.Error(),
		}}
		failed := projectRuntimeSessionState(
			state.session.SessionID,
			normalized,
			resolved,
			policyResolution,
			failureOutcome,
			startedAt,
		)
		failed.startRequest = cloneStartRequestPtr(state.startRequest)
		failed.resolvedSource = state.resolvedSource
		failed.sourceContent = state.sourceContent
		failed.runCancel = state.runCancel
		*state = failed
	}
	s.mu.Unlock()
}

func (s *JavaScriptRuntimeService) applyTerminalRuntimeState(
	state *runtimeSessionState,
	terminal runtimeSessionState,
	outcome workflowruntime.Outcome,
	startedAt time.Time,
) {
	finishedAt := s.now()
	applyTerminalRuntimeProjection(state, terminal, outcome)
	if state.session.Lifecycle == nil {
		state.session.Lifecycle = &LifecycleTimestamps{}
	}
	if state.session.Lifecycle.StartedAt == nil {
		state.session.Lifecycle.StartedAt = &startedAt
	}
	state.session.Lifecycle.FinishedAt = &finishedAt
	state.result.SessionStatus = state.session.Status
}

// InspectionLinksForSession builds API-relative inspection links for one durable session.
func InspectionLinksForSession(sessionID string, includeEvents bool) InspectionLinks {
	base := fmt.Sprintf("/factory-sessions/%s", sessionID)
	links := InspectionLinks{
		Session:    base,
		Status:     base,
		Results:    base + "/results",
		Dispatches: base + "/dispatches",
		Artifacts:  base + "/artifacts",
	}
	if includeEvents {
		links.Events = base + "/events"
	}
	return links
}

// LifecycleControlLinksForSession builds post-control inspection links for one durable session.
func LifecycleControlLinksForSession(sessionID string, includeEvents bool) LifecycleControlLinks {
	inspection := InspectionLinksForSession(sessionID, includeEvents)
	return LifecycleControlLinks{
		Session:    inspection.Session,
		Status:     inspection.Status,
		Results:    inspection.Results,
		Dispatches: inspection.Dispatches,
		Artifacts:  inspection.Artifacts,
		Events:     inspection.Events,
	}
}

// LiveLifecycleControlLinksForSession builds post-control inspection links for
// one live workspace Factory Session.
func LiveLifecycleControlLinksForSession(sessionID string) LifecycleControlLinks {
	base := fmt.Sprintf("/factory-sessions/%s", strings.TrimSpace(sessionID))
	return LifecycleControlLinks{
		Session: base,
		Status:  base,
		Results: base + "/result",
		Events:  base + "/events",
	}
}

// StartSourceContext supplies filesystem roots for durable start source resolution.
type StartSourceContext struct {
	ProjectRoot string
}

func startSourceRequest(source Source) workflowsource.Request {
	switch source.Kind {
	case workflowsource.KindFactoryID:
		return workflowsource.Request{
			Kind:  source.Kind,
			Value: source.FactoryID,
		}
	case workflowsource.KindFactoryInline:
		return workflowsource.Request{
			Kind:  source.Kind,
			Value: string(source.FactoryInline),
		}
	case workflowsource.KindWorkflowFile:
		return workflowsource.Request{
			Kind:  source.Kind,
			Value: source.WorkflowFile,
		}
	case workflowsource.KindWorkflowName:
		return workflowsource.Request{
			Kind:  source.Kind,
			Value: source.WorkflowName,
		}
	case workflowsource.KindInlineWorkflow:
		inline := source.InlineWorkflow
		if inline == nil {
			return workflowsource.Request{Kind: source.Kind}
		}
		return workflowsource.Request{
			Kind:         source.Kind,
			Value:        inline.InlineSource,
			InlineSource: inline.InlineSource,
		}
	default:
		return workflowsource.Request{Kind: source.Kind}
	}
}

func resolutionOrderForLookupStage(stage workflowsource.LookupStage) string {
	switch stage {
	case workflowsource.LookupStageProjectClaude, workflowsource.LookupStageExplicitSourceKind:
		return "PROJECT_CLAUDE_WORKFLOWS"
	case workflowsource.LookupStageGlobalUser:
		return "USER_YOU_AGENT_FACTORY_WORKFLOWS"
	case workflowsource.LookupStagePackageRelative:
		return "PACKAGE_RELATIVE_WORKFLOW_DIRECTORIES"
	case workflowsource.LookupStageNamedJavaScript:
		return "BUILTIN_GLOBAL_JAVASCRIPT_FACTORIES"
	case workflowsource.LookupStageExplicitFactory:
		return "EXPLICIT_FACTORY_LOOKUP"
	default:
		return ""
	}
}

// ExecutionProvider selects which durable Factory Session execution backend serves
// start and inspection calls at the shared service boundary.
type ExecutionProvider string

const (
	// ExecutionProviderFake selects the deterministic in-memory fake session path.
	ExecutionProviderFake ExecutionProvider = "fake"
	// ExecutionProviderJavaScriptRuntime selects the real simple JavaScript runtime path.
	ExecutionProviderJavaScriptRuntime ExecutionProvider = "javascript-runtime"
)

// ServiceConfig carries dependencies required by production execution providers.
type ServiceConfig struct {
	ProjectRoot       string
	ChildExecutorMode string
	Provider          workers.Provider
	ProviderExecutor  providerexecution.Executor
	FakeOptions       []FakeServiceOption
	Persistence       PersistenceChoice
	Clock             factory.Clock
	WorkerPresetIDs   map[string]struct{}
	WorkerSettings    workflowruntime.WorkerSettingsConfig
}

// PersistenceChoice makes durable snapshot ownership explicit at composition.
// Construct it with EnabledPersistence or DisabledPersistence.
type PersistenceChoice struct {
	store    runtimepersist.Store
	disabled bool
}

// PersistencePolicy is the application-level durable snapshot policy. The
// zero value preserves production persistence; callers must select Disabled
// explicitly when durable snapshots are not wanted.
type PersistencePolicy string

const (
	PersistencePolicyEnabled  PersistencePolicy = "enabled"
	PersistencePolicyDisabled PersistencePolicy = "disabled"
)

// PersistenceChoiceForPolicy resolves application policy into the closed
// persistence choice consumed by durable execution composition.
func PersistenceChoiceForPolicy(policy PersistencePolicy, projectRoot string) (PersistenceChoice, error) {
	switch policy {
	case "", PersistencePolicyEnabled:
		return ProjectPersistence(projectRoot)
	case PersistencePolicyDisabled:
		return DisabledPersistence(), nil
	default:
		return PersistenceChoice{}, NewValidationError(
			"persistence.policy",
			fmt.Sprintf("unsupported durable session persistence policy %q", policy),
		)
	}
}

// EnabledPersistence selects durable snapshots through the injected store.
func EnabledPersistence(store runtimepersist.Store) PersistenceChoice {
	return PersistenceChoice{store: store}
}

// Store returns the enabled persistence collaborator. Disabled persistence has
// no store and returns nil.
func (choice PersistenceChoice) Store() runtimepersist.Store {
	return choice.store
}

// Validate reports whether composition explicitly selected enabled or disabled
// persistence without performing snapshot IO.
func (choice PersistenceChoice) Validate() error {
	_, err := choice.resolve()
	return err
}

// DisabledPersistence explicitly selects in-memory-only session execution.
func DisabledPersistence() PersistenceChoice {
	return PersistenceChoice{disabled: true}
}

// ProjectPersistence initializes the established project-local snapshot store.
func ProjectPersistence(projectRoot string) (PersistenceChoice, error) {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		return PersistenceChoice{}, NewValidationError("persistence.projectRoot", "project root is required for persistence")
	}
	store, err := runtimepersist.NewProjectStore(root)
	if err != nil {
		return PersistenceChoice{}, NewValidationError("persistence", "initialize durable session persistence: "+err.Error())
	}
	return EnabledPersistence(store), nil
}

func (choice PersistenceChoice) resolve() (runtimepersist.Store, error) {
	switch {
	case choice.disabled && choice.store != nil:
		return nil, NewValidationError("persistence", "persistence cannot be both enabled and disabled")
	case choice.disabled:
		return nil, nil
	case choice.store == nil:
		return nil, NewValidationError("persistence", "persistence must be explicitly enabled with a store or disabled")
	default:
		return choice.store, nil
	}
}

// NewExecutionService constructs one shared Factory Session execution service for
// the requested provider.
func NewExecutionService(provider ExecutionProvider, config ServiceConfig) (Service, error) {
	switch provider {
	case ExecutionProviderFake:
		return NewFakeService(config.FakeOptions...), nil
	case ExecutionProviderJavaScriptRuntime:
		projectRoot := strings.TrimSpace(config.ProjectRoot)
		if projectRoot == "" {
			return nil, NewValidationError("projectRoot", "projectRoot is required")
		}
		if config.Clock == nil {
			return nil, NewValidationError("clock", "clock is required")
		}
		childExecutorMode := normalizeChildExecutorMode(config.ChildExecutorMode)
		executor := config.ProviderExecutor
		if executor == nil && config.Provider != nil {
			executor = providerexecution.NewProviderExecutor(config.Provider)
		}
		if childExecutorMode == ChildExecutorModeLive && executor == nil {
			return nil, NewValidationError("runtime.childExecutorMode", "provider is required for live-provider child execution")
		}
		if err := validateLiveChildExecutorConfig(childExecutorMode, config.Provider); err != nil && executor == nil {
			return nil, err
		}
		persistence, err := config.Persistence.resolve()
		if err != nil {
			return nil, err
		}
		return NewJavaScriptRuntimeService(JavaScriptRuntimeServiceConfig{
			ProjectRoot:       projectRoot,
			ChildExecutorMode: childExecutorMode,
			Provider:          config.Provider,
			ProviderExecutor:  executor,
			Persistence:       persistence,
			Clock:             config.Clock,
			WorkerPresetIDs:   config.WorkerPresetIDs,
			WorkerSettings:    config.WorkerSettings,
		}), nil
	default:
		return nil, NewValidationError("provider", "unsupported execution provider")
	}
}

// SmokeLiveChildProvider returns a deterministic mock provider for CLI and
// fixture-backed live-provider child smoke without MCP host startup. Scope for
// this provider is the completed CLI live-dispatch smoke lane; MCP live serve and
// website inspection remain deferred follow-up cells.
func SmokeLiveChildProvider() workers.Provider {
	return smokeLiveChildProvider{}
}

type smokeLiveChildProvider struct{}

func (smokeLiveChildProvider) Infer(_ context.Context, _ workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	return workerexecution.InferenceResponse{
		Content: `{"text":"live:agent-run-fake-child:summarize-findings:summarize workflows:workflows"}`,
		ProviderSession: &workerexecution.ProviderSessionMetadata{
			Provider: "mock",
			Kind:     "session_id",
			ID:       "live-provider-session-1",
		},
	}, nil
}

// RuntimeExecutionProjection carries durable dispatch, artifact, phase, and progress
// state projected from ordered workflow runtime records.
type RuntimeExecutionProjection struct {
	Phase                     string
	PhaseCount                int
	PhaseSummaries            []PhaseSummary
	Dispatches                []DispatchSummary
	DispatchJavaScript        map[string]DispatchJavaScriptProjection
	DispatchStatusTransitions map[string][]DispatchStatus
	Artifacts                 []ArtifactSummary
	Progress                  ProgressCounts
}

// ProjectRuntimeExecutionRecords maps ordered runtime host-effect records into
// durable session dispatch, artifact, phase, and progress projections.
func ProjectRuntimeExecutionRecords(
	sessionID string,
	records []workflowruntime.RuntimeRecord,
	observedAt time.Time,
) RuntimeExecutionProjection {
	projection := RuntimeExecutionProjection{}
	if len(records) == 0 {
		return projection
	}

	currentPhase := ""
	dispatchOrder := make([]string, 0)
	dispatchByID := make(map[string]DispatchSummary)
	dispatchJavaScript := make(map[string]DispatchJavaScriptProjection)
	dispatchStatusTransitions := make(map[string][]DispatchStatus)
	artifactByID := make(map[string]ArtifactSummary)

	for _, record := range records {
		switch record.Kind {
		case workflowruntime.RecordKindPhase:
			if record.Phase == nil {
				continue
			}
			name := strings.TrimSpace(record.Phase.Name)
			if name == "" {
				continue
			}
			currentPhase = name
			projection.PhaseCount++
			projection.Phase = name
		case workflowruntime.RecordKindArtifact:
			if record.Artifact == nil {
				continue
			}
			summary := artifactSummaryFromRuntimeRecord(sessionID, *record.Artifact, observedAt)
			artifactByID[summary.ID] = summary
		case workflowruntime.RecordKindChildDispatch:
			if record.ChildDispatch == nil {
				continue
			}
			summary := dispatchSummaryFromChildRecord(currentPhase, *record.ChildDispatch)
			dispatchByID[summary.ID] = summary
			dispatchJavaScript[summary.ID] = dispatchJavaScriptFromChildRecord(*record.ChildDispatch)
			appendDispatchStatusTransition(dispatchStatusTransitions, summary.ID, summary.Status)
			if _, seen := indexOfString(dispatchOrder, summary.ID); !seen {
				dispatchOrder = append(dispatchOrder, summary.ID)
			}
			if artifact, ok := childArtifactFromDispatch(sessionID, *record.ChildDispatch, observedAt); ok {
				artifactByID[artifact.ID] = artifact
			}
		}
	}

	projection.Dispatches = make([]DispatchSummary, 0, len(dispatchOrder))
	for _, dispatchID := range dispatchOrder {
		projection.Dispatches = append(projection.Dispatches, dispatchByID[dispatchID])
	}
	projection.DispatchJavaScript = dispatchJavaScript
	projection.DispatchStatusTransitions = dispatchStatusTransitions
	projection.Artifacts = orderedArtifactSummaries(artifactByID)
	projection.Progress = progressCountsFromDispatches(projection.Dispatches, projection.PhaseCount)
	projection.PhaseSummaries = phaseSummariesFromRuntimeRecords(records, projection.Dispatches)
	return projection
}

func phaseSummariesFromRuntimeRecords(records []workflowruntime.RuntimeRecord, dispatches []DispatchSummary) []PhaseSummary {
	ordered := make([]PhaseSummary, 0)
	indexByPhase := make(map[string]int)
	for _, record := range records {
		if record.Kind != workflowruntime.RecordKindPhase || record.Phase == nil {
			continue
		}
		phase := strings.TrimSpace(record.Phase.Name)
		if phase == "" {
			continue
		}
		if _, exists := indexByPhase[phase]; !exists {
			indexByPhase[phase] = len(ordered)
			ordered = append(ordered, PhaseSummary{Phase: phase})
		}
	}
	for _, dispatch := range dispatches {
		phase := strings.TrimSpace(dispatch.Phase)
		index, exists := indexByPhase[phase]
		if !exists || phase == "" {
			continue
		}
		ordered[index].DispatchCount++
		switch dispatch.Status {
		case DispatchStatusCompleted:
			ordered[index].CompletedDispatchCount++
		case DispatchStatusFailed, DispatchStatusCanceled, DispatchStatusTimedOut, DispatchStatusInterrupted:
			ordered[index].FailedDispatchCount++
		}
	}
	return ordered
}

func appendDispatchStatusTransition(
	transitions map[string][]DispatchStatus,
	dispatchID string,
	status DispatchStatus,
) {
	if dispatchID == "" || status == "" {
		return
	}
	existing := transitions[dispatchID]
	if len(existing) > 0 && existing[len(existing)-1] == status {
		return
	}
	transitions[dispatchID] = append(existing, status)
}

func artifactSummaryFromRuntimeRecord(
	sessionID string,
	record workflowruntime.ArtifactRecord,
	observedAt time.Time,
) ArtifactSummary {
	return ArtifactSummary{
		ID:           record.ID,
		Kind:         record.Kind,
		Visibility:   record.Visibility,
		Label:        record.Label,
		ContentHash:  record.ContentHash,
		SizeBytes:    record.SizeBytes,
		CreatedAt:    timePtr(observedAt.UTC()),
		RetrievalRef: artifactRetrievalRefForSession(sessionID, record.ID),
	}
}

func childArtifactFromDispatch(
	sessionID string,
	child workflowruntime.ChildDispatchRecord,
	observedAt time.Time,
) (ArtifactSummary, bool) {
	if strings.TrimSpace(child.Status) != workflowruntime.ChildDispatchStatusCompleted {
		return ArtifactSummary{}, false
	}
	parsed, issues := workflowresult.ParseArtifactURI(strings.TrimSpace(child.ArtifactRef))
	if len(issues) > 0 || parsed.ArtifactID == "" {
		return ArtifactSummary{}, false
	}
	return ArtifactSummary{
		ID:           parsed.ArtifactID,
		Kind:         "CHILD_RESULT",
		Visibility:   "WORKFLOW_RUNTIME",
		Label:        child.Label,
		DispatchID:   child.DispatchID,
		CreatedAt:    timePtr(observedAt.UTC()),
		RetrievalRef: artifactRetrievalRefForSession(sessionID, parsed.ArtifactID),
	}, true
}

func dispatchSummaryFromChildRecord(currentPhase string, child workflowruntime.ChildDispatchRecord) DispatchSummary {
	summary := DispatchSummary{
		ID:                    child.DispatchID,
		Status:                DispatchStatus(strings.TrimSpace(child.Status)),
		DispatchKind:          "JAVASCRIPT_AGENT",
		Phase:                 currentPhase,
		Label:                 child.Label,
		Attempt:               positiveDispatchAttempt(child.Attempt),
		Retryable:             cloneBoolPtr(child.Retryable),
		FailureClassification: strings.TrimSpace(string(child.FailureClassification)),
		RunnerID:              strings.TrimSpace(child.RunnerID),
		PresetID:              strings.TrimSpace(child.Preset),
		ModelProvider:         strings.TrimSpace(child.ModelProvider),
		Model:                 strings.TrimSpace(child.Model),
		ReasoningEffort:       strings.TrimSpace(child.ReasoningEffort),
	}
	if javascript := dispatchJavaScriptFromChildRecord(child); strings.TrimSpace(javascript.TaskKind) != "" {
		summary.JavaScript = &javascript
	}
	if ref := strings.TrimSpace(child.ProviderSessionRef); ref != "" {
		provider := strings.TrimSpace(child.Provider)
		if provider == "" && strings.TrimSpace(child.ExecutionMode) == workflowruntime.ChildExecutionModeFake {
			provider = "fake"
		}
		summary.Provider = provider
		summary.ProviderSessionRefs = []ProviderSessionRef{{
			Provider: provider,
			Kind:     "session_id",
			ID:       ref,
		}}
	}
	if artifactID := artifactIDFromRef(child.ArtifactRef); artifactID != "" &&
		summary.Status == DispatchStatusCompleted {
		summary.OutputArtifactIDs = []string{artifactID}
	}
	if summary.Status == DispatchStatusFailed {
		summary.FailureDetail = dispatchFailureDetailFromChildRecord(child)
	}
	return summary
}

func cloneBoolPtr(value *bool) *bool {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func positiveDispatchAttempt(attempt int) int {
	if attempt > 0 {
		return attempt
	}
	return 1
}

func dispatchFailureDetailFromChildRecord(child workflowruntime.ChildDispatchRecord) *DispatchFailureDetail {
	if child.FailureDetail == nil {
		return nil
	}
	reason := strings.TrimSpace(string(child.FailureDetail.Reason))
	message := strings.TrimSpace(child.FailureDetail.Message)
	if reason == "" || message == "" {
		return nil
	}
	return &DispatchFailureDetail{Reason: reason, Message: message}
}

func dispatchJavaScriptFromChildRecord(child workflowruntime.ChildDispatchRecord) DispatchJavaScriptProjection {
	return DispatchJavaScriptProjection{
		TaskKind:      "AGENT",
		TaskLabel:     child.Label,
		ExecutionMode: strings.TrimSpace(child.ExecutionMode),
	}
}

func cloneDispatchJavaScriptProjections(
	source map[string]DispatchJavaScriptProjection,
) map[string]DispatchJavaScriptProjection {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]DispatchJavaScriptProjection, len(source))
	for id, projection := range source {
		cloned[id] = projection
	}
	return cloned
}

func cloneRuntimeRecords(records []workflowruntime.RuntimeRecord) []workflowruntime.RuntimeRecord {
	if len(records) == 0 {
		return nil
	}
	cloned := make([]workflowruntime.RuntimeRecord, len(records))
	for i, record := range records {
		cloned[i] = cloneRuntimeRecord(record)
	}
	return cloned
}

func cloneRuntimeRecord(record workflowruntime.RuntimeRecord) workflowruntime.RuntimeRecord {
	cloned := record
	if record.Phase != nil {
		phase := *record.Phase
		cloned.Phase = &phase
	}
	if record.Artifact != nil {
		artifact := *record.Artifact
		cloned.Artifact = &artifact
	}
	if record.Log != nil {
		logRecord := *record.Log
		logRecord.Fields = cloneArgs(record.Log.Fields)
		cloned.Log = &logRecord
	}
	if record.Checkpoint != nil {
		checkpoint := *record.Checkpoint
		checkpoint.State = cloneArgs(record.Checkpoint.State)
		cloned.Checkpoint = &checkpoint
	}
	if record.Budget != nil {
		budget := *record.Budget
		cloned.Budget = &budget
	}
	if record.ChildDispatch != nil {
		child := *record.ChildDispatch
		child.Output = cloneArgs(record.ChildDispatch.Output)
		if record.ChildDispatch.FailureDetail != nil {
			failure := *record.ChildDispatch.FailureDetail
			child.FailureDetail = &failure
		}
		if record.ChildDispatch.Retryable != nil {
			retryable := *record.ChildDispatch.Retryable
			child.Retryable = &retryable
		}
		cloned.ChildDispatch = &child
	}
	return cloned
}

func cloneDispatchStatusTransitions(
	source map[string][]DispatchStatus,
) map[string][]DispatchStatus {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string][]DispatchStatus, len(source))
	for id, transitions := range source {
		cloned[id] = append([]DispatchStatus(nil), transitions...)
	}
	return cloned
}

func artifactIDFromRef(raw string) string {
	parsed, issues := workflowresult.ParseArtifactURI(strings.TrimSpace(raw))
	if len(issues) > 0 {
		return ""
	}
	return parsed.ArtifactID
}

func artifactRetrievalRefForSession(sessionID, artifactID string) *ArtifactRetrievalRef {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(artifactID) == "" {
		return nil
	}
	return &ArtifactRetrievalRef{
		Href:   fmt.Sprintf("/factory-sessions/%s/artifacts/%s", sessionID, artifactID),
		Method: "GET",
	}
}

func orderedArtifactSummaries(artifactByID map[string]ArtifactSummary) []ArtifactSummary {
	if len(artifactByID) == 0 {
		return nil
	}
	ids := make([]string, 0, len(artifactByID))
	for id := range artifactByID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	artifacts := make([]ArtifactSummary, 0, len(ids))
	for _, id := range ids {
		artifacts = append(artifacts, artifactByID[id])
	}
	return artifacts
}

func progressCountsFromDispatches(dispatches []DispatchSummary, phaseCount int) ProgressCounts {
	progress := ProgressCounts{PhaseCount: phaseCount}
	for _, dispatch := range dispatches {
		progress.TotalDispatches++
		switch dispatch.Status {
		case DispatchStatusCompleted:
			progress.CompletedDispatches++
		case DispatchStatusFailed:
			progress.FailedDispatches++
		case DispatchStatusQueued:
			progress.QueuedDispatches++
			progress.InFlightDispatches++
		case DispatchStatusRunning:
			progress.RunningDispatches++
			progress.InFlightDispatches++
		case DispatchStatusCanceled:
			progress.CanceledDispatches++
		case DispatchStatusTimedOut:
			progress.TimedOutDispatches++
		case DispatchStatusSkipped:
			progress.SkippedDispatches++
		case DispatchStatusInterrupted:
			progress.InterruptedDispatches++
		}
	}
	return progress
}

func indexOfString(values []string, target string) (int, bool) {
	for index, value := range values {
		if value == target {
			return index, true
		}
	}
	return -1, false
}

// SeedRuntimeSessionWithRunningDispatch seeds one in-memory JavaScript runtime session
// with a running child dispatch for interrupt-dispatch integration tests.
func SeedRuntimeSessionWithRunningDispatch(
	service *JavaScriptRuntimeService,
	sessionID, dispatchID, label string,
) error {
	if service == nil {
		return NewValidationError("service", "service is required")
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return err
	}
	dispatchID = strings.TrimSpace(dispatchID)
	if dispatchID == "" {
		return NewValidationError("dispatchId", "dispatchId is required")
	}

	now := time.Now().UTC()
	session := SessionReadResult{
		SessionID:        id,
		Status:           LifecycleStatusRunning,
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		Phase:            "execute",
		Lifecycle:        &LifecycleTimestamps{StartedAt: &now},
		Links:            InspectionLinksForSession(id, true),
		Progress: &ProgressCounts{
			TotalDispatches:    1,
			InFlightDispatches: 1,
		},
	}
	result := ResultReadResult{
		SessionID:     id,
		SessionStatus: LifecycleStatusRunning,
		ResultStatus:  ResultStatusNotReady,
		Availability: &ResultAvailabilityDetail{
			Reason:    "RESULT_NOT_READY",
			Message:   "Session is still running.",
			Retryable: true,
		},
	}
	dispatches := []DispatchSummary{{
		ID:     dispatchID,
		Status: DispatchStatusRunning,
		Phase:  "execute",
		Label:  label,
	}}
	dispatchStatusTransitions := map[string][]DispatchStatus{
		dispatchID: {DispatchStatusQueued, DispatchStatusRunning},
	}
	state := &runtimeSessionState{
		session:                   session,
		result:                    result,
		dispatches:                dispatches,
		dispatchStatusTransitions: dispatchStatusTransitions,
	}
	state.events = rebuildRuntimeSessionCanonicalEvents(state)

	service.mu.Lock()
	defer service.mu.Unlock()
	service.sessions[id] = state
	return nil
}

// ApplyRuntimeTerminalOutcomeForTests merges one terminal workflow outcome into a
// seeded JavaScript runtime session for interrupt-dispatch race integration tests.
func ApplyRuntimeTerminalOutcomeForTests(
	service *JavaScriptRuntimeService,
	sessionID string,
	outcome workflowruntime.Outcome,
) error {
	if service == nil {
		return NewValidationError("service", "service is required")
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	state, ok := service.sessions[id]
	if !ok {
		return ErrSessionNotFound
	}

	finishedAt := time.Now().UTC()
	terminal := runtimeSessionState{
		session: cloneSessionRead(state.session),
		result:  cloneResultRead(state.result),
	}
	if outcome.OK {
		applyRuntimeSuccessProjection(&terminal, id, outcome, finishedAt)
	} else if len(outcome.Records) > 0 {
		applyRuntimeExecutionRecordProjection(&terminal, id, outcome.Records, finishedAt)
		projectRuntimeFailure(&terminal.session, &terminal.result, outcome)
	}
	applyTerminalRuntimeProjection(state, terminal, outcome)
	return nil
}
