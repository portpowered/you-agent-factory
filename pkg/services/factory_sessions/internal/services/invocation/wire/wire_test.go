package wire

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	legacyinvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
	invocationservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/invocation"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestNewExposesSinglePrivateInvocationAuthority(t *testing.T) {
	t.Parallel()

	const sessionID = "session-wire-1"
	configCalls, observeCalls := 0, 0
	peer := &fakeWorkPeer{
		submitResult: work.WorkRequestSubmitResult{RequestID: "request-wire-1", TraceID: "trace-wire-1", Accepted: true},
	}

	var service invocationservice.Service
	var err error
	service, err = New(invocationservice.Dependencies{
		FactoryConfig: func(gotSessionID string) (*factorydefinitions.FactoryConfig, error) {
			configCalls++
			if gotSessionID != sessionID {
				t.Fatalf("FactoryConfig session ID = %q, want %q", gotSessionID, sessionID)
			}
			return &factorydefinitions.FactoryConfig{WorkTypes: []factorydefinitions.WorkTypeConfig{{
				Name: "task", HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault},
			}}}, nil
		},
		Work: peer,
		Observe: func(_ context.Context, gotSessionID string, _ legacyinvocation.SessionInvocationWaitInput) (legacyinvocation.SessionInvocationObservation, error) {
			observeCalls++
			if gotSessionID != sessionID {
				t.Fatalf("Observe session ID = %q, want %q", gotSessionID, sessionID)
			}
			item := work.FactoryWorkItem{
				ID: "work-1", WorkTypeID: "task", State: "done", TraceID: "trace-wire-1",
				Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "ok"}},
			}
			return legacyinvocation.SessionInvocationObservation{WorldState: factorydefinitions.FactoryWorldState{
				WorkRequestsByID: map[string]factorydefinitions.WorkRequestPayload{"request-wire-1": {
					RequestID: "request-wire-1", TraceID: "trace-wire-1", WorkItems: []work.FactoryWorkItem{item},
				}},
				TerminalWorkByID: map[string]factorydefinitions.FactoryTerminalWork{item.ID: {WorkItem: item, Status: "done"}},
			}}, nil
		},
		Interpolation: factorydefinitionfixtures.InvocationInterpolation{},
		WorkTypes:     staticWorkType("task"),
		InputFiles:    func(string) ([]byte, error) { return nil, nil },
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	if service == nil {
		t.Fatal("New() service = nil")
	}

	sourceKind := factorysessions.InvocationInputSourceKindText
	result, err := service.InvokeFactorySession(context.Background(), sessionID, factorysessions.InvocationRequest{
		ContentProvided: true,
		SourceKind:      &sourceKind,
		Content:         []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("InvokeFactorySession through wire-constructed Service: %v", err)
	}
	if result.Status != factorydefinitions.InvocationTerminalStatusCompleted ||
		result.RequestID != "request-wire-1" ||
		result.TraceID != "trace-wire-1" {
		t.Fatalf("result = %#v, want completed outcome through private ownership", result)
	}
	if len(result.PrimaryResult) != 1 || result.PrimaryResult[0].Text != "ok" {
		t.Fatalf("primary result = %#v, want ok through wire-constructed Service", result.PrimaryResult)
	}
	if configCalls != 1 || peer.submitCalls != 1 || observeCalls != 1 {
		t.Fatalf("prepare/command/observe calls = config:%d submit:%d observe:%d, want 1 each through wire", configCalls, peer.submitCalls, observeCalls)
	}
	if peer.lastSessionID != sessionID {
		t.Fatalf("commanded session ID = %q, want %q", peer.lastSessionID, sessionID)
	}
	if peer.lastRequest.Type != work.WorkRequestTypeFactoryRequestBatch ||
		len(peer.lastRequest.Works) != 1 ||
		peer.lastRequest.Works[0].WorkTypeID != "task" ||
		len(peer.lastRequest.Works[0].Content) != 1 ||
		peer.lastRequest.Works[0].Content[0].Text != "hello" {
		t.Fatalf("commanded WorkRequest = %#v, want prepared task content through CTR-WORK peer root", peer.lastRequest)
	}
}

type staticWorkType string

func (workType staticWorkType) DefaultWorkType(*factorydefinitions.FactoryConfig) (string, error) {
	return string(workType), nil
}

type fakeWorkPeer struct {
	submitResult  work.WorkRequestSubmitResult
	submitCalls   int
	lastSessionID string
	lastRequest   work.WorkRequest
}

func (f *fakeWorkPeer) SubmitWorkRequestForSession(
	_ context.Context,
	sessionID string,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	f.submitCalls++
	f.lastSessionID = sessionID
	f.lastRequest = request
	return f.submitResult, nil
}
