package factorysessionexecution

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type sessionProjectionReducer struct {
	session SessionReadResult
	result  ResultReadResult
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
	}
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal SESSION_RESULT_UPDATED payload: %w", err)
	}
	r.mergeSessionIdentity(envelope.Context)
	r.applyResultStatus(payload.ResultStatus, summaryTextFromWorkContent(payload.ResultSummary))
	r.replaceArtifactStubs(payload.ArtifactIDs)
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
	if r.result.ResultStatus == ResultStatusNotReady && !IsTerminalLifecycleStatus(r.session.Status) {
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
