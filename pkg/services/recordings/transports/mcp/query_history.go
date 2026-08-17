package recordingmcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
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

// FactorySessionInspectionService is the narrow Recordings-owned operation
// set consumed by Factory Session inspection tools. The concrete Recordings
// Service satisfies it in production; the legacy bridge below exists only for
// standalone fixture sessions that have no process Recordings ledger.
type FactorySessionInspectionService interface {
	QueryRecordingStatus(recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error)
	QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error)
	BuildPortableArtifact(recordings.BuildPortableArtifactRequest) (recordings.BuildPortableArtifactResult, error)
	ReconstructWorldState(recordings.ReconstructWorldStateRequest) (recordings.ReconstructWorldStateResult, error)
	SubscribeFrom(context.Context, recordings.SubscribeRequest) (recordings.SubscribeResult, error)
}

// LegacyFactorySessionInspection is the compatibility input used only by the
// standalone fixture opener. It is deliberately kept beside the Recordings
// adapter so Factory Sessions does not retain a peer-facing history contract.
type LegacyFactorySessionInspection interface {
	QueryDispatches(context.Context, factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error)
	ListArtifacts(context.Context, string) (factorysessions.ListArtifactsResult, error)
	ReadEvents(context.Context, string, factorysessions.EventReconnectRequest) (factorysessions.EventReadResult, error)
}

type legacyFactorySessionInspection struct {
	legacy LegacyFactorySessionInspection
}

// NewLegacyFactorySessionInspection adapts the standalone fixture execution
// capability to the owner operation set. Runtime-backed MCP always supplies a
// real Recordings Service instead.
func NewLegacyFactorySessionInspection(value any) FactorySessionInspectionService {
	legacy, ok := value.(LegacyFactorySessionInspection)
	if !ok || legacy == nil {
		return nil
	}
	return legacyFactorySessionInspection{legacy: legacy}
}

func (adapter legacyFactorySessionInspection) QueryRecordingStatus(
	request recordings.RecordingStatusRequest,
) (recordings.RecordingStatusResult, error) {
	events, err := adapter.canonicalEvents(context.Background(), string(request.RecordingID))
	if err != nil {
		return recordings.RecordingStatusResult{}, err
	}
	status := recordings.RecordingStatusFacts{
		RecordingID: request.RecordingID,
		Artifact:    recordings.RecordingArtifactReference("standalone://" + string(request.RecordingID)),
		Scope:       recordings.CanonicalEventScope{FactorySessionID: string(request.RecordingID)},
		State:       recordings.RecordingFinalized,
	}
	if len(events) > 0 {
		last := events[len(events)-1].Cursor
		status.LastEvent = &last
	}
	return recordings.RecordingStatusResult{Status: status}, nil
}

func (adapter legacyFactorySessionInspection) QueryHistoricalRecording(
	request recordings.HistoricalRecordingQueryRequest,
) (recordings.HistoricalRecordingQueryResult, error) {
	sessionID := string(request.Recording.RecordingID)
	events, err := adapter.canonicalEvents(context.Background(), sessionID)
	if err != nil {
		return recordings.HistoricalRecordingQueryResult{}, err
	}
	dispatches, err := adapter.legacy.QueryDispatches(context.Background(), factorysessions.DispatchQueryRequest{
		SessionID: sessionID,
	})
	if err != nil {
		return recordings.HistoricalRecordingQueryResult{}, err
	}
	result := recordings.HistoricalRecordingQueryResult{
		Recording: request.Recording,
		Status: recordings.RecordingStatusFacts{
			RecordingID: request.Recording.RecordingID,
			Artifact:    request.Recording.Artifact,
			Scope:       request.Recording.Scope,
			State:       recordings.RecordingFinalized,
		},
		Events: events,
	}
	for _, dispatch := range dispatches.Dispatches {
		result.Dispatches = append(result.Dispatches, recordings.HistoricalDispatch{
			ID: dispatch.ID, Status: recordings.FactoryDispatchStatus(dispatch.Status),
			DispatchKind: recordings.FactoryDispatchKind(dispatch.DispatchKind),
		})
	}
	return result, nil
}

func (adapter legacyFactorySessionInspection) BuildPortableArtifact(
	request recordings.BuildPortableArtifactRequest,
) (recordings.BuildPortableArtifactResult, error) {
	events, err := adapter.canonicalEvents(context.Background(), string(request.RecordingID))
	if err != nil {
		return recordings.BuildPortableArtifactResult{}, err
	}
	summary := recordings.PortableArtifactSummary{
		RecordingID: request.RecordingID,
		Reference:   recordings.RecordingArtifactReference("standalone://" + string(request.RecordingID)),
		Scope:       recordings.CanonicalEventScope{FactorySessionID: string(request.RecordingID)},
		State:       recordings.RecordingFinalized,
		EventCount:  len(events),
		Available:   true,
	}
	if len(events) > 0 {
		first, last := events[0].Cursor, events[len(events)-1].Cursor
		summary.FirstCursor = &first
		summary.LastCursor = &last
	}
	return recordings.BuildPortableArtifactResult{Artifact: recordings.PortableArtifact{
		SchemaVersion: recordings.PortableArtifactSchemaV1,
		Summary:       summary,
		Events:        events,
	}}, nil
}

func (adapter legacyFactorySessionInspection) ReconstructWorldState(
	request recordings.ReconstructWorldStateRequest,
) (recordings.ReconstructWorldStateResult, error) {
	artifacts, err := adapter.legacy.ListArtifacts(context.Background(), request.Scope.FactorySessionID)
	if err != nil {
		return recordings.ReconstructWorldStateResult{}, err
	}
	states := make([]factorydefinitions.FactorySessionArtifactState, 0, len(artifacts.Artifacts))
	for _, artifact := range artifacts.Artifacts {
		states = append(states, factorydefinitions.FactorySessionArtifactState{
			ID: artifact.ID, Kind: artifact.Kind, Visibility: artifact.Visibility,
			Label: artifact.Label, ContentHash: artifact.ContentHash,
			SizeBytes: artifact.SizeBytes, CaptureMetadata: map[string]string{
				"sourceDispatchId": artifact.DispatchID,
			},
		})
	}
	payload, err := json.Marshal(factorydefinitions.FactoryWorldState{Artifacts: states})
	if err != nil {
		return recordings.ReconstructWorldStateResult{}, err
	}
	return recordings.ReconstructWorldStateResult{
		WorldState: recordings.WorldStateView{
			SchemaVersion: recordings.WorldStateViewSchemaV1,
			Scope:         request.Scope,
			Payload:       string(payload),
		},
	}, nil
}

func (adapter legacyFactorySessionInspection) SubscribeFrom(
	ctx context.Context,
	request recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	reconnect := factorysessions.EventReconnectRequest{}
	if request.Cursor != nil {
		sequence := int(request.Cursor.Sequence)
		reconnect.AfterSequence = &sequence
	}
	events, err := adapter.legacy.ReadEvents(ctx, request.Scope.FactorySessionID, reconnect)
	if err != nil {
		return recordings.SubscribeResult{}, err
	}
	canonical, err := adapter.canonicalEventsFromResult(request.Scope.FactorySessionID, events)
	if err != nil {
		return recordings.SubscribeResult{}, err
	}
	return recordings.SubscribeResult{
		RetainedEventCount: len(canonical),
		Subscription: func(next context.Context) recordings.SubscriptionOutcome {
			if len(canonical) == 0 {
				return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
			}
			if err := next.Err(); err != nil {
				return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
			}
			event := canonical[0]
			canonical = canonical[1:]
			return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: event}
		},
	}, nil
}

func (adapter legacyFactorySessionInspection) canonicalEvents(
	ctx context.Context,
	sessionID string,
) ([]recordings.CanonicalEvent, error) {
	result, err := adapter.legacy.ReadEvents(ctx, sessionID, factorysessions.EventReconnectRequest{})
	if err != nil {
		return nil, err
	}
	return adapter.canonicalEventsFromResult(sessionID, result)
}

func (legacyFactorySessionInspection) canonicalEventsFromResult(
	sessionID string,
	result factorysessions.EventReadResult,
) ([]recordings.CanonicalEvent, error) {
	events := make([]recordings.CanonicalEvent, 0, len(result.Events))
	for _, raw := range result.Events {
		var event factorydefinitions.FactoryEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			return nil, fmt.Errorf("decode standalone Factory Event: %w", err)
		}
		contextPayload, err := json.Marshal(event.Context)
		if err != nil {
			return nil, err
		}
		eventSessionID := sessionID
		if event.Context.SessionID != nil {
			eventSessionID = *event.Context.SessionID
		}
		sequence := recordings.CanonicalEventSequence(event.Context.Sequence)
		events = append(events, recordings.CanonicalEvent{
			ID: recordings.CanonicalEventID(event.Id), Sequence: sequence,
			FactoryTick: event.Context.Tick,
			Scope:       recordings.CanonicalEventScope{FactorySessionID: eventSessionID},
			Cursor: recordings.CanonicalEventCursor{
				StreamGenerationID: "standalone-factory-session/" + sessionID,
				Sequence:           sequence,
			},
			RecordedAt: event.Context.EventTime, Kind: recordings.CanonicalEventKind(event.Type),
			Payload: string(event.Payload), SourceContext: string(contextPayload),
		})
	}
	return events, nil
}

// ErrServiceUnavailable keeps the compatibility envelope stable when the
// Recordings owner has not been bound into a Factory Sessions transport.
var ErrServiceUnavailable = errors.New("recordings service is required")

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
	dispatches := make([]factorysessions.DispatchSummary, 0, len(history.Dispatches))
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
		dispatches = append(dispatches, factorysessions.DispatchSummary{
			ID: dispatch.ID, Status: factorysessions.DispatchStatus(dispatch.Status), DispatchKind: kind,
		})
	}
	return factorysessionmapping.ListDispatchesResponseToAPI(factorysessions.ListDispatchesResult{
		SessionID: input.SessionID, Dispatches: dispatches,
	}), nil
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
		return &factorysessions.ExecutionValidationError{Field: "status", Message: "invalid status"}
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
