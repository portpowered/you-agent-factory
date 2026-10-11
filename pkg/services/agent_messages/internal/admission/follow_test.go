package admission

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/events"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// The coordinator consumes a controlled Events port. Real stream attachment,
// process reconstruction and CLI/HTTP follow belong to public functional proof.
type followStream struct {
	observationAppender
	head       events.ReadResult
	request    events.SubscribeRequest
	deliveries []events.Delivery
	beforeNext func()
	readError  error
	registered context.Context
}

func (s *followStream) Read(context.Context, events.ReadRequest) (events.ReadResult, error) {
	return s.head, s.readError
}

func (s *followStream) Subscribe(ctx context.Context, r events.SubscribeRequest) (events.Subscription, error) {
	s.request, s.registered = r, ctx
	return func(context.Context) events.Delivery {
		if s.beforeNext != nil {
			s.beforeNext()
		}
		if len(s.deliveries) == 0 {
			return events.Delivery{Kind: events.DeliveryClosed}
		}
		d := s.deliveries[0]
		s.deliveries = s.deliveries[1:]
		return d
	}, nil
}

func newFollowStream() *followStream {
	topic := agentmessages.ObservationStream
	return &followStream{head: events.ReadResult{Outcome: events.ReadOutcomeAtHead,
		Next: events.Cursor{Topic: topic, Position: 4}, Retained: events.RetainedRange{Topic: topic, Earliest: 1, Head: 4}}}
}

type followAuthority struct {
	Authority
	tokens []string
}

func (a *followAuthority) Authenticate(ctx context.Context, caller *workersessions.CallerIdentity) (Identity, error) {
	a.tokens = append(a.tokens, caller.Token)
	return a.Authority.Authenticate(ctx, caller)
}

func followDelivery(t *testing.T, m agentmessages.Message, position events.AggregateSequence) events.Delivery {
	t.Helper()
	payload, err := json.Marshal(agentmessages.Observation{RecordID: "record", Sequence: 1, Kind: "SENT", Message: m})
	if err != nil {
		t.Fatal(err)
	}
	topic := agentmessages.ObservationStream
	return events.Delivery{Kind: events.DeliveryRecord, Cursor: events.Cursor{Topic: topic, Position: position},
		Record: events.Record{ID: events.RecordID{Topic: topic, Position: position}, SourceType: "agent-message",
			SourceID: events.SourceID(m.MessageID), SourceSequence: 1, SourceEventID: "record",
			SchemaID: agentmessages.ObservationSchema, Payload: payload}}
}

func TestFollowAuthorizesEveryObservationAndFiltersWithoutReading(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	m, err := f.engine.Send(context.Background(), engineRequest("one", "body"), f.caller, "factory")
	if err != nil {
		t.Fatal(err)
	}
	f.authority.sender = engineIdentity("lead")
	s := newFollowStream()
	f.engine.events = s
	other := m
	other.MessageID = "unknown"
	s.deliveries = []events.Delivery{followDelivery(t, other, 5), followDelivery(t, m, 6)}
	seen := 0
	err = f.engine.Follow(context.Background(), agentmessages.ListRequest{Caller: f.caller, ToMe: true, ThreadID: m.ThreadID,
		Statuses: []agentmessages.Status{agentmessages.Queued}}, func(o agentmessages.Observation) error {
		seen++
		if o.Message != m {
			t.Fatal("follow changed source-native observation")
		}
		// Customer callbacks can enter messaging again: no admission lock is held.
		_, err := f.engine.List(context.Background(), agentmessages.ListRequest{})
		return err
	})
	if err != nil || seen != 1 {
		t.Fatalf("follow result: seen=%d error=%v", seen, err)
	}
	if s.request.From.Position != 4 || s.request.Validate() != nil || s.request.MaxPendingBytes != followPendingBytes {
		t.Fatal("subscription lost live head or bounded backpressure")
	}
	if s.registered.Err() != context.Canceled || len(f.ledger.transactions) != 1 || f.ledger.entries[0].Message.Status != agentmessages.Queued {
		t.Fatal("follow retained subscription or mutated durable inbox")
	}
}

func TestFollowDeniesForeignIdentityAndOwnerLossBeforeCallback(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"foreign", "owner-lost", "filter", "operator", "continuation", "own-work"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			f := newEngineFixture()
			m, err := f.engine.Send(context.Background(), engineRequest("one", "body"), f.caller, "factory")
			if err != nil {
				t.Fatal(err)
			}
			s := newFollowStream()
			f.engine.events = s
			s.deliveries = []events.Delivery{followDelivery(t, m, 5)}
			r := agentmessages.ListRequest{Caller: f.caller, ToMe: true}
			switch mode {
			case "foreign":
				f.authority.sender = engineIdentity("unrelated")
			case "owner-lost":
				f.authority.sender = engineIdentity("lead")
				s.beforeNext = func() { f.authority.finalError = agentmessages.ErrNotPermitted }
			case "filter":
				r.ThreadID = "other-thread"
			case "operator":
				r.Caller, r.ToMe = nil, false
			case "continuation":
				f.authority.sender = engineIdentity("successor")
				f.authority.sender.Chain = engineIdentity("lead").Chain
			case "own-work":
				f.authority.sender = engineIdentity("redispatch")
				f.authority.sender.Work = engineIdentity("lead").Work
			}
			seen := 0
			err = f.engine.Follow(context.Background(), r, func(agentmessages.Observation) error { seen++; return nil })
			assertFollowAuthorityResult(t, mode, err)
			want := 0
			if mode == "operator" || mode == "continuation" || mode == "own-work" {
				want = 1
			}
			if seen != want {
				t.Fatalf("callbacks=%d want=%d", seen, want)
			}
		})
	}
}

func assertFollowAuthorityResult(t *testing.T, mode string, err error) {
	t.Helper()
	if mode == "owner-lost" {
		if !errors.Is(err, agentmessages.ErrNotPermitted) {
			t.Fatalf("lost owner result: %v", err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
}

func TestFollowReportsLossCancellationAndObserverFailure(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"gap", "backpressure", "malformed", "cancel", "observer", "unavailable"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			f := newEngineFixture()
			m, err := f.engine.Send(context.Background(), engineRequest("one", "body"), f.caller, "factory")
			if err != nil {
				t.Fatal(err)
			}
			s := newFollowStream()
			f.engine.events = s
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want := agentmessages.ErrStreamUnavailable
			observerError := errors.New("observer stopped")
			s.deliveries = []events.Delivery{followDelivery(t, m, 5)}
			switch mode {
			case "gap":
				want = agentmessages.ErrStreamGap
				s.deliveries = []events.Delivery{{Kind: events.DeliveryGap, Gap: &events.GapFacts{Topic: agentmessages.ObservationStream, Requested: 4, EarliestRetained: 6, Head: 7}}}
			case "backpressure":
				want = agentmessages.ErrStreamBackpressure
				s.deliveries = []events.Delivery{{Kind: events.DeliveryBackpressure}}
			case "malformed":
				s.deliveries[0].Record.SourceID = "different-message"
			case "cancel":
				want = context.Canceled
				s.beforeNext = cancel
			case "observer":
				want = observerError
			case "unavailable":
				s.readError = errors.New("secret diagnostic")
			}
			err = f.engine.Follow(ctx, agentmessages.ListRequest{}, func(agentmessages.Observation) error { return observerError })
			if !errors.Is(err, want) {
				t.Fatalf("follow error=%v want=%v", err, want)
			}
			if s.registered != nil && s.registered.Err() != context.Canceled {
				t.Fatal("subscription not released")
			}
		})
	}
}

func TestFollowRejectsSubstitutedContentAndPreservesHistoricalStatus(t *testing.T) {
	t.Parallel()
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "historical", true: "substituted"}[changed], func(t *testing.T) {
			t.Parallel()
			f := newEngineFixture()
			m, err := f.engine.Send(context.Background(), engineRequest("one", "body"), f.caller, "factory")
			if err != nil {
				t.Fatal(err)
			}
			f.authority.sender = engineIdentity("lead")
			if _, err := f.engine.Get(context.Background(), agentmessages.GetRequest{Caller: f.caller, MessageID: m.MessageID}); err != nil {
				t.Fatal(err)
			}
			s := newFollowStream()
			f.engine.events = s
			if changed {
				m.Body = "unadmitted secret"
			}
			s.deliveries = []events.Delivery{followDelivery(t, m, 5)}
			seen := 0
			err = f.engine.Follow(context.Background(), agentmessages.ListRequest{Caller: f.caller, ToMe: true}, func(o agentmessages.Observation) error {
				seen++
				if o.Message.Status != agentmessages.Queued {
					t.Fatal("source-native historical status was rewritten")
				}
				return nil
			})
			if changed {
				if !errors.Is(err, agentmessages.ErrStreamUnavailable) || seen != 0 {
					t.Fatal("substituted content was published")
				}
			} else if err != nil || seen != 1 {
				t.Fatalf("historical observation result: %v, callbacks=%d", err, seen)
			}
		})
	}
}

func TestFollowValidatesBeforeRegisteringAndDetachesCredentials(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	s := newFollowStream()
	f.engine.events = s
	for _, r := range []agentmessages.ListRequest{{ToMe: true}, {MarkRead: true}, {NextToken: "token"}, {MaxResults: -1}, {Statuses: []agentmessages.Status{"invalid"}}} {
		if err := f.engine.Follow(context.Background(), r, func(agentmessages.Observation) error { t.Fatal("invalid follow published"); return nil }); err == nil {
			t.Fatal("invalid follow succeeded")
		}
		if s.registered != nil {
			t.Fatal("invalid follow registered a subscription")
		}
	}
	f.engine.enabled = false
	if err := f.engine.Follow(context.Background(), agentmessages.ListRequest{}, nil); !errors.Is(err, agentmessages.ErrDisabled) {
		t.Fatal("disabled follow did not return typed error")
	}
	f.engine.enabled = true
	authority := &followAuthority{Authority: f.authority}
	f.engine.authority = authority
	original := f.caller.Token
	s.beforeNext = func() { f.caller.Token = "mutated-token" }
	if err := f.engine.Follow(context.Background(), agentmessages.ListRequest{Caller: f.caller}, func(agentmessages.Observation) error { return nil }); err != nil {
		t.Fatal(err)
	}
	for _, token := range authority.tokens {
		if token != original {
			t.Fatal("follow authentication observed caller-owned credential mutation")
		}
	}
}
