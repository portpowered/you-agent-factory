package factorysessionexecution

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	workflowvalidation "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/validation"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/pkg/workers/providerexecution"
)

func validateLiveChildExecutorConfig(mode string, provider workers.Provider) error {
	if mode != ChildExecutorModeLive {
		return nil
	}
	if provider == nil {
		return NewValidationError("runtime.childExecutorMode", "provider is required for live-provider child execution")
	}
	return nil
}

func validateLiveChildProviderExecutor(mode string, executor providerexecution.Executor) error {
	if mode == ChildExecutorModeLive && executor == nil {
		return NewValidationError("runtime.childExecutorMode", "provider is required for live-provider child execution")
	}
	return nil
}

// StartPrepareContext supplies filesystem and deployment inputs for durable start
// preparation before runtime execution begins.
type StartPrepareContext struct {
	StartSourceContext
	DeploymentCap   int
	WorkerPresetIDs map[string]struct{}
}

// PreparedStart is the normalized, validated durable start tuple shared by
// runtime-backed async and sync session starts.
type PreparedStart struct {
	Request         StartRequest
	ResolvedSource  ResolvedSource
	Policy          PolicyProjection
	EffectivePolicy workflowpolicy.EffectivePolicy
	SourceRef       string
	SourceContent   string
	TupleHash       string
}

func normalizeStartTuple(req StartRequest) (StartRequest, string, error) {
	normalized, err := NormalizeStartRequest(req)
	if err != nil {
		return StartRequest{}, "", err
	}
	tupleHash, err := IdempotencyTupleHash(normalized)
	if err != nil {
		return StartRequest{}, "", err
	}
	return normalized, tupleHash, nil
}

// PrepareStart normalizes one durable start request, resolves workflow source,
// validates source/args/policy/wait inputs, and projects effective policy before
// runtime execution begins.
func PrepareStart(req StartRequest, ctx StartPrepareContext) (PreparedStart, error) {
	normalized, err := NormalizeStartRequest(req)
	if err != nil {
		return PreparedStart{}, err
	}
	if err := validateStartArgs(normalized.Args); err != nil {
		return PreparedStart{}, err
	}
	if err := validateStartWait(normalized.Wait); err != nil {
		return PreparedStart{}, err
	}

	resolved, resolution, err := resolveStartSourceWithResolution(normalized, ctx.StartSourceContext)
	if err != nil {
		return PreparedStart{}, err
	}
	if err := validateResolvedSourceContent(resolution); err != nil {
		return PreparedStart{}, err
	}
	if err := workflowvalidation.ValidateArgs(resolution.ArgsSchema, normalized.Args); err != nil {
		return PreparedStart{}, NewValidationError("args", err.Error())
	}
	if err := validateNamedAgentPresets(resolution.Agents, ctx.WorkerPresetIDs); err != nil {
		return PreparedStart{}, err
	}

	policyResolution := workflowpolicy.Resolve(workflowpolicy.Request{
		Requested:      normalized.RequestedPolicy,
		FactoryDefault: resolution.DefaultPolicy,
		DeploymentCap:  ctx.DeploymentCap,
	})
	if err := validationErrorFromPolicyIssues(policyResolution.Issues); err != nil {
		return PreparedStart{}, err
	}
	effectiveMap, err := effectivePolicyMap(policyResolution.Policy)
	if err != nil {
		return PreparedStart{}, fmt.Errorf("marshal effective policy: %w", err)
	}

	tupleHash, err := IdempotencyTupleHash(normalized)
	if err != nil {
		return PreparedStart{}, err
	}

	executableSource := strings.TrimSpace(resolution.Content)
	loaded, loadIssues := workflowvalidation.Load(workflowvalidation.LoadRequest{
		SourceRef: resolution.SourceRef,
		Content:   executableSource,
	})
	if len(loadIssues) > 0 {
		return PreparedStart{}, validationErrorFromSourceIssues(loadIssues)
	}

	return PreparedStart{
		Request:        normalized,
		ResolvedSource: resolved,
		Policy: PolicyProjection{
			Requested:     cloneArgs(normalized.RequestedPolicy),
			Effective:     effectiveMap,
			EffectiveHash: policyResolution.Hash,
		},
		EffectivePolicy: policyResolution.Policy,
		SourceRef:       loaded.SourceRef,
		SourceContent:   loaded.ExecutableSource,
		TupleHash:       tupleHash,
	}, nil
}

func validateNamedAgentPresets(agents map[string]interfaces.FactoryOrchestratorJavaScriptAgent, presetIDs map[string]struct{}) error {
	for agentID, agent := range agents {
		presetID := strings.TrimSpace(agent.Preset)
		if _, ok := presetIDs[presetID]; !ok {
			return NewValidationError(
				"orchestrator.javascript.agents."+agentID+".preset",
				fmt.Sprintf("factory agent %q references unknown operator worker preset %q", agentID, presetID),
			)
		}
	}
	return nil
}

func resolveStartSourceWithResolution(req StartRequest, ctx StartSourceContext) (ResolvedSource, workflowsource.Resolution, error) {
	projectRoot := strings.TrimSpace(ctx.ProjectRoot)
	if projectRoot == "" {
		return ResolvedSource{}, workflowsource.Resolution{}, NewValidationError("projectRoot", "projectRoot is required")
	}

	sourceCtx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		return ResolvedSource{}, workflowsource.Resolution{}, NewValidationError("projectRoot", err.Error())
	}

	resolution := workflowsource.Resolve(startSourceRequest(req.Source), sourceCtx)
	applyInlineFactoryDeclaration(&resolution, req.Source)
	if !resolution.Found {
		message := "workflow source could not be resolved"
		if len(resolution.Diagnostics) > 0 && strings.TrimSpace(resolution.Diagnostics[0].Message) != "" {
			message = resolution.Diagnostics[0].Message
		}
		return ResolvedSource{}, workflowsource.Resolution{}, NewValidationError("source", message)
	}

	resolved := ResolvedSource{
		Kind:       resolution.ResolvedKind,
		SourceRef:  resolution.SourceRef,
		SourceHash: resolution.SourceHash,
		Dialect:    resolution.Dialect,
		Metadata: map[string]string{
			"project": sourceCtx.ProjectRoot,
		},
		Agents:        resolution.Agents,
		ArgsSchema:    append(json.RawMessage(nil), resolution.ArgsSchema...),
		DefaultPolicy: append(json.RawMessage(nil), resolution.DefaultPolicy...),
	}
	if stage := resolutionOrderForLookupStage(resolution.LookupStage); stage != "" {
		resolved.ResolutionOrder = []string{stage}
	}
	return resolved, resolution, nil
}

func applyInlineFactoryDeclaration(resolution *workflowsource.Resolution, source Source) {
	if resolution == nil || source.Kind != workflowsource.KindInlineWorkflow || source.InlineWorkflow == nil {
		return
	}
	inline := source.InlineWorkflow
	if sourceRef := strings.TrimSpace(inline.Metadata["sourceRef"]); sourceRef != "" {
		resolution.SourceRef = sourceRef
	}
	resolution.Agents = cloneJavaScriptAgents(inline.Agents)
	resolution.ArgsSchema = append(json.RawMessage(nil), inline.ArgsSchema...)
	resolution.DefaultPolicy = append(json.RawMessage(nil), inline.DefaultPolicy...)
}

func validateResolvedSourceContent(resolution workflowsource.Resolution) error {
	content := strings.TrimSpace(resolution.Content)
	if content == "" {
		return NewValidationError("source", "workflow source content is empty")
	}

	loaded, loadIssues := workflowvalidation.Load(workflowvalidation.LoadRequest{
		SourceRef: resolution.SourceRef,
		Content:   content,
	})
	if len(loadIssues) > 0 {
		return validationErrorFromSourceIssues(loadIssues)
	}

	validationResult := workflowvalidation.Validate(workflowvalidation.Request{
		Source:     wrapWorkflowSourceForValidation(loaded.ExecutableSource),
		SourceRef:  resolution.SourceRef,
		ConfigPath: "orchestrator.javascript",
		Metadata:   map[string]string{"project": resolution.SourceRef},
		ArgsSchema: resolution.ArgsSchema,
	})
	if validationResult.HasIssues() {
		return validationErrorFromSourceIssues(validationResult.Issues)
	}
	return nil
}

func wrapWorkflowSourceForValidation(source string) string {
	return "(function(){\n" + source + "\n})()"
}

func validateStartArgs(args map[string]any) error {
	if len(args) == 0 {
		return nil
	}
	if _, err := canonicalizeMap(args); err != nil {
		return NewValidationError("args", "workflow args must be JSON-compatible")
	}
	if _, err := json.Marshal(args); err != nil {
		return NewValidationError("args", "workflow args must be JSON-compatible")
	}
	return nil
}

func validateStartWait(wait *WaitOptions) error {
	if wait == nil || wait.TimeoutMillis == nil {
		return nil
	}
	if *wait.TimeoutMillis < 1 {
		return NewValidationError("wait.timeoutMillis", "timeoutMillis must be greater than zero when provided")
	}
	return nil
}

func validationErrorFromSourceIssues(issues []workflowvalidation.Issue) error {
	if len(issues) == 0 {
		return NewValidationError("source", "workflow source validation failed")
	}
	issue := issues[0]
	message := strings.TrimSpace(issue.Message)
	if message == "" {
		message = "workflow source validation failed"
	}
	message += issue.LocationSuffix()
	return NewValidationError("source", message)
}

func validationErrorFromPolicyIssues(issues []workflowpolicy.Issue) error {
	if len(issues) == 0 {
		return nil
	}
	issue := issues[0]
	message := strings.TrimSpace(issue.Message)
	if message == "" {
		message = "requested policy is invalid"
	}
	return NewValidationError("requestedPolicy", message)
}

func effectivePolicyMap(policy workflowpolicy.EffectivePolicy) (map[string]any, error) {
	encoded, err := json.Marshal(policy)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(encoded, &out); err != nil {
		return nil, err
	}
	return out, nil
}

const (
	dispatchQueuedEventIDPrefix                   = "factory-event/dispatch-queued"
	dispatchReconciledEventIDPrefix               = "factory-event/dispatch-reconciled"
	dispatchReconciliationSourceProviderSession   = "PROVIDER_SESSION"
	dispatchReconciliationSourceRuntimeReconciler = "RUNTIME_RECONCILER"
)

// RuntimeDispatchEventInput carries durable dispatch projection inputs used to
// synthesize canonical DISPATCH_* events for runtime-backed sessions.
type RuntimeDispatchEventInput struct {
	Dispatches                []DispatchSummary
	DispatchStatusTransitions map[string][]DispatchStatus
	DispatchJavaScript        map[string]DispatchJavaScriptProjection
	Artifacts                 []ArtifactSummary
	CheckpointEvents          []RuntimeCheckpointEventProjection
}

// RuntimeCheckpointEventProjection carries replay-safe checkpoint lineage for one
// ORCHESTRATOR_CHECKPOINT_WRITTEN event.
type RuntimeCheckpointEventProjection struct {
	CheckpointID       string
	Label              string
	Summary            string
	SourceHash         string
	ResumabilityStatus string
	Timestamp          time.Time
}

func runtimeDispatchEventInputFromState(state *runtimeSessionState) RuntimeDispatchEventInput {
	if state == nil {
		return RuntimeDispatchEventInput{}
	}
	return RuntimeDispatchEventInput{
		Dispatches:                state.dispatches,
		DispatchStatusTransitions: state.dispatchStatusTransitions,
		DispatchJavaScript:        state.dispatchJavaScript,
		Artifacts:                 state.artifacts,
		CheckpointEvents:          checkpointEventsFromRuntimeState(state),
	}
}

func checkpointEventsFromRuntimeState(state *runtimeSessionState) []RuntimeCheckpointEventProjection {
	if state == nil {
		return nil
	}
	resumability := "UNKNOWN"
	if state.checkpointSummary != nil {
		resumability = "RESUMABLE"
	}
	sourceHash := strings.TrimSpace(state.session.SourceHash)
	events := make([]RuntimeCheckpointEventProjection, 0)
	for _, record := range state.runtimeRecords {
		if record.Kind != workflowruntime.RecordKindCheckpoint || record.Checkpoint == nil {
			continue
		}
		checkpoint := record.Checkpoint
		projection := RuntimeCheckpointEventProjection{
			CheckpointID:       strings.TrimSpace(checkpoint.ID),
			Label:              strings.TrimSpace(checkpoint.Label),
			Summary:            strings.TrimSpace(checkpoint.Summary),
			SourceHash:         sourceHash,
			ResumabilityStatus: resumability,
		}
		if state.checkpointSummary != nil && !state.checkpointSummary.CreatedAt.IsZero() {
			projection.Timestamp = state.checkpointSummary.CreatedAt.UTC()
		}
		events = append(events, projection)
	}
	return events
}

func appendCanonicalOrchestratorCheckpointEvents(
	events []json.RawMessage,
	session SessionReadResult,
	checkpoints []RuntimeCheckpointEventProjection,
	source string,
) []json.RawMessage {
	if len(checkpoints) == 0 {
		return events
	}
	eventTime := canonicalSessionEventTime(session)
	sessionID := session.SessionID
	orchestratorKind := string(session.OrchestratorKind)
	var orchestratorDialect *string
	if dialect := strings.TrimSpace(session.Dialect); dialect != "" {
		orchestratorDialect = &dialect
	}
	var phaseID *string
	var phaseName *string
	if phase := strings.TrimSpace(session.Phase); phase != "" {
		phaseID = &phase
		phaseName = &phase
	}
	builder := canonicalSessionEventBuilder{
		sessionID:           sessionID,
		orchestratorKind:    orchestratorKind,
		orchestratorDialect: orchestratorDialect,
		phaseID:             phaseID,
		phaseName:           phaseName,
		source:              source,
		eventTime:           eventTime,
	}
	checkpointEvents := make([]json.RawMessage, 0, len(checkpoints))
	for index, checkpoint := range checkpoints {
		checkpointID := strings.TrimSpace(checkpoint.CheckpointID)
		if checkpointID == "" {
			continue
		}
		payload := map[string]any{
			"label":              checkpoint.Label,
			"resumabilityStatus": checkpoint.ResumabilityStatus,
		}
		if summary := strings.TrimSpace(checkpoint.Summary); summary != "" {
			payload["summary"] = summary
		}
		if sourceHash := strings.TrimSpace(checkpoint.SourceHash); sourceHash != "" {
			payload["sourceHash"] = sourceHash
		}
		timestamp := checkpoint.Timestamp
		if timestamp.IsZero() {
			timestamp = eventTime.Add(time.Duration(index+1) * time.Second)
		}
		payload["timestamp"] = timestamp.UTC().Format(time.RFC3339)
		sequence := nextCanonicalSessionEventSequence(events) + len(checkpointEvents)
		id := fmt.Sprintf("orchestrator-checkpoint-written/%s/%s", sessionID, checkpointID)
		checkpointEvents = append(checkpointEvents, builder.eventWithCheckpoint(
			"ORCHESTRATOR_CHECKPOINT_WRITTEN",
			id,
			sequence,
			&checkpointID,
			mustMarshalPayload(payload),
		))
	}
	if len(checkpointEvents) == 0 {
		return events
	}
	return insertEventsBeforeSessionCompleted(events, checkpointEvents)
}

func rebuildRuntimeSessionCanonicalEvents(state *runtimeSessionState) []json.RawMessage {
	if state == nil {
		return nil
	}
	preserved := extractDispatchInterruptedEvents(state.events)
	projected := BuildCanonicalRuntimeSessionEvents(
		state.session,
		state.result,
		runtimeDispatchEventInputFromState(state),
	)
	return mergePreservedDispatchInterruptedEvents(projected, preserved)
}

type dispatchQueuedEventPayload struct {
	DispatchKind    string `json:"dispatchKind"`
	Label           string `json:"label,omitempty"`
	RunnerID        string `json:"runnerId,omitempty"`
	PresetID        string `json:"presetId,omitempty"`
	ModelProvider   string `json:"modelProvider,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	Provider        string `json:"provider,omitempty"`
	QueuePosition   *int   `json:"queuePosition,omitempty"`
}

type dispatchReconciledEventPayload struct {
	ReconciledStatus     string                       `json:"reconciledStatus"`
	ReconciliationSource string                       `json:"reconciliationSource"`
	Replayed             bool                         `json:"replayed"`
	ProviderSessionRefs  []ProviderSessionRef         `json:"providerSessionRefs,omitempty"`
	ArtifactIDs          []string                     `json:"artifactIds,omitempty"`
	FailureDetail        *dispatchFailureEventPayload `json:"failureDetail,omitempty"`
}

type dispatchFailureEventPayload struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

func appendCanonicalRuntimeDispatchLifecycleEvents(
	events []json.RawMessage,
	session SessionReadResult,
	input RuntimeDispatchEventInput,
	source string,
) []json.RawMessage {
	if len(input.Dispatches) == 0 {
		return events
	}
	dispatchEvents := make([]json.RawMessage, 0, len(input.Dispatches)*2)
	for index, dispatch := range input.Dispatches {
		if strings.TrimSpace(dispatch.ID) == "" {
			continue
		}
		if dispatch.Status == DispatchStatusInterrupted {
			continue
		}
		dispatchEvents = buildDispatchQueuedEvent(events, dispatchEvents, session, dispatch, source, index)
		if isReconciledDispatchStatus(dispatch.Status) {
			dispatchEvents = buildDispatchReconciledEvent(events, dispatchEvents, session, dispatch, source)
		}
	}
	if len(dispatchEvents) == 0 {
		return events
	}
	return insertEventsBeforeSessionCompleted(events, dispatchEvents)
}

func buildDispatchQueuedEvent(
	baseEvents []json.RawMessage,
	pending []json.RawMessage,
	session SessionReadResult,
	dispatch DispatchSummary,
	source string,
	queueIndex int,
) []json.RawMessage {
	dispatchKind := strings.TrimSpace(dispatch.DispatchKind)
	if dispatchKind == "" {
		dispatchKind = "JAVASCRIPT_AGENT"
	}
	payload := dispatchQueuedEventPayload{DispatchKind: dispatchKind}
	if label := strings.TrimSpace(dispatch.Label); label != "" {
		payload.Label = label
	}
	if runnerID := strings.TrimSpace(dispatch.RunnerID); runnerID != "" {
		payload.RunnerID = runnerID
	}
	if presetID := strings.TrimSpace(dispatch.PresetID); presetID != "" {
		payload.PresetID = presetID
	}
	if modelProvider := strings.TrimSpace(dispatch.ModelProvider); modelProvider != "" {
		payload.ModelProvider = modelProvider
	}
	if model := strings.TrimSpace(dispatch.Model); model != "" {
		payload.Model = model
	}
	if reasoningEffort := strings.TrimSpace(dispatch.ReasoningEffort); reasoningEffort != "" {
		payload.ReasoningEffort = reasoningEffort
	}
	if provider := strings.TrimSpace(dispatch.Provider); provider != "" {
		payload.Provider = provider
	}
	position := queueIndex
	payload.QueuePosition = &position

	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return pending
	}
	return append(pending, dispatchLifecycleEvent(
		baseEvents,
		pending,
		"DISPATCH_QUEUED",
		fmt.Sprintf("%s/%s", dispatchQueuedEventIDPrefix, dispatch.ID),
		session,
		dispatch,
		source,
		encodedPayload,
	))
}

func buildDispatchReconciledEvent(
	baseEvents []json.RawMessage,
	pending []json.RawMessage,
	session SessionReadResult,
	dispatch DispatchSummary,
	source string,
) []json.RawMessage {
	payload := dispatchReconciledEventPayload{
		ReconciledStatus:     string(dispatch.Status),
		ReconciliationSource: dispatchReconciliationSource(dispatch),
		Replayed:             false,
	}
	if len(dispatch.ProviderSessionRefs) > 0 {
		payload.ProviderSessionRefs = cloneProviderSessionRefs(dispatch.ProviderSessionRefs)
	}
	if len(dispatch.OutputArtifactIDs) > 0 {
		payload.ArtifactIDs = append([]string(nil), dispatch.OutputArtifactIDs...)
	}
	if dispatch.FailureDetail != nil {
		payload.FailureDetail = &dispatchFailureEventPayload{
			Reason:  strings.TrimSpace(dispatch.FailureDetail.Reason),
			Message: strings.TrimSpace(dispatch.FailureDetail.Message),
		}
	}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return pending
	}
	return append(pending, dispatchLifecycleEvent(
		baseEvents,
		pending,
		"DISPATCH_RECONCILED",
		fmt.Sprintf("%s/%s", dispatchReconciledEventIDPrefix, dispatch.ID),
		session,
		dispatch,
		source,
		encodedPayload,
	))
}

func dispatchReconciliationSource(dispatch DispatchSummary) string {
	if len(dispatch.ProviderSessionRefs) > 0 {
		return dispatchReconciliationSourceProviderSession
	}
	return dispatchReconciliationSourceRuntimeReconciler
}

func isReconciledDispatchStatus(status DispatchStatus) bool {
	switch status {
	case DispatchStatusCompleted,
		DispatchStatusFailed,
		DispatchStatusCanceled,
		DispatchStatusTimedOut,
		DispatchStatusSkipped:
		return true
	default:
		return false
	}
}

func dispatchLifecycleEvent(
	baseEvents []json.RawMessage,
	pending []json.RawMessage,
	eventType, id string,
	session SessionReadResult,
	dispatch DispatchSummary,
	source string,
	payload json.RawMessage,
) json.RawMessage {
	priorEvents := make([]json.RawMessage, 0, len(baseEvents)+len(pending))
	priorEvents = append(priorEvents, baseEvents...)
	priorEvents = append(priorEvents, pending...)
	sequence, sessionSequence := nextCanonicalEventSequence(priorEvents)
	eventTime := canonicalSessionEventTime(session).Add(time.Duration(sessionSequence) * time.Second)

	sessionID := session.SessionID
	orchestratorKind := strings.ToUpper(strings.TrimSpace(session.OrchestratorKind))
	var orchestratorDialect *string
	if dialect := strings.TrimSpace(session.Dialect); dialect != "" {
		orchestratorDialect = &dialect
	}
	var phaseID *string
	var phaseName *string
	if phase := strings.TrimSpace(dispatch.Phase); phase != "" {
		phaseID = &phase
		phaseName = &phase
	} else if phase := strings.TrimSpace(session.Phase); phase != "" {
		phaseID = &phase
		phaseName = &phase
	}
	dispatchID := dispatch.ID

	context := canonicalFactoryEventContext{
		Sequence:        sequence,
		Tick:            sequence,
		EventTime:       eventTime,
		SessionID:       &sessionID,
		SessionSequence: intPtr(sessionSequence),
		Source:          &source,
		DispatchID:      &dispatchID,
	}
	if orchestratorKind != "" {
		context.OrchestratorKind = &orchestratorKind
	}
	if orchestratorDialect != nil {
		context.OrchestratorDialect = orchestratorDialect
	}
	if phaseID != nil {
		context.PhaseID = phaseID
	}
	if phaseName != nil {
		context.PhaseName = phaseName
	}

	encoded, err := json.Marshal(canonicalFactoryEvent{
		SchemaVersion: canonicalFactoryEventSchemaVersion,
		ID:            id,
		Type:          eventType,
		Context:       context,
		Payload:       payload,
	})
	if err != nil {
		return json.RawMessage("{}")
	}
	return encoded
}

func insertEventsBeforeSessionCompleted(events, insertion []json.RawMessage) []json.RawMessage {
	if len(insertion) == 0 {
		return events
	}
	completedIndex := len(events)
	for index, raw := range events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}
		if strings.TrimSpace(envelope.Type) == "SESSION_COMPLETED" {
			completedIndex = index
			break
		}
	}
	merged := make([]json.RawMessage, 0, len(events)+len(insertion))
	merged = append(merged, events[:completedIndex]...)
	merged = append(merged, insertion...)
	merged = append(merged, events[completedIndex:]...)
	return resequenceCanonicalEvents(merged)
}

func resequenceCanonicalEvents(events []json.RawMessage) []json.RawMessage {
	for index, raw := range events {
		var event canonicalFactoryEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			continue
		}
		event.Context.Sequence = index + 1
		event.Context.Tick = index + 1
		event.Context.SessionSequence = intPtr(index)
		if encoded, err := json.Marshal(event); err == nil {
			events[index] = encoded
		}
	}
	return events
}
