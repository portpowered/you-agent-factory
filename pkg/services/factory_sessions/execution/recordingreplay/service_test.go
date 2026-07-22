package recordingreplay

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution"
	recording "github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestServiceExposesRecordedSessionResultAndEvents(t *testing.T) {
	t.Parallel()
	projection, err := ReplayRecording(buildTerminalRecording(t, "SUCCEEDED", terminalResult()))
	if err != nil {
		t.Fatalf("ReplayRecording: %v", err)
	}
	service := NewService(projection)
	ctx := context.Background()

	session, err := service.GetSession(ctx, projection.Session.SessionID)
	if err != nil || session.SessionID != projection.Session.SessionID {
		t.Fatalf("GetSession = %#v, %v", session, err)
	}
	result, err := service.GetResult(ctx, session.SessionID, fse.ResultRequest{Mode: fse.ResultModeFinal, IncludeArtifacts: true})
	if err != nil || len(result.ArtifactRefs) != 1 || !result.IncludeArtifacts {
		t.Fatalf("GetResult with artifacts = %#v, %v", result, err)
	}
	result, err = service.GetResult(ctx, session.SessionID, fse.ResultRequest{Mode: fse.ResultModePartial})
	if err != nil || result.Mode != fse.ResultModePartial || len(result.ArtifactRefs) != 0 {
		t.Fatalf("GetResult without artifacts = %#v, %v", result, err)
	}
	if _, err := service.GetResult(ctx, session.SessionID, fse.ResultRequest{Mode: "invalid"}); err == nil {
		t.Fatal("GetResult invalid mode succeeded")
	}

	events, err := service.ReadEvents(ctx, session.SessionID, fse.EventReconnectRequest{})
	if err != nil || len(events.Events) != len(projection.Events.Events) {
		t.Fatalf("ReadEvents = %#v, %v", events, err)
	}
}

func TestServiceExposesRecordedArtifactsAndEmptyDispatches(t *testing.T) {
	t.Parallel()
	projection, err := ReplayRecording(buildTerminalRecording(t, "SUCCEEDED", terminalResult()))
	if err != nil {
		t.Fatalf("ReplayRecording: %v", err)
	}
	service := NewService(projection)
	ctx := context.Background()
	sessionID := projection.Session.SessionID

	artifacts, err := service.ListArtifacts(ctx, sessionID)
	if err != nil || len(artifacts.Artifacts) != 1 {
		t.Fatalf("ListArtifacts = %#v, %v", artifacts, err)
	}
	artifact, err := service.GetArtifact(ctx, sessionID, artifacts.Artifacts[0].ID)
	if err != nil || artifact.ID != artifacts.Artifacts[0].ID {
		t.Fatalf("GetArtifact = %#v, %v", artifact, err)
	}
	if _, err := service.GetArtifact(ctx, sessionID, "missing"); !errors.Is(err, fse.ErrArtifactNotFound) {
		t.Fatalf("GetArtifact missing error = %v", err)
	}
	dispatches, err := service.ListDispatches(ctx, sessionID)
	if err != nil || len(dispatches.Dispatches) != 0 {
		t.Fatalf("ListDispatches = %#v, %v", dispatches, err)
	}
	if _, err := service.GetDispatch(ctx, sessionID, "missing"); !errors.Is(err, fse.ErrDispatchNotFound) {
		t.Fatalf("GetDispatch error = %v", err)
	}

	sessions, err := service.ListSessions(ctx, fse.ListSessionsRequest{})
	if err != nil || len(sessions.DurableSessions) != 1 || sessions.DurableSessions[0].SessionID != sessionID {
		t.Fatalf("ListSessions = %#v, %v", sessions, err)
	}
}

func TestServiceRejectsUnknownSessionsAndLiveOperations(t *testing.T) {
	t.Parallel()
	projection, err := ReplayRecording(buildTerminalRecording(t, "SUCCEEDED", terminalResult()))
	if err != nil {
		t.Fatalf("ReplayRecording: %v", err)
	}
	service := NewService(projection)
	ctx := context.Background()

	if _, err := service.GetSession(ctx, "missing"); !errors.Is(err, fse.ErrSessionNotFound) {
		t.Fatalf("GetSession missing error = %v", err)
	}
	var nilService *Service
	if _, err := nilService.GetSession(ctx, projection.Session.SessionID); !errors.Is(err, fse.ErrSessionNotFound) {
		t.Fatalf("nil GetSession error = %v", err)
	}

	operations := []struct {
		name string
		run  func() error
	}{
		{"start async", func() error { _, err := service.StartAsync(ctx, fse.StartRequest{}); return err }},
		{"start sync", func() error { _, err := service.StartSync(ctx, fse.StartRequest{}); return err }},
		{"resume interrupted", func() error {
			_, err := service.ResumeInterruptedSession(ctx, projection.Session.SessionID, fse.ResumeSessionRequest{})
			return err
		}},
		{"pause", func() error {
			_, err := service.Pause(ctx, projection.Session.SessionID, fse.ControlRequest{})
			return err
		}},
		{"resume", func() error {
			_, err := service.Resume(ctx, projection.Session.SessionID, fse.ControlRequest{})
			return err
		}},
		{"cancel", func() error {
			_, err := service.Cancel(ctx, projection.Session.SessionID, fse.ControlRequest{})
			return err
		}},
		{"terminate", func() error {
			_, err := service.Terminate(ctx, projection.Session.SessionID, fse.ControlRequest{})
			return err
		}},
		{"approve", func() error {
			_, err := service.Approve(ctx, projection.Session.SessionID, fse.ApproveRequest{})
			return err
		}},
		{"retry dispatch", func() error {
			_, err := service.RetryDispatch(ctx, projection.Session.SessionID, fse.RetryDispatchRequest{})
			return err
		}},
		{"interrupt dispatch", func() error {
			_, err := service.InterruptDispatch(ctx, projection.Session.SessionID, fse.InterruptDispatchRequest{})
			return err
		}},
	}
	for _, operation := range operations {
		operation := operation
		t.Run(operation.name, func(t *testing.T) {
			t.Parallel()
			if err := operation.run(); !errors.Is(err, ErrNonLiveReplay) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func terminalResult() *recording.PortableRecordingCanonicalResult {
	return &recording.PortableRecordingCanonicalResult{
		Status: "FINAL", Mode: "final", PrimaryResult: json.RawMessage(`{"answer":"done"}`), ArtifactIDs: []string{"artifact-result"},
	}
}
