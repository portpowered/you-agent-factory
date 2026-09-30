package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
)

type inspectionDurableFake struct {
	durableexecution.Service
	eventID, dispatchID, artifactID                                string
	reconnect                                                      factorysessions.EventReconnectRequest
	eventCalls, dispatchCalls, artifactListCalls, artifactGetCalls int
	eventError                                                     error
	eventPayload                                                   json.RawMessage
}

func (fake *inspectionDurableFake) ReadEvents(_ context.Context, sessionID string, request factorysessions.EventReconnectRequest) (factorysessions.EventReadResult, error) {
	fake.eventCalls++
	fake.eventID, fake.reconnect = sessionID, request
	if fake.eventError != nil {
		return factorysessions.EventReadResult{}, fake.eventError
	}
	return factorysessions.EventReadResult{SessionID: sessionID, Events: []json.RawMessage{fake.eventPayload}}, nil
}

func (fake *inspectionDurableFake) GetDispatch(_ context.Context, sessionID, dispatchID string) (factorysessions.DispatchDetail, error) {
	fake.dispatchCalls++
	fake.eventID, fake.dispatchID = sessionID, dispatchID
	return factorysessions.DispatchDetail{SessionID: sessionID, DispatchSummary: factorysessions.DispatchSummary{ID: dispatchID}}, nil
}

func (fake *inspectionDurableFake) ListArtifacts(_ context.Context, sessionID string) (factorysessions.ListArtifactsResult, error) {
	fake.artifactListCalls++
	fake.eventID = sessionID
	return factorysessions.ListArtifactsResult{SessionID: sessionID, Artifacts: []factorysessions.ArtifactSummary{{ID: "artifact-1"}}}, nil
}

func (fake *inspectionDurableFake) GetArtifact(_ context.Context, sessionID, artifactID string) (factorysessions.ArtifactDetail, error) {
	fake.artifactGetCalls++
	fake.eventID, fake.artifactID = sessionID, artifactID
	return factorysessions.ArtifactDetail{SessionID: sessionID, ArtifactSummary: factorysessions.ArtifactSummary{ID: artifactID}}, nil
}

func TestSessionInspectionCommandsReadOneDurableSession(t *testing.T) {
	fake := &inspectionDurableFake{eventPayload: json.RawMessage(`{"id":"one"}`)}
	service := &Service{durable: fake}
	assertInspectionEvents(t, service, fake)
	assertInspectionDispatchArtifacts(t, service, fake)
}

func assertInspectionEvents(t *testing.T, service *Service, fake *inspectionDurableFake) {
	t.Helper()
	ctx := context.Background()
	sequence := 3
	events, err := service.QueryEvents(ctx, factorysessions.SessionEventQueryRequest{
		SessionID: " session-1 ", Reconnect: factorysessions.EventReconnectRequest{AfterEventID: " event-2 ", AfterSequence: &sequence},
	})
	if err != nil || events.SessionID != "session-1" || fake.eventID != "session-1" || fake.reconnect.AfterEventID != "event-2" {
		t.Fatalf("QueryEvents = (%#v, %v), request = %#v", events, err, fake.reconnect)
	}
	events.Events[0][0] = 'x'
	if string(fake.eventPayload) != `{"id":"one"}` {
		t.Fatalf("QueryEvents returned mutable durable event storage: %q", fake.eventPayload)
	}
	if err := service.ProbeEvents(ctx, factorysessions.SessionEventQueryRequest{SessionID: "session-1"}); err != nil || fake.eventCalls != 2 {
		t.Fatalf("ProbeEvents error = %v, calls = %d", err, fake.eventCalls)
	}
}

func assertInspectionDispatchArtifacts(t *testing.T, service *Service, fake *inspectionDurableFake) {
	t.Helper()
	ctx := context.Background()
	dispatch, err := service.InspectDispatch(ctx, factorysessions.SessionDispatchInspectRequest{SessionID: " session-1 ", DispatchID: " dispatch-1 "})
	if err != nil || dispatch.ID != "dispatch-1" || fake.dispatchID != "dispatch-1" {
		t.Fatalf("InspectDispatch = (%#v, %v), forwarded ID = %q", dispatch, err, fake.dispatchID)
	}
	artifacts, err := service.QueryArtifacts(ctx, factorysessions.SessionArtifactQueryRequest{SessionID: " session-1 "})
	if err != nil || len(artifacts.Artifacts) != 1 || fake.artifactListCalls != 1 {
		t.Fatalf("QueryArtifacts = (%#v, %v), calls = %d", artifacts, err, fake.artifactListCalls)
	}
	artifact, err := service.InspectArtifact(ctx, factorysessions.SessionArtifactInspectRequest{SessionID: " session-1 ", ArtifactID: " artifact-1 "})
	if err != nil || artifact.ID != "artifact-1" || fake.artifactID != "artifact-1" {
		t.Fatalf("InspectArtifact = (%#v, %v), forwarded ID = %q", artifact, err, fake.artifactID)
	}
}

func TestSessionInspectionRejectsInvalidRequestsBeforeDurableRead(t *testing.T) {
	ctx := context.Background()
	fake := &inspectionDurableFake{}
	service := &Service{durable: fake}
	negative := -1
	if _, err := service.QueryEvents(ctx, factorysessions.SessionEventQueryRequest{SessionID: "session-1", Reconnect: factorysessions.EventReconnectRequest{AfterSequence: &negative}}); err == nil {
		t.Fatal("QueryEvents accepted negative sequence")
	}
	if _, err := service.InspectDispatch(ctx, factorysessions.SessionDispatchInspectRequest{SessionID: "session-1"}); err == nil {
		t.Fatal("InspectDispatch accepted empty dispatch ID")
	}
	if _, err := service.QueryArtifacts(ctx, factorysessions.SessionArtifactQueryRequest{}); err == nil {
		t.Fatal("QueryArtifacts accepted empty session ID")
	}
	if _, err := service.InspectArtifact(ctx, factorysessions.SessionArtifactInspectRequest{SessionID: "session-1"}); err == nil {
		t.Fatal("InspectArtifact accepted empty artifact ID")
	}
	if fake.eventCalls+fake.dispatchCalls+fake.artifactListCalls+fake.artifactGetCalls != 0 {
		t.Fatal("invalid request reached durable execution")
	}
	want := errors.New("cursor unavailable")
	fake.eventError = want
	if err := service.ProbeEvents(ctx, factorysessions.SessionEventQueryRequest{SessionID: "session-1"}); !errors.Is(err, want) {
		t.Fatalf("ProbeEvents error = %v, want %v", err, want)
	}
}
