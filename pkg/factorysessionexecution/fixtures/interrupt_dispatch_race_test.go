package fixtures_test

import (
	"context"
	"encoding/json"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

func TestFakeService_InterruptDispatchRace_ObservableServiceOutcomes(t *testing.T) {
	service := newContractFakeService(t)
	started := startAsyncByRequestID(t, service, "req-js-run-n-001")

	before, err := service.ListDispatches(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches before interrupt: %v", err)
	}
	if len(before.Dispatches) < 2 {
		t.Fatalf("dispatches = %#v, want fixture running session dispatches", before.Dispatches)
	}

	interruptResult, err := service.InterruptDispatch(context.Background(), started.SessionID, fse.InterruptDispatchRequest{
		ControlRequest: fse.ControlRequest{Reason: "operator stop before completion"},
		DispatchID:     "disp-js-002",
	})
	if err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}
	if interruptResult.Outcome != fse.LifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", interruptResult.Outcome)
	}
	if interruptResult.DispatchID != "disp-js-002" {
		t.Fatalf("dispatchId = %q, want disp-js-002", interruptResult.DispatchID)
	}

	dispatch, err := service.GetDispatch(context.Background(), started.SessionID, "disp-js-002")
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if dispatch.Status != fse.DispatchStatusInterrupted {
		t.Fatalf("dispatch status = %q, want INTERRUPTED", dispatch.Status)
	}
	if dispatch.FailureDetail == nil || dispatch.FailureDetail.Message != "operator stop before completion" {
		t.Fatalf("failureDetail = %#v, want operator stop before completion", dispatch.FailureDetail)
	}

	events, err := service.ReadEvents(context.Background(), started.SessionID, fse.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	assertDispatchInterruptedEvent(t, events.Events, "disp-js-002", "operator stop before completion", factoryapi.FactoryDispatchStatusRUNNING)

	replayed, err := fse.ReplayDispatchProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplayDispatchProjection: %v", err)
	}
	if len(replayed) != 1 || replayed[0].Status != fse.DispatchStatusInterrupted {
		t.Fatalf("replayed dispatches = %#v, want one interrupted dispatch", replayed)
	}

	session, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	after, err := service.ListDispatches(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches after interrupt: %v", err)
	}
	if err := fse.ValidateDispatchListMatchesSessionProgress(session, after.Dispatches); err != nil {
		t.Fatalf("ValidateDispatchListMatchesSessionProgress: %v", err)
	}
}

func assertDispatchInterruptedEvent(
	t *testing.T,
	events []json.RawMessage,
	dispatchID string,
	reason string,
	observedStatus factoryapi.FactoryDispatchStatus,
) {
	t.Helper()
	for _, raw := range events {
		var envelope struct {
			Type    string `json:"type"`
			Context struct {
				DispatchID *string `json:"dispatchId"`
			} `json:"context"`
			Payload struct {
				Reason         string `json:"reason"`
				ObservedStatus string `json:"observedStatus"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if envelope.Type != "DISPATCH_INTERRUPTED" {
			continue
		}
		if envelope.Context.DispatchID == nil || *envelope.Context.DispatchID != dispatchID {
			continue
		}
		if envelope.Payload.Reason != reason {
			t.Fatalf("reason = %q, want %q", envelope.Payload.Reason, reason)
		}
		if envelope.Payload.ObservedStatus != string(observedStatus) {
			t.Fatalf("observedStatus = %q, want %s", envelope.Payload.ObservedStatus, observedStatus)
		}
		return
	}
	t.Fatalf("DISPATCH_INTERRUPTED event for %s not found", dispatchID)
}
