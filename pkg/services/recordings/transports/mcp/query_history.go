package recordingmcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingshttp "github.com/portpowered/infinite-you/pkg/services/recordings/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

// QueryHistoryInput is the MCP request shape for
// you.recording.query_history. The identity fields are explicit so a caller
// cannot accidentally read a recording outside its Factory Session scope.
type QueryHistoryInput struct {
	RecordingID      string `json:"recordingId"`
	Artifact         string `json:"artifact"`
	FactorySessionID string `json:"factorySessionId"`
}

// QueryHistory returns ordered canonical history and detached projections
// through the you.recording.query_history MCP tool.
func QueryHistory(
	ctx context.Context,
	service recordings.Service,
	input QueryHistoryInput,
) ToolResponse[recordings.HistoricalRecordingQueryResult] {
	if ctx == nil {
		envelope := executionErrorEnvelope(input.RecordingID, errMissingRequestContext)
		return ToolResponse[recordings.HistoricalRecordingQueryResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[recordings.HistoricalRecordingQueryResult](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[recordings.HistoricalRecordingQueryResult]{Error: &envelope}
	}

	result, err := service.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{
		Recording: recordings.HistoricalRecordingIdentity{
			RecordingID: recordings.RecordingID(strings.TrimSpace(input.RecordingID)),
			Artifact:    recordings.RecordingArtifactReference(strings.TrimSpace(input.Artifact)),
			Scope: recordings.CanonicalEventScope{
				FactorySessionID: strings.TrimSpace(input.FactorySessionID),
			},
		},
	})
	if err != nil {
		envelope := historicalQueryErrorEnvelope(input.RecordingID, err)
		return ToolResponse[recordings.HistoricalRecordingQueryResult]{Error: &envelope}
	}
	return ToolResponse[recordings.HistoricalRecordingQueryResult]{Result: &result}
}

func historicalQueryErrorEnvelope(recordingID string, err error) ToolErrorEnvelope {
	var typed *recordings.HistoricalRecordingQueryError
	if errors.As(err, &typed) {
		code := "recording.history.internal"
		message := "historical recording query failed"
		retryable := false
		switch typed.Kind {
		case recordings.HistoricalRecordingQueryErrorInvalidRequest:
			code = "recording.history.invalid"
			message = "invalid historical recording query"
		case recordings.HistoricalRecordingQueryErrorMissingHistory:
			code = "recording.history.not_found"
			message = "historical recording history not found"
		case recordings.HistoricalRecordingQueryErrorCorruptHistory:
			code = "recording.history.corrupt"
			message = "historical recording history is corrupt"
		case recordings.HistoricalRecordingQueryErrorUnavailable:
			code = "recording.history.unavailable"
			message = "historical recording history is unavailable"
			retryable = true
		}
		return ToolErrorEnvelope{
			Code:        code,
			Message:     message,
			Retryable:   retryable,
			RecordingID: strings.TrimSpace(recordingID),
			Details: map[string]any{
				"reason": string(typed.Kind),
			},
		}
	}
	return executionErrorEnvelope(recordingID, err)
}

// FactorySessionInspectionService is the local compatibility seam used by the
// standalone Recordings transport adapter. Factory Sessions owns its own
// consumer view so this transport package does not publish a peer-facing
// service-root interface.
type FactorySessionInspectionService interface {
	QueryRecordingStatus(recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error)
	QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error)
	BuildPortableArtifact(recordings.BuildPortableArtifactRequest) (recordings.BuildPortableArtifactResult, error)
	ReconstructWorldState(recordings.ReconstructWorldStateRequest) (recordings.ReconstructWorldStateResult, error)
	SubscribeFrom(context.Context, recordings.SubscribeRequest) (recordings.SubscribeResult, error)
}

// ErrServiceUnavailable keeps the compatibility envelope stable when the
// Recordings owner has not been bound into a Factory Sessions transport.
var ErrServiceUnavailable = recordings.ErrServiceUnavailable

// FactorySessionListDispatchesInput preserves the established MCP request
// shape while routing the read through Recordings.
type FactorySessionListDispatchesInput struct {
	SessionID string `json:"sessionId"`
	Phase     string `json:"phase,omitempty"`
	Status    string `json:"status,omitempty"`
}

// FactorySessionListArtifactsInput preserves the established MCP request
// shape while routing the read through Recordings.
type FactorySessionListArtifactsInput struct {
	SessionID string `json:"sessionId"`
}

// FactorySessionReadEventsInput preserves the established reconnect request
// shape while routing canonical event reads through Recordings.
type FactorySessionReadEventsInput struct {
	SessionID     string `json:"sessionId"`
	AfterEventID  string `json:"afterEventId,omitempty"`
	AfterSequence *int   `json:"afterSequence,omitempty"`
}

// FactorySessionReadEventsResult preserves the established MCP result shape.
type FactorySessionReadEventsResult struct {
	SessionID string                    `json:"sessionId"`
	Events    []factoryapi.FactoryEvent `json:"events,omitempty"`
}

// ListFactorySessionDispatches serves the compatibility tool through the
// Recordings historical query. The public envelope and tool name remain owned
// by the Factory Sessions protocol surface until the shared MCP registry cutover.
func ListFactorySessionDispatches(
	ctx context.Context,
	service FactorySessionInspectionService,
	input FactorySessionListDispatchesInput,
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
	dispatches := make([]factorysessionmapping.HistoricalDispatchInput, 0, len(history.Dispatches))
	for _, dispatch := range history.Dispatches {
		// HistoricalDispatch deliberately exposes no mutable workflow phase. An
		// explicit phase filter therefore has the same empty-result behavior as
		// the HTTP historical adapter.
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
		dispatches = append(dispatches, factorysessionmapping.HistoricalDispatchInput{
			ID: dispatch.ID, Status: string(dispatch.Status), DispatchKind: kind,
			ConfirmationState: "CONFIRMED", Usage: dispatch.Usage,
		})
	}
	return factorysessionmapping.HistoricalDispatchListToAPI(input.SessionID, dispatches), nil
}

// ListFactorySessionArtifacts serves the compatibility tool from detached
// Recordings artifact and world-state projections.
func ListFactorySessionArtifacts(
	ctx context.Context,
	service FactorySessionInspectionService,
	input FactorySessionListArtifactsInput,
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
	reconstructed, err := service.ReconstructWorldState(
		recordingshttp.ReconstructWorldStateRequestFromPortableArtifact(built.Artifact),
	)
	if err != nil {
		return factoryapi.ListFactorySessionArtifactsResponse{}, err
	}
	artifacts, err := recordingshttp.ArtifactStatesFromWorldStatePayload(reconstructed.WorldState.Payload)
	if err != nil {
		return factoryapi.ListFactorySessionArtifactsResponse{}, err
	}
	return recordingshttp.ArtifactListResponseToAPI(input.SessionID, artifacts), nil
}

// ReadFactorySessionEvents serves the compatibility reconnect tool from the
// Recordings canonical ledger. It consumes only the retained prefix, so the
// request returns without creating a transport-specific live stream.
func ReadFactorySessionEvents(
	ctx context.Context,
	service FactorySessionInspectionService,
	input FactorySessionReadEventsInput,
) (FactorySessionReadEventsResult, error) {
	if err := validateInspectionRequest(ctx, service, input.SessionID); err != nil {
		return FactorySessionReadEventsResult{}, err
	}
	status, err := service.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: recordings.RecordingID(input.SessionID),
	})
	if err != nil {
		return FactorySessionReadEventsResult{}, err
	}
	request := recordings.SubscribeRequest{
		Scope: recordings.CanonicalEventScope{FactorySessionID: input.SessionID},
	}
	afterEventID := strings.TrimSpace(input.AfterEventID)
	if afterEventID == "" && input.AfterSequence != nil {
		if *input.AfterSequence < 0 {
			return FactorySessionReadEventsResult{}, recordings.ErrInvalidReconnectCursor
		}
		if status.Status.LastEvent == nil {
			return FactorySessionReadEventsResult{}, recordings.ErrReconnectCursorNotFound
		}
		cursor := *status.Status.LastEvent
		cursor.Sequence = recordings.CanonicalEventSequence(*input.AfterSequence)
		request.Cursor = &cursor
	}
	subscribed, err := service.SubscribeFrom(ctx, request)
	if err != nil {
		return FactorySessionReadEventsResult{}, err
	}
	events, found, err := consumeRetainedEvents(ctx, subscribed, afterEventID)
	if err != nil {
		return FactorySessionReadEventsResult{}, err
	}
	if afterEventID != "" && !found {
		return FactorySessionReadEventsResult{}, recordings.ErrReconnectCursorNotFound
	}
	mapped := make([]factoryapi.FactoryEvent, 0, len(events))
	for _, event := range events {
		mappedEvent, err := recordingshttp.FactoryEventToAPI(event)
		if err != nil {
			return FactorySessionReadEventsResult{}, err
		}
		mapped = append(mapped, mappedEvent)
	}
	return FactorySessionReadEventsResult{SessionID: input.SessionID, Events: mapped}, nil
}

func validateInspectionRequest(ctx context.Context, service FactorySessionInspectionService, sessionID string) error {
	if ctx == nil {
		return errors.New("MCP request context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if service == nil {
		return ErrServiceUnavailable
	}
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("sessionId is required")
	}
	return nil
}

func validateDispatchStatus(status string) error {
	switch strings.TrimSpace(status) {
	case "", "COMPLETED", "FAILED", "INTERRUPTED", "QUEUED", "RUNNING":
		return nil
	default:
		return factorysessionmapping.NewExecutionValidationError("status", "invalid status")
	}
}

func historicalRecording(
	ctx context.Context,
	service FactorySessionInspectionService,
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
			Kind:        recordings.HistoricalRecordingQueryErrorUnavailable,
			RecordingID: recordings.RecordingID(sessionID),
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
