package chatsessions_test

// This file is the story-004 external-consumer/boundary proof: it builds
// representative detached values for every published model using only
// chatsessions' public fields and invokes their Validate/CanTransitionTo
// behavior from outside the package. It complements contracts_test.go, which
// already proves a compile-time fake Service consumer and its typed
// not-found/busy/stale-version/invalid-transition/unsupported outcomes.
// Import-boundary evidence (stdlib-only production imports, no ACP/HTTP/
// CLI/OpenAPI/Factory Sessions/Worker Sessions/persistence dependency) is
// covered by the repository's existing go run ./cmd/pkgboundarycheck tool
// rather than a duplicate source-scanning test here.

import (
	"errors"
	"testing"
	"time"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
)

func TestConsumer_ConstructsAndValidatesRepresentativeValues(t *testing.T) {
	now := time.Unix(0, 1)
	later := now.Add(time.Second)

	target := chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/review"}
	if err := target.Validate(); err != nil {
		t.Fatalf("valid ChatTargetRef: %v", err)
	}
	if err := (chatsessions.ChatTargetRef{Kind: "BOGUS", Ref: "x"}).Validate(); !errors.Is(err, chatsessions.ErrUnknownEnumValue) {
		t.Fatalf("ChatTargetRef unknown kind: got %v, want ErrUnknownEnumValue", err)
	}
	if err := (chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory}).Validate(); !errors.Is(err, chatsessions.ErrRequiredValue) {
		t.Fatalf("ChatTargetRef blank ref: got %v, want ErrRequiredValue", err)
	}

	session := chatsessions.Session{
		ID:             "session-1",
		State:          chatsessions.SessionStateCreated,
		SelectedTarget: target,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := session.Validate(); err != nil {
		t.Fatalf("valid Session: %v", err)
	}
	inconsistent := session
	inconsistent.ActiveTurnID = "turn-1"
	if err := inconsistent.Validate(); !errors.Is(err, chatsessions.ErrInconsistentValue) {
		t.Fatalf("CREATED Session with ActiveTurnID: got %v, want ErrInconsistentValue", err)
	}

	episode := chatsessions.TargetEpisode{
		Number: 1, State: chatsessions.TargetEpisodeStateOpen, Target: target, StartedAt: now,
	}
	if err := episode.Validate(); err != nil {
		t.Fatalf("valid TargetEpisode: %v", err)
	}
	closedBeforeStart := episode
	closedBeforeStart.State = chatsessions.TargetEpisodeStateClosed
	early := now.Add(-time.Second)
	closedBeforeStart.ClosedAt = &early
	if err := closedBeforeStart.Validate(); !errors.Is(err, chatsessions.ErrInconsistentValue) {
		t.Fatalf("TargetEpisode ClosedAt before StartedAt: got %v, want ErrInconsistentValue", err)
	}

	turn := chatsessions.Turn{
		ID: "turn-1", Episode: 1, State: chatsessions.TurnStateAdmitted,
		RequestID: chatsessions.RequestIdentity{ConnectionID: "conn-1", RequestToken: "req-1"},
	}
	if err := turn.Validate(); err != nil {
		t.Fatalf("valid Turn: %v", err)
	}
	terminalWithoutSequence := turn
	terminalWithoutSequence.State = chatsessions.TurnStateCompleted
	if err := terminalWithoutSequence.Validate(); !errors.Is(err, chatsessions.ErrInconsistentValue) {
		t.Fatalf("terminal Turn without TerminalSequence: got %v, want ErrInconsistentValue", err)
	}

	attachment := chatsessions.Attachment{ID: "attachment-1", SessionID: session.ID, ConnectionID: "conn-2"}
	if err := attachment.Validate(); err != nil {
		t.Fatalf("valid Attachment: %v", err)
	}
	if err := (chatsessions.Attachment{SessionID: session.ID, ConnectionID: "conn-2"}).Validate(); !errors.Is(err, chatsessions.ErrRequiredValue) {
		t.Fatalf("Attachment blank ID: got %v, want ErrRequiredValue", err)
	}

	intent := chatsessions.ControlIntent{
		RequestID:     chatsessions.RequestIdentity{ConnectionID: "conn-1", RequestToken: "req-2"},
		SessionID:     session.ID,
		TurnID:        turn.ID,
		TargetEpisode: episode.Number,
		Action:        chatsessions.ControlActionCancel,
		State:         chatsessions.ControlIntentStateRequested,
		RequestedAt:   later,
	}
	if err := intent.Validate(); err != nil {
		t.Fatalf("valid ControlIntent: %v", err)
	}
	unsupported := intent
	unsupported.Action = chatsessions.ControlActionPause
	if err := unsupported.Validate(); !errors.Is(err, chatsessions.ErrUnsupportedControlAction) {
		t.Fatalf("ControlIntent PAUSE action: got %v, want ErrUnsupportedControlAction", err)
	}
}

func TestConsumer_InvokesTransitionBehavior(t *testing.T) {
	if err := chatsessions.SessionStateCreated.CanTransitionTo(chatsessions.SessionStateActive); err != nil {
		t.Fatalf("Session CREATED->ACTIVE: %v", err)
	}
	if err := chatsessions.SessionStateClosed.CanTransitionTo(chatsessions.SessionStateActive); !errors.Is(err, chatsessions.ErrInvalidTransition) {
		t.Fatalf("Session CLOSED->ACTIVE: got %v, want ErrInvalidTransition", err)
	}

	if err := chatsessions.TargetEpisodeStateOpen.CanTransitionTo(chatsessions.TargetEpisodeStateClosed); err != nil {
		t.Fatalf("TargetEpisode OPEN->CLOSED: %v", err)
	}
	if err := chatsessions.TargetEpisodeStateClosed.CanTransitionTo(chatsessions.TargetEpisodeStateOpen); !errors.Is(err, chatsessions.ErrInvalidTransition) {
		t.Fatalf("TargetEpisode CLOSED->OPEN: got %v, want ErrInvalidTransition", err)
	}

	if err := chatsessions.TurnStateAdmitted.CanTransitionTo(chatsessions.TurnStateRunning); err != nil {
		t.Fatalf("Turn ADMITTED->RUNNING: %v", err)
	}
	if err := chatsessions.TurnStateCompleted.CanTransitionTo(chatsessions.TurnStateRunning); !errors.Is(err, chatsessions.ErrInvalidTransition) {
		t.Fatalf("Turn COMPLETED->RUNNING: got %v, want ErrInvalidTransition", err)
	}

	if err := chatsessions.ControlIntentStateRequested.CanTransitionTo(chatsessions.ControlIntentStateCommitted); err != nil {
		t.Fatalf("ControlIntent REQUESTED->COMMITTED: %v", err)
	}
	if err := chatsessions.ControlIntentStateNoop.CanTransitionTo(chatsessions.ControlIntentStateSuperseded); !errors.Is(err, chatsessions.ErrInvalidTransition) {
		t.Fatalf("ControlIntent NOOP->SUPERSEDED: got %v, want ErrInvalidTransition", err)
	}
}
