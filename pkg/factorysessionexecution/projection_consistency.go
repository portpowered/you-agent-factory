package factorysessionexecution

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
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

// ValidateSessionDetailMatchesListSummary checks that one session detail read
// aligns with the durable list row for the same session id.
func ValidateSessionDetailMatchesListSummary(
	detail SessionReadResult,
	summary DurableSessionListSummary,
) error {
	if strings.TrimSpace(detail.SessionID) == "" || strings.TrimSpace(summary.SessionID) == "" {
		return nil
	}
	if detail.SessionID != summary.SessionID {
		return fmt.Errorf("detail sessionId %q does not match list summary %q", detail.SessionID, summary.SessionID)
	}
	if detail.Status != summary.Status {
		return fmt.Errorf("detail status %q does not match list summary %q", detail.Status, summary.Status)
	}
	if strings.TrimSpace(detail.Phase) != strings.TrimSpace(summary.Phase) {
		return fmt.Errorf("detail phase %q does not match list summary %q", detail.Phase, summary.Phase)
	}
	if err := validateProgressMatches(detail.Progress, summary.Progress); err != nil {
		return err
	}
	if err := validateResultSummaryMatches(detail.ResultSummary, summary.ResultSummary); err != nil {
		return err
	}
	detailArtifactCount := detail.ArtifactCount
	if detailArtifactCount == 0 {
		detailArtifactCount = len(detail.ArtifactRefs)
	}
	if detailArtifactCount != summary.ArtifactCount {
		return fmt.Errorf("detail artifactCount %d does not match list summary %d", detailArtifactCount, summary.ArtifactCount)
	}
	derivedActions := DeriveSessionActionAvailability(detail.Status)
	if derivedActions != summary.Actions {
		return fmt.Errorf("detail actions %#v do not match list summary %#v", derivedActions, summary.Actions)
	}
	if detail.Links != summary.Links {
		return fmt.Errorf("detail links %#v do not match list summary %#v", detail.Links, summary.Links)
	}
	if summary.Recoverable != IsRecoverableSession(detail.Status, detail.StaleLease) {
		return fmt.Errorf("detail recoverability does not match list summary recoverable=%t", summary.Recoverable)
	}
	return nil
}

func validateProgressMatches(detail, summary *ProgressCounts) error {
	switch {
	case detail == nil && summary == nil:
		return nil
	case detail == nil || summary == nil:
		return fmt.Errorf("detail progress %#v does not match list summary %#v", detail, summary)
	case detail.TotalDispatches != summary.TotalDispatches,
		detail.CompletedDispatches != summary.CompletedDispatches,
		detail.FailedDispatches != summary.FailedDispatches,
		detail.InFlightDispatches != summary.InFlightDispatches:
		return fmt.Errorf("detail progress %#v does not match list summary %#v", detail, summary)
	default:
		return nil
	}
}

func validateResultSummaryMatches(detail, summary *ResultSummary) error {
	switch {
	case detail == nil && summary == nil:
		return nil
	case detail == nil || summary == nil:
		return fmt.Errorf("detail resultSummary %#v does not match list summary %#v", detail, summary)
	case strings.TrimSpace(detail.ResultStatus) != strings.TrimSpace(summary.ResultStatus):
		return fmt.Errorf("detail resultStatus %q does not match list summary %q", detail.ResultStatus, summary.ResultStatus)
	default:
		return nil
	}
}

// ValidateDispatchDetailMatchesListSummary checks that one dispatch detail read
// aligns with the dispatch list row for the same dispatch id.
func ValidateDispatchDetailMatchesListSummary(detail DispatchDetail, summary DispatchSummary) error {
	if detail.ID != summary.ID {
		return fmt.Errorf("detail id %q does not match list summary %q", detail.ID, summary.ID)
	}
	if detail.Status != summary.Status {
		return fmt.Errorf("detail status %q does not match list summary %q", detail.Status, summary.Status)
	}
	if detail.DispatchKind != summary.DispatchKind {
		return fmt.Errorf("detail dispatchKind %q does not match list summary %q", detail.DispatchKind, summary.DispatchKind)
	}
	if strings.TrimSpace(detail.Label) != strings.TrimSpace(summary.Label) {
		return fmt.Errorf("detail label %q does not match list summary %q", detail.Label, summary.Label)
	}
	if detail.Attempt != summary.Attempt {
		return fmt.Errorf("detail attempt %d does not match list summary %d", detail.Attempt, summary.Attempt)
	}
	if detail.FailureDetail != nil || summary.FailureDetail != nil {
		switch {
		case detail.FailureDetail == nil || summary.FailureDetail == nil:
			return fmt.Errorf("detail failureDetail %#v does not match list summary %#v", detail.FailureDetail, summary.FailureDetail)
		case detail.FailureDetail.Reason != summary.FailureDetail.Reason,
			detail.FailureDetail.Message != summary.FailureDetail.Message:
			return fmt.Errorf("detail failureDetail %#v does not match list summary %#v", detail.FailureDetail, summary.FailureDetail)
		}
	}
	return nil
}

// ValidateArtifactDetailMatchesListSummary checks that one artifact detail read
// aligns with the artifact list row for the same artifact id.
func ValidateArtifactDetailMatchesListSummary(detail ArtifactDetail, summary ArtifactSummary) error {
	if detail.ID != summary.ID {
		return fmt.Errorf("detail id %q does not match list summary %q", detail.ID, summary.ID)
	}
	if detail.Kind != summary.Kind {
		return fmt.Errorf("detail kind %q does not match list summary %q", detail.Kind, summary.Kind)
	}
	if detail.Visibility != summary.Visibility {
		return fmt.Errorf("detail visibility %q does not match list summary %q", detail.Visibility, summary.Visibility)
	}
	if strings.TrimSpace(detail.Label) != strings.TrimSpace(summary.Label) {
		return fmt.Errorf("detail label %q does not match list summary %q", detail.Label, summary.Label)
	}
	if detail.ContentHash != summary.ContentHash {
		return fmt.Errorf("detail contentHash %q does not match list summary %q", detail.ContentHash, summary.ContentHash)
	}
	if detail.SizeBytes != summary.SizeBytes {
		return fmt.Errorf("detail sizeBytes %d does not match list summary %d", detail.SizeBytes, summary.SizeBytes)
	}
	if detail.DispatchID != summary.DispatchID {
		return fmt.Errorf("detail dispatchId %q does not match list summary %q", detail.DispatchID, summary.DispatchID)
	}
	detailRef := detail.RetrievalRef
	if detailRef == nil {
		detailRef = detail.ContentRef
	}
	if detailRef != nil || summary.RetrievalRef != nil {
		switch {
		case detailRef == nil || summary.RetrievalRef == nil:
			return fmt.Errorf("detail retrieval ref %#v does not match list summary %#v", detailRef, summary.RetrievalRef)
		case detailRef.Href != summary.RetrievalRef.Href,
			detailRef.Method != summary.RetrievalRef.Method:
			return fmt.Errorf("detail retrieval ref %#v does not match list summary %#v", detailRef, summary.RetrievalRef)
		}
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
