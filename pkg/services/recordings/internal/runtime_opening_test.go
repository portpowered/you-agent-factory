package internal

import (
	"context"
	"errors"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestRuntimeRootKeepsConcurrentLedgersIsolatedAndReleasesRoutes(t *testing.T) {
	root := NewRuntimeRoot(nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if root == nil {
		t.Fatal("NewRuntimeRoot() returned nil")
	}
	topology := runtimeOpeningTopology{}
	now := func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }

	first, err := root.OpenRuntime(context.Background(), recordings.RuntimeScopeRequest{
		Topology:         topology,
		Now:              now,
		RecordingID:      "recording-one",
		FactorySessionID: "session-one",
	})
	if err != nil {
		t.Fatalf("OpenRuntime(first): %v", err)
	}
	second, err := root.OpenRuntime(context.Background(), recordings.RuntimeScopeRequest{
		Topology:         topology,
		Now:              now,
		RecordingID:      "recording-two",
		FactorySessionID: "session-two",
	})
	if err != nil {
		t.Fatalf("OpenRuntime(second): %v", err)
	}
	if first.Ledger == second.Ledger {
		t.Fatal("OpenRuntime returned the same ledger for concurrent sessions")
	}

	first.Ledger.RecordRunRequest()
	if got := len(first.Ledger.CanonicalEvents()); got != 1 {
		t.Fatalf("first ledger events = %d, want 1", got)
	}
	if got := len(second.Ledger.CanonicalEvents()); got != 0 {
		t.Fatalf("second ledger events = %d, want 0 before its own append", got)
	}

	finishedAt := now().Add(time.Second)
	if err := first.Recorder.Finalize(finishedAt); err != nil {
		t.Fatalf("Finalize(first): %v", err)
	}
	if err := first.Recorder.Finalize(finishedAt.Add(time.Second)); err != nil {
		t.Fatalf("Finalize(first) second call: %v", err)
	}
	if _, err := root.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Scope: recordings.CanonicalEventScope{FactorySessionID: "session-one"},
	}); !errors.Is(err, recordings.ErrReconnectCursorUnavailable) {
		t.Fatalf("Subscribe(closed first session) = %v, want isolated route failure", err)
	}

	second.Ledger.RecordRunRequest()
	if got := len(second.Ledger.CanonicalEvents()); got != 1 {
		t.Fatalf("second ledger events = %d, want 1", got)
	}
	if err := second.Recorder.Finalize(finishedAt); err != nil {
		t.Fatalf("Finalize(second): %v", err)
	}
}

func TestRuntimeRootActiveRecordingOwnsOpaqueScopeAndFinalizesOnce(t *testing.T) {
	snapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"id": "runtime-opening-test",
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	root := NewRuntimeRoot(
		nil,
		nil,
		nil,
		func(
			factorydefinitions.FactorySnapshotSource,
			string,
			map[string]string,
		) (*factorydefinitions.FactorySnapshot, error) {
			return snapshot, nil
		},
		nil,
		nil,
		nil,
	)
	now := func() time.Time { return time.Unix(1_700_000_100, 0).UTC() }
	opened, err := root.OpenRuntime(context.Background(), recordings.RuntimeScopeRequest{
		Topology:         runtimeOpeningTopology{},
		Now:              now,
		RecordingID:      "recording-active",
		RecordPath:       "recording.json",
		FactorySessionID: "session-active",
	})
	if err != nil {
		t.Fatalf("OpenRuntime(active): %v", err)
	}
	if opened.Scope.IsZero() {
		t.Fatal("OpenRuntime(active) returned a zero scope")
	}
	status, err := root.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: "recording-active",
	})
	if err != nil {
		t.Fatalf("QueryRecordingStatus(active): %v", err)
	}
	if status.Status.AcceptedEvents != 1 {
		t.Fatalf("active recording events = %d, want initial snapshot event", status.Status.AcceptedEvents)
	}
	opened.Ledger.RecordRunRequest()
	scopeStatus, err := root.QueryRecordingScope(context.Background(), recordings.QueryRecordingScopeRequest{
		Scope: opened.Scope,
	})
	if err != nil || scopeStatus.Status.LastEvent == nil {
		t.Fatalf("QueryRecordingScope(active) = (%#v, %v), want initial cursor", scopeStatus, err)
	}
	nextSequence := scopeStatus.Status.LastEvent.Sequence + 1
	scopeEvent := scopedScopeEvent("runtime-scope-event", nextSequence, scopeStatus.Status.EventScope)
	scopeEvent.Cursor.StreamGenerationID = scopeStatus.Status.LastEvent.StreamGenerationID
	appended, err := root.AppendRecordingScopeEvent(context.Background(), recordings.AppendRecordingScopeEventRequest{
		Scope: opened.Scope,
		Event: scopeEvent,
	})
	if err != nil || appended.Status.AcceptedEvents != 2 {
		t.Fatalf("AppendRecordingScopeEvent(active) = (%#v, %v), want second accepted event", appended, err)
	}
	finishedAt := now().Add(time.Second)
	if err := opened.Recorder.Finalize(finishedAt); err != nil {
		t.Fatalf("Finalize(active): %v", err)
	}
	if err := opened.Recorder.Finalize(finishedAt.Add(time.Second)); err != nil {
		t.Fatalf("Finalize(active) second call: %v", err)
	}
	status, err = root.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: "recording-active",
	})
	if err != nil {
		t.Fatalf("QueryRecordingStatus(finalized): %v", err)
	}
	if status.Status.State != recordings.RecordingFinalized || status.Status.AcceptedEvents != 3 {
		t.Fatalf("finalized active recording = %#v, want FINALIZED with initial, scoped, and terminal events", status.Status)
	}
	if _, err := root.QueryRecordingScope(context.Background(), recordings.QueryRecordingScopeRequest{
		Scope: opened.Scope,
	}); !errors.Is(err, recordings.ErrRecordingScopeClosed) {
		t.Fatalf("QueryRecordingScope(closed): %v, want closed-scope error", err)
	}
}

type runtimeOpeningTopology struct{}

func (runtimeOpeningTopology) RecordingInitialStructure(
	...factorydefinitions.RuntimeDefinitionLookup,
) recordings.InitialStructurePayload {
	return recordings.InitialStructurePayload{}
}

var _ recordings.InitialStructureSource = runtimeOpeningTopology{}
