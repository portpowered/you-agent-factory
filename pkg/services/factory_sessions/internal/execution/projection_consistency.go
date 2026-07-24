// backendsizecheck:ignore-file service-ownership migration preserves this consolidated surface until a dedicated responsibility split removes the exemption.
// pkgmaintcheck:ignore-file-lines service-ownership migration preserves this consolidated file; split responsibilities and remove this exemption.
package factorysessionexecution

import (
	"context"
	"encoding/json"
	"fmt"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recording "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// RecordingError correlates a safe export failure without changing the live session.
type RecordingError struct {
	SessionID, Path string
	Err             error
}

func (e *RecordingError) Error() string {
	return fmt.Sprintf("record Factory Session %q to %q: %v", e.SessionID, e.Path, e.Err)
}
func (e *RecordingError) Unwrap() error { return e.Err }

// WriteRecording snapshots canonical public facts and writes their portable form.
func (s *JavaScriptRuntimeService) WriteRecording(ctx context.Context, sessionID, path string) error {
	if err := ctx.Err(); err != nil {
		return &RecordingError{sessionID, path, err}
	}
	s.mu.RLock()
	state, found := s.sessions[sessionID]
	if !found {
		s.mu.RUnlock()
		return &RecordingError{sessionID, path, ErrSessionNotFound}
	}
	snapshot := cloneRuntimeSessionState(state)
	s.mu.RUnlock()
	facts := recording.PortableRecordingCanonicalFacts{SessionID: snapshot.session.SessionID, Status: string(snapshot.session.Status), OrchestratorKind: snapshot.session.OrchestratorKind, SourceRef: snapshot.session.ResolvedSource.SourceRef, SourceHash: snapshot.session.SourceHash, PolicyHash: snapshot.session.Policy.EffectiveHash, Events: snapshot.events}
	facts.Result = canonicalRecordingResult(snapshot.session.Status, snapshot.result)
	if checkpoint := snapshot.checkpointSummary; checkpoint != nil {
		facts.Checkpoint = &recording.PortableRecordingCanonicalCheckpoint{ID: checkpoint.CheckpointID, Label: checkpoint.Label, Summary: checkpoint.Phase, Timestamp: checkpoint.CreatedAt}
	}
	if snapshot.startRequest != nil {
		facts.Arguments = snapshot.startRequest.Args
	}
	for _, artifact := range snapshot.artifacts {
		createdAt, secrets := time.Time{}, int64(0)
		if artifact.CreatedAt != nil {
			createdAt = *artifact.CreatedAt
		}
		if artifact.RedactionCounts != nil {
			secrets = int64(artifact.RedactionCounts.Secrets)
		}
		facts.Artifacts = append(facts.Artifacts, recording.PortableRecordingCanonicalArtifact{ID: artifact.ID, Kind: artifact.Kind, Visibility: artifact.Visibility, Label: artifact.Label, ContentHash: artifact.ContentHash, SizeBytes: artifact.SizeBytes, CreatedAt: createdAt, SecretsRedacted: secrets})
		if facts.Checkpoint != nil && facts.Checkpoint.ArtifactID == "" {
			for _, checkpointArtifactID := range snapshot.checkpointSummary.ArtifactIDs {
				if checkpointArtifactID == artifact.ID {
					facts.Checkpoint.ArtifactID = artifact.ID
					break
				}
			}
		}
	}
	value, err := recording.BuildPortableRecording(facts)
	if err == nil {
		if s.recordingWriter == nil {
			err = fmt.Errorf("portable recording writer is required")
		} else {
			err = s.recordingWriter.Write(path, value)
		}
	}
	if err != nil {
		return &RecordingError{sessionID, path, err}
	}
	return nil
}

func canonicalRecordingResult(sessionStatus LifecycleStatus, result ResultReadResult) *recording.PortableRecordingCanonicalResult {
	projected := &recording.PortableRecordingCanonicalResult{
		Status: string(result.ResultStatus), Mode: string(result.Mode),
		PrimaryResult: append(json.RawMessage(nil), result.PrimaryResult...),
		ArtifactIDs:   append([]string(nil), result.ArtifactIDs...),
	}
	if projected.Mode == "" {
		projected.Mode = string(ResultModeFinal)
	}
	if projected.Status == "" {
		switch sessionStatus {
		case LifecycleStatusSucceeded:
			projected.Status = string(ResultStatusFinal)
		case LifecycleStatusFailed:
			projected.Status = string(ResultStatusUnavailable)
		default:
			projected.Status = string(ResultStatusNotReady)
		}
	}
	if result.Failure != nil {
		projected.Failure = &recording.PortableRecordingFailureSummary{Reason: result.Failure.Reason, Message: result.Failure.Message, PartialResultAvailable: result.Failure.PartialResultAvailable}
	}
	if result.Availability != nil {
		projected.Availability = &recording.PortableRecordingAvailability{Reason: result.Availability.Reason, Message: result.Availability.Message, Retryable: result.Availability.Retryable}
	}
	return projected
}

// projectPetriRunningSessionState initializes the canonical read model owned by
// Factory Session execution before the first typed Petri mutation is recorded.
func projectPetriRunningSessionState(sessionID string, startedAt time.Time) runtimeSessionState {
	session := SessionReadResult{
		SessionID: sessionID, Status: LifecycleStatusRunning,
		OrchestratorKind: interfaces.OrchestratorKindPetri,
		Usage:            EmptySessionUsage(), ResultSummary: &ResultSummary{ResultStatus: string(ResultStatusNotReady)},
		Lifecycle: &LifecycleTimestamps{StartedAt: &startedAt}, Links: InspectionLinksForSession(sessionID, true),
	}
	result := ResultReadResult{
		SessionID: sessionID, Mode: ResultModeFinal, ResultStatus: ResultStatusNotReady,
		SessionStatus: LifecycleStatusRunning,
		Availability:  &ResultAvailabilityDetail{Reason: "RESULT_NOT_READY", Message: "Session is still running.", Retryable: true},
	}
	state := runtimeSessionState{session: session, result: result}
	state.events = BuildCanonicalRuntimeSessionEvents(session, result, RuntimeDispatchEventInput{})
	return state
}

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

const (
	DispatchStatusQueued      DispatchStatus = "QUEUED"
	DispatchStatusRunning     DispatchStatus = "RUNNING"
	DispatchStatusCompleted   DispatchStatus = "COMPLETED"
	DispatchStatusFailed      DispatchStatus = "FAILED"
	DispatchStatusCanceled    DispatchStatus = "CANCELED"
	DispatchStatusTimedOut    DispatchStatus = "TIMED_OUT"
	DispatchStatusSkipped     DispatchStatus = "SKIPPED"
	DispatchStatusInterrupted DispatchStatus = "INTERRUPTED"
)

// NormalizeDispatchFilters validates and canonicalizes transport-provided filters.
func NormalizeDispatchFilters(filters DispatchFilters) (DispatchFilters, error) {
	filters.Phase = strings.TrimSpace(filters.Phase)
	status := DispatchStatus(strings.ToUpper(strings.TrimSpace(string(filters.Status))))
	if status != "" && !isCanonicalDispatchStatus(status) {
		return DispatchFilters{}, NewValidationError("status", "status must be QUEUED, RUNNING, COMPLETED, FAILED, CANCELED, TIMED_OUT, SKIPPED, or INTERRUPTED")
	}
	filters.Status = status
	return filters, nil
}

// FilterDispatches applies canonical phase/status semantics without changing session order.
func FilterDispatches(result ListDispatchesResult, filters DispatchFilters) (ListDispatchesResult, error) {
	normalized, err := NormalizeDispatchFilters(filters)
	if err != nil {
		return ListDispatchesResult{}, err
	}
	filtered := make([]DispatchSummary, 0, len(result.Dispatches))
	for _, dispatch := range result.Dispatches {
		if normalized.Phase != "" && dispatch.Phase != normalized.Phase {
			continue
		}
		if normalized.Status != "" && dispatch.Status != normalized.Status {
			continue
		}
		filtered = append(filtered, dispatch)
	}
	result.Dispatches = filtered
	return result, nil
}

type dispatchListReader interface {
	ListDispatches(context.Context, string) (ListDispatchesResult, error)
}

func queryDispatches(
	ctx context.Context,
	service dispatchListReader,
	request DispatchQueryRequest,
) (ListDispatchesResult, error) {
	result, err := service.ListDispatches(ctx, request.SessionID)
	if err != nil {
		return ListDispatchesResult{}, err
	}
	return FilterDispatches(result, request.Filters)
}

func isCanonicalDispatchStatus(status DispatchStatus) bool {
	switch status {
	case DispatchStatusQueued, DispatchStatusRunning, DispatchStatusCompleted,
		DispatchStatusFailed, DispatchStatusCanceled, DispatchStatusTimedOut,
		DispatchStatusSkipped, DispatchStatusInterrupted:
		return true
	default:
		return false
	}
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
	terminal           bool
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
	if r.terminal {
		return nil
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
	case "ORCHESTRATOR_PHASE_CHANGED", "JAVASCRIPT_PHASE_CHANGE":
		return r.applyPhaseChanged(envelope)
	case "ORCHESTRATOR_CHECKPOINT_WRITTEN", "JAVASCRIPT_CHECKPOINT_REF":
		return r.applyCheckpointWritten(envelope)
	case "ARTIFACT_CREATED":
		return r.applyArtifactCreated(envelope)
	default:
		return nil
	}
}

func (r *sessionProjectionReducer) applyPhaseChanged(envelope canonicalFactoryEvent) error {
	phase := strings.TrimSpace(stringValuePtr(envelope.Context.PhaseName))
	if phase == "" {
		phase = strings.TrimSpace(stringValuePtr(envelope.Context.PhaseID))
	}
	if phase == "" {
		return nil
	}
	r.session.Phase = phase
	for _, summary := range r.session.PhaseSummaries {
		if summary.Phase == phase {
			return nil
		}
	}
	r.session.PhaseSummaries = append(r.session.PhaseSummaries, PhaseSummary{Phase: phase})
	if r.session.Progress == nil {
		r.session.Progress = &ProgressCounts{}
	}
	r.session.Progress.PhaseCount = len(r.session.PhaseSummaries)
	return nil
}

func (r *sessionProjectionReducer) applyCheckpointWritten(envelope canonicalFactoryEvent) error {
	checkpointID := strings.TrimSpace(stringValuePtr(envelope.Context.CheckpointID))
	if checkpointID == "" {
		return nil
	}
	var payload struct {
		Label string `json:"label"`
	}
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal ORCHESTRATOR_CHECKPOINT_WRITTEN payload: %w", err)
	}
	r.session.LatestCheckpoint = &CheckpointRef{
		ID: checkpointID, Label: strings.TrimSpace(payload.Label), Phase: strings.TrimSpace(r.session.Phase),
	}
	return nil
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
	switch strings.TrimSpace(payload.Status) {
	case string(LifecycleStatusResuming):
		r.session.Status = LifecycleStatusResuming
		r.result.SessionStatus = LifecycleStatusResuming
	default:
		r.session.Status = LifecycleStatusRunning
		r.result.SessionStatus = LifecycleStatusRunning
	}
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
	if len(payload.ResultSummary) > 0 {
		r.result.PrimaryResult = append(json.RawMessage(nil), payload.ResultSummary...)
	}
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
			Reason  *string `json:"reason"`
			Message *string `json:"message"`
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
			Reason:  stringValuePtr(payload.FailureDetail.Reason),
			Message: stringValuePtr(payload.FailureDetail.Message),
		}
	}
	r.terminal = true
	return nil
}

func (r *sessionProjectionReducer) applyArtifactCreated(envelope canonicalFactoryEvent) error {
	var payload struct {
		Artifact struct {
			ID          string `json:"id"`
			Kind        string `json:"kind"`
			Visibility  string `json:"visibility"`
			ContentHash string `json:"contentHash"`
			SizeBytes   int64  `json:"sizeBytes"`
		} `json:"artifact"`
	}
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal ARTIFACT_CREATED payload: %w", err)
	}
	artifactID := strings.TrimSpace(payload.Artifact.ID)
	if artifactID == "" {
		return fmt.Errorf("ARTIFACT_CREATED missing artifact id")
	}
	r.mergeSessionIdentity(envelope.Context)
	r.upsertArtifactRef(ArtifactRefSummary{
		ID:          artifactID,
		Kind:        strings.TrimSpace(payload.Artifact.Kind),
		Visibility:  strings.TrimSpace(payload.Artifact.Visibility),
		ContentHash: strings.TrimSpace(payload.Artifact.ContentHash),
		SizeBytes:   payload.Artifact.SizeBytes,
	})
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
			ref := ArtifactRefSummary{ID: id}
			for _, existing := range r.session.ArtifactRefs {
				if existing.ID == id {
					ref = existing
					break
				}
			}
			refs = append(refs, ref)
		}
	}
	r.session.ArtifactRefs = refs
	r.session.ArtifactCount = len(refs)
}

func (r *sessionProjectionReducer) upsertArtifactRef(ref ArtifactRefSummary) {
	for index := range r.session.ArtifactRefs {
		if r.session.ArtifactRefs[index].ID == ref.ID {
			r.session.ArtifactRefs[index] = ref
			r.session.ArtifactCount = len(r.session.ArtifactRefs)
			return
		}
	}
	r.session.ArtifactRefs = append(r.session.ArtifactRefs, ref)
	r.session.ArtifactCount = len(r.session.ArtifactRefs)
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
		PrimaryResult: append(json.RawMessage(nil), r.result.PrimaryResult...),
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
	var parts []work.WorkContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}
	for _, part := range parts {
		if part.Type.Normalized() == work.WorkContentPartTypeText {
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

func MaterializeEventReadStream(result EventReadResult) *interfaces.FactoryEventStream {
	return factorysessions.MaterializeEventReadStream(result)
}
