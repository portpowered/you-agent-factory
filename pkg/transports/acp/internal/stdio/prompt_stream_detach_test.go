package stdio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
)

// These component witnesses observe the injected collaborators, not their
// implementation. Each connection owns its cache and only its cached identities.
func detachTestServer(t *testing.T, logger logging.Logger) (*Server, *fakeChatSessionsService, *fakeFactoryTargetService, *attachmentCache, *attachmentCache) {
	t.Helper()
	chat := &fakeChatSessionsService{}
	factory := &fakeFactoryTargetService{}
	server := New(logger, chat, nil, factory, &fakeEventsService{}, nil, nil, nil, testStartResolver, testInvocationScope)
	owned, peer := &attachmentCache{}, &attachmentCache{}
	for _, cache := range []*attachmentCache{owned, peer} {
		connection := "owned"
		if cache == peer {
			connection = "peer"
		}
		for _, session := range []string{"session-a", "session-b"} {
			if _, ok, err := server.ensureAttachment(contextWithAttachmentCache(context.Background(), cache), connection, session); err != nil || !ok {
				t.Fatalf("attach %s/%s: %v, %v", connection, session, ok, err)
			}
		}
	}
	return server, chat, factory, owned, peer
}

func assertDetachOutcomes(t *testing.T, server *Server, chat *fakeChatSessionsService, factory *fakeFactoryTargetService, owned, peer *attachmentCache, failedFirst bool) {
	t.Helper()
	if len(chat.detachCalls) != 2 {
		t.Fatalf("detach calls = %+v, want two", chat.detachCalls)
	}
	seen := map[string]bool{}
	for index, req := range chat.detachCalls {
		attachment, ok := owned.get(req.SessionID)
		if !ok || req.AttachmentID != attachment.ID || seen[req.SessionID] {
			t.Fatalf("unexpected or duplicate owned detach: %+v", req)
		}
		seen[req.SessionID] = true
		if got, want := chat.attachments[req.AttachmentID].Detached, !failedFirst || index != 0; got != want {
			t.Fatalf("%+v detached = %v, want %v", req, got, want)
		}
	}
	assertNoDetachMutation(t, chat, factory)
	for _, session := range []string{"session-a", "session-b"} {
		attachment, _ := peer.get(session)
		if chat.attachments[attachment.ID].Detached {
			t.Fatalf("peer %s detached", session)
		}
		eventsSvc := server.events.(*fakeEventsService)
		eventsSvc.seedItem(t, session, "peer-item", "", "MESSAGE", "COMPLETED", assistantMessagePayload("peer answer"))
		notify, notified := captureNotifier()
		if _, err := server.streamTurnUpdates(contextWithAttachmentCache(context.Background(), peer), "peer", session, 1, notify); err != nil || len(*notified) != 1 {
			t.Fatalf("peer %s unreadable: %v, notifications=%+v", session, err, *notified)
		}
	}
}

func assertNoDetachMutation(t *testing.T, chat *fakeChatSessionsService, factory *fakeFactoryTargetService) {
	t.Helper()
	if chat.startTurnCalled || chat.advanceTurnCalled || len(chat.requestControlReqs)+len(chat.advanceControlReqs) != 0 ||
		len(factory.startCalls)+len(factory.sessionStartCalls)+len(factory.invokeCalls)+len(factory.controlCalls)+len(factory.cancelCalls)+len(factory.closeCalls)+len(factory.terminateCalls) != 0 {
		t.Fatal("disconnect mutated turn or Factory control")
	}
}

func TestDetachAttachmentsReleasesEveryCachedAttachmentWithoutTouchingTheTurn(t *testing.T) {
	t.Parallel()
	logger := &recordingLogger{}
	server, chat, factory, owned, peer := detachTestServer(t, logger)
	server.detachAttachments(context.Background(), owned)
	assertDetachOutcomes(t, server, chat, factory, owned, peer, false)
	if len(logger.entries) != 0 {
		t.Fatalf("successful cleanup logged: %+v", logger.entries)
	}
}

func TestDetachAttachmentsCompletesAfterContextCancellation(t *testing.T) {
	t.Parallel()
	server, chat, factory, owned, peer := detachTestServer(t, logging.NoopLogger{})
	type contextKey struct{}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), contextKey{}, "cleanup-value"))
	cancel()
	observations := 0
	chat.detachContextObserver = func(ctx context.Context) error {
		observations++
		if ctx.Value(contextKey{}) != "cleanup-value" {
			t.Error("cleanup lost context value")
		}
		if ctx.Err() != nil || ctx.Done() != nil {
			t.Error("cleanup received canceled context")
			return ctx.Err()
		}
		return nil
	}
	server.detachAttachments(ctx, owned)
	if observations != 2 {
		t.Fatalf("context observations = %d, want two", observations)
	}
	assertDetachOutcomes(t, server, chat, factory, owned, peer, false)
}

func TestDetachAttachmentsContinuesAfterOneDetachFailure(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		err     error
		outcome string
	}{
		{"ordinary", errors.New("prompt-secret payload-secret credential-secret raw-error-secret"), "error"},
		{"cancelled", fmt.Errorf("credential-secret: %w", context.Canceled), "cancelled"},
		{"deadline", fmt.Errorf("payload-secret: %w", context.DeadlineExceeded), "cancelled"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			logger := &recordingLogger{}
			server, chat, factory, owned, peer := detachTestServer(t, logger)
			chat.detachErrs = []error{test.err}
			server.detachAttachments(context.Background(), owned)
			assertDetachOutcomes(t, server, chat, factory, owned, peer, true)
			want := []logEntry{{level: "warn", message: "acp stdio attachment detach failed", fields: map[string]any{
				"sessionId": chat.detachCalls[0].SessionID, "outcome": test.outcome,
			}}}
			if !reflect.DeepEqual(logger.entries, want) {
				t.Fatalf("diagnostics = %+v, want %+v", logger.entries, want)
			}
			for _, secret := range []string{"prompt-secret", "payload-secret", "credential-secret", "raw-error-secret"} {
				if strings.Contains(fmt.Sprint(logger.entries), secret) {
					t.Fatalf("diagnostics leaked %s", secret)
				}
			}
		})
	}
}

func TestDetachAttachmentsNoopLoggerPreservesCleanupAndProtocol(t *testing.T) {
	t.Parallel()
	capture := &recordingLogger{}
	var calls [][]chatsessions.DetachRequest
	var outputs []string
	for _, logger := range []logging.Logger{capture, logging.NoopLogger{}} {
		server, chat, factory, owned, peer := detachTestServer(t, logger)
		// A named failure keeps exact per-attachment outcomes independent of map order.
		failed, _ := owned.get("session-a")
		chat.detachAttachmentErrs = map[string]error{failed.ID: errors.New("credential-secret")}
		server.detachAttachments(context.Background(), owned)
		for _, req := range chat.detachCalls {
			if got, want := chat.attachments[req.AttachmentID].Detached, req.AttachmentID != failed.ID; got != want {
				t.Fatalf("detach %+v released=%v, want %v", req, got, want)
			}
		}
		calls = append(calls, chat.detachCalls)
		chat.detachCalls = nil
		chat.detachAttachmentErrs = nil
		server.detachAttachments(context.Background(), owned)
		assertDetachOutcomes(t, server, chat, factory, owned, peer, false)
		var out bytes.Buffer
		input := "{\"jsonrpc\":\"2.0\",\"id\":6,\"method\":\"initialize\",\"params\":{\"protocolVersion\":1,\"clientCapabilities\":{}}}\n" +
			"{\"jsonrpc\":\"2.0\",\"id\":7,\"method\":\"unsupported\",\"params\":{}}\n"
		if err := server.Serve(context.Background(), strings.NewReader(input), &out); err != nil {
			t.Fatal(err)
		}
		outputs = append(outputs, out.String())
	}
	for _, req := range calls[0] {
		found := false
		for _, other := range calls[1] {
			if req == other {
				found = true
			}
		}
		if !found {
			t.Fatalf("Noop cleanup requests differ: %+v", calls)
		}
	}
	if len(calls[0]) != 2 || len(calls[1]) != 2 || outputs[0] != outputs[1] || !strings.Contains(outputs[0], "error") || !strings.Contains(outputs[0], "result") {
		t.Fatalf("Noop protocol/cleanup differs: calls=%+v outputs=%q", calls, outputs)
	}
}
