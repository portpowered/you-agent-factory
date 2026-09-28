package factorysession

import (
	"context"
	"errors"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

type admissionTarget struct {
	factorysessions.Service
	started bool
}

func (target *admissionTarget) Start(context.Context, factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
	target.started = true
	return factorysessions.SessionStartResult{}, nil
}

func TestBoundedSubagentCallRejectsFullCapacityWithoutStartingWork(t *testing.T) {
	slots := make(chan struct{}, 1)
	slots <- struct{}{}
	called := false
	_, err := boundedSubagentCall(context.Background(), slots, func() (int, error) {
		called = true
		return 1, nil
	})
	if !errors.Is(err, errSubagentCapacity) || called {
		t.Fatalf("full capacity returned err=%v, called=%t", err, called)
	}
}

func TestSubagentFullAdmissionCapacityDoesNotStartSession(t *testing.T) {
	for i := 0; i < cap(subagentAdmissionSlots); i++ {
		subagentAdmissionSlots <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(subagentAdmissionSlots); i++ {
			<-subagentAdmissionSlots
		}
	}()
	target := &admissionTarget{}
	response := Subagent(context.Background(), target, "", func() string { return "full-admission" }, SubagentInput{Prompt: "Do work"})
	if response.Error == nil || response.Error.Code != "factory_session.subagent.capacity_exhausted" || !response.Error.Retryable || target.started {
		t.Fatalf("full admission response = %#v, started = %t", response, target.started)
	}
}
