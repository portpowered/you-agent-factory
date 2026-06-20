package runtime

import (
	"context"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/logging"
)

func TestPauseResume_EmitCanonicalSessionLifecycleEvents(t *testing.T) {
	f, err := New(
		factory.WithNet(buildMoveControlNet()),
		factory.WithInlineDispatch(),
		factory.WithLogger(logging.NoopLogger{}),
		factory.WithWorkflowContext(&factory_context.FactoryContext{SessionID: "session-pause-resume"}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if err := f.Pause(ctx); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if err := f.Resume(ctx); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	events, err := f.GetFactoryEvents(ctx)
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}

	var paused, resumed bool
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeSessionPaused:
			paused = true
			assertLifecycleEventSessionID(t, event, "session-pause-resume")
		case factoryapi.FactoryEventTypeSessionResumed:
			resumed = true
			assertLifecycleEventSessionID(t, event, "session-pause-resume")
		}
	}
	if !paused || !resumed {
		t.Fatalf("events missing pause/resume markers: paused=%v resumed=%v", paused, resumed)
	}

	worldState, err := projections.ReconstructFactoryWorldState(events, len(events)-1)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if worldState.SessionBracket == nil || worldState.SessionBracket.LifecycleControlStatus != string(factoryapi.FactorySessionDurableLifecycleStatusRunning) {
		t.Fatalf("session bracket = %#v, want RUNNING lifecycle control status", worldState.SessionBracket)
	}
}

func TestPauseResume_NoOpDoesNotEmitAdditionalLifecycleEvents(t *testing.T) {
	f, err := New(
		factory.WithNet(buildMoveControlNet()),
		factory.WithInlineDispatch(),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if err := f.Pause(ctx); err != nil {
		t.Fatalf("first Pause: %v", err)
	}
	if err := f.Pause(ctx); err != nil {
		t.Fatalf("second Pause: %v", err)
	}
	if err := f.Resume(ctx); err != nil {
		t.Fatalf("first Resume: %v", err)
	}
	if err := f.Resume(ctx); err != nil {
		t.Fatalf("second Resume: %v", err)
	}

	events, err := f.GetFactoryEvents(ctx)
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	pauseCount, resumeCount := 0, 0
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeSessionPaused:
			pauseCount++
		case factoryapi.FactoryEventTypeSessionResumed:
			resumeCount++
		}
	}
	if pauseCount != 1 || resumeCount != 1 {
		t.Fatalf("lifecycle event counts = pause %d resume %d, want one each", pauseCount, resumeCount)
	}
}

func TestPauseResume_ReplayPreservesFinalPausedStatus(t *testing.T) {
	t0 := time.Date(2026, 6, 20, 11, 0, 0, 0, time.UTC)
	f, err := New(
		factory.WithNet(buildMoveControlNet()),
		factory.WithInlineDispatch(),
		factory.WithLogger(logging.NoopLogger{}),
		factory.WithClock(replay.NewDeterministicClock(t0, time.Second)),
		factory.WithWorkflowContext(&factory_context.FactoryContext{SessionID: "session-paused-only"}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if err := f.Pause(ctx); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	snapshot, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after pause: %v", err)
	}
	if snapshot.LifecycleControlStatus != string(factoryapi.FactorySessionDurableLifecycleStatusPaused) {
		t.Fatalf("lifecycleControlStatus = %q, want PAUSED", snapshot.LifecycleControlStatus)
	}

	events, err := f.GetFactoryEvents(ctx)
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	worldState, err := projections.ReconstructFactoryWorldState(events, len(events)-1)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if worldState.SessionBracket == nil || worldState.SessionBracket.LifecycleControlStatus != string(factoryapi.FactorySessionDurableLifecycleStatusPaused) {
		t.Fatalf("session bracket = %#v, want PAUSED lifecycle control status", worldState.SessionBracket)
	}
}

func assertLifecycleEventSessionID(t *testing.T, event factoryapi.FactoryEvent, wantSessionID string) {
	t.Helper()
	if event.Context.SessionId == nil || *event.Context.SessionId != wantSessionID {
		t.Fatalf("%s session id = %#v, want %s", event.Type, event.Context.SessionId, wantSessionID)
	}
}
