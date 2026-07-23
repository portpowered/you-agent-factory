package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	internalcontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/contracts"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	recording "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	"sort"
	"strings"
	"time"
)

func reconcileAppendOnlyCanonicalEvents(previous, projected []json.RawMessage) []json.RawMessage {
	if len(previous) == 0 {
		return resequenceCanonicalEvents(cloneRawMessages(projected))
	}
	result := cloneRawMessages(previous)
	seen := make(map[string]struct{}, len(result))
	for _, raw := range result {
		if id := canonicalEventID(raw); id != "" {
			seen[id] = struct{}{}
		}
	}
	for _, raw := range projected {
		id := canonicalEventID(raw)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, resequenceCanonicalEvent(raw, len(result)))
	}
	return result
}

func canonicalEventID(raw json.RawMessage) string {
	var event struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &event) != nil {
		return ""
	}
	return strings.TrimSpace(event.ID)
}

func resequenceCanonicalEvent(raw json.RawMessage, index int) json.RawMessage {
	var event canonicalFactoryEvent
	if json.Unmarshal(raw, &event) != nil {
		return append(json.RawMessage(nil), raw...)
	}
	event.Context.Sequence = index + 1
	event.Context.Tick = index + 1
	event.Context.SessionSequence = intPtr(index)
	encoded, err := json.Marshal(event)
	if err != nil {
		return append(json.RawMessage(nil), raw...)
	}
	return encoded
}

func cloneRawMessages(events []json.RawMessage) []json.RawMessage {
	cloned := make([]json.RawMessage, len(events))
	for index, event := range events {
		cloned[index] = append(json.RawMessage(nil), event...)
	}
	return cloned
}

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
	QueryDispatches(ctx context.Context, request DispatchQueryRequest) (ListDispatchesResult, error)
	GetDispatch(ctx context.Context, sessionID, dispatchID string) (DispatchDetail, error)
	ListArtifacts(ctx context.Context, sessionID string) (ListArtifactsResult, error)
	GetArtifact(ctx context.Context, sessionID, artifactID string) (ArtifactDetail, error)
	ReadEvents(ctx context.Context, sessionID string, req EventReconnectRequest) (EventReadResult, error)
	ListSessions(ctx context.Context, req ListSessionsRequest) (ListSessionsResult, error)
}

// SyncWaitScheduler owns the blocking primitive used while a synchronous
// durable start waits for terminal session state. Wire supplies the production
// scheduler; the execution service never selects an ambient timer fallback.
type SyncWaitScheduler interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
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
	projected := mergePreservedDispatchInterruptedEvents(
		events,
		extractDispatchInterruptedEvents(candidate.events),
	)
	candidate.events = projected
	if target.eventConsumer != nil {
		candidate.events = reconcileAppendOnlyCanonicalEvents(target.events, projected)
	}
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
	outcome factory.JavaScriptRuntimeOutcome,
	startedAt time.Time,
) runtimeSessionState {
	candidate := cloneRuntimeSessionState(state)
	s.applyTerminalRuntimeState(&candidate, terminal, outcome, startedAt)
	candidate.runtimeRecords = mergeRuntimeRecords(state.runtimeRecords, outcome.Records)
	if candidate.session.Status == LifecycleStatusInterrupted {
		candidate.checkpointSummary = latestCheckpointSummaryFromRuntime(
			s.checkpointSummaries,
			candidate.session.SessionID,
			&candidate,
			candidate.runtimeRecords,
		)
	}
	return candidate
}

func (s *JavaScriptRuntimeService) publishAsyncTerminalCandidate(
	state *runtimeSessionState,
	candidate runtimeSessionState,
	normalized StartRequest,
	resolved ResolvedSource,
	policyResolution factory.JavaScriptPolicyResolution,
	startedAt time.Time,
) {
	sessionID := state.session.SessionID
	if err := s.recordCanonicalTerminalState(state, candidate); err != nil {
		failureOutcome := factory.JavaScriptRuntimeOutcome{Failure: factory.JavaScriptRuntimeFailure{
			Code:    factory.JavaScriptRuntimeCodeScriptError,
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
	s.presentCurrentFactoryEvents(sessionID)
}

func (s *JavaScriptRuntimeService) applyTerminalRuntimeState(
	state *runtimeSessionState,
	terminal runtimeSessionState,
	outcome factory.JavaScriptRuntimeOutcome,
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

func startSourceRequest(source Source) factory.WorkflowSourceRequest {
	switch source.Kind {
	case factory.WorkflowSourceKindFactoryID:
		return factory.WorkflowSourceRequest{
			Kind:  source.Kind,
			Value: source.FactoryID,
		}
	case factory.WorkflowSourceKindFactoryInline:
		return factory.WorkflowSourceRequest{
			Kind:  source.Kind,
			Value: string(source.FactoryInline),
		}
	case factory.WorkflowSourceKindWorkflowFile:
		return factory.WorkflowSourceRequest{
			Kind:  source.Kind,
			Value: source.WorkflowFile,
		}
	case factory.WorkflowSourceKindWorkflowName:
		return factory.WorkflowSourceRequest{
			Kind:  source.Kind,
			Value: source.WorkflowName,
		}
	case factory.WorkflowSourceKindInlineWorkflow:
		inline := source.InlineWorkflow
		if inline == nil {
			return factory.WorkflowSourceRequest{Kind: source.Kind}
		}
		return factory.WorkflowSourceRequest{
			Kind:         source.Kind,
			Value:        inline.InlineSource,
			InlineSource: inline.InlineSource,
		}
	default:
		return factory.WorkflowSourceRequest{Kind: source.Kind}
	}
}

func resolutionOrderForLookupStage(stage factory.WorkflowSourceLookupStage) string {
	switch stage {
	case factory.WorkflowSourceLookupStageProjectClaude, factory.WorkflowSourceLookupStageExplicitSourceKind:
		return "PROJECT_CLAUDE_WORKFLOWS"
	case factory.WorkflowSourceLookupStageGlobalUser:
		return "USER_YOU_AGENT_FACTORY_WORKFLOWS"
	case factory.WorkflowSourceLookupStagePackageRelative:
		return "PACKAGE_RELATIVE_WORKFLOW_DIRECTORIES"
	case factory.WorkflowSourceLookupStageNamedJavaScript:
		return "BUILTIN_GLOBAL_JAVASCRIPT_FACTORIES"
	case factory.WorkflowSourceLookupStageExplicitFactory:
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
func PersistenceChoiceForPolicy(
	policy PersistencePolicy,
	projectRoot string,
	stores func(string) (runtimepersist.Store, error),
) (PersistenceChoice, error) {
	switch policy {
	case "", PersistencePolicyEnabled:
		return ProjectPersistence(projectRoot, stores)
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

// DisabledPersistence explicitly selects in-memory-only session execution.
func DisabledPersistence() PersistenceChoice {
	return PersistenceChoice{disabled: true}
}

// ProjectPersistence initializes the established project-local snapshot store.
func ProjectPersistence(
	projectRoot string,
	stores func(string) (runtimepersist.Store, error),
) (PersistenceChoice, error) {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		return PersistenceChoice{}, NewValidationError("persistence.projectRoot", "project root is required for persistence")
	}
	if stores == nil {
		return PersistenceChoice{}, NewValidationError("persistence", "runtime persistence store factory is required")
	}
	store, err := stores(root)
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

// NewJavaScriptExecutionService validates and constructs durable JavaScript
// execution from explicit collaborators.
func NewJavaScriptExecutionService(
	projectRoot string,
	childExecutorMode string,
	executor workers.InvocationExecutor,
	persistenceChoice PersistenceChoice,
	clock factory.Clock,
	syncWaits SyncWaitScheduler,
	checkpointSummaries factory.JavaScriptCheckpointSummaries,
	workflowDefinitions factory.JavaScriptWorkflowDefinitions,
	workflowRuntime factory.JavaScriptWorkflowRuntime,
	childValues factory.JavaScriptChildValues,
	workerPresetIDs map[string]struct{},
	workerSettings factory.JavaScriptWorkerSettings,
	recordingWriter recording.PortableRecordingWriter,
	generateSessionID internalcontracts.SessionIDGenerator,
) (Service, error) {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return nil, NewValidationError("projectRoot", "projectRoot is required")
	}
	if clock == nil {
		return nil, NewValidationError("clock", "clock is required")
	}
	if syncWaits == nil {
		return nil, NewValidationError("syncWaits", "sync wait scheduler is required")
	}
	if checkpointSummaries == nil {
		return nil, NewValidationError("checkpointSummaries", "Factory Runtime checkpoint summaries are required")
	}
	if workflowDefinitions == nil {
		return nil, NewValidationError("workflowDefinitions", "Factory Runtime JavaScript workflow definitions are required")
	}
	if workflowRuntime == nil {
		return nil, NewValidationError("workflowRuntime", "Factory Runtime JavaScript workflow runtime is required")
	}
	if childValues == nil {
		return nil, NewValidationError("childValues", "Factory Runtime JavaScript child values are required")
	}
	childExecutorMode = normalizeChildExecutorMode(childExecutorMode)
	if childExecutorMode == ChildExecutorModeLive && executor == nil {
		return nil, NewValidationError("runtime.childExecutorMode", "worker invocation executor is required for live child execution")
	}
	if recordingWriter == nil {
		return nil, NewValidationError("recordingWriter", "portable recording writer is required")
	}
	if generateSessionID == nil {
		return nil, NewValidationError("generateSessionID", "Factory Session ID generator is required")
	}
	persistence, err := persistenceChoice.resolve()
	if err != nil {
		return nil, err
	}
	return NewJavaScriptRuntimeService(
		projectRoot, childExecutorMode, executor, persistence, clock, syncWaits,
		checkpointSummaries,
		workflowDefinitions, workflowRuntime, childValues,
		workerPresetIDs, workerSettings, recordingWriter,
		generateSessionID,
	), nil
}

// SmokeLiveChildProvider returns a deterministic mock provider for CLI and
// fixture-backed live-provider child smoke without MCP host startup. Scope for
// this provider is the completed CLI live-dispatch smoke lane; MCP live serve and
// website inspection remain deferred follow-up cells.
func SmokeLiveChildProvider() workerprovider.Provider {
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
	records []factory.JavaScriptRuntimeRecord,
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
		case factory.JavaScriptRecordKindPhase:
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
		case factory.JavaScriptRecordKindArtifact:
			if record.Artifact == nil {
				continue
			}
			summary := artifactSummaryFromRuntimeRecord(sessionID, *record.Artifact, observedAt)
			artifactByID[summary.ID] = summary
		case factory.JavaScriptRecordKindChildDispatch:
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

func phaseSummariesFromRuntimeRecords(records []factory.JavaScriptRuntimeRecord, dispatches []DispatchSummary) []PhaseSummary {
	ordered := make([]PhaseSummary, 0)
	indexByPhase := make(map[string]int)
	for _, record := range records {
		if record.Kind != factory.JavaScriptRecordKindPhase || record.Phase == nil {
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
	record factory.JavaScriptArtifactRecord,
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
	child factory.JavaScriptChildDispatchRecord,
	observedAt time.Time,
) (ArtifactSummary, bool) {
	if strings.TrimSpace(child.Status) != factory.JavaScriptChildDispatchStatusCompleted {
		return ArtifactSummary{}, false
	}
	parsed, issues := factory.ParseArtifactURI(strings.TrimSpace(child.ArtifactRef))
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

func dispatchSummaryFromChildRecord(currentPhase string, child factory.JavaScriptChildDispatchRecord) DispatchSummary {
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
		if provider == "" && strings.TrimSpace(child.ExecutionMode) == factory.JavaScriptChildExecutionModeFake {
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

func dispatchFailureDetailFromChildRecord(child factory.JavaScriptChildDispatchRecord) *DispatchFailureDetail {
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

func dispatchJavaScriptFromChildRecord(child factory.JavaScriptChildDispatchRecord) DispatchJavaScriptProjection {
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

func cloneRuntimeRecords(records []factory.JavaScriptRuntimeRecord) []factory.JavaScriptRuntimeRecord {
	if len(records) == 0 {
		return nil
	}
	cloned := make([]factory.JavaScriptRuntimeRecord, len(records))
	for i, record := range records {
		cloned[i] = cloneRuntimeRecord(record)
	}
	return cloned
}

func cloneRuntimeRecord(record factory.JavaScriptRuntimeRecord) factory.JavaScriptRuntimeRecord {
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
	parsed, issues := factory.ParseArtifactURI(strings.TrimSpace(raw))
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
