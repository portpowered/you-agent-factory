package factorysession_test

import (
	"context"
	"reflect"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestLiveAPI_MapsCompleteLifecycleThroughNarrowControl(t *testing.T) {
	t.Parallel()

	control := &liveControlSpy{}
	api := factorysession.NewLiveAPI(control, nil)
	ctx := context.Background()
	name := "goal"

	opened, err := api.OpenFactorySession(ctx, factoryapi.OpenFactorySessionRequest{
		FolderPath: "/workspace",
		Target: &factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKindNamed,
			Name: &name,
		},
	})
	if err != nil {
		t.Fatalf("OpenFactorySession: %v", err)
	}
	if opened.Session == nil || opened.Session.Id != liveControlSessionID {
		t.Fatalf("opened session = %#v, want %q", opened.Session, liveControlSessionID)
	}
	if control.openRequest.Target == nil || control.openRequest.Target.Name != name {
		t.Fatalf("open request = %#v, want named target %q", control.openRequest, name)
	}

	listed, err := api.ListFactorySessions(ctx)
	if err != nil {
		t.Fatalf("ListFactorySessions: %v", err)
	}
	if len(listed.Sessions) != 1 || listed.Sessions[0].Id != liveControlSessionID {
		t.Fatalf("listed sessions = %#v, want %q", listed.Sessions, liveControlSessionID)
	}

	read, err := api.GetFactorySession(ctx, liveControlSessionID)
	if err != nil {
		t.Fatalf("GetFactorySession: %v", err)
	}
	if read.Id != liveControlSessionID {
		t.Fatalf("read session = %#v, want %q", read, liveControlSessionID)
	}

	pause, err := api.PauseLiveFactorySession(ctx, liveControlSessionID, factorysessions.LiveControlRequest{
		RequestID: "pause-1",
		Reason:    "operator pause",
	})
	if err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}
	if string(pause.Operation) != "PAUSE" || string(pause.Outcome) != "ACCEPTED" {
		t.Fatalf("pause response = %#v, want accepted pause", pause)
	}

	resume, err := api.ResumeLiveFactorySession(ctx, liveControlSessionID, factorysessions.LiveControlRequest{
		RequestID: "resume-1",
		Reason:    "operator resume",
	})
	if err != nil {
		t.Fatalf("ResumeLiveFactorySession: %v", err)
	}
	if string(resume.Operation) != "RESUME" || string(resume.Outcome) != "ACCEPTED" {
		t.Fatalf("resume response = %#v, want accepted resume", resume)
	}

	if err := api.CloseFactorySession(ctx, liveControlSessionID); err != nil {
		t.Fatalf("CloseFactorySession: %v", err)
	}

	if got, want := control.calls, []string{"open", "list", "get", "pause", "resume", "delete"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("control calls = %#v, want %#v", got, want)
	}
	if control.pauseRequest.RequestID != "pause-1" || control.resumeRequest.RequestID != "resume-1" || control.deletedID != liveControlSessionID {
		t.Fatalf("control requests = pause:%#v resume:%#v delete:%q", control.pauseRequest, control.resumeRequest, control.deletedID)
	}
}

func TestLiveAPI_MapsCancelAndTerminateThroughLifecycleCapability(t *testing.T) {
	t.Parallel()

	control := &liveControlSpy{}
	api := factorysession.NewLiveAPI(control, nil)
	ctx := context.Background()

	canceled, err := api.CancelLiveFactorySession(ctx, liveControlSessionID, factorysessions.LiveControlRequest{RequestID: "cancel-1"})
	if err != nil {
		t.Fatalf("CancelLiveFactorySession: %v", err)
	}
	if canceled.Operation != factoryapi.FactorySessionLifecycleControlKindCancel || canceled.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("cancel response = %#v, want accepted CANCEL", canceled)
	}

	terminated, err := api.TerminateLiveFactorySession(ctx, liveControlSessionID, factorysessions.LiveControlRequest{RequestID: "terminate-1"})
	if err != nil {
		t.Fatalf("TerminateLiveFactorySession: %v", err)
	}
	if terminated.Operation != factoryapi.FactorySessionLifecycleControlKindTerminate || terminated.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("terminate response = %#v, want accepted TERMINATE", terminated)
	}
	if got, want := control.calls, []string{"cancel", "terminate"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("control calls = %#v, want %#v", got, want)
	}
	if control.cancelRequest.RequestID != "cancel-1" || control.terminateRequest.RequestID != "terminate-1" {
		t.Fatalf("control requests = cancel:%#v terminate:%#v", control.cancelRequest, control.terminateRequest)
	}
}

const liveControlSessionID = "live-session-1"

type liveControlSpy struct {
	calls            []string
	openRequest      factorysessions.LiveControlOpenRequest
	pauseRequest     factorysessions.LiveControlRequest
	resumeRequest    factorysessions.LiveControlRequest
	cancelRequest    factorysessions.LiveControlRequest
	terminateRequest factorysessions.LiveControlRequest
	deletedID        string
}

var _ factorysessions.LiveControlService = (*liveControlSpy)(nil)
var _ factorysessions.LiveLifecycleControlService = (*liveControlSpy)(nil)

func (s *liveControlSpy) OpenFactorySession(
	_ context.Context,
	request factorysessions.LiveControlOpenRequest,
) (*factorysessions.LiveControlOpenResult, error) {
	s.calls = append(s.calls, "open")
	s.openRequest = request
	return &factorysessions.OpenResult{SessionID: liveControlSessionID, Session: liveControlSummary()}, nil
}

func (s *liveControlSpy) ListFactorySessions(context.Context) ([]factorysessions.LiveControlListItem, error) {
	s.calls = append(s.calls, "list")
	return []factorysessions.ReadProjection{{
		Context: factorysessions.ProjectionContext{
			Session: liveControlSummary(), FactorySessionID: liveControlSessionID,
		},
	}}, nil
}

func (s *liveControlSpy) GetFactorySession(context.Context, string) (factorysessions.LiveControlSnapshot, error) {
	s.calls = append(s.calls, "get")
	return factorysessions.SessionProjection{
		Context: factorysessions.ProjectionContext{
			Session: liveControlSummary(), FactorySessionID: liveControlSessionID,
		},
	}, nil
}

func (s *liveControlSpy) PauseLiveFactorySession(
	_ context.Context,
	_ string,
	request factorysessions.LiveControlRequest,
) (factorysessions.LiveControlResult, error) {
	s.calls = append(s.calls, "pause")
	s.pauseRequest = request
	return liveControlResult("PAUSE", "PAUSED"), nil
}

func (s *liveControlSpy) ResumeLiveFactorySession(
	_ context.Context,
	_ string,
	request factorysessions.LiveControlRequest,
) (factorysessions.LiveControlResult, error) {
	s.calls = append(s.calls, "resume")
	s.resumeRequest = request
	return liveControlResult("RESUME", "RUNNING"), nil
}

func (s *liveControlSpy) CancelLiveFactorySession(
	_ context.Context,
	_ string,
	request factorysessions.LiveControlRequest,
) (factorysessions.LiveControlResult, error) {
	s.calls = append(s.calls, "cancel")
	s.cancelRequest = request
	return liveControlResult("CANCEL", "SUCCEEDED"), nil
}

func (s *liveControlSpy) TerminateLiveFactorySession(
	_ context.Context,
	_ string,
	request factorysessions.LiveControlRequest,
) (factorysessions.LiveControlResult, error) {
	s.calls = append(s.calls, "terminate")
	s.terminateRequest = request
	return liveControlResult("TERMINATE", "SUCCEEDED"), nil
}

func (s *liveControlSpy) CloseFactorySession(_ context.Context, sessionID string) error {
	s.calls = append(s.calls, "close")
	s.deletedID = sessionID
	return nil
}

func (s *liveControlSpy) DeleteFactorySession(_ context.Context, sessionID string) error {
	s.calls = append(s.calls, "delete")
	s.deletedID = sessionID
	return nil
}

func liveControlSummary() *factorysessions.ScopedLiveSessionSummary {
	return &factorysessions.ScopedLiveSessionSummary{
		ID:         liveControlSessionID,
		FactoryDir: "/workspace/factory/goal",
		FolderPath: "/workspace",
		Project:    "demo",
		Target: factorysessions.TargetRef{
			Kind: factorysessions.TargetKindNamed,
			Name: "goal",
		},
	}
}

func liveControlResult(operation, status string) factorysessions.LiveControlResult {
	return factorysessions.LifecycleControlResult{
		SessionID: liveControlSessionID,
		Operation: factorysessions.LifecycleControlKind(operation),
		Outcome:   factorysessions.LifecycleControlOutcome("ACCEPTED"),
		Status:    factorysessions.LifecycleStatus(status),
	}
}
