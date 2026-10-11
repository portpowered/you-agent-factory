package admission

import (
	"context"
	"errors"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"reflect"
	"testing"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/agent_messages/internal/store"
)

type expiryScheduler struct {
	platformclock.TimerSource
	created chan platformclock.Timer
	stopped chan struct{}
}

func (s *expiryScheduler) NewTimer(duration time.Duration) platformclock.Timer {
	timer := &expiryTimer{Timer: s.TimerSource.NewTimer(duration), stopped: s.stopped}
	s.created <- timer
	return timer
}

type expiryTimer struct {
	platformclock.Timer
	stopped chan struct{}
}

func (timer *expiryTimer) Stop() bool {
	result := timer.Timer.Stop()
	timer.stopped <- struct{}{}
	return result
}

func TestExpiryTimerSharedUntilLastHostReleaseAndDisabledAvoidsIt(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	clock := platformclock.NewDeterministic(f.now, time.Second)
	scheduler := &expiryScheduler{TimerSource: clock, created: make(chan platformclock.Timer, 8), stopped: make(chan struct{}, 8)}
	f.engine.now, f.engine.scheduler = clock.Now, scheduler
	request := engineRequest("held", "held content")
	request.IfEnded = "HOLD"
	if _, err := f.engine.Send(t.Context(), request, f.caller, ""); err != nil {
		t.Fatal(err)
	}
	first, second := f.engine.BindHost(), f.engine.BindHost()
	t.Cleanup(first)
	t.Cleanup(second)
	select {
	case <-scheduler.created:
	case <-t.Context().Done():
		t.Fatal("host did not schedule expiry")
	}
	first()
	select {
	case <-scheduler.stopped:
		t.Fatal("one host release canceled shared expiry")
	default:
	}
	second() // Joins maintenance and cancels its timer.
	select {
	case <-scheduler.stopped:
	default:
		t.Fatal("last host release retained the expiry timer")
	}
	select {
	case <-scheduler.created:
		t.Fatal("shared hosts started multiple expiry schedulers")
	default:
	}
	f.engine.enabled = false
	f.engine.BindHost()()
	select {
	case <-scheduler.created:
		t.Fatal("disabled messaging started an expiry timer")
	default:
	}
}

func TestAutonomousExpiryCommitsOnlyDueUnrepliedMessages(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	prepareExpiryEntries(t, f)
	life := &deliveryLifetime{expiryWake: make(chan struct{}, 1)}
	f.engine.deliveryLife = life
	deadline, err := f.engine.expireDue(t.Context(), life)
	if err != nil || deadline != f.now.Add(24*time.Hour) || len(f.ledger.transactions) != 6 {
		t.Fatal("expiry changed messages before deadline", deadline, err)
	}
	f.now = deadline
	deadline, err = f.engine.expireDue(t.Context(), life)
	if err != nil || deadline != f.now.Add(time.Hour) {
		t.Fatal("expiry lost the later deadline", deadline, err)
	}
	tx := f.ledger.transactions[len(f.ledger.transactions)-1]
	if tx.Kind != store.Expired || len(tx.Messages) != 4 || len(tx.Requests) != 0 {
		t.Fatal("expiry did not atomically commit eligible content")
	}
	for _, entry := range f.ledger.entries {
		want := agentmessages.Expired
		switch entry.Message.Body {
		case "replied":
			want = agentmessages.Replied
		case "later":
			want = agentmessages.Queued
		}
		if entry.Message.Status != want {
			t.Fatal("autonomous expiry regressed terminal or future content")
		}
	}
	if _, err := f.engine.expireDue(t.Context(), life); err != nil || len(f.ledger.transactions) != 7 {
		t.Fatal("expiry repeated an already committed transition", err)
	}
}

func prepareExpiryEntries(t *testing.T, f *engineFixture) {
	t.Helper()
	for _, name := range []string{"queued", "held", "read", "delivered", "replied", "later"} {
		request := engineRequest(name, name)
		if name == "held" {
			request.IfEnded = "HOLD"
		}
		if _, err := f.engine.Send(t.Context(), request, f.caller, ""); err != nil {
			t.Fatal(err)
		}
	}
	for i := range f.ledger.entries {
		entry := &f.ledger.entries[i]
		switch entry.Message.Body {
		case "read":
			entry.Message.Status = agentmessages.Read
		case "delivered":
			entry.Message.Status = agentmessages.DeliveredRevive
		case "replied":
			entry.Message.Status = agentmessages.Replied
		case "later":
			entry.Message.ExpiresAt = entry.Message.ExpiresAt.Add(time.Hour)
		}
	}
}

func TestAutonomousExpiryPreservesStoreFaultAndHostOwnership(t *testing.T) {
	t.Parallel()
	for _, cause := range []error{store.ErrUnavailable, store.ErrCorrupt} {
		f := newEngineFixture()
		message, err := f.engine.Send(t.Context(), engineRequest("expiry", "preserve content"), f.caller, "")
		if err != nil {
			t.Fatal(err)
		}
		life := &deliveryLifetime{expiryWake: make(chan struct{}, 1)}
		f.engine.deliveryLife = life
		f.now = message.ExpiresAt
		f.ledger.commitError = cause
		if _, err := f.engine.expireDue(t.Context(), life); !errors.Is(err, cause) {
			t.Fatal("expiry hid the durable store fault", err)
		}
		if f.ledger.entries[0].Message.Status != agentmessages.Queued || len(f.ledger.transactions) != 1 {
			t.Fatal("failed expiry published an uncommitted status")
		}
		f.engine.deliveryLife = nil
		f.ledger.commitError = nil
		if _, err := f.engine.expireDue(t.Context(), life); !errors.Is(err, context.Canceled) {
			t.Fatal("released host retained expiry authority", err)
		}
	}
}

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
