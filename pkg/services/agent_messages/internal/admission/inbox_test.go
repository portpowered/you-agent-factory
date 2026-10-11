package admission

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/agent_messages/internal/store"
)

func TestEngineGetOnlyRecipientMarksRead(t *testing.T) {
	t.Parallel()
	for _, actor := range []string{"operator", "sender", "recipient", "unrelated"} {
		t.Run(actor, func(t *testing.T) {
			t.Parallel()
			f := newEngineFixture()
			original, err := f.engine.Send(context.Background(), engineRequest("request", "question"), f.caller, "")
			if err != nil {
				t.Fatal(err)
			}
			request := f.readerRequest(actor, original.MessageID)
			message, err := f.engine.Get(context.Background(), request)
			if actor == "unrelated" {
				if !errors.Is(err, agentmessages.ErrNotPermitted) || message.MessageID != "" {
					t.Fatal("unrelated reader received content")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := agentmessages.Queued
			if actor == "recipient" {
				want = agentmessages.Read
			}
			if message.Status != want || f.ledger.entries[0].Message.Status != want {
				t.Fatalf("status = %s, want %s", message.Status, want)
			}
			before := len(f.ledger.transactions)
			repeated, err := f.engine.Get(context.Background(), request)
			if err != nil || !reflect.DeepEqual(message, repeated) || len(f.ledger.transactions) != before {
				t.Fatal("repeated get was not idempotent")
			}
		})
	}
}

func (f *engineFixture) readerRequest(actor, message string) agentmessages.GetRequest {
	request := agentmessages.GetRequest{MessageID: message, Caller: f.caller}
	switch actor {
	case "operator":
		request.Caller = nil
	case "recipient":
		f.authority.sender = engineIdentity("lead")
	case "unrelated":
		f.authority.sender = engineIdentity("unrelated")
	}
	return request
}

func TestEngineGetExpiryPersistsWithoutRegressingTerminalState(t *testing.T) {
	t.Parallel()
	for _, status := range []agentmessages.Status{agentmessages.Queued, agentmessages.Read, agentmessages.Replied, agentmessages.Expired} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			f := newEngineFixture()
			original, err := f.engine.Send(context.Background(), engineRequest("request", "question"), f.caller, "")
			if err != nil {
				t.Fatal(err)
			}
			f.ledger.entries[0].Message.Status = status
			f.now = original.ExpiresAt
			message, err := f.engine.Get(context.Background(), agentmessages.GetRequest{MessageID: original.MessageID})
			if err != nil {
				t.Fatal(err)
			}
			want := agentmessages.Expired
			if status == agentmessages.Replied {
				want = agentmessages.Replied
			}
			if message.Status != want || f.ledger.entries[0].Message.Status != want {
				t.Fatalf("expiry status = %s, want %s", message.Status, want)
			}
		})
	}
}

func TestEngineGetFailedOrUnauthorizedUpdatePublishesNothing(t *testing.T) {
	t.Parallel()
	for _, failure := range []error{store.ErrUnavailable, agentmessages.ErrNotPermitted, context.Canceled} {
		t.Run(failure.Error(), func(t *testing.T) {
			t.Parallel()
			f := newEngineFixture()
			original, err := f.engine.Send(context.Background(), engineRequest("request", "question"), f.caller, "")
			if err != nil {
				t.Fatal(err)
			}
			f.authority.sender = engineIdentity("lead")
			ctx := context.Background()
			switch {
			case errors.Is(failure, store.ErrUnavailable):
				f.ledger.commitError = failure
			case errors.Is(failure, agentmessages.ErrNotPermitted):
				f.authority.finalError = failure
			case errors.Is(failure, context.Canceled):
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = canceled
			}
			message, err := f.engine.Get(ctx, agentmessages.GetRequest{MessageID: original.MessageID, Caller: f.caller})
			if !errors.Is(err, failure) || message.MessageID != "" || !reflect.DeepEqual(f.ledger.entries[0].Message, original) {
				t.Fatal("failed get published content or changed durable status")
			}
		})
	}
}

func TestEngineReplyToExpiredParentDoesNotSpendQuota(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	parent, err := f.engine.Send(context.Background(), engineRequest("question", "question"), f.caller, "")
	if err != nil {
		t.Fatal(err)
	}
	f.now = parent.ExpiresAt.Add(time.Second)
	f.authority.sender = engineIdentity("lead")
	reply := agentmessages.SendRequest{RequestID: "reply", Body: "answer", InReplyTo: parent.MessageID}
	if _, err := f.engine.Send(context.Background(), reply, f.caller, ""); !errors.Is(err, agentmessages.ErrBadRequest) {
		t.Fatalf("expired reply = %v", err)
	}
	if len(f.ledger.transactions) != 1 || f.quota.calls != 1 {
		t.Fatal("expired reply changed state or spent quota")
	}
}
