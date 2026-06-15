package factorysessionexecution

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
)

// SessionProjectionEventKinds lists canonical event types that project durable
// session lifecycle, result, dispatch, artifact, phase, checkpoint, and budget state.
var SessionProjectionEventKinds = []string{
	"SESSION_STARTED",
	"SESSION_RESULT_UPDATED",
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
	TaskKind  string
	TaskLabel string
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
	SessionID        string
	OrchestratorKind string
	ArtifactIDs      []string
	Petri            *DispatchPetriProjection
	JavaScript       *DispatchJavaScriptProjection
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
	case "SESSION_RESULT_UPDATED":
		return r.applySessionResultUpdated(envelope)
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

// RuntimeExecutionProjection carries durable dispatch, artifact, phase, and progress
// state projected from ordered workflow runtime records.
type RuntimeExecutionProjection struct {
	Phase      string
	PhaseCount int
	Dispatches []DispatchSummary
	Artifacts  []ArtifactSummary
	Progress   ProgressCounts
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
	projection.Artifacts = orderedArtifactSummaries(artifactByID)
	projection.Progress = progressCountsFromDispatches(projection.Dispatches, projection.PhaseCount)
	return projection
}

func artifactSummaryFromRuntimeRecord(
	sessionID string,
	record workflowruntime.ArtifactRecord,
	observedAt time.Time,
) ArtifactSummary {
	return ArtifactSummary{
		ID:          record.ID,
		Kind:        record.Kind,
		Visibility:  record.Visibility,
		Label:       record.Label,
		ContentHash: record.ContentHash,
		SizeBytes:   record.SizeBytes,
		CreatedAt:   timePtr(observedAt.UTC()),
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
		Kind:         "CHILD_OUTPUT",
		Visibility:   "WORKFLOW_RUNTIME",
		Label:        child.Label,
		DispatchID:   child.DispatchID,
		CreatedAt:    timePtr(observedAt.UTC()),
		RetrievalRef: artifactRetrievalRefForSession(sessionID, parsed.ArtifactID),
	}, true
}

func dispatchSummaryFromChildRecord(currentPhase string, child workflowruntime.ChildDispatchRecord) DispatchSummary {
	summary := DispatchSummary{
		ID:           child.DispatchID,
		Status:       DispatchStatus(strings.TrimSpace(child.Status)),
		DispatchKind: "JAVASCRIPT_AGENT",
		Phase:        currentPhase,
		Label:        child.Label,
		Attempt:      1,
		RunnerID:     strings.TrimSpace(child.RunnerID),
		Model:        strings.TrimSpace(child.Model),
	}
	if ref := strings.TrimSpace(child.ProviderSessionRef); ref != "" {
		summary.Provider = "fake"
		summary.ProviderSessionRefs = []ProviderSessionRef{{
			Provider: "fake",
			Kind:     "AGENT",
			ID:       ref,
		}}
	}
	if artifactID := artifactIDFromRef(child.ArtifactRef); artifactID != "" &&
		summary.Status == DispatchStatusCompleted {
		summary.OutputArtifactIDs = []string{artifactID}
	}
	return summary
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
		case DispatchStatusQueued, DispatchStatusRunning:
			progress.InFlightDispatches++
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
