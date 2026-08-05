package stdio

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/mapping"
)

func TestWorkerChildProjectionCacheFailsClosedAcrossRestoreBoundaries(t *testing.T) {
	var nilCache *attachmentCache
	if nilCache.beginWorkerChildBudget("session") {
		t.Fatal("nil cache initialized a child budget")
	}
	nilCache.resetWorkerChildBudgetInitialization("session")

	item := chatsessions.SequencedItem{
		ParentItemID: "worker", WorkerSessionAssociation: &chatsessions.WorkerSessionAssociation{DispatchID: "dispatch", WorkerSessionID: "worker"},
		Kind: workers.KindMessage, Phase: workers.PhaseDelta,
		Payload: json.RawMessage(`{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":"text"}`),
	}
	update, err := mapping.ProjectWorkerChild(item)
	if err != nil || update == nil || update.ToolCallUpdate == nil {
		t.Fatalf("ProjectWorkerChild() = (%#v, %v), want content update", update, err)
	}
	if bounded, err := nilCache.boundWorkerChildProjection("session", item, update); err != nil || bounded == nil {
		t.Fatalf("nil cache bound = (%#v, %v), want bounded update", bounded, err)
	}
	unsafe := &acpsdk.SessionUpdate{ToolCallUpdate: &acpsdk.SessionToolCallUpdate{ToolCallId: "worker", RawInput: math.Inf(1)}}
	if bounded, err := (&attachmentCache{}).boundWorkerChildProjection("session", item, unsafe); bounded != nil || !errors.Is(err, mapping.ErrMalformedRecord) {
		t.Fatalf("unsafe cached projection = (%#v, %v), want malformed", bounded, err)
	}

	server, eventsSvc := newStreamingTestServer(t, &fakeFactoryTargetService{})
	readErr := errors.New("read failed")
	eventsSvc.readErr = readErr
	cache := &attachmentCache{}
	ctx := contextWithAttachmentCache(context.Background(), cache)
	if err := server.restoreWorkerChildProjectionBudget(ctx, streamingTestSessionID, 1); !errors.Is(err, readErr) {
		t.Fatalf("restore read error = %v, want %v", err, readErr)
	}
	if !cache.beginWorkerChildBudget(streamingTestSessionID) {
		t.Fatal("failed restore did not reset budget initialization for retry")
	}

	eventsSvc.readErr = nil
	if err := server.restoreWorkerChildProjectionBudget(contextWithAttachmentCache(context.Background(), &attachmentCache{}), streamingTestSessionID, 1); !errors.Is(err, errStreamGapEncountered) {
		t.Fatalf("restore invalid cursor = %v, want %v", err, errStreamGapEncountered)
	}

	eventsSvc.seed(t, streamingTestSessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("one"))
	eventsSvc.seed(t, streamingTestSessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("two"))
	eventsSvc.markEvictedThrough(streamingTestSessionID, 1)
	if err := server.restoreWorkerChildProjectionBudget(contextWithAttachmentCache(context.Background(), &attachmentCache{}), streamingTestSessionID, 2); err == nil {
		t.Fatal("restore retention gap error = nil, want failure")
	}
}

func TestRebuildWorkerChildProjectionBudgetIsolatesMalformedChildRecords(t *testing.T) {
	server, _ := newStreamingTestServer(t, &fakeFactoryTargetService{})
	cache := &attachmentCache{}
	if err := server.rebuildWorkerChildProjectionBudget(cache, streamingTestSessionID, events.Record{Payload: json.RawMessage(`not-json`)}); !errors.Is(err, errMalformedSequencedEnvelope) {
		t.Fatalf("rebuild malformed envelope error = %v, want %v", err, errMalformedSequencedEnvelope)
	}

	badChild, err := json.Marshal(chatsessions.SequencedItem{
		ParentItemID: "worker", WorkerSessionAssociation: &chatsessions.WorkerSessionAssociation{DispatchID: "dispatch", WorkerSessionID: "worker"},
		Kind: workers.KindError, Phase: workers.PhaseUpdated, Payload: json.RawMessage(`{"code":"bad"}`),
	})
	if err != nil {
		t.Fatalf("marshal bad child: %v", err)
	}
	if err := server.rebuildWorkerChildProjectionBudget(cache, streamingTestSessionID, events.Record{Payload: badChild}); err != nil {
		t.Fatalf("rebuild malformed child error = %v, want isolated skip", err)
	}
	if cache.workerChildBudgets != nil {
		t.Fatalf("malformed child populated budgets = %#v, want none", cache.workerChildBudgets)
	}
}

func TestWorkerChildProjectionSkipLoggingAcceptsAbsentLogger(t *testing.T) {
	item := chatsessions.SequencedItem{WorkerSessionAssociation: &chatsessions.WorkerSessionAssociation{DispatchID: "dispatch", WorkerSessionID: "worker"}}
	var nilServer *Server
	nilServer.logWorkerChildProjectionSkipped(item)
	(&Server{}).logWorkerChildProjectionSkipped(item)
}
