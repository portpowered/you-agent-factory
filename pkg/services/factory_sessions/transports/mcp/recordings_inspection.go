package factorysession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	apifactorysession "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

// The compatibility inspection tools retain their established Factory Session
// names and API shapes, but their canonical facts come from the Recordings
// root. Keeping the conversion here avoids making a peer service's transport
// package part of the Factory Sessions transport dependency graph.
func listFactorySessionDispatches(
	ctx context.Context,
	service recordings.FactorySessionInspectionService,
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
	dispatches := make([]factorysessions.DispatchSummary, 0, len(history.Dispatches))
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
		dispatches = append(dispatches, factorysessions.DispatchSummary{
			ID: dispatch.ID, Status: factorysessions.DispatchStatus(dispatch.Status), DispatchKind: kind,
		})
	}
	return apifactorysession.ListDispatchesResponseToAPI(factorysessions.ListDispatchesResult{
		SessionID: input.SessionID, Dispatches: dispatches,
	}), nil
}

func listFactorySessionArtifacts(
	ctx context.Context,
	service recordings.FactorySessionInspectionService,
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
	summaries := make([]factorysessions.ArtifactSummary, 0, len(artifacts))
	for _, artifact := range artifacts {
		summaries = append(summaries, artifactSummaryFromState(input.SessionID, artifact))
	}
	return apifactorysession.ListArtifactsResponseToAPI(factorysessions.ListArtifactsResult{
		SessionID: input.SessionID, Artifacts: summaries,
	}), nil
}

func readFactorySessionEvents(
	ctx context.Context,
	service recordings.FactorySessionInspectionService,
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
	service recordings.FactorySessionInspectionService,
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
	service recordings.FactorySessionInspectionService,
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
) factorysessions.ArtifactSummary {
	var counts *factorysessions.ArtifactRedactionCounts
	if len(artifact.RedactionCounts) > 0 {
		counts = &factorysessions.ArtifactRedactionCounts{
			Paths:   int32(artifact.RedactionCounts["paths"]),
			Secrets: int32(artifact.RedactionCounts["secrets"]),
			Tokens:  int32(artifact.RedactionCounts["tokens"]),
		}
	}
	var dispatchID string
	if artifact.CaptureMetadata != nil {
		dispatchID = strings.TrimSpace(artifact.CaptureMetadata["sourceDispatchId"])
	}
	return factorysessions.ArtifactSummary{
		ID: artifact.ID, Kind: artifact.Kind, Visibility: artifact.Visibility,
		Label: artifact.Label, ContentHash: artifact.ContentHash, SizeBytes: artifact.SizeBytes,
		CreatedAt: optionalArtifactTime(artifact.CapturedAt), DispatchID: dispatchID,
		AuditMode: artifact.AuditMode, RedactionCounts: counts,
		RetrievalRef: &factorysessions.ArtifactRetrievalRef{
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
