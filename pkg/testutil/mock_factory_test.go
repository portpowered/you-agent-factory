package testutil_test

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestMockFactory_GetEngineStateSnapshot_ReturnsConfiguredEngineStateAndCountsCall(t *testing.T) {
	expected := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
		InFlightCount: 2,
	}
	mf := &testutil.MockFactory{EngineState: expected}

	got, err := mf.GetEngineStateSnapshot(t.Context())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if got != expected {
		t.Fatalf("GetEngineStateSnapshot() = %#v, want configured snapshot %#v", got, expected)
	}
	if mf.EngineStateSnapshotCalls != 1 {
		t.Fatalf("EngineStateSnapshotCalls = %d, want 1", mf.EngineStateSnapshotCalls)
	}
}

func TestMockFactory_GetEngineStateSnapshot_BuildsAggregateSnapshotFromConfiguredFields(t *testing.T) {
	net := &state.Net{ID: "test-net"}
	marking := &petri.MarkingSnapshot{
		Tokens: map[string]*interfaces.Token{
			"tok-1": {ID: "tok-1", PlaceID: "task:init"},
		},
		PlaceTokens: map[string][]string{"task:init": {"tok-1"}},
	}
	mf := &testutil.MockFactory{
		State:   interfaces.FactoryStateRunning,
		Marking: marking,
		Net:     net,
		Uptime:  5 * time.Second,
	}

	snapshot, err := mf.GetEngineStateSnapshot(t.Context())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snapshot == nil {
		t.Fatal("GetEngineStateSnapshot() = nil, want snapshot")
	}
	if snapshot.FactoryState != string(interfaces.FactoryStateRunning) {
		t.Fatalf("FactoryState = %q, want %q", snapshot.FactoryState, interfaces.FactoryStateRunning)
	}
	if snapshot.Uptime != 5*time.Second {
		t.Fatalf("Uptime = %v, want %v", snapshot.Uptime, 5*time.Second)
	}
	if snapshot.Topology != net {
		t.Fatal("Topology did not use configured net")
	}
	if snapshot.Marking.Tokens["tok-1"] == nil {
		t.Fatalf("Marking did not include configured token: %#v", snapshot.Marking.Tokens)
	}
}

func TestMockFactory_GetEngineStateSnapshot_ReturnsConfiguredError(t *testing.T) {
	wantErr := assertErr{}
	mf := &testutil.MockFactory{EngineStateSnapshotErr: wantErr}

	_, err := mf.GetEngineStateSnapshot(t.Context())
	if err != wantErr {
		t.Fatalf("GetEngineStateSnapshot error = %v, want %v", err, wantErr)
	}
	if mf.EngineStateSnapshotCalls != 1 {
		t.Fatalf("EngineStateSnapshotCalls = %d, want 1", mf.EngineStateSnapshotCalls)
	}
}

func TestMockFactory_GetFactoryEvents_ReturnsCopy(t *testing.T) {
	mf := &testutil.MockFactory{
		FactoryEvents: []factoryapi.FactoryEvent{
			{
				Id:            "event-1",
				SchemaVersion: factoryapi.AgentFactoryEventV1,
				Type:          factoryapi.FactoryEventTypeRunRequest,
			},
		},
	}

	events, err := mf.GetFactoryEvents(t.Context())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	if len(events) != 1 || events[0].Id != "event-1" {
		t.Fatalf("GetFactoryEvents() = %#v, want configured event", events)
	}

	events[0].Id = "mutated"
	again, err := mf.GetFactoryEvents(t.Context())
	if err != nil {
		t.Fatalf("GetFactoryEvents second call: %v", err)
	}
	if again[0].Id != "event-1" {
		t.Fatalf("GetFactoryEvents returned mutable backing slice, got id %q", again[0].Id)
	}
}

func TestMockFactory_SubscribeFactoryEvents_ReturnsHistoryAndCapturesContext(t *testing.T) {
	mf := &testutil.MockFactory{
		FactoryEvents: []factoryapi.FactoryEvent{
			{
				Id:            "event-1",
				SchemaVersion: factoryapi.AgentFactoryEventV1,
				Type:          factoryapi.FactoryEventTypeRunRequest,
			},
		},
	}

	stream, err := mf.SubscribeFactoryEvents(t.Context())
	if err != nil {
		t.Fatalf("SubscribeFactoryEvents: %v", err)
	}
	if stream == nil {
		t.Fatal("SubscribeFactoryEvents() = nil, want stream")
	}
	if len(stream.History) != 1 || stream.History[0].Id != "event-1" {
		t.Fatalf("stream history = %#v, want configured event", stream.History)
	}
	if mf.FactoryEventStreamCtx == nil {
		t.Fatal("FactoryEventStreamCtx was not captured")
	}
}

type assertErr struct{}

func (assertErr) Error() string { return "assert error" }
