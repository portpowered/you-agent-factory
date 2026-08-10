package stdio

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// TestLiveDrainTurnUpdatesRetriesGapAcknowledgementConflictBeforeCatchUp
// proves a response bridge version advance after a live retention-gap notice
// reaches the client cannot leave that already-delivered gap unacknowledged.
// The refreshed acknowledgement must preserve exactly-once delivery through
// the subsequent retained sweep and allow the retained record after the gap
// to continue in the live drain.
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestLiveDrainTurnUpdatesRetriesGapAcknowledgementConflictBeforeCatchUp(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)
	seedRetentionGap(t, eventsSvc, streamingTestSessionID)
	chatSessions := server.chatSessions.(*fakeChatSessionsService)
	chatSessions.getSessionResults = []chatsessions.GetSessionResult{
		{Session: chatsessions.Session{Version: 1}},
		{Session: chatsessions.Session{Version: 2}},
	}
	chatSessions.getSessionResult = chatsessions.GetSessionResult{Session: chatsessions.Session{Version: 2, StreamHead: 2}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cache := &attachmentCache{}
	ctx = contextWithAttachmentCache(ctx, cache)
	var notified []acpsdk.SessionNotification
	notify := func(notification acpsdk.SessionNotification) error {
		notified = append(notified, notification)
		if len(notified) == 1 {
			chatSessions.mu.Lock()
			chatSessions.acknowledgeAttachmentErrs = []error{
				&chatsessions.ConflictError{Value: "Session", ID: streamingTestSessionID, Expected: 1, Actual: 2},
			}
			chatSessions.mu.Unlock()
		}
		if len(notified) == 2 {
			cancel()
		}
		return nil
	}

	if delivered := server.liveDrainTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify); !delivered {
		t.Fatal("liveDrainTurnUpdates() deliveredMessage = false, want retained catch-up after the gap")
	}
	if len(notified) != 2 {
		t.Fatalf("notify call count = %d, want exactly 2 (the gap once, then retained catch-up)", len(notified))
	}
	if notified[0].Update.AgentThoughtChunk == nil {
		t.Fatalf("notification[0] = %+v, want the retention-gap notice", notified[0])
	}
	retained := notified[1].Update.AgentMessageChunk
	if retained == nil || retained.Content.Text == nil || retained.Content.Text.Text != "retained" {
		t.Fatalf("notification[1] = %+v, want retained catch-up after the gap", notified[1])
	}
	if len(chatSessions.acknowledgeAttachmentReqs) != 3 {
		t.Fatalf("acknowledge attempt count = %d, want exactly 3 (stale gap, refreshed gap, retained record)", len(chatSessions.acknowledgeAttachmentReqs))
	}
	if got := chatSessions.acknowledgeAttachmentReqs[0].ExpectedVersion; got != 1 {
		t.Fatalf("first gap acknowledgement expected version = %d, want 1", got)
	}
	if got := chatSessions.acknowledgeAttachmentReqs[1].ExpectedVersion; got != 2 {
		t.Fatalf("retried gap acknowledgement expected version = %d, want refreshed version 2", got)
	}
	attachment, ok := cache.get(streamingTestSessionID)
	if !ok || attachment.AfterSequence != 2 {
		t.Fatalf("cached attachment = %+v, %v, want retained sequence 2 acknowledged", attachment, ok)
	}

	catchUpNotify, catchUpNotifications := captureNotifier()
	catchUpCtx := contextWithAttachmentCache(context.Background(), cache)
	delivered, err := server.streamTurnUpdates(catchUpCtx, "conn-a", streamingTestSessionID, 1, catchUpNotify)
	if err != nil || delivered {
		t.Fatalf("post-live streamTurnUpdates() = %v, %v, want delivered=false and err=nil", delivered, err)
	}
	if len(*catchUpNotifications) != 0 {
		t.Fatalf("post-live catch-up notify count = %d, want 0 (the acknowledged gap must not replay)", len(*catchUpNotifications))
	}
}

// TestLiveDrainTurnUpdatesNoEventsIsNoOp proves a Server constructed without
// the Events collaborator reports no live delivery rather than panicking.
func TestLiveDrainTurnUpdatesNoEventsIsNoOp(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, _ := newStreamingTestServer(t, factoryTarget)
	server.events = nil

	notify, _ := captureNotifier()
	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	if delivered := server.liveDrainTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify); delivered {
		t.Fatalf("liveDrainTurnUpdates(no events) = %v, want false", delivered)
	}
}

// TestLiveDrainTurnUpdatesNoAttachmentIsNoOp proves a blank connectionID
// (ensureAttachment reports ok=false) is a silent no-op, matching
// streamTurnUpdates' own convention.
func TestLiveDrainTurnUpdatesNoAttachmentIsNoOp(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, _ := newStreamingTestServer(t, factoryTarget)

	notify, _ := captureNotifier()
	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	if delivered := server.liveDrainTurnUpdates(ctx, "", streamingTestSessionID, 1, notify); delivered {
		t.Fatalf("liveDrainTurnUpdates(blank connectionID) = %v, want false", delivered)
	}
}

// TestLiveDrainTurnUpdatesSubscribeFailureIsNoOp proves a genuine
// Events.Subscribe failure is a silent no-op: the guaranteed-correct
// post-invocation streamTurnUpdates sweep is the backstop for whatever this
// best-effort live drain could not start.
func TestLiveDrainTurnUpdatesSubscribeFailureIsNoOp(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)
	eventsSvc.subscribeErr = errors.New("subscribe failed")

	notify, _ := captureNotifier()
	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	if delivered := server.liveDrainTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify); delivered {
		t.Fatalf("liveDrainTurnUpdates(subscribe failure) = %v, want false", delivered)
	}
}

// TestWaitForLiveStreamHead covers the live subscription's delivery guard:
// a record must not be notified until the response bridge has advanced the
// Chat Session stream head to cover its aggregate position. The canceled
// case uses the invocation-owned context's explicit completion signal rather
// than waiting for the guard's retry interval.
func TestWaitForLiveStreamHead(t *testing.T) {
	t.Run("returns current version once stream head covers record", func(t *testing.T) {
		factoryTarget := &fakeFactoryTargetService{}
		server, _ := newStreamingTestServer(t, factoryTarget)
		chatSessions := server.chatSessions.(*fakeChatSessionsService)
		chatSessions.getSessionResult.Session.Version = 7
		chatSessions.getSessionResult.Session.StreamHead = 3

		version, ready := server.waitForLiveStreamHead(context.Background(), streamingTestSessionID, 3)

		if !ready || version != 7 {
			t.Fatalf("waitForLiveStreamHead() = (%d, %v), want (7, true)", version, ready)
		}
	})

	t.Run("stops when session lookup fails", func(t *testing.T) {
		factoryTarget := &fakeFactoryTargetService{}
		server, _ := newStreamingTestServer(t, factoryTarget)
		server.chatSessions.(*fakeChatSessionsService).getSessionErr = errors.New("get session failed")

		version, ready := server.waitForLiveStreamHead(context.Background(), streamingTestSessionID, 1)

		if ready || version != 0 {
			t.Fatalf("waitForLiveStreamHead() = (%d, %v), want (0, false)", version, ready)
		}
	})

	t.Run("stops when turn completes before stream head advances", func(t *testing.T) {
		factoryTarget := &fakeFactoryTargetService{}
		server, _ := newStreamingTestServer(t, factoryTarget)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		version, ready := server.waitForLiveStreamHead(ctx, streamingTestSessionID, 1)

		if ready || version != 0 {
			t.Fatalf("waitForLiveStreamHead() = (%d, %v), want (0, false)", version, ready)
		}
	})
}

// TestLiveDrainTurnUpdatesVersionFailureStopsOnRecordDelivery proves a
// genuine GetSession failure while re-reading the session's current version
// for a delivered record stops the live drain rather than dispatching
// drainRecords against a stale/unknown version.
func TestLiveDrainTurnUpdatesVersionFailureStopsOnRecordDelivery(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)
	chatSessions := server.chatSessions.(*fakeChatSessionsService)

	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	notify, _ := captureNotifier()

	// Prime the attachment and Subscribe registration before injecting the
	// GetSession failure, matching this fake's existing sequencing
	// convention elsewhere in this file (ensure Subscribe has already
	// observed the request before the drain can reach a delivery).
	drainDone := make(chan bool, 1)
	go func() {
		drainDone <- server.liveDrainTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify)
	}()
	eventsSvc.waitForSubscriber(t)
	chatSessions.getSessionErr = errors.New("get session failed")
	eventsSvc.seed(t, streamingTestSessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("hello"))

	select {
	case delivered := <-drainDone:
		if delivered {
			t.Fatalf("liveDrainTurnUpdates(version failure) = %v, want false", delivered)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("liveDrainTurnUpdates did not stop after a GetSession failure")
	}
}

// TestLiveDrainTurnUpdatesDeliversGapThenStopsOnCancel proves a retention
// gap observed mid-subscription is delivered as a gap notice (the same
// deliverReadTimeGap path streamTurnUpdates' own retained catch-up uses),
// and the drain loop continues rather than treating a successful gap
// delivery as terminal.

func TestLiveDrainTurnUpdatesDeliversGapThenStopsOnCancel(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)
	seedRetentionGap(t, eventsSvc, streamingTestSessionID)
	// The live record immediately after the gap must be acknowledgeable for
	// this test to prove the drain continues beyond the gap rather than merely
	// waiting for cancellation after one notification.
	chatSessions := server.chatSessions.(*fakeChatSessionsService)
	chatSessions.mu.Lock()
	chatSessions.getSessionResult.Session.StreamHead = 2
	chatSessions.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	ctx = contextWithAttachmentCache(ctx, &attachmentCache{})
	var mu sync.Mutex
	var notified []acpsdk.SessionNotification
	retainedDelivered := make(chan struct{})
	var retainedOnce sync.Once
	notify := func(notification acpsdk.SessionNotification) error {
		mu.Lock()
		notified = append(notified, notification)
		count := len(notified)
		mu.Unlock()
		if count == 2 {
			retainedOnce.Do(func() { close(retainedDelivered) })
		}
		return nil
	}

	drainDone := make(chan bool, 1)
	go func() {
		drainDone <- server.liveDrainTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify)
	}()

	// This direct notification signal replaces the prior timing delay: the
	// gap must be delivered first and the retained record immediately after it
	// must also arrive before cancellation can end the subscription.
	<-retainedDelivered
	select {
	case <-drainDone:
		t.Fatal("liveDrainTurnUpdates returned after the gap instead of continuing to retained catch-up")
	default:
	}
	cancel()

	if delivered := <-drainDone; !delivered {
		t.Fatal("liveDrainTurnUpdates deliveredMessage = false, want the retained message delivered after the gap")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(notified) != 2 {
		t.Fatalf("notify call count = %d, want exactly 2 (gap notice, then retained record)", len(notified))
	}
	if notified[0].Update.AgentThoughtChunk == nil {
		t.Fatalf("notification[0] = %+v, want the gap notice before retained catch-up", notified[0])
	}
	retained := notified[1].Update.AgentMessageChunk
	if retained == nil || retained.Content.Text == nil || retained.Content.Text.Text != "retained" {
		t.Fatalf("notification[1] = %+v, want retained message after the gap", notified[1])
	}
}

// TestLiveDrainTurnUpdatesVersionFailureStopsOnGap proves a genuine
// GetSession failure while re-reading the session's current version for an
// observed gap stops the live drain the same way it does for an ordinary
// record.
func TestLiveDrainTurnUpdatesVersionFailureStopsOnGap(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)
	chatSessions := server.chatSessions.(*fakeChatSessionsService)

	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	notify, _ := captureNotifier()

	drainDone := make(chan bool, 1)
	go func() {
		drainDone <- server.liveDrainTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify)
	}()
	eventsSvc.waitForSubscriber(t)
	chatSessions.getSessionErr = errors.New("get session failed")
	seedRetentionGap(t, eventsSvc, streamingTestSessionID)

	select {
	case delivered := <-drainDone:
		if delivered {
			t.Fatalf("liveDrainTurnUpdates(version failure on gap) = %v, want false", delivered)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("liveDrainTurnUpdates did not stop after a GetSession failure")
	}
}

// TestLiveDrainTurnUpdatesGapAcknowledgeFailureStopsDrain proves a genuine
// deliverReadTimeGap failure (a notifier failure while delivering the gap
// notice) stops the live drain the same way it stops streamTurnUpdates.
func TestLiveDrainTurnUpdatesGapAcknowledgeFailureStopsDrain(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)
	seedRetentionGap(t, eventsSvc, streamingTestSessionID)

	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	notify := func(acpsdk.SessionNotification) error {
		return errors.New("notify failed")
	}

	drainDone := make(chan bool, 1)
	go func() {
		drainDone <- server.liveDrainTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify)
	}()

	select {
	case delivered := <-drainDone:
		if delivered {
			t.Fatalf("liveDrainTurnUpdates(gap notify failure) = %v, want false", delivered)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("liveDrainTurnUpdates did not stop after a gap notifier failure")
	}
}
