package sessions_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
)

// TestLiveChangeCoordinatorClosesAdmittedApplicationFailure proves the public
// Factory Sessions wire contract closes an admitted application failure once,
// keeps the revision unchanged, and replays that terminal failure without
// invoking the application again.
func TestLiveChangeCoordinatorClosesAdmittedApplicationFailure(t *testing.T) {
	factorySnapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{"name": "live-change-contract"})
	if err != nil {
		t.Fatalf("create Factory snapshot: %v", err)
	}
	events := &functionalLiveChangeEventLog{}
	application := &functionalFailingLiveChangeApplication{}
	coordinator := factorysessionwire.NewLiveChangeCoordinator()
	request := factorysessions.LiveChangeRequest{
		RequestID:        "functional-failure-request",
		ExpectedRevision: 0,
		Operation:        "resource.capacity.set",
		TargetID:         "reviewers",
		RequestedValue:   json.RawMessage("8"),
		Source:           "cli",
		Reason:           "operator supplied secret must not escape",
	}
	operation := factorysessions.LiveChangeOperation{
		StateProvider: func(context.Context, string) (factorysessions.LiveChangeSessionState, error) {
			return factorysessions.LiveChangeSessionState{
				SessionID:         "functional-live-change-session",
				Lifecycle:         factorysessions.LiveChangeLifecycleRunning,
				EffectiveRevision: 0,
				Factory:           factorySnapshot,
			}, nil
		},
		Events:      events,
		Application: application,
		Now: func() time.Time {
			return time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
		},
	}

	first, firstErr := coordinator.ApplyLiveChange(
		context.Background(),
		"functional-live-change-session",
		request,
		operation,
	)
	if !errors.Is(firstErr, factorysessions.ErrLiveChangeApplicationFailed) ||
		first.Outcome != factorysessions.LiveChangeOutcomeFailed ||
		first.PreviousRevision != 0 || first.NewRevision != 0 ||
		first.FailureCode != string(factorysessions.LiveChangeErrorApplicationFailed) {
		t.Fatalf("first failure result=%#v error=%v, want safe admitted failure at unchanged revision", first, firstErr)
	}
	if application.calls != 1 || len(events.events) != 2 {
		t.Fatalf("application calls=%d events=%d, want one application and request/failure pair", application.calls, len(events.events))
	}
	if events.events[0].Type != factorydefinitions.FactoryEventTypeFactoryChangeRequest ||
		events.events[1].Type != factorydefinitions.FactoryEventTypeFactoryChangeFailed {
		t.Fatalf("live-change event types = %#v, want FACTORY_CHANGE_REQUEST then FACTORY_CHANGE_FAILED", events.events)
	}
	var failure factorydefinitions.FactoryChangeFailedEventPayload
	if err := events.events[1].DecodePayload(&failure); err != nil {
		t.Fatalf("decode failure event: %v", err)
	}
	if failure.FailureCode != string(factorysessions.LiveChangeErrorApplicationFailed) ||
		failure.FailureMessage != "live change application failed" ||
		failure.PreviousRevision != 0 ||
		failure.ChangeID != "live-change/functional-failure-request" {
		t.Fatalf("failure payload = %#v, want safe unchanged-revision closure", failure)
	}

	replayed, replayErr := coordinator.ApplyLiveChange(
		context.Background(),
		"functional-live-change-session",
		request,
		operation,
	)
	if !errors.Is(replayErr, factorysessions.ErrLiveChangeApplicationFailed) ||
		replayed.Outcome != factorysessions.LiveChangeOutcomeReplayed ||
		replayed.FailureCode != string(factorysessions.LiveChangeErrorApplicationFailed) ||
		application.calls != 1 || len(events.events) != 2 {
		t.Fatalf("replayed failure=%#v error=%v calls=%d events=%d, want one terminal replay", replayed, replayErr, application.calls, len(events.events))
	}
}

type functionalLiveChangeEventLog struct {
	events []factorydefinitions.FactoryEvent
}

func (l *functionalLiveChangeEventLog) AppendLiveChangeEvent(event factorydefinitions.FactoryEvent) (factorydefinitions.FactoryEvent, error) {
	event.Context.Sequence = len(l.events) + 1
	l.events = append(l.events, event)
	return event, nil
}

func (l *functionalLiveChangeEventLog) LiveChangeEvents() []factorydefinitions.FactoryEvent {
	return append([]factorydefinitions.FactoryEvent(nil), l.events...)
}

type functionalFailingLiveChangeApplication struct {
	calls int
}

func (a *functionalFailingLiveChangeApplication) ApplyLiveChange(context.Context, factorysessions.LiveChangeApplicationRequest) (factorysessions.LiveChangeApplicationResult, error) {
	a.calls++
	return factorysessions.LiveChangeApplicationResult{}, errors.New("provider secret and stack should not escape")
}
