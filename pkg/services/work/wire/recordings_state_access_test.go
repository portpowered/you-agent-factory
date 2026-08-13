package wire_test

import (
	"context"
	"encoding/json"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
)

func TestRecordingsStateAccessServiceNilRootReturnsNil(t *testing.T) {
	t.Parallel()

	if got := workwire.RecordingsStateAccessService(nil); got != nil {
		t.Fatalf("RecordingsStateAccessService(nil) = %#v, want nil", got)
	}
}

func TestRecordingsStateAccessServiceProjectsListAndGetOntoWorkRoot(t *testing.T) {
	t.Parallel()

	scope := recordings.CanonicalEventScope{FactorySessionID: "session-recordings-wire"}
	fake := &recordingsRootFake{
		events: []recordings.CanonicalEvent{{
			ID:          "event-1",
			FactoryTick: 2,
			Scope:       scope,
			Kind:        recordings.CanonicalEventKind(interfaces.FactoryEventTypeWorkRequest),
		}},
		worldState: recordings.WorldStateView{
			SchemaVersion: recordings.WorldStateViewSchemaV1,
			Payload:       recordingsBackedWorldPayload(t, "work-story", "story", "review"),
		},
	}
	ctx := context.Background()
	svc := workwire.RecordingsStateAccessService(fake)
	if svc == nil {
		t.Fatal("RecordingsStateAccessService(fake) = nil, want work.Service")
	}

	list, err := svc.ListWork(ctx, "session-recordings-wire", work.ListOptions{})
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(list.Results) != 1 || list.Results[0].WorkID != "work-story" {
		t.Fatalf("ListWork = %#v, want one story work item", list)
	}

	got, err := svc.GetWork(ctx, "session-recordings-wire", "work-story")
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	if got.WorkID != "work-story" {
		t.Fatalf("GetWork = %#v, want work-story", got)
	}
}

func TestRecordingsStateAccessServiceRejectsUnsupportedWorkRootRoles(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := workwire.RecordingsStateAccessService(&recordingsRootFake{})

	_, err := svc.PrepareWorkRequest(ctx, work.WorkRequestPreparation{})
	if err == nil {
		t.Fatal("PrepareWorkRequest() error = nil, want unsupported role")
	}

	_, err = svc.StageContent(ctx, work.StageContentRequest{})
	if err == nil {
		t.Fatal("StageContent() error = nil, want unsupported role")
	}

	_, err = svc.PrepareContent(ctx, nil)
	if err == nil {
		t.Fatal("PrepareContent() error = nil, want unsupported role")
	}

	_, err = svc.ResolveContent(ctx, "staged-id")
	if err == nil {
		t.Fatal("ResolveContent() error = nil, want unsupported role")
	}

	if err := svc.CleanupContent(ctx, "staged-id"); err == nil {
		t.Fatal("CleanupContent() error = nil, want unsupported role")
	}

	_, _, err = svc.MaterializeContentURL(ctx, "https://example.com/content")
	if err == nil {
		t.Fatal("MaterializeContentURL() error = nil, want unsupported role")
	}

	_, err = svc.MaterializeWorkerOutput(ctx, work.MaterializeWorkerOutputRequest{})
	if err == nil {
		t.Fatal("MaterializeWorkerOutput() error = nil, want unsupported role")
	}

	_, err = svc.PrepareInvocationInput(ctx, work.InvocationInputPreparationRequest{})
	if err == nil {
		t.Fatal("PrepareInvocationInput() error = nil, want unsupported role")
	}

	_, err = svc.ResolvePrimaryResult(ctx, work.PrimaryResultSelectionInput{})
	if err == nil {
		t.Fatal("ResolvePrimaryResult() error = nil, want unsupported role")
	}
}

func TestRecordingsStateAccessServiceDelegatesSessionMutations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := workwire.RecordingsStateAccessService(&recordingsRootFake{})

	_, err := svc.SubmitWorkRequestForSession(ctx, "session-1", work.WorkRequest{})
	if err == nil {
		t.Fatal("SubmitWorkRequestForSession() error = nil, want unavailable session adapter")
	}

	_, err = svc.MoveWorkForSession(ctx, "session-1", "work-1", "done", "req-1")
	if err == nil {
		t.Fatal("MoveWorkForSession() error = nil, want unavailable session adapter")
	}

	_, err = svc.MoveWorkAndRead(ctx, "session-1", "work-1", "done", "req-1")
	if err == nil {
		t.Fatal("MoveWorkAndRead() error = nil, want unavailable session adapter")
	}
}

type recordingsRootFake struct {
	events     []recordings.CanonicalEvent
	worldState recordings.WorldStateView
}

func (fake *recordingsRootFake) SubscribeFrom(
	_ context.Context,
	_ recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	index := 0
	return recordings.SubscribeResult{
		Subscription: func(context.Context) recordings.SubscriptionOutcome {
			if index >= len(fake.events) {
				return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
			}
			outcome := recordings.SubscriptionOutcome{
				Kind:  recordings.SubscriptionEvent,
				Event: fake.events[index],
			}
			index++
			return outcome
		},
	}, nil
}

func (fake *recordingsRootFake) ReconstructWorldState(
	recordings.ReconstructWorldStateRequest,
) (recordings.ReconstructWorldStateResult, error) {
	return recordings.ReconstructWorldStateResult{WorldState: fake.worldState}, nil
}

func recordingsBackedWorldPayload(t *testing.T, workID, workTypeID, stateName string) string {
	t.Helper()
	state := interfaces.FactoryWorldState{
		Topology: interfaces.InitialStructurePayload{
			WorkTypes: []interfaces.FactoryWorkType{{
				ID: workTypeID,
				States: []interfaces.FactoryStateDefinition{
					{Value: "review", Category: work.StateTypeProcessing},
				},
			}},
		},
		WorkItemsByID: map[string]work.FactoryWorkItem{
			workID: {ID: workID, WorkTypeID: workTypeID, State: stateName},
		},
	}
	payload, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal world state: %v", err)
	}
	return string(payload)
}
