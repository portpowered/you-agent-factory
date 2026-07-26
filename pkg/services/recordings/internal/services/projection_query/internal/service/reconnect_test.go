package service

import (
	"errors"
	"reflect"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestReconnectReplayResumesStrictlyAfterEventID(t *testing.T) {
	t.Parallel()

	history := representativeHistory(t)
	scope := factorydefinitions.FactoryEventReconnectScope{SessionID: "factory-session-1"}
	cursor := factorydefinitions.FactoryEventReconnectCursor{AfterEventID: "work-request"}

	first, err := reconnectReplay(history, cursor, scope)
	if err != nil {
		t.Fatalf("first reconnect replay: %v", err)
	}
	second, err := reconnectReplay(history, cursor, scope)
	if err != nil {
		t.Fatalf("second reconnect replay: %v", err)
	}
	assertEventIDs(t, first, "dispatch-request", "dispatch-response", "session-completed")
	if !reflect.DeepEqual(first, second) {
		t.Fatal("identical event-ID reconnect inputs produced different continuations")
	}

	first[0].Id = "mutated"
	again, err := reconnectReplay(history, cursor, scope)
	if err != nil {
		t.Fatalf("detached reconnect replay: %v", err)
	}
	assertEventIDs(t, again, "dispatch-request", "dispatch-response", "session-completed")
}

func TestReconnectReplaySessionSequenceIsScopedAndConverges(t *testing.T) {
	t.Parallel()

	history := representativeHistory(t)
	otherSessionID := "factory-session-2"
	other := history[3].Clone()
	other.Id = "other-session-dispatch"
	other.Context.SessionID = &otherSessionID
	otherSequence := 4
	other.Context.SessionSequence = &otherSequence
	sessionless := history[3].Clone()
	sessionless.Id = "sessionless-dispatch"
	sessionless.Context.SessionID = nil
	history = append(
		history[:4],
		append([]factorydefinitions.FactoryEvent{sessionless, other}, history[4:]...)...,
	)

	sessionSequence := 3
	scope := factorydefinitions.FactoryEventReconnectScope{SessionID: "factory-session-1"}
	cursor := factorydefinitions.FactoryEventReconnectCursor{AfterSequence: &sessionSequence}
	continuation, err := reconnectReplay(history, cursor, scope)
	if err != nil {
		t.Fatalf("session-sequence reconnect replay: %v", err)
	}
	assertEventIDs(t, continuation, "dispatch-request", "dispatch-response", "session-completed")

	scopedHistory := eventsForSession(history, scope.SessionID)
	prefix := scopedHistory[:3]
	resumedHistory := append(cloneEvents(prefix), continuation...)
	uninterrupted, err := New().ReconstructFactoryWorldState(scopedHistory, 5)
	if err != nil {
		t.Fatalf("uninterrupted projection: %v", err)
	}
	resumed, err := New().ReconstructFactoryWorldState(resumedHistory, 5)
	if err != nil {
		t.Fatalf("resumed projection: %v", err)
	}
	if !reflect.DeepEqual(resumed, uninterrupted) {
		t.Fatal("cursor prefix plus continuation did not converge with uninterrupted projection")
	}
}

func TestReconnectReplayEmptyContinuationIsStable(t *testing.T) {
	t.Parallel()

	history := representativeHistory(t)
	scope := factorydefinitions.FactoryEventReconnectScope{SessionID: "factory-session-1"}
	cursor := factorydefinitions.FactoryEventReconnectCursor{AfterEventID: "session-completed"}

	for attempt := 0; attempt < 2; attempt++ {
		continuation, err := reconnectReplay(history, cursor, scope)
		if err != nil {
			t.Fatalf("reconnect replay attempt %d: %v", attempt, err)
		}
		if len(continuation) != 0 {
			t.Fatalf("continuation attempt %d = %#v, want no new events", attempt, continuation)
		}
	}
}

func TestReconnectReplayRejectsUnusableCursorsWithoutPartialContinuation(t *testing.T) {
	t.Parallel()

	history := representativeHistory(t)
	otherSessionID := "factory-session-2"
	other := history[2].Clone()
	other.Id = "other-session-work"
	other.Context.SessionID = &otherSessionID
	otherSessionSequence := 3
	other.Context.SessionSequence = &otherSessionSequence
	history = append(history, other)

	missingSequence := 99
	tests := []struct {
		name   string
		cursor factorydefinitions.FactoryEventReconnectCursor
		scope  factorydefinitions.FactoryEventReconnectScope
	}{
		{
			name:   "missing event ID",
			cursor: factorydefinitions.FactoryEventReconnectCursor{AfterEventID: "missing"},
			scope:  factorydefinitions.FactoryEventReconnectScope{SessionID: "factory-session-1"},
		},
		{
			name:   "event ID from another session",
			cursor: factorydefinitions.FactoryEventReconnectCursor{AfterEventID: "other-session-work"},
			scope:  factorydefinitions.FactoryEventReconnectScope{SessionID: "factory-session-1"},
		},
		{
			name:   "missing session sequence",
			cursor: factorydefinitions.FactoryEventReconnectCursor{AfterSequence: &missingSequence},
			scope:  factorydefinitions.FactoryEventReconnectScope{SessionID: "factory-session-1"},
		},
		{
			name:   "sequence from another session",
			cursor: factorydefinitions.FactoryEventReconnectCursor{AfterSequence: &otherSessionSequence},
			scope:  factorydefinitions.FactoryEventReconnectScope{SessionID: "factory-session-3"},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			continuation, err := reconnectReplay(history, test.cursor, test.scope)
			if !errors.Is(err, recordings.ErrReconnectCursorNotFound) {
				t.Fatalf("error = %v, want ErrReconnectCursorNotFound", err)
			}
			if continuation != nil {
				t.Fatalf("partial continuation = %#v, want nil", continuation)
			}
			if err := New().ValidateReconnectReplay(history, test.cursor, test.scope); !errors.Is(
				err,
				recordings.ErrReconnectCursorNotFound,
			) {
				t.Fatalf("accepted validation error = %v, want ErrReconnectCursorNotFound", err)
			}
		})
	}
}

func eventsForSession(
	events []factorydefinitions.FactoryEvent,
	sessionID string,
) []factorydefinitions.FactoryEvent {
	scoped := make([]factorydefinitions.FactoryEvent, 0, len(events))
	for _, event := range events {
		if eventBelongsToSession(event, sessionID) {
			scoped = append(scoped, event.Clone())
		}
	}
	return scoped
}

func assertEventIDs(t *testing.T, events []factorydefinitions.FactoryEvent, want ...string) {
	t.Helper()

	got := make([]string, len(events))
	for index, event := range events {
		got[index] = event.Id
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("event IDs = %#v, want %#v", got, want)
	}
}
