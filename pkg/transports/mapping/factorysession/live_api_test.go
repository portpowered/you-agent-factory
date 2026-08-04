package factorysession_test

import (
	"context"
	"reflect"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

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

	if got, want := control.calls, []string{"open", "list", "get", "pause", "resume", "close"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("control calls = %#v, want %#v", got, want)
	}
	if control.pauseRequest.RequestID != "pause-1" || control.resumeRequest.RequestID != "resume-1" || control.closedID != liveControlSessionID {
		t.Fatalf("control requests = pause:%#v resume:%#v close:%q", control.pauseRequest, control.resumeRequest, control.closedID)
	}
}

const liveControlSessionID = "live-session-1"

type liveControlSpy struct {
	calls         []string
	openRequest   factorysessions.LiveControlOpenRequest
	pauseRequest  factorysessions.LiveControlRequest
	resumeRequest factorysessions.LiveControlRequest
	closedID      string
}

var _ factorysessions.LiveControlService = (*liveControlSpy)(nil)

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

func (s *liveControlSpy) CloseFactorySession(_ context.Context, sessionID string) error {
	s.calls = append(s.calls, "close")
	s.closedID = sessionID
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
