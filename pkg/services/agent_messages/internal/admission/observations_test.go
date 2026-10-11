package admission

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"strings"
	"sync"
	"testing"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/agent_messages/internal/store"
	"github.com/portpowered/infinite-you/pkg/services/events"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

type recoveryCell struct {
	name, state                                       string
	progress, complete, active, malformed, writeFault bool
	replayError                                       error
}

func TestRecoveryUsesOriginalAdmissionClock(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"recorded", "missing", "zero", "future", "foreign chain", "foreign predecessor", "foreign successor"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			f, _, intent := admittedEngineFixture(t)
			f.engine.revivesPerHour = 1
			original := intent.AcceptedAt
			intent.State, intent.Reason, intent.AcceptedAt = store.IntentPending, "", time.Time{}
			f.ledger.intents[intent.RequestID] = intent
			f.ledger.entries[0].Message.Reason = ""
			f.now = f.now.Add(2 * time.Hour)
			f.authority.recipients[intent.SuccessorWorkerSessionID] = recoverySuccessorClock(intent, original, f.now, cell)
			configureRecoveryReplay(t, f, intent, recoveryCell{progress: true, complete: true})
			f.engine.mu.Lock()
			err := f.engine.recoverDelivery(t.Context(), intent)
			f.engine.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			current := f.ledger.intents[intent.RequestID]
			if cell == "recorded" {
				if current.State != store.IntentAccepted || !current.AcceptedAt.Equal(original) {
					t.Fatal("recovery refreshed the original admission clock", current)
				}
				if _, blocked := f.engine.reviveGate(intent.RecipientChainIdentity, []store.DeliveryIntent{current}); blocked {
					t.Fatal("recovery extended an expired volume window")
				}
			} else if current.State != store.IntentUncertain || current.Reason != "REVIVE_FAILED" || !current.AcceptedAt.IsZero() || f.ledger.entries[0].Message.RevivedWorkerSessionID != "" {
				t.Fatal("recovery guessed admission time or delivered facts", current)
			}
		})
	}
}

func recoverySuccessorClock(intent store.DeliveryIntent, original, now time.Time, cell string) Identity {
	successor := Identity{Chain: intent.RecipientChainIdentity, Observation: workersessions.Observation{
		WorkerSessionID: intent.SuccessorWorkerSessionID, FactorySessionID: intent.FactorySessionID,
		PredecessorWorkerSessionID: intent.SourceWorkerSessionID, StartedAt: &original}}

	switch cell {
	case "missing":
		successor.Observation.StartedAt = nil
	case "zero":
		stamp := time.Time{}
		successor.Observation.StartedAt = &stamp
	case "future":
		stamp := now.Add(time.Minute)
		successor.Observation.StartedAt = &stamp
	case "foreign chain":
		successor.Chain = "foreign"
	case "foreign predecessor":
		successor.Observation.PredecessorWorkerSessionID = "foreign"
	case "foreign successor":
		successor.Observation.WorkerSessionID = "foreign"
	}
	return successor
}

func TestRecoveryRequiresExactAdmissionAndProviderProgress(t *testing.T) {
	t.Parallel()
	for _, cell := range []recoveryCell{
		{name: "completed confirmed", state: store.IntentAccepted, progress: true, complete: true},
		{name: "active confirmed", state: store.IntentAccepted, progress: true, complete: true, active: true},
		{name: "active pending", state: store.IntentAdmitted, complete: true, active: true},
		{name: "completed without progress", state: store.IntentUncertain, complete: true},
		{name: "incomplete replay", state: store.IntentUncertain},
		{name: "progress in incomplete replay", state: store.IntentUncertain, progress: true},
		{name: "foreign successor", state: store.IntentUncertain, malformed: true},
		{name: "unavailable replay", state: store.IntentUncertain, replayError: workersessions.ErrContinuationExecutionUnavailable},
		{name: "rejected replay", state: store.IntentUncertain, replayError: workersessions.ErrContinuationNotAccepted},
		{name: "refused recovery flush", state: store.IntentAdmitted, progress: true, complete: true, writeFault: true},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			f, message, intent := admittedEngineFixture(t)
			f.now = f.now.Add(10 * time.Minute)
			calls := configureRecoveryReplay(t, f, intent, cell)
			if cell.writeFault {
				f.ledger.commitError = store.ErrUnavailable
			}
			f.engine.mu.Lock()
			err := f.engine.recoverDelivery(t.Context(), intent)
			f.engine.mu.Unlock()
			if cell.writeFault != errors.Is(err, store.ErrUnavailable) || *calls != 1 {
				t.Fatal("incorrect replay/flush outcome", err)
			}
			current := f.ledger.intents[intent.RequestID]
			if current.State != cell.state {
				t.Fatalf("recovery state = %s", current.State)
			}
			if (cell.state == store.IntentAccepted || cell.state == store.IntentAdmitted) && !current.AcceptedAt.Equal(intent.AcceptedAt) {
				t.Fatal("recovery changed volume timestamp")
			}
			got := f.ledger.entries[0].Message
			if cell.state == store.IntentAccepted {
				if got.Status != agentmessages.DeliveredRevive || got.RevivedWorkerSessionID != intent.SuccessorWorkerSessionID {
					t.Fatal("confirmed recovery lost delivery facts")
				}
			} else if got.Status != agentmessages.Queued || got.RevivedWorkerSessionID != "" || got.Body != message.Body {
				t.Fatal("uncertain recovery guessed delivery or changed input")
			}
		})
	}
}

func configureRecoveryReplay(t *testing.T, f *engineFixture, intent store.DeliveryIntent, cell recoveryCell) *int {
	t.Helper()
	calls := new(int)
	f.engine.continueSession = func(_ context.Context, req workersessions.ContinueRequest) (workersessions.ContinueResult, error) {
		*calls++
		if req.RequestID != intent.RequestID || req.SourceWorkerSessionID != intent.SourceWorkerSessionID || req.SuccessorWorkerSessionID != intent.SuccessorWorkerSessionID || req.FollowUpInput != intent.FollowUpInput || !req.ResolveHead {
			t.Fatal("recovery rewrote the admitted effect tuple")
		}
		state := workersessions.StateCompleted
		if cell.active {
			state = workersessions.StateRunning
		}
		result := workersessions.ContinueResult{RequestID: req.RequestID, SourceWorkerSessionID: req.SourceWorkerSessionID,
			SuccessorWorkerSessionID: req.SuccessorWorkerSessionID, Session: workersessions.Session{ID: req.SuccessorWorkerSessionID, State: state}}
		if cell.malformed {
			result.Session.ID = "foreign"
		}
		return result, cell.replayError
	}
	f.engine.readObservations = func(_ context.Context, req workersessions.StreamObservationsByWorkerSessionIDRequest) (workersessions.ObservationSubscription, error) {
		if req.WorkerSessionID != intent.SuccessorWorkerSessionID || req.FactorySessionID != intent.FactorySessionID || !req.ReplayOnly {
			t.Fatal("recovery inspected a different or unbounded source")
		}
		progress := cell.progress
		return workersessions.ObservationSubscription{NextFunc: func(context.Context) workersessions.ObservationDelivery {
			if progress {
				progress = false
				return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryRecord, Event: workersessions.ObservationEvent{SchemaID: "workers.draft.v1", Payload: json.RawMessage(`{"kind":"RUN","phase":"STARTED","provenance":{"provider":"codex","delivery":"NATIVE_STREAM","nativeEventType":"turn.started"}}`)}}
			}
			return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryReplaySummary, Summary: &workersessions.ReplaySummary{Complete: cell.complete}}
		}}, nil
	}
	return calls
}

type observationAppender struct {
	requests    []events.AppendRequest
	attachments []events.AttachSourceRequest
	attachError error
	err         error
	before      func(context.Context, events.AppendRequest)
}

func (a *observationAppender) AttachSource(_ context.Context, r events.AttachSourceRequest) (events.AttachSourceResult, error) {
	a.attachments = append(a.attachments, r)
	return events.AttachSourceResult{}, a.attachError
}

func (*observationAppender) Read(context.Context, events.ReadRequest) (events.ReadResult, error) {
	return events.ReadResult{}, agentmessages.ErrStreamUnavailable
}

func (*observationAppender) Subscribe(context.Context, events.SubscribeRequest) (events.Subscription, error) {
	return nil, agentmessages.ErrStreamUnavailable
}

func (a *observationAppender) Append(ctx context.Context, r events.AppendRequest) (events.AppendResult, error) {
	if a.before != nil {
		a.before(ctx, r)
	}
	a.requests = append(a.requests, r.Detached())
	return events.AppendResult{}, a.err
}

func decodedObservations(t *testing.T, a *observationAppender) []agentmessages.Observation {
	t.Helper()
	result := make([]agentmessages.Observation, 0, len(a.requests))
	for _, r := range a.requests {
		if err := r.Validate(); err != nil {
			t.Fatalf("invalid observation envelope: %v", err)
		}
		if r.SchemaID != agentmessages.ObservationSchema || r.SourceType != "agent-message" {
			t.Fatal("observation escaped its source-native messaging contract")
		}
		var observation agentmessages.Observation
		if err := json.Unmarshal(r.Payload, &observation); err != nil {
			t.Fatal(err)
		}
		if r.Topic != observation.Message.To.ObservationTopic() {
			t.Fatal("observation escaped its exact recipient stream")
		}
		if string(r.SourceID) != observation.Message.MessageID || string(r.SourceEventID) != observation.RecordID || uint64(r.SourceSequence) != observation.Sequence {
			t.Fatal("observation lost its committed identity")
		}
		result = append(result, observation)
	}
	return result
}

func TestObservationsFollowDurableSendReplyAndRead(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	a := &observationAppender{}
	f.engine.events = a
	a.before = func(_ context.Context, r events.AppendRequest) {
		assertObservationCommitted(t, f.ledger, r)
	}
	first, err := f.engine.Send(context.Background(), engineRequest("first", "question"), f.caller, "factory")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"first", "alias"} {
		if _, err := f.engine.Send(context.Background(), engineRequest(key, "question"), f.caller, "factory"); err != nil {
			t.Fatal(err)
		}
	}
	if len(a.requests) != 1 {
		t.Fatal("idempotent retry or alias fabricated a message observation")
	}
	assertAggregateAttachment(t, a, first)
	f.authority.sender = engineIdentity("lead")
	if _, err := f.engine.Get(context.Background(), agentmessages.GetRequest{MessageID: first.MessageID, Caller: f.caller}); err != nil {
		t.Fatal(err)
	}
	reply, err := f.engine.Send(context.Background(), agentmessages.SendRequest{RequestID: "reply", InReplyTo: first.MessageID, Body: "answer"}, f.caller, "factory")
	if err != nil {
		t.Fatal(err)
	}
	observations := decodedObservations(t, a)
	assertObservationKinds(t, observations, []string{store.Sent, store.Read, store.Sent, store.Replied})
	child, parent := observations[2], observations[3]
	if a.requests[0].Topic != a.requests[1].Topic || a.requests[0].Topic != a.requests[3].Topic ||
		a.requests[2].Topic == a.requests[3].Topic {
		t.Fatal("reply or parent transition was routed to the wrong recipient")
	}
	if child.RecordID != parent.RecordID || child.Sequence != parent.Sequence || child.Message.InReplyTo != first.MessageID ||
		child.Message.ThreadID != first.ThreadID || parent.Message.RepliedByMessageID != reply.MessageID {
		t.Fatal("atomic reply observations lost their common commit or relationship")
	}
}

func TestReviveOutcomePublishesDeliveryWithoutRegressingInbox(t *testing.T) {
	t.Parallel()
	for _, status := range []agentmessages.Status{agentmessages.Read, agentmessages.Replied, agentmessages.Expired} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			f := newEngineFixture()
			a := &observationAppender{}
			f.engine.events = a
			a.before = func(_ context.Context, r events.AppendRequest) { assertObservationCommitted(t, f.ledger, r) }
			recipient := f.authority.recipients["lead"]
			recipient.Observation.State, recipient.Observation.Revivable = "COMPLETED", true
			f.authority.recipients["lead"] = recipient
			f.engine.continueSession = func(_ context.Context, req workersessions.ContinueRequest) (workersessions.ContinueResult, error) {
				// Model the ledger's independently committed read/reply/expiry
				// while admission waits, without composing another component.
				f.engine.mu.Lock()
				f.ledger.entries[0].Message.Status = status
				if status == agentmessages.Replied {
					f.ledger.entries[0].Message.RepliedByMessageID = "reply"
				}
				f.ledger.sequence++
				f.engine.mu.Unlock()
				return workersessions.ContinueResult{RequestID: req.RequestID, SourceWorkerSessionID: req.SourceWorkerSessionID,
					SuccessorWorkerSessionID: req.SuccessorWorkerSessionID, Session: workersessions.Session{ID: req.SuccessorWorkerSessionID}}, nil
			}
			got, err := f.engine.Send(t.Context(), engineRequest("ask", "safe input"), f.caller, "")
			got = confirmEngineDelivery(t, f, got)
			if err != nil || got.Status != status || got.RevivedWorkerSessionID == "" ||
				(status == agentmessages.Replied && got.RepliedByMessageID != "reply") {
				t.Fatal("admission outcome lost newer inbox state", got, err)
			}
			observations := decodedObservations(t, a)
			assertObservationKinds(t, observations, []string{store.Sent, string(status), string(agentmessages.DeliveredRevive)})
			if observations[2].Message != got || !validObservationKind(observations[2]) {
				t.Fatal("delivered observation lost monotonic state or cannot follow")
			}
		})
	}
}

func assertAggregateAttachment(t *testing.T, a *observationAppender, message agentmessages.Message) {
	t.Helper()
	if len(a.attachments) != 1 || a.attachments[0].Validate() != nil ||
		a.attachments[0].Source != message.To.ObservationTopic() ||
		a.attachments[0].Destination != agentmessages.ObservationStream || a.attachments[0].Mode != events.AttachModeLiveOnly {
		t.Fatal("recipient stream was not attached to authorized query aggregate")
	}
}

func assertObservationCommitted(t *testing.T, ledger *engineLedger, r events.AppendRequest) {
	t.Helper()
	var observation agentmessages.Observation
	if err := json.Unmarshal(r.Payload, &observation); err != nil {
		t.Fatal(err)
	}
	entry, exists, sequence, err := ledger.LookupMessage(observation.Message.MessageID)
	if err != nil || !exists || sequence != observation.Sequence || entry.Message.Status != observation.Message.Status {
		t.Fatal("observation published before durable transaction")
	}
}

func TestObservationsSeparateLegacyRecipientScopes(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	a := &observationAppender{}
	f.engine.events = a
	a.before = func(_ context.Context, r events.AppendRequest) {
		assertObservationCommitted(t, f.ledger, r)
	}
	first, err := f.engine.Send(context.Background(), engineRequest("first", "question"), f.caller, "factory")
	if err != nil {
		t.Fatal(err)
	}
	// Authorization and address resolution are controlled collaborators here.
	// A second exact owner has the same legacy public Worker Session ID.
	other := engineIdentity("lead")
	other.Observation.FactorySessionID = "other-factory"
	other.Chain = "other-lead-chain"
	other.Work = "other-lead-work"
	f.authority.recipients["lead"] = other
	second, err := f.engine.Send(context.Background(), engineRequest("second", "question"), f.caller, "other-factory")
	if err != nil {
		t.Fatal(err)
	}
	observations := decodedObservations(t, a)
	if len(observations) != 2 || first.MessageID == second.MessageID || a.requests[0].Topic == a.requests[1].Topic {
		t.Fatal("distinct legacy recipient scopes shared a record or stream")
	}
	if observations[0].Message.To.FactorySessionID != "factory" || observations[1].Message.To.FactorySessionID != "other-factory" {
		t.Fatal("publication lost the admitted exact recipient scope")
	}
}

func assertObservationKinds(t *testing.T, observations []agentmessages.Observation, kinds []string) {
	t.Helper()
	if len(observations) != len(kinds) {
		t.Fatalf("observation count = %d, want %d", len(observations), len(kinds))
	}
	for i, kind := range kinds {
		if observations[i].Kind != kind {
			t.Fatalf("observation %d kind = %q, want %q", i, observations[i].Kind, kind)
		}
	}
}

func TestObservationsBatchReadAndAdmissionExpiry(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	a := &observationAppender{}
	f.engine.events = a
	for _, key := range []string{"one", "two"} {
		if _, err := f.engine.Send(context.Background(), engineRequest(key, key), f.caller, "factory"); err != nil {
			t.Fatal(err)
		}
	}
	f.authority.sender = engineIdentity("lead")
	if _, err := f.engine.List(context.Background(), agentmessages.ListRequest{Caller: f.caller, ToMe: true, MarkRead: true}); err != nil {
		t.Fatal(err)
	}
	observations := decodedObservations(t, a)
	if len(observations) != 4 || observations[2].Kind != store.Read || observations[3].Kind != store.Read ||
		observations[2].Sequence != observations[3].Sequence || observations[2].RecordID != observations[3].RecordID {
		t.Fatal("page read did not publish one committed batch")
	}
	f.authority.sender = engineIdentity("worker")
	f.now = f.now.Add(24 * time.Hour)
	if _, err := f.engine.Send(context.Background(), engineRequest("three", "three"), f.caller, "factory"); err != nil {
		t.Fatal(err)
	}
	observations = decodedObservations(t, a)
	if len(observations) != 7 || observations[4].Kind != store.Sent || observations[5].Kind != store.Expired || observations[6].Kind != store.Expired ||
		observations[4].RecordID != observations[6].RecordID {
		t.Fatal("admission maintenance lost its durable expiry observations")
	}
}

func TestObservationsOmitDeniedAndFailedTransactions(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"denied", "owner-lost", "store"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			f := newEngineFixture()
			a := &observationAppender{}
			f.engine.events = a
			switch failure {
			case "denied":
				f.authority.denied = true
			case "owner-lost":
				f.authority.finalError = agentmessages.ErrNotPermitted
			case "store":
				f.ledger.commitError = store.ErrUnavailable
			}
			if _, err := f.engine.Send(context.Background(), engineRequest("one", "question"), f.caller, "factory"); err == nil {
				t.Fatal("failed admission succeeded")
			}
			if len(a.requests) != 0 || len(f.ledger.entries) != 0 {
				t.Fatal("failed admission published an observation or durable message")
			}
		})
	}
}

func TestObservationFailurePreservesDisconnectedDurableSuccessAndSafeTelemetry(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	var telemetry bytes.Buffer
	f.engine.logger = observationLogger{buffer: &telemetry}
	a := &observationAppender{err: errors.New("planted-secret caller-secret")}
	f.engine.events = a
	ctx, cancel := context.WithCancel(context.Background())
	f.ledger.beforeCommit = func(store.Transaction) { cancel() }
	a.before = func(ctx context.Context, _ events.AppendRequest) {
		if ctx.Err() != nil {
			t.Fatal("client disconnection suppressed a committed observation")
		}
	}
	message, err := f.engine.Send(ctx, engineRequest("one", "safe normalized body"), f.caller, "factory")
	if err != nil || len(f.ledger.entries) != 1 || len(a.requests) != 1 {
		t.Fatalf("durable success changed by stream failure: %v", err)
	}
	f.ledger.beforeCommit = nil
	retry, err := f.engine.Send(context.Background(), engineRequest("one", "safe normalized body"), f.caller, "factory")
	if err != nil || retry.MessageID != message.MessageID || len(a.requests) != 1 {
		t.Fatal("stream failure changed idempotent retry or fabricated an observation")
	}
	for _, secret := range []string{"planted-secret", f.caller.Token, "safe normalized body"} {
		if strings.Contains(telemetry.String(), secret) {
			t.Fatal("operational telemetry leaked collaborator diagnostics or content")
		}
	}
	if !strings.Contains(telemetry.String(), "Agent Message observation unavailable") || bytes.Contains(a.requests[0].Payload, []byte(f.caller.Token)) {
		t.Fatal("missing safe failure diagnostic or leaked caller credentials")
	}
}

func TestAggregateAttachmentFailurePreservesRecipientObservation(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	var telemetry bytes.Buffer
	f.engine.logger = observationLogger{buffer: &telemetry}
	a := &observationAppender{attachError: errors.New("caller-secret planted-secret")}
	f.engine.events = a
	m, err := f.engine.Send(context.Background(), engineRequest("one", "body"), f.caller, "factory")
	if err != nil || len(a.requests) != 1 || len(f.ledger.entries) != 1 {
		t.Fatalf("attachment changed admission/publication: %v", err)
	}
	if a.requests[0].Topic != m.To.ObservationTopic() {
		t.Fatal("attachment failure redirected recipient observation")
	}
	if strings.Contains(telemetry.String(), "planted-secret") || strings.Contains(telemetry.String(), f.caller.Token) {
		t.Fatal("attachment diagnostics leaked")
	}
}

// Controlled Events registration exposes the interval after the head read and
// before live subscription. The coordinator must recheck authoritative state;
// a stream need not retain the terminal notification for delivery to proceed.
type recipientRegistrationStream struct {
	observationAppender
	register func(events.SubscribeRequest) (events.Subscription, error)
	read     func(events.ReadRequest) (events.ReadResult, error)
}

func (s *recipientRegistrationStream) Subscribe(_ context.Context, request events.SubscribeRequest) (events.Subscription, error) {
	return s.register(request)
}

func (s *recipientRegistrationStream) Read(_ context.Context, request events.ReadRequest) (events.ReadResult, error) {
	return s.read(request)
}

func TestRecipientEndDuringStreamRegistrationUsesAuthoritativeState(t *testing.T) {
	t.Parallel()
	for _, boundary := range []string{"initial", "unavailable", "recovered"} {
		t.Run(boundary, func(t *testing.T) {
			t.Parallel()
			f := newEngineFixture()
			message, err := f.engine.Send(t.Context(), engineRequest("ask", "retained input"), f.caller, "")
			if err != nil {
				t.Fatal(err)
			}
			release := f.engine.BindHost() // Events is not installed until this boundary test.
			defer release()
			topic := workersessions.Topic("lead")
			registrations, calls := 0, 0
			f.engine.events = &recipientRegistrationStream{register: func(request events.SubscribeRequest) (events.Subscription, error) {
				registrations++
				if boundary != "recovered" || registrations == 2 {
					ended := f.authority.recipients["lead"]
					ended.Observation.State, ended.Observation.Revivable = workersessions.StateCompleted, true
					f.authority.recipients["lead"] = ended
				}
				if boundary == "unavailable" {
					return nil, agentmessages.ErrStreamUnavailable
				}
				if registrations == 2 && request.From != (events.Cursor{Topic: topic, Position: 9}) {
					t.Fatal("recovery did not register at the authoritative Events head")
				}
				return func(context.Context) events.Delivery { return events.Delivery{Kind: events.DeliveryBackpressure} }, nil
			}, read: func(events.ReadRequest) (events.ReadResult, error) {
				return events.ReadResult{Outcome: events.ReadOutcomeAtHead, Next: events.Cursor{Topic: topic, Position: 9},
					Retained: events.RetainedRange{Topic: topic, Earliest: 1, Head: 9}}, nil
			}}
			f.engine.continueSession = func(_ context.Context, request workersessions.ContinueRequest) (workersessions.ContinueResult, error) {
				calls++
				if request.RequestID != message.MessageID || request.FollowUpInput != message.Body || !request.ResolveHead {
					t.Fatal("registration race lost the durable delivery identity or input")
				}
				return workersessions.ContinueResult{}, workersessions.ErrContinuationNotAccepted
			}
			entry := f.ledger.entries[0]
			f.engine.watchRecipient(t.Context(), entry, topic, "end:factory:lead", &deliveryObserver{cancel: func() {}})
			if calls != 1 || len(f.ledger.intents) != 1 || f.ledger.entries[0].Message.Reason != "REVIVE_FAILED" {
				t.Fatal("lost terminal notification suppressed admission or guessed delivery")
			}
		})
	}
}

func TestConcurrentRecipientEndWakeupsSharePendingAdmission(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	for _, body := range []string{"first", "second"} {
		if _, err := f.engine.Send(t.Context(), engineRequest(body, body), f.caller, ""); err != nil {
			t.Fatal(err)
		}
	}
	entry := f.ledger.entries[0]
	ended := f.authority.recipients["lead"]
	ended.Observation.State, ended.Observation.Revivable = workersessions.StateCompleted, true
	f.authority.recipients["lead"] = ended
	started, proceed, finished := make(chan workersessions.ContinueRequest, 2), make(chan struct{}), make(chan struct{})
	f.engine.continueSession = func(_ context.Context, request workersessions.ContinueRequest) (workersessions.ContinueResult, error) {
		started <- request
		<-proceed
		return workersessions.ContinueResult{RequestID: request.RequestID, SourceWorkerSessionID: request.SourceWorkerSessionID,
			SuccessorWorkerSessionID: request.SuccessorWorkerSessionID, Session: workersessions.Session{ID: request.SuccessorWorkerSessionID}}, nil
	}
	release := f.engine.BindHost()
	defer release()
	var unblock sync.Once
	observer := &deliveryObserver{cancel: func() {}}
	go func() {
		defer close(finished)
		f.engine.deliverEndedRecipient(t.Context(), entry, "end:factory:lead", observer)
	}()
	defer func() {
		unblock.Do(func() { close(proceed) })
		<-finished
	}()
	request := <-started
	if request.RequestID != entry.Message.MessageID || request.FollowUpInput != "first\n\nsecond" {
		t.Fatal("end admission did not freeze the ordered batch")
	}
	var wakeups sync.WaitGroup
	for range 8 {
		wakeups.Add(1)
		go func() {
			defer wakeups.Done()
			if !f.engine.deliverEndedRecipient(t.Context(), entry, "end:factory:lead", observer) {
				t.Error("duplicate terminal wakeup lost authoritative completion")
			}
		}()
	}
	wakeups.Wait()
	select {
	case <-started:
		t.Fatal("duplicate terminal wakeups branched a pending admission")
	default:
	}
	unblock.Do(func() { close(proceed) })
	<-finished
	if len(f.ledger.intents) != 1 || len(f.ledger.entries) != 2 {
		t.Fatal("duplicate wakeups invented an intent or public message")
	}
	for _, retained := range f.ledger.entries {
		if retained.Message.Status != agentmessages.Queued || retained.Message.Reason != "REVIVE_PENDING" || retained.Message.RevivedWorkerSessionID != "" {
			t.Fatal("duplicate wakeups guessed provider-confirmed delivery")
		}
	}
}

type observationLogger struct {
	logging.NoopLogger
	buffer *bytes.Buffer
}

func (l observationLogger) Warn(message string, fields ...any) {
	fmt.Fprintln(l.buffer, message, fields)
}
