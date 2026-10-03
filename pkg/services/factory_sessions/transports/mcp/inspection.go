package factorysession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	apifactorysession "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

// RecordingsInspection is the narrow Recordings capability consumed by the
// Factory Sessions inspection tools. The consumer owns this view so fixture
// bridges can satisfy the same detached operations without expanding the
// Recordings service-root interface set.
type RecordingsInspection interface {
	QueryRecordingStatus(recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error)
	QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error)
	BuildPortableArtifact(recordings.BuildPortableArtifactRequest) (recordings.BuildPortableArtifactResult, error)
	ReconstructWorldState(recordings.ReconstructWorldStateRequest) (recordings.ReconstructWorldStateResult, error)
	SubscribeFrom(context.Context, recordings.SubscribeRequest) (recordings.SubscribeResult, error)
}

// ListDispatchesInput is the MCP request shape for you.factory_session.list_dispatches.
type ListDispatchesInput struct {
	SessionID string `json:"sessionId"`
	Phase     string `json:"phase,omitempty"`
	Status    string `json:"status,omitempty"`
}

// ListArtifactsInput is the MCP request shape for you.factory_session.list_artifacts.
type ListArtifactsInput struct {
	SessionID string `json:"sessionId"`
}

// ListArtifacts returns deterministic FactoryArtifact summaries for one Factory
// Session through the you.factory_session.list_artifacts MCP tool. Recordings
// owns the artifact and reconstructed-world-state reads.
func ListArtifacts(
	ctx context.Context,
	service RecordingsInspection,
	input ListArtifactsInput,
) ToolResponse[factoryapi.ListFactorySessionArtifactsResponse] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[factoryapi.ListFactorySessionArtifactsResponse]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryapi.ListFactorySessionArtifactsResponse](ctx); done {
		return response
	}
	result, err := listFactorySessionArtifacts(ctx, service, input)
	if err != nil {
		envelope := readErrorEnvelope(input.SessionID, err)
		return ToolResponse[factoryapi.ListFactorySessionArtifactsResponse]{Error: &envelope}
	}
	return ToolResponse[factoryapi.ListFactorySessionArtifactsResponse]{Result: &result}
}

// ReadEventsInput is the MCP request shape for you.factory_session.read_events.
type ReadEventsInput struct {
	SessionID     string `json:"sessionId"`
	AfterEventID  string `json:"afterEventId,omitempty"`
	AfterSequence *int   `json:"afterSequence,omitempty"`
}

// ReadEventsResult is the MCP response shape for you.factory_session.read_events.
type ReadEventsResult struct {
	SessionID string                    `json:"sessionId"`
	Events    []factoryapi.FactoryEvent `json:"events,omitempty"`
}

// readEventsCanonical keeps retained Recordings authoritative, using the
// already-injected durable owner only when Recordings has no target. This MCP
// representation adapter preserves canonical facts without rewriting history.
func readEventsCanonical(
	ctx context.Context,
	sessions factorysessionexecution.Service,
	service RecordingsInspection,
	input ReadEventsInput,
) ToolResponse[ReadEventsResult] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[ReadEventsResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[ReadEventsResult](ctx); done {
		return response
	}
	result, err := readFactorySessionEvents(ctx, service, input)
	if errors.Is(err, recordings.ErrMissingRecordingTarget) {
		result, err = rootReadEvents(ctx, sessions, input)
		if err != nil {
			envelope := eventReadErrorEnvelope(input.SessionID, err)
			var validationErr *factorysessionexecution.ExecutionValidationError
			if envelope.Code == errorCodeBadRequest && !errors.Is(err, recordings.ErrInvalidReconnectCursor) && !errors.As(err, &validationErr) {
				// Durable failures and malformed canonical facts are not caller
				// validation messages. Keep their internal contents private.
				envelope = unmappedExecutionErrorEnvelope()
			}
			return ToolResponse[ReadEventsResult]{Error: &envelope}
		}
	}
	if err != nil {
		envelope := eventReadErrorEnvelope(input.SessionID, err)
		return ToolResponse[ReadEventsResult]{Error: &envelope}
	}
	return ToolResponse[ReadEventsResult]{Result: &result}
}

func rootReadEvents(
	ctx context.Context,
	sessions factorysessionexecution.Service,
	input ReadEventsInput,
) (ReadEventsResult, error) {
	if _, err := sessions.Get(ctx, factorysessionexecution.SessionGetRequest{
		SessionID: input.SessionID, Mode: factorysessionexecution.SessionOperationModeDurable,
	}); err != nil {
		return ReadEventsResult{}, err
	}
	reconnect := factorysessionexecution.EventReconnectRequest{AfterEventID: strings.TrimSpace(input.AfterEventID)}
	if reconnect.AfterEventID == "" {
		if input.AfterSequence != nil && *input.AfterSequence < 0 {
			return ReadEventsResult{}, recordings.ErrInvalidReconnectCursor
		}
		reconnect.AfterSequence = input.AfterSequence
	}
	facts, err := sessions.ReadEvents(ctx, input.SessionID, reconnect)
	if err != nil {
		return ReadEventsResult{}, err
	}
	if facts.SessionID != input.SessionID {
		return ReadEventsResult{}, errors.New("canonical event read returned a different Session")
	}
	events := make([]factoryapi.FactoryEvent, 0, len(facts.Events))
	for _, raw := range facts.Events {
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			return ReadEventsResult{}, err
		}
		if event.Context.SessionId == nil || *event.Context.SessionId != facts.SessionID {
			return ReadEventsResult{}, errors.New("canonical event belongs to a different Session")
		}
		events = append(events, event)
	}
	return ReadEventsResult{SessionID: facts.SessionID, Events: events}, nil
}

// ControlInput is the MCP request shape for you.factory_session.control.
type ControlInput struct {
	SessionID         string                                        `json:"sessionId"`
	Operation         factoryapi.FactorySessionLifecycleControlKind `json:"operation"`
	RequestID         *string                                       `json:"requestId,omitempty"`
	Reason            *string                                       `json:"reason,omitempty"`
	DispatchID        *string                                       `json:"dispatchId,omitempty"`
	ApprovalPreviewID *string                                       `json:"approvalPreviewId,omitempty"`
	ApprovedPolicy    *map[string]any                               `json:"approvedPolicy,omitempty"`
}

func prepareControlInput(prepare RequestPreparation, input ControlInput) (factorysessionexecution.ControlRequest, error) {
	if prepare == nil {
		return factorysessionexecution.ControlRequest{}, errors.New("Factory Session request preparation is required")
	}
	return prepare.PrepareControl(factorysessionexecution.ControlRequest{
		RequestID: derefString(input.RequestID),
		Reason:    derefString(input.Reason),
	})
}

func prepareApproveInput(prepare RequestPreparation, input ControlInput) (factorysessionexecution.ApproveRequest, error) {
	if prepare == nil {
		return factorysessionexecution.ApproveRequest{}, errors.New("Factory Session request preparation is required")
	}
	approve := factorysessionexecution.ApproveRequest{
		ControlRequest: factorysessionexecution.ControlRequest{
			RequestID: derefString(input.RequestID),
			Reason:    derefString(input.Reason),
		},
		ApprovalPreviewID: derefString(input.ApprovalPreviewID),
	}
	if input.ApprovedPolicy != nil {
		approve.ApprovedPolicy = *input.ApprovedPolicy
	}
	return prepare.PrepareApprove(approve)
}

func prepareRetryDispatchInput(prepare RequestPreparation, input ControlInput) (factorysessionexecution.RetryDispatchRequest, error) {
	if prepare == nil {
		return factorysessionexecution.RetryDispatchRequest{}, errors.New("Factory Session request preparation is required")
	}
	retry := factorysessionexecution.RetryDispatchRequest{
		ControlRequest: factorysessionexecution.ControlRequest{
			RequestID: derefString(input.RequestID),
			Reason:    derefString(input.Reason),
		},
		DispatchID: derefString(input.DispatchID),
	}
	return prepare.PrepareRetryDispatch(retry)
}

func prepareInterruptDispatchInput(prepare RequestPreparation, input ControlInput) (factorysessionexecution.InterruptDispatchRequest, error) {
	if prepare == nil {
		return factorysessionexecution.InterruptDispatchRequest{}, errors.New("Factory Session request preparation is required")
	}
	interrupt := factorysessionexecution.InterruptDispatchRequest{
		ControlRequest: factorysessionexecution.ControlRequest{
			RequestID: derefString(input.RequestID),
			Reason:    derefString(input.Reason),
		},
		DispatchID: derefString(input.DispatchID),
	}
	return prepare.PrepareInterruptDispatch(interrupt)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// Factory Session inspection reads canonical facts from the Recordings root.
func listFactorySessionDispatches(
	ctx context.Context,
	service RecordingsInspection,
	input ListDispatchesInput,
) (factoryapi.ListFactorySessionDispatchesResponse, error) {
	if err := validateDispatchStatus(input.Status); err != nil {
		return factoryapi.ListFactorySessionDispatchesResponse{}, err
	}
	if err := validateInspectionRequest(ctx, service, input.SessionID); err != nil {
		return factoryapi.ListFactorySessionDispatchesResponse{}, err
	}
	history, err := historicalRecording(ctx, service, input.SessionID)
	if err != nil {
		return factoryapi.ListFactorySessionDispatchesResponse{}, err
	}
	dispatches := make([]factorysessionexecution.DispatchSummary, 0, len(history.Dispatches))
	for _, dispatch := range history.Dispatches {
		// Historical dispatches deliberately do not expose a mutable workflow
		// phase. Preserve the established empty-result behavior for a phase
		// filter instead of inventing a phase projection here.
		if strings.TrimSpace(input.Phase) != "" {
			continue
		}
		if status := strings.TrimSpace(input.Status); status != "" && status != string(dispatch.Status) {
			continue
		}
		kind := strings.TrimSpace(string(dispatch.DispatchKind))
		if kind == "" {
			kind = "PETRI_TRANSITION"
		}
		dispatches = append(dispatches, factorysessionexecution.DispatchSummary{
			ID: dispatch.ID, Status: factorysessionexecution.DispatchStatus(dispatch.Status), DispatchKind: kind,
		})
	}
	return apifactorysession.ListDispatchesResponseToAPI(factorysessionexecution.ListDispatchesResult{
		SessionID: input.SessionID, Dispatches: dispatches,
	}), nil
}

func listFactorySessionArtifacts(
	ctx context.Context,
	service RecordingsInspection,
	input ListArtifactsInput,
) (factoryapi.ListFactorySessionArtifactsResponse, error) {
	if err := validateInspectionRequest(ctx, service, input.SessionID); err != nil {
		return factoryapi.ListFactorySessionArtifactsResponse{}, err
	}
	if _, err := service.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: recordings.RecordingID(input.SessionID),
	}); err != nil {
		return factoryapi.ListFactorySessionArtifactsResponse{}, err
	}
	built, err := service.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: recordings.RecordingID(input.SessionID),
	})
	if err != nil {
		return factoryapi.ListFactorySessionArtifactsResponse{}, err
	}
	if err := ctx.Err(); err != nil {
		return factoryapi.ListFactorySessionArtifactsResponse{}, err
	}
	selectedTick := 0
	for _, event := range built.Artifact.Events {
		if tick := int(event.FactoryTick); tick > selectedTick {
			selectedTick = tick
		}
	}
	reconstructed, err := service.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		Scope:        built.Artifact.Summary.Scope,
		Events:       append([]recordings.CanonicalEvent(nil), built.Artifact.Events...),
		SelectedTick: selectedTick,
	})
	if err != nil {
		return factoryapi.ListFactorySessionArtifactsResponse{}, err
	}
	artifacts, err := artifactStatesFromWorldState(reconstructed.WorldState.Payload)
	if err != nil {
		return factoryapi.ListFactorySessionArtifactsResponse{}, err
	}
	summaries := make([]factorysessionexecution.ArtifactSummary, 0, len(artifacts))
	for _, artifact := range artifacts {
		summaries = append(summaries, artifactSummaryFromState(input.SessionID, artifact))
	}
	return apifactorysession.ListArtifactsResponseToAPI(factorysessionexecution.ListArtifactsResult{
		SessionID: input.SessionID, Artifacts: summaries,
	}), nil
}

func readFactorySessionEvents(
	ctx context.Context,
	service RecordingsInspection,
	input ReadEventsInput,
) (ReadEventsResult, error) {
	if err := validateInspectionRequest(ctx, service, input.SessionID); err != nil {
		return ReadEventsResult{}, err
	}
	status, err := service.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: recordings.RecordingID(input.SessionID),
	})
	if err != nil {
		return ReadEventsResult{}, err
	}
	request := recordings.SubscribeRequest{
		Scope: recordings.CanonicalEventScope{FactorySessionID: input.SessionID},
	}
	afterEventID := strings.TrimSpace(input.AfterEventID)
	if afterEventID == "" && input.AfterSequence != nil {
		if *input.AfterSequence < 0 {
			return ReadEventsResult{}, recordings.ErrInvalidReconnectCursor
		}
		if status.Status.LastEvent == nil {
			return ReadEventsResult{}, recordings.ErrReconnectCursorNotFound
		}
		cursor := *status.Status.LastEvent
		cursor.Sequence = recordings.CanonicalEventSequence(*input.AfterSequence)
		request.Cursor = &cursor
	}
	subscribed, err := service.SubscribeFrom(ctx, request)
	if err != nil {
		return ReadEventsResult{}, err
	}
	events, found, err := consumeRetainedEvents(ctx, subscribed, afterEventID)
	if err != nil {
		return ReadEventsResult{}, err
	}
	if afterEventID != "" && !found {
		return ReadEventsResult{}, recordings.ErrReconnectCursorNotFound
	}
	mapped := make([]factoryapi.FactoryEvent, 0, len(events))
	for _, event := range events {
		mappedEvent, err := canonicalEventToAPI(event)
		if err != nil {
			return ReadEventsResult{}, err
		}
		mapped = append(mapped, mappedEvent)
	}
	return ReadEventsResult{SessionID: input.SessionID, Events: mapped}, nil
}

func validateInspectionRequest(
	ctx context.Context,
	service RecordingsInspection,
	sessionID string,
) error {
	if ctx == nil {
		return errors.New("MCP request context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if service == nil {
		return recordings.ErrServiceUnavailable
	}
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("sessionId is required")
	}
	return nil
}

func canUseLiveDispatchFallback(err error) bool {
	var queryErr *recordings.HistoricalRecordingQueryError
	if errors.As(err, &queryErr) {
		return queryErr.Kind == recordings.HistoricalRecordingQueryErrorUnavailable
	}
	return errors.Is(err, recordings.ErrServiceUnavailable) ||
		errors.Is(err, recordings.ErrMissingRecordingTarget)
}

func liveDispatchesResponse(
	input ListDispatchesInput,
	result factorysessionexecution.ListDispatchesResult,
) factoryapi.ListFactorySessionDispatchesResponse {
	filtered := make([]factorysessionexecution.DispatchSummary, 0, len(result.Dispatches))
	for _, dispatch := range result.Dispatches {
		if strings.TrimSpace(input.Phase) != "" {
			continue
		}
		if status := strings.TrimSpace(input.Status); status != "" && status != string(dispatch.Status) {
			continue
		}
		filtered = append(filtered, dispatch)
	}
	result.SessionID = input.SessionID
	result.Dispatches = filtered
	return apifactorysession.ListDispatchesResponseToAPI(result)
}

func validateDispatchStatus(status string) error {
	switch strings.TrimSpace(status) {
	case "", "COMPLETED", "FAILED", "INTERRUPTED", "QUEUED", "RUNNING":
		return nil
	default:
		return &factorysessionexecution.ExecutionValidationError{Field: "status", Message: "invalid status"}
	}
}

func historicalRecording(
	ctx context.Context,
	service RecordingsInspection,
	sessionID string,
) (recordings.HistoricalRecordingQueryResult, error) {
	status, err := service.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: recordings.RecordingID(sessionID),
	})
	if err != nil {
		return recordings.HistoricalRecordingQueryResult{}, err
	}
	artifact := strings.TrimSpace(string(status.Status.Artifact))
	if artifact == "" {
		return recordings.HistoricalRecordingQueryResult{}, &recordings.HistoricalRecordingQueryError{
			Kind: recordings.HistoricalRecordingQueryErrorUnavailable, RecordingID: recordings.RecordingID(sessionID),
		}
	}
	result, err := service.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{
		Recording: recordings.HistoricalRecordingIdentity{
			RecordingID: recordings.RecordingID(sessionID),
			Artifact:    recordings.RecordingArtifactReference(artifact),
			Scope:       recordings.CanonicalEventScope{FactorySessionID: sessionID},
		},
	})
	if err != nil {
		return recordings.HistoricalRecordingQueryResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return recordings.HistoricalRecordingQueryResult{}, err
	}
	return result, nil
}

func consumeRetainedEvents(
	ctx context.Context,
	subscribed recordings.SubscribeResult,
	afterEventID string,
) ([]recordings.CanonicalEvent, bool, error) {
	if subscribed.Subscription == nil && subscribed.RetainedEventCount > 0 {
		return nil, false, errors.New("recordings subscription is unavailable")
	}
	events := make([]recordings.CanonicalEvent, 0, subscribed.RetainedEventCount)
	found := afterEventID == ""
	for index := 0; index < subscribed.RetainedEventCount; index++ {
		outcome := subscribed.Subscription.Next(ctx)
		switch outcome.Kind {
		case recordings.SubscriptionEvent:
			if !found {
				if string(outcome.Event.ID) == afterEventID {
					found = true
				}
				continue
			}
			events = append(events, outcome.Event)
		case recordings.SubscriptionGap:
			return nil, false, recordings.ErrReconnectCursorExpired
		case recordings.SubscriptionClosed:
			return nil, false, recordings.ErrReconnectCursorNotFound
		default:
			return nil, false, errors.New("recordings subscription returned an unknown outcome")
		}
	}
	return events, found, nil
}

func artifactStatesFromWorldState(payload string) ([]interfaces.FactorySessionArtifactState, error) {
	if strings.TrimSpace(payload) == "" {
		return nil, nil
	}
	var state interfaces.FactoryWorldState
	if err := json.Unmarshal([]byte(payload), &state); err != nil {
		return nil, err
	}
	artifacts := append([]interfaces.FactorySessionArtifactState(nil), state.Artifacts...)
	if state.JavaScriptRuntime != nil {
		artifacts = append(artifacts, state.JavaScriptRuntime.Artifacts...)
	}
	seen := make(map[string]struct{}, len(artifacts))
	deduped := make([]interfaces.FactorySessionArtifactState, 0, len(artifacts))
	for _, artifact := range artifacts {
		id := strings.TrimSpace(artifact.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		artifact.ID = id
		deduped = append(deduped, artifact)
	}
	return deduped, nil
}

func artifactSummaryFromState(
	sessionID string,
	artifact interfaces.FactorySessionArtifactState,
) factorysessionexecution.ArtifactSummary {
	var counts *factorysessionexecution.ArtifactRedactionCounts
	if len(artifact.RedactionCounts) > 0 {
		counts = &factorysessionexecution.ArtifactRedactionCounts{
			Paths:   int32(artifact.RedactionCounts["paths"]),
			Secrets: int32(artifact.RedactionCounts["secrets"]),
			Tokens:  int32(artifact.RedactionCounts["tokens"]),
		}
	}
	var dispatchID string
	if artifact.CaptureMetadata != nil {
		dispatchID = strings.TrimSpace(artifact.CaptureMetadata["sourceDispatchId"])
	}
	return factorysessionexecution.ArtifactSummary{
		ID: artifact.ID, Kind: artifact.Kind, Visibility: artifact.Visibility,
		Label: artifact.Label, ContentHash: artifact.ContentHash, SizeBytes: artifact.SizeBytes,
		CreatedAt: optionalArtifactTime(artifact.CapturedAt), DispatchID: dispatchID,
		AuditMode: artifact.AuditMode, RedactionCounts: counts,
		RetrievalRef: &factorysessionexecution.ArtifactRetrievalRef{
			Href:   fmt.Sprintf("/factory-sessions/%s/artifacts/%s", strings.TrimSpace(sessionID), artifact.ID),
			Method: "GET",
		},
	}
}

func optionalArtifactTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	value = value.UTC()
	return &value
}

func canonicalEventToAPI(event recordings.CanonicalEvent) (factoryapi.FactoryEvent, error) {
	legacy := interfaces.FactoryEvent{
		Id: string(event.ID), Type: interfaces.FactoryEventType(event.Kind),
		Payload: json.RawMessage(event.Payload), SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Context: interfaces.FactoryEventContext{
			EventTime: event.RecordedAt, Sequence: int(event.Sequence), Tick: event.FactoryTick,
		},
	}
	if sessionID := strings.TrimSpace(event.Scope.FactorySessionID); sessionID != "" {
		legacy.Context.SessionID = &sessionID
	}
	if len(event.SourceContext) > 0 && json.Valid([]byte(event.SourceContext)) {
		var context interfaces.FactoryEventContext
		if err := json.Unmarshal([]byte(event.SourceContext), &context); err == nil {
			legacy.Context = context
		}
	}
	return apisurface.FactoryEventToAPI(legacy)
}
