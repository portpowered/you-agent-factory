package wire

import (
	"context"
	"errors"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

type workerSessionControlRouterServiceFake struct {
	workersessions.Service
	id           string
	controlCalls int
}

func (service *workerSessionControlRouterServiceFake) Get(
	_ context.Context,
	request workersessions.GetRequest,
) (workersessions.Session, error) {
	if request.ID != service.id {
		return workersessions.Session{}, workersessions.ErrSessionNotFound
	}
	return workersessions.Session{ID: service.id, State: workersessions.StateRunning}, nil
}

func (service *workerSessionControlRouterServiceFake) Cancel(
	_ context.Context,
	request workersessions.ControlRequest,
) (workersessions.ControlResult, error) {
	service.controlCalls++
	return workersessions.ControlResult{
		Session: workersessions.Session{ID: request.ID, State: workersessions.StateCanceled},
		Action:  workersessions.ControlActionCancel,
		Outcome: workersessions.ControlOutcomeApplied,
	}, nil
}

func TestWorkerSessionControlRouterUsesExactIdentityOwner(t *testing.T) {
	firstRuntime := &workerSessionControlRouterServiceFake{id: "another-session"}
	owner := &workerSessionControlRouterServiceFake{id: "target-session"}
	processDefault := &workerSessionControlRouterServiceFake{id: "target-session"}
	router := workerSessionControlRouter{sources: func(context.Context) ([]workersessions.Service, error) {
		return []workersessions.Service{firstRuntime, owner, processDefault}, nil
	}}

	result, err := router.Cancel(context.Background(), workersessions.ControlRequest{ID: "target-session"})
	if err != nil {
		t.Fatalf("Cancel() error = %v, want exact owner control", err)
	}
	if result.Session.ID != "target-session" || result.Session.State != workersessions.StateCanceled || result.Outcome != workersessions.ControlOutcomeApplied {
		t.Fatalf("Cancel() = %#v, want APPLIED CANCELED target session", result)
	}
	if firstRuntime.controlCalls != 0 || owner.controlCalls != 1 || processDefault.controlCalls != 0 {
		t.Fatalf("control calls = first %d, owner %d, process default %d; want only exact owner", firstRuntime.controlCalls, owner.controlCalls, processDefault.controlCalls)
	}
}

func TestWorkerSessionControlRouterKeepsUnknownIdentityNotFound(t *testing.T) {
	owner := &workerSessionControlRouterServiceFake{id: "known-session"}
	router := workerSessionControlRouter{sources: func(context.Context) ([]workersessions.Service, error) {
		return []workersessions.Service{owner}, nil
	}}

	_, err := router.Cancel(context.Background(), workersessions.ControlRequest{ID: "unknown-session"})
	if !errors.Is(err, workersessions.ErrSessionNotFound) {
		t.Fatalf("Cancel(unknown) error = %v, want ErrSessionNotFound", err)
	}
	if owner.controlCalls != 0 {
		t.Fatalf("unknown identity caused %d control calls, want zero", owner.controlCalls)
	}
}
