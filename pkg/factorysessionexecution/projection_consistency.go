package factorysessionexecution

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	"github.com/portpowered/infinite-you/pkg/workcontent"
)

// SessionProjectionEventKinds lists canonical event types that project durable
// session lifecycle, result, dispatch, artifact, phase, checkpoint, and budget state.
var SessionProjectionEventKinds = []string{
	"SESSION_STARTED",
	"SESSION_PAUSED",
	"SESSION_RESUMED",
	"SESSION_RESULT_UPDATED",
	"SESSION_LIFECYCLE_CONTROL",
	"SESSION_COMPLETED",
	"ORCHESTRATOR_PHASE_CHANGED",
	"ORCHESTRATOR_CHECKPOINT_WRITTEN",
	"DISPATCH_QUEUED",
	"DISPATCH_INTERRUPTED",
	"DISPATCH_RECONCILED",
	"ARTIFACT_CREATED",
	"JAVASCRIPT_PHASE_CHANGE",
	"JAVASCRIPT_CHECKPOINT_REF",
}

type sessionResultUpdatedProjection struct {
	ResultStatus string `json:"resultStatus"`
}

type factoryEventEnvelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// ValidateResultMatchesSessionRead checks that one result read aligns with the
// session read summary when both are present.
func ValidateResultMatchesSessionRead(session SessionReadResult, result ResultReadResult) error {
	if strings.TrimSpace(session.SessionID) == "" || strings.TrimSpace(result.SessionID) == "" {
		return nil
	}
	if session.SessionID != result.SessionID {
		return fmt.Errorf("result sessionId %q does not match session read %q", result.SessionID, session.SessionID)
	}
	if session.ResultSummary != nil {
		summaryStatus := strings.TrimSpace(session.ResultSummary.ResultStatus)
		if summaryStatus != "" && summaryStatus != string(result.ResultStatus) {
			return fmt.Errorf("result status %q does not match session resultSummary %q", result.ResultStatus, summaryStatus)
		}
	}
	if session.Status != "" && result.SessionStatus != "" && session.Status != result.SessionStatus {
		return fmt.Errorf("result sessionStatus %q does not match session read %q", result.SessionStatus, session.Status)
	}
	return nil
}

// ValidateDispatchListMatchesSessionProgress checks dispatch list counts against
// one session read progress summary when present.
func ValidateDispatchListMatchesSessionProgress(session SessionReadResult, dispatches []DispatchSummary) error {
	if session.Progress == nil {
		return nil
	}
	total := len(dispatches)
	if session.Progress.TotalDispatches > 0 && total > session.Progress.TotalDispatches {
		return fmt.Errorf("dispatch list length %d exceeds session progress total %d", total, session.Progress.TotalDispatches)
	}
	return nil
}

// LatestResultStatusFromEvents returns the resultStatus from the latest
// SESSION_RESULT_UPDATED event when present.
func LatestResultStatusFromEvents(events []json.RawMessage) (ResultStatus, bool) {
	var latest ResultStatus
	found := false
	for _, raw := range events {
		var envelope factoryEventEnvelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}
		if strings.TrimSpace(envelope.Type) != "SESSION_RESULT_UPDATED" {
			continue
		}
		var payload sessionResultUpdatedProjection
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			continue
		}
		status := strings.TrimSpace(payload.ResultStatus)
		if status == "" {
			continue
		}
		latest = ResultStatus(status)
		found = true
	}
	return latest, found
}

// ValidateResultMatchesEventProjection checks that one result read matches the
// latest SESSION_RESULT_UPDATED event projection when events are present.
func ValidateResultMatchesEventProjection(result ResultReadResult, events []json.RawMessage) error {
	eventStatus, ok := LatestResultStatusFromEvents(events)
	if !ok {
		return nil
	}
	if eventStatus != result.ResultStatus {
		return fmt.Errorf("result status %q does not match latest SESSION_RESULT_UPDATED event %q", result.ResultStatus, eventStatus)
	}
	return nil
}

// ResultStatus is the customer-visible durable session result availability.
type ResultStatus string

const (
	ResultStatusNotReady          ResultStatus = "NOT_READY"
	ResultStatusPartial           ResultStatus = "PARTIAL"
	ResultStatusFinal             ResultStatus = "FINAL"
	ResultStatusFailedWithPartial ResultStatus = "FAILED_WITH_PARTIAL"
	ResultStatusUnavailable       ResultStatus = "UNAVAILABLE"
)

// ResultMode selects final or partial durable result retrieval.
type ResultMode string

const (
	ResultModeFinal   ResultMode = "final"
	ResultModePartial ResultMode = "partial"
)

// ResultRequest normalizes durable session result read parameters.
type ResultRequest struct {
	Mode             ResultMode
	IncludeArtifacts bool
}

// ResultAvailabilityDetail explains why a durable result is not ready or unavailable.
type ResultAvailabilityDetail struct {
	Reason    string
	Message   string
	Retryable bool
}

// ResultReadResult is the shared durable session result projection consumed by API,
// CLI, MCP, and UI transports.
type ResultReadResult struct {
	SessionID        string
	ResultStatus       ResultStatus
	SessionStatus    LifecycleStatus
	Mode             ResultMode
	IncludeArtifacts bool
	PrimaryResult    json.RawMessage
	ArtifactIDs      []string
	ArtifactRefs     []ArtifactRefSummary
	Failure          *FailureSummary
	Availability     *ResultAvailabilityDetail
}

// DispatchStatus is the canonical dispatch lifecycle status shared across orchestrators.
type DispatchStatus string

const (
	DispatchStatusQueued    DispatchStatus = "QUEUED"
	DispatchStatusRunning   DispatchStatus = "RUNNING"
	DispatchStatusCompleted DispatchStatus = "COMPLETED"
	DispatchStatusFailed    DispatchStatus = "FAILED"
	DispatchStatusCanceled  DispatchStatus = "CANCELED"
	DispatchStatusTimedOut  DispatchStatus = "TIMED_OUT"
	DispatchStatusSkipped   DispatchStatus = "SKIPPED"
)

// DispatchUsage summarizes one dispatch execution.
type DispatchUsage struct {
	InputTokens    int64
	OutputTokens   int64
	TotalTokens    int64
	DurationMillis int64
	CostUSD        float64
	RetryCount     int32
}

// DispatchWarning exposes one customer-visible dispatch warning.
type DispatchWarning struct {
	Code    string
	Message string
}

// DispatchFailureDetail exposes one customer-visible dispatch failure.
type DispatchFailureDetail struct {
	Reason     string
	Message    string
	ErrorClass string
}

// DispatchPetriProjection carries Petri-specific dispatch metadata.
type DispatchPetriProjection struct {
	TransitionID    string
	WorkstationName string
	WorkerType      string
}

// DispatchJavaScriptProjection carries JavaScript-specific dispatch metadata.
type DispatchJavaScriptProjection struct {
	TaskKind      string
	TaskLabel     string
	ExecutionMode string
}

// ProviderSessionRef correlates one dispatch with a provider session identity.
type ProviderSessionRef struct {
	Provider string
	Kind     string
	ID       string
}

// DispatchSummary is the shared durable dispatch list projection.
type DispatchSummary struct {
	ID                  string
	Status              DispatchStatus
	DispatchKind        string
	Phase               string
	Label               string
	Attempt             int
	RunnerID            string
	Model               string
	Provider            string
	ProviderSessionRefs []ProviderSessionRef
	OutputArtifactIDs   []string
	Usage               *DispatchUsage
	Warnings            []DispatchWarning
	FailureDetail       *DispatchFailureDetail
}

// DispatchDetail is the shared durable dispatch read projection.
type DispatchDetail struct {
	DispatchSummary
	SessionID          string
	OrchestratorKind   string
	ArtifactIDs        []string
	StatusTransitions  []DispatchStatus
	Petri              *DispatchPetriProjection
	JavaScript         *DispatchJavaScriptProjection
}

// ListDispatchesResult is the shared durable dispatch list outcome.
type ListDispatchesResult struct {
	SessionID  string
	Dispatches []DispatchSummary
}

// ArtifactRetrievalRef is a safe API-relative artifact retrieval reference.
type ArtifactRetrievalRef struct {
	Href   string
	Method string
}

// ArtifactRedactionCounts summarizes secret suppression for one artifact.
type ArtifactRedactionCounts struct {
	Paths   int32
	Secrets int32
	Tokens  int32
}

// ArtifactSummary is the shared durable artifact list projection.
type ArtifactSummary struct {
	ID              string
	Kind            string
	Visibility      string
	Label           string
	ContentHash     string
	SizeBytes       int64
	CreatedAt       *time.Time
	DispatchID      string
	AuditMode       string
	RedactionCounts *ArtifactRedactionCounts
	RetrievalRef    *ArtifactRetrievalRef
}

// ArtifactDetail is the shared durable artifact read projection.
type ArtifactDetail struct {
	ArtifactSummary
	SessionID       string
	Summary         string
	CaptureMetadata map[string]any
	Content         json.RawMessage
	ContentRef      *ArtifactRetrievalRef
}

// ListArtifactsResult is the shared durable artifact list outcome.
type ListArtifactsResult struct {
	SessionID string
	Artifacts []ArtifactSummary
}

// EventReconnectRequest identifies the last acknowledged durable session event.
type EventReconnectRequest struct {
	AfterEventID  string
	AfterSequence *int
}

// EventReadResult carries replayed canonical session events for one durable session.
type EventReadResult struct {
	SessionID string
	Events    []json.RawMessage
}
// NormalizeResultRequest validates and normalizes one durable result read request.
func NormalizeResultRequest(req ResultRequest) (ResultRequest, error) {
	mode := req.Mode
	if mode == "" {
		mode = ResultModeFinal
	}
	switch mode {
	case ResultModeFinal, ResultModePartial:
	default:
		return ResultRequest{}, NewValidationError("mode", "mode must be final or partial")
	}
	return ResultRequest{
		Mode:             mode,
		IncludeArtifacts: req.IncludeArtifacts,
	}, nil
}

// NormalizeEventReconnectRequest validates one durable session event reconnect request.
func NormalizeEventReconnectRequest(req EventReconnectRequest) (EventReconnectRequest, error) {
	normalized := EventReconnectRequest{
		AfterEventID: strings.TrimSpace(req.AfterEventID),
	}
	if req.AfterSequence != nil {
		sequence := *req.AfterSequence
		if sequence < 0 {
			return EventReconnectRequest{}, NewValidationError("afterSequence", "afterSequence must be non-negative")
		}
		normalized.AfterSequence = &sequence
	}
	return normalized, nil
}

type sessionProjectionReducer struct {
	session            SessionReadResult
	result             ResultReadResult
	resultAvailability *ResultAvailabilityDetail
}

// ReplaySessionProjection reconstructs durable session and result read projections
// from canonical session lifecycle events. Replaying the same event sequence more
// than once yields the same projection without duplicating summaries or stubs.
func ReplaySessionProjection(events []json.RawMessage) (SessionReadResult, ResultReadResult, error) {
	reducer := sessionProjectionReducer{}
	for index, raw := range events {
		if err := reducer.apply(raw); err != nil {
			return SessionReadResult{}, ResultReadResult{}, fmt.Errorf("apply event %d: %w", index, err)
		}
	}
	reducer.finalize()
	return reducer.session, reducer.result, nil
}

func (r *sessionProjectionReducer) apply(raw json.RawMessage) error {
	var envelope canonicalFactoryEvent
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("unmarshal event envelope: %w", err)
	}
	switch strings.TrimSpace(envelope.Type) {
	case "SESSION_STARTED":
		return r.applySessionStarted(envelope)
	case "SESSION_PAUSED":
		return r.applySessionPaused(envelope)
	case "SESSION_RESUMED":
		return r.applySessionResumed(envelope)
	case "SESSION_RESULT_UPDATED":
		return r.applySessionResultUpdated(envelope)
	case "SESSION_LIFECYCLE_CONTROL":
		return r.applySessionLifecycleControl(envelope)
	case "SESSION_COMPLETED":
		return r.applySessionCompleted(envelope)
	default:
		return nil
	}
}

func (r *sessionProjectionReducer) applySessionStarted(envelope canonicalFactoryEvent) error {
	var payload struct {
		SourceRef  *string `json:"sourceRef"`
		SourceHash *string `json:"sourceHash"`
		PolicyHash *string `json:"policyHash"`
		StartedAt  string  `json:"startedAt"`
	}
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal SESSION_STARTED payload: %w", err)
	}

	sessionID := stringValuePtr(envelope.Context.SessionID)
	if sessionID == "" {
		return fmt.Errorf("SESSION_STARTED missing sessionId")
	}
	startedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(payload.StartedAt))
	if err != nil {
		return fmt.Errorf("parse startedAt: %w", err)
	}

	sourceRef := stringValuePtr(payload.SourceRef)
	sourceHash := stringValuePtr(payload.SourceHash)
	policyHash := stringValuePtr(payload.PolicyHash)

	r.session.SessionID = sessionID
	r.session.Status = LifecycleStatusRunning
	r.session.OrchestratorKind = strings.ToUpper(stringValuePtr(envelope.Context.OrchestratorKind))
	r.session.Dialect = stringValuePtr(envelope.Context.OrchestratorDialect)
	r.session.Phase = stringValuePtr(envelope.Context.PhaseName)
	r.session.SourceHash = sourceHash
	r.session.ResolvedSource = ResolvedSource{
		SourceRef:  sourceRef,
		SourceHash: sourceHash,
		Dialect:    r.session.Dialect,
	}
	r.session.Policy = PolicyProjection{
		EffectiveHash: policyHash,
	}
	r.session.ResultSummary = &ResultSummary{
		ResultStatus: string(ResultStatusNotReady),
	}
	r.session.Lifecycle = &LifecycleTimestamps{
		StartedAt: timePtr(startedAt.UTC()),
	}
	r.session.Progress = &ProgressCounts{}
	r.session.Usage = EmptySessionUsage()
	return nil
}

func (r *sessionProjectionReducer) applySessionPaused(envelope canonicalFactoryEvent) error {
	var payload struct {
		Status   string `json:"status"`
		PausedAt string `json:"pausedAt"`
	}
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal SESSION_PAUSED payload: %w", err)
	}
	pausedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(payload.PausedAt))
	if err != nil {
		return fmt.Errorf("parse pausedAt: %w", err)
	}
	r.session.Status = LifecycleStatusPaused
	r.result.SessionStatus = LifecycleStatusPaused
	if r.session.Lifecycle == nil {
		r.session.Lifecycle = &LifecycleTimestamps{}
	}
	r.session.Lifecycle.PausedAt = timePtr(pausedAt.UTC())
	return nil
}

func (r *sessionProjectionReducer) applySessionResumed(envelope canonicalFactoryEvent) error {
	var payload struct {
		Status    string `json:"status"`
		ResumedAt string `json:"resumedAt"`
	}
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal SESSION_RESUMED payload: %w", err)
	}
	resumedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(payload.ResumedAt))
	if err != nil {
		return fmt.Errorf("parse resumedAt: %w", err)
	}
	r.session.Status = LifecycleStatusRunning
	r.result.SessionStatus = LifecycleStatusRunning
	if r.session.Lifecycle == nil {
		r.session.Lifecycle = &LifecycleTimestamps{}
	}
	r.session.Lifecycle.ResumedAt = timePtr(resumedAt.UTC())
	return nil
}

func (r *sessionProjectionReducer) applySessionResultUpdated(envelope canonicalFactoryEvent) error {
	var payload struct {
		ResultStatus  string          `json:"resultStatus"`
		ArtifactIDs   []string        `json:"artifactIds"`
		ResultSummary json.RawMessage `json:"resultSummary"`
		Availability  *struct {
			Reason    string `json:"reason"`
			Message   string `json:"message"`
			Retryable bool   `json:"retryable"`
		} `json:"availability"`
	}
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal SESSION_RESULT_UPDATED payload: %w", err)
	}
	r.mergeSessionIdentity(envelope.Context)
	r.applyResultStatus(payload.ResultStatus, summaryTextFromWorkContent(payload.ResultSummary))
	r.replaceArtifactStubs(payload.ArtifactIDs)
	if payload.Availability != nil {
		r.resultAvailability = &ResultAvailabilityDetail{
			Reason:    strings.TrimSpace(payload.Availability.Reason),
			Message:   strings.TrimSpace(payload.Availability.Message),
			Retryable: payload.Availability.Retryable,
		}
	}
	return nil
}

func (r *sessionProjectionReducer) applySessionLifecycleControl(envelope canonicalFactoryEvent) error {
	var payload struct {
		Operation      string `json:"operation"`
		Outcome        string `json:"outcome"`
		PreviousStatus string `json:"previousStatus"`
		NewStatus      string `json:"newStatus"`
		OccurredAt     string `json:"occurredAt"`
	}
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal SESSION_LIFECYCLE_CONTROL payload: %w", err)
	}
	if strings.TrimSpace(payload.Outcome) != string(LifecycleControlOutcomeAccepted) {
		return nil
	}
	r.mergeSessionIdentity(envelope.Context)

	newStatus := strings.TrimSpace(payload.NewStatus)
	if newStatus != "" {
		r.session.Status = LifecycleStatus(newStatus)
	}
	occurredAt, err := time.Parse(time.RFC3339, strings.TrimSpace(payload.OccurredAt))
	if err != nil {
		return fmt.Errorf("parse occurredAt: %w", err)
	}
	if r.session.Lifecycle == nil {
		r.session.Lifecycle = &LifecycleTimestamps{}
	}
	operation := strings.TrimSpace(payload.Operation)
	switch operation {
	case string(LifecycleControlPause):
		r.session.Lifecycle.PausedAt = timePtr(occurredAt.UTC())
	case string(LifecycleControlResume):
		r.session.Lifecycle.ResumedAt = timePtr(occurredAt.UTC())
	}
	return nil
}

func (r *sessionProjectionReducer) applySessionCompleted(envelope canonicalFactoryEvent) error {
	var payload struct {
		FinalStatus   string   `json:"finalStatus"`
		CompletedAt   string   `json:"completedAt"`
		ResultStatus  *string  `json:"resultStatus"`
		ArtifactIDs   []string `json:"artifactIds"`
		FailureDetail *struct {
			Reason     *string `json:"reason"`
			Message    *string `json:"message"`
			ErrorClass *string `json:"errorClass"`
		} `json:"failureDetail"`
	}
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal SESSION_COMPLETED payload: %w", err)
	}
	r.mergeSessionIdentity(envelope.Context)

	finalStatus := strings.TrimSpace(payload.FinalStatus)
	if finalStatus != "" {
		r.session.Status = LifecycleStatus(finalStatus)
	}
	completedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(payload.CompletedAt))
	if err != nil {
		return fmt.Errorf("parse completedAt: %w", err)
	}
	if r.session.Lifecycle == nil {
		r.session.Lifecycle = &LifecycleTimestamps{}
	}
	r.session.Lifecycle.FinishedAt = timePtr(completedAt.UTC())

	resultStatus := ""
	if payload.ResultStatus != nil {
		resultStatus = strings.TrimSpace(*payload.ResultStatus)
	}
	r.applyResultStatus(resultStatus, "")
	r.replaceArtifactStubs(payload.ArtifactIDs)

	if payload.FailureDetail != nil {
		r.session.Failure = &FailureSummary{
			Reason:     stringValuePtr(payload.FailureDetail.Reason),
			Message:    stringValuePtr(payload.FailureDetail.Message),
			ErrorClass: stringValuePtr(payload.FailureDetail.ErrorClass),
		}
	}
	return nil
}

func (r *sessionProjectionReducer) mergeSessionIdentity(context canonicalFactoryEventContext) {
	if sessionID := stringValuePtr(context.SessionID); sessionID != "" && r.session.SessionID == "" {
		r.session.SessionID = sessionID
	}
	if kind := stringValuePtr(context.OrchestratorKind); kind != "" && r.session.OrchestratorKind == "" {
		r.session.OrchestratorKind = strings.ToUpper(kind)
	}
	if dialect := stringValuePtr(context.OrchestratorDialect); dialect != "" && r.session.Dialect == "" {
		r.session.Dialect = dialect
	}
	if phase := stringValuePtr(context.PhaseName); phase != "" && r.session.Phase == "" {
		r.session.Phase = phase
	}
}

func (r *sessionProjectionReducer) applyResultStatus(resultStatus, summaryText string) {
	status := strings.TrimSpace(resultStatus)
	if status == "" {
		return
	}
	if r.session.ResultSummary == nil {
		r.session.ResultSummary = &ResultSummary{}
	}
	r.session.ResultSummary.ResultStatus = status
	if summaryText != "" {
		r.session.ResultSummary.Summary = summaryText
	}
}

func (r *sessionProjectionReducer) replaceArtifactStubs(artifactIDs []string) {
	if len(artifactIDs) == 0 {
		return
	}
	refs := make([]ArtifactRefSummary, 0, len(artifactIDs))
	for _, artifactID := range artifactIDs {
		if id := strings.TrimSpace(artifactID); id != "" {
			refs = append(refs, ArtifactRefSummary{ID: id})
		}
	}
	r.session.ArtifactRefs = refs
	r.session.ArtifactCount = len(refs)
}

func (r *sessionProjectionReducer) finalize() {
	if strings.TrimSpace(r.session.SessionID) == "" {
		return
	}
	r.session.Links = InspectionLinksForSession(r.session.SessionID, true)
	if r.session.Progress == nil {
		r.session.Progress = &ProgressCounts{}
	}
	if r.session.Usage.Resources == nil {
		r.session.Usage = EmptySessionUsage()
	}

	r.result = ResultReadResult{
		SessionID:     r.session.SessionID,
		SessionStatus: r.session.Status,
		ArtifactIDs:   artifactIDsFromRefSummaries(r.session.ArtifactRefs),
	}
	if r.session.ResultSummary != nil {
		r.result.ResultStatus = ResultStatus(strings.TrimSpace(r.session.ResultSummary.ResultStatus))
	}
	if r.session.Failure != nil {
		r.result.Failure = cloneFailureSummary(r.session.Failure)
	}
	if r.resultAvailability != nil {
		r.result.Availability = cloneResultAvailability(r.resultAvailability)
	} else if r.result.ResultStatus == ResultStatusNotReady && !IsTerminalLifecycleStatus(r.session.Status) {
		r.result.Availability = defaultNotReadyAvailability(r.session)
	}
	if r.result.ResultStatus == ResultStatusUnavailable {
		r.result.Availability = defaultUnavailableAvailability()
	}
}

func summaryTextFromWorkContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var parts []interfaces.WorkContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}
	for _, part := range parts {
		if part.Type.Normalized() == interfaces.WorkContentPartTypeText {
			if text := strings.TrimSpace(part.Text); text != "" {
				return text
			}
		}
	}
	return ""
}

func artifactIDsFromRefSummaries(refs []ArtifactRefSummary) []string {
	if len(refs) == 0 {
		return nil
	}
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if id := strings.TrimSpace(ref.ID); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}

func stringValuePtr(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func timePtr(value time.Time) *time.Time {
	cloned := value
	return &cloned
}

func applyRuntimeSessionFields(target *runtimeSessionState, source runtimeSessionState) {
	target.session = source.session
	target.result = source.result
	target.dispatches = cloneDispatchSummaries(source.dispatches)
	target.dispatchJavaScript = cloneDispatchJavaScriptProjections(source.dispatchJavaScript)
	target.dispatchStatusTransitions = cloneDispatchStatusTransitions(source.dispatchStatusTransitions)
	target.artifacts = cloneArtifactSummaries(source.artifacts)
	target.events = source.events
}

func applyRuntimeExecutionRecordProjection(
	state *runtimeSessionState,
	sessionID string,
	records []workflowruntime.RuntimeRecord,
	finishedAt time.Time,
) {
	recordProjection := ProjectRuntimeExecutionRecords(sessionID, records, finishedAt)
	if recordProjection.Phase != "" {
		state.session.Phase = recordProjection.Phase
	}
	state.dispatches = cloneDispatchSummaries(recordProjection.Dispatches)
	state.dispatchJavaScript = cloneDispatchJavaScriptProjections(recordProjection.DispatchJavaScript)
	state.dispatchStatusTransitions = cloneDispatchStatusTransitions(recordProjection.DispatchStatusTransitions)
	state.artifacts = cloneArtifactSummaries(recordProjection.Artifacts)
	state.session.Progress = &recordProjection.Progress
	state.session.ArtifactRefs = artifactRefsFromSummaries(state.artifacts)
	state.session.ArtifactCount = len(state.session.ArtifactRefs)
}

func applyRuntimeSuccessProjection(
	state *runtimeSessionState,
	sessionID string,
	outcome workflowruntime.Outcome,
	finishedAt time.Time,
) {
	applyRuntimeExecutionRecordProjection(state, sessionID, outcome.Records, finishedAt)

	projected, resultSummary, err := projectRuntimeSuccessResult(sessionID, outcome.Value, state.artifacts)
	if err != nil {
		state.session.Status = LifecycleStatusFailed
		state.session.Failure = &FailureSummary{
			Reason:  "WORKFLOW_RUNTIME_INVALID_RESULT",
			Message: err.Error(),
		}
		state.session.ResultSummary = &ResultSummary{
			ResultStatus: string(ResultStatusUnavailable),
		}
		state.result = ResultReadResult{
			SessionID:     sessionID,
			ResultStatus:  ResultStatusUnavailable,
			SessionStatus: LifecycleStatusFailed,
			Failure:       cloneFailureSummary(state.session.Failure),
			Availability:  defaultUnavailableAvailability(),
		}
		return
	}
	state.session.Status = LifecycleStatusSucceeded
	state.session.ResultSummary = resultSummary
	state.result = projected
}

func projectRuntimeSuccessResult(
	sessionID string,
	value workflowresult.TypedValue,
	artifacts []ArtifactSummary,
) (ResultReadResult, *ResultSummary, error) {
	parts, validation := workflowresult.ProjectPrimaryResult(sessionID, value, artifactStatesFromSummaries(artifacts))
	if validation.HasIssues() {
		return ResultReadResult{}, nil, fmt.Errorf("project primary result: %v", validation.Issues)
	}

	primaryJSON := workContentJSONFromParts(parts)
	result := ResultReadResult{
		SessionID:     sessionID,
		ResultStatus:  ResultStatusFinal,
		SessionStatus: LifecycleStatusSucceeded,
		PrimaryResult: primaryJSON,
		ArtifactIDs:   artifactIDsFromSummaries(artifacts),
	}
	summary := &ResultSummary{
		ResultStatus: string(ResultStatusFinal),
		Summary:      resultSummaryTextFromParts(parts),
	}
	return result, summary, nil
}

func workContentJSONFromParts(parts []interfaces.WorkContentPart) json.RawMessage {
	content := workcontent.GeneratedPtrFromParts(parts)
	if content == nil {
		return nil
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return nil
	}
	return encoded
}

func resultSummaryTextFromParts(parts []interfaces.WorkContentPart) string {
	for _, part := range parts {
		if part.Type.Normalized() == interfaces.WorkContentPartTypeText {
			if text := strings.TrimSpace(part.Text); text != "" {
				return text
			}
		}
	}
	return ""
}

func artifactStatesFromSummaries(artifacts []ArtifactSummary) []interfaces.FactorySessionArtifactState {
	if len(artifacts) == 0 {
		return nil
	}
	states := make([]interfaces.FactorySessionArtifactState, 0, len(artifacts))
	for _, artifact := range artifacts {
		states = append(states, interfaces.FactorySessionArtifactState{
			ID:          artifact.ID,
			Kind:        artifact.Kind,
			Visibility:  artifact.Visibility,
			Label:       artifact.Label,
			ContentHash: artifact.ContentHash,
			SizeBytes:   artifact.SizeBytes,
			AuditMode:   artifact.AuditMode,
		})
	}
	return states
}

// PersistedRuntimeSessionState is a JSON-serializable durable runtime session snapshot
// used to reload terminal JavaScript runtime sessions across CLI invocations.
type PersistedRuntimeSessionState struct {
	Session                   SessionReadResult
	Result                    ResultReadResult
	Dispatches                []DispatchSummary
	DispatchJavaScript        map[string]DispatchJavaScriptProjection
	DispatchStatusTransitions map[string][]DispatchStatus
	Artifacts                 []ArtifactSummary
	Events                    []json.RawMessage
}
