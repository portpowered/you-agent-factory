package admission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"reflect"
	"sync"
	"testing"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/agent_messages/internal/store"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// These doubles isolate the admission coordinator; real authorization,
// validation, quota and journal behavior have their own component tests.
type engineAuthority struct {
	sender     Identity
	recipients map[string]Identity
	denied     bool
	finalError error
	finalCalls int
}

func (a *engineAuthority) Authenticate(context.Context, *workersessions.CallerIdentity) (Identity, error) {
	return a.sender, nil
}

func (a *engineAuthority) Recipient(_ context.Context, worker, factory string) (Identity, error) {
	r, found := a.recipients[worker]
	if !found || (factory != "" && r.Observation.FactorySessionID != factory) {
		return Identity{}, agentmessages.ErrRecipientNotFound
	}
	return r, nil
}

func (a *engineAuthority) PermitSend(context.Context, Identity, Identity) error {
	if a.denied {
		return agentmessages.ErrNotPermitted
	}
	return nil
}

func (a *engineAuthority) Revalidate(context.Context, *workersessions.CallerIdentity) error {
	a.finalCalls++
	return a.finalError
}

type engineValidator struct{}

func (engineValidator) Prepare(r agentmessages.SendRequest, _ string) (Prepared, error) {
	r = detachedDefaults(r, DefaultExpirySeconds)
	return Prepared{Request: r, BodySHA256: digest([]byte(r.Body))}, nil
}

type engineQuota struct {
	limit int
	calls int
}

func (q *engineQuota) Check(_ time.Time, _ string, _ string, _ int, entries []store.Entry) error {
	q.calls++
	if len(entries) >= q.limit {
		return agentmessages.ErrLimitExceeded
	}
	return nil
}

type engineLedger struct {
	entries      []store.Entry
	requests     map[string]store.Request
	sequence     uint64
	transactions []store.Transaction
	commitError  error
	intents      map[string]store.DeliveryIntent
	beforeCommit func(store.Transaction)
}

func (l *engineLedger) DeliveryIntents() ([]store.DeliveryIntent, error) {
	result := []store.DeliveryIntent{}
	for _, intent := range l.intents {
		result = append(result, intent)
	}
	return result, nil
}

func (l *engineLedger) Entries() ([]store.Entry, uint64, error) {
	return append([]store.Entry{}, l.entries...), l.sequence, nil
}

func (l *engineLedger) Matching(filter store.Filter) ([]store.Entry, uint64, error) {
	result := []store.Entry{}
	for _, entry := range l.entries {
		m := entry.Message
		if !ledgerAudience(filter, entry) {
			continue
		}
		if (filter.ToWorkerSessionID == "" || filter.ToWorkerSessionID == m.To.WorkerSessionID) &&
			(filter.FromWorkerSessionID == "" || filter.FromWorkerSessionID == m.From.WorkerSessionID) &&
			(filter.ThreadID == "" || filter.ThreadID == m.ThreadID) &&
			(filter.WorkID == "" || filter.WorkID == m.Correlation.WorkID) &&
			(filter.FactorySessionID == "" || filter.FactorySessionID == m.Correlation.FactorySessionID) {
			result = append(result, entry)
		}
	}
	return result, l.sequence, nil
}

func ledgerAudience(filter store.Filter, entry store.Entry) bool {
	if filter.ReaderChain == "" && filter.ReaderWork == "" {
		return true
	}
	recipient := (filter.ReaderChain != "" && filter.ReaderChain == entry.RecipientChainIdentity) ||
		(filter.ReaderWork != "" && filter.ReaderWork == entry.RecipientWorkIdentity)
	sender := (filter.ReaderChain != "" && filter.ReaderChain == entry.SenderChainIdentity) ||
		(filter.ReaderWork != "" && filter.ReaderWork == entry.SenderWorkIdentity)
	return recipient || (!filter.RecipientOnly && sender)
}

func (l *engineLedger) LookupMessage(id string) (store.Entry, bool, uint64, error) {
	for _, entry := range l.entries {
		if entry.Message.MessageID == id {
			return entry, true, l.sequence, nil
		}
	}
	return store.Entry{}, false, l.sequence, nil
}

func (l *engineLedger) LookupRequest(sender, request string) (store.Request, bool, error) {
	r, found := l.requests[sender+":"+request]
	return r, found, nil
}

func (l *engineLedger) Commit(transaction store.Transaction) error {
	if l.beforeCommit != nil {
		l.beforeCommit(transaction)
	}
	if l.commitError != nil {
		return l.commitError
	}
	for _, changed := range transaction.Messages {
		found := false
		for i, old := range l.entries {
			if old.Message.MessageID == changed.Message.MessageID {
				l.entries[i], found = changed, true
			}
		}
		if !found {
			l.entries = append(l.entries, changed)
		}
	}
	for _, request := range transaction.Requests {
		l.requests[request.SenderIdentity+":"+request.RequestID] = request
	}
	if l.intents == nil {
		l.intents = map[string]store.DeliveryIntent{}
	}
	for _, intent := range transaction.DeliveryIntents {
		l.intents[intent.RequestID] = intent
	}
	l.sequence = transaction.Sequence
	l.transactions = append(l.transactions, transaction)
	return nil
}

type engineFixture struct {
	engine    *Engine
	authority *engineAuthority
	ledger    *engineLedger
	quota     *engineQuota
	caller    *workersessions.CallerIdentity
	now       time.Time
}

func newEngineFixture() *engineFixture {
	f := &engineFixture{now: time.Date(2026, 10, 11, 1, 0, 0, 0, time.UTC)}
	f.authority = &engineAuthority{sender: engineIdentity("worker"), recipients: map[string]Identity{"lead": engineIdentity("lead"), "worker": engineIdentity("worker")}}
	f.ledger = &engineLedger{requests: make(map[string]store.Request)}
	f.quota = &engineQuota{limit: 100}
	f.caller = &workersessions.CallerIdentity{WorkerSessionID: "worker", Token: "caller-secret"}
	id := 0
	f.engine = NewEngine(true, f.authority, engineValidator{}, f.quota, f.ledger, func() time.Time { return f.now }, platformclock.Real{}, func() string { id++; return fmt.Sprintf("id-%d", id) }, []byte("controlled-cursor-signing-key-32bytes"), nil, nil,
		func(context.Context, workersessions.ContinueRequest) (workersessions.ContinueResult, error) {
			return workersessions.ContinueResult{}, workersessions.ErrContinuationExecutionUnavailable
		}, nil, nil)
	return f
}

func engineIdentity(id string) Identity {
	return Identity{Chain: id + "-chain", Work: id + "-work", Observation: workersessions.Observation{WorkerSessionID: id, FactorySessionID: "factory", State: workersessions.StateRunning, Direct: true}}
}

func engineRequest(id, body string) agentmessages.SendRequest {
	return agentmessages.SendRequest{RequestID: id, To: &agentmessages.Address{WorkerSessionID: "lead"}, Body: body}
}

func TestRecipientEndConsumesDurableAuthorityWithoutSenderCredentials(t *testing.T) {
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
	f.authority.finalError = agentmessages.ErrNotPermitted // the sender has ended
	calls := 0
	f.engine.continueSession = func(_ context.Context, req workersessions.ContinueRequest) (workersessions.ContinueResult, error) {
		calls++
		if req.FollowUpInput != "first\n\nsecond" || req.RequestID != entry.Message.MessageID || !req.ResolveHead {
			t.Fatal("end delivery changed durable order or request identity")
		}
		return workersessions.ContinueResult{RequestID: req.RequestID, SourceWorkerSessionID: req.SourceWorkerSessionID,
			SuccessorWorkerSessionID: req.SuccessorWorkerSessionID, Session: workersessions.Session{ID: req.SuccessorWorkerSessionID}}, nil
	}
	release := f.engine.BindHost()
	defer release()
	observer := &deliveryObserver{cancel: func() {}}
	for range 2 {
		if !f.engine.deliverEndedRecipient(t.Context(), entry, "end:factory:lead", observer) {
			t.Fatal("authoritative terminal head did not settle end observation")
		}
	}
	if calls != 1 || f.authority.finalCalls != 2 || len(f.ledger.intents) != 1 {
		t.Fatal("replayed terminal wakeup branched or reused sender credentials")
	}
	for _, entry := range f.ledger.entries {
		if entry.Message.Status != agentmessages.Queued || entry.Message.Reason != "REVIVE_PENDING" || entry.Message.RevivedWorkerSessionID != "" {
			t.Fatal("end admission claimed unconfirmed provider delivery")
		}
	}
}

func TestSendRevivePublishesOnlyDurableAdmission(t *testing.T) {
	t.Parallel()
	for _, cell := range []struct {
		name, state, reason string
		fault               error
		malformed           bool
	}{
		{"admitted", store.IntentAdmitted, "REVIVE_PENDING", nil, false},
		{"unsupported", store.IntentFailed, "REVIVE_UNSUPPORTED", workersessions.ErrContinuationProviderSessionMissing, false},
		{"rejected", store.IntentFailed, "REVIVE_FAILED", workersessions.ErrContinuationNotAccepted, false},
		{"canceled wait", store.IntentUncertain, "REVIVE_FAILED", context.Canceled, false},
		{"unknown outcome", store.IntentUncertain, "REVIVE_FAILED", errors.New("private diagnostic"), false},
		{"foreign result", store.IntentUncertain, "REVIVE_FAILED", nil, true},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			f := newEngineFixture()
			recipient := f.authority.recipients["lead"]
			recipient.Observation.State, recipient.Observation.Revivable = workersessions.StateCompleted, true
			f.authority.recipients["lead"] = recipient
			calls := 0
			f.engine.continueSession = func(_ context.Context, req workersessions.ContinueRequest) (workersessions.ContinueResult, error) {
				calls++
				assertPendingIntent(t, f, req)
				result := workersessions.ContinueResult{RequestID: req.RequestID, SourceWorkerSessionID: req.SourceWorkerSessionID,
					SuccessorWorkerSessionID: req.SuccessorWorkerSessionID, Session: workersessions.Session{ID: req.SuccessorWorkerSessionID}}
				if cell.malformed {
					result.Session.ID = "foreign"
				}
				return result, cell.fault
			}
			request := engineRequest("ask", "safe input")
			got, err := f.engine.Send(t.Context(), request, f.caller, "")
			if err != nil || got.Reason != cell.reason {
				t.Fatalf("send: %+v %v", got, err)
			}
			intents, _ := f.ledger.DeliveryIntents()
			if len(intents) != 1 || intents[0].State != cell.state {
				t.Fatalf("outcome: %+v", intents)
			}
			assertReviveMessage(t, got, intents[0])
			if retry, err := f.engine.Send(t.Context(), request, f.caller, ""); err != nil || retry != got || calls != 1 {
				t.Fatalf("retry repeated delivery: %+v %v %d", retry, err, calls)
			}
		})
	}
}

func TestProviderProgressRequiresResumedWork(t *testing.T) {
	t.Parallel()
	for _, cell := range []struct {
		kind, phase, delivery, native string
		confirms                      bool
	}{
		{"RUN", "STARTED", "NATIVE_STREAM", "turn.started", true},
		{"MESSAGE", "COMPLETED", "NATIVE_FINAL", "message.completed", true},
		{"SESSION", "STARTED", "SYNTHESIZED", "worker_session_lifecycle", false},
		{"PROGRESS", "UPDATED", "NATIVE_STREAM", "session.started", false},
		{"PROGRESS", "UPDATED", "NATIVE_STREAM", "progress.updated", false},
		{"ERROR", "FAILED", "NATIVE_STREAM", "error", false},
		{"RUN", "STARTED", "SYNTHESIZED", "run.started", false},
		{"RUN", "FAILED", "NATIVE_STREAM", "turn.failed", false},
		{"RUN", "INVALID", "NATIVE_STREAM", "turn.started", false},
	} {
		record := providerRecord{Kind: cell.kind, Phase: cell.phase}
		record.Provenance.Provider, record.Provenance.Delivery, record.Provenance.NativeEventType = "codex", cell.delivery, cell.native
		if providerProgress(record) != cell.confirms {
			t.Fatalf("classification differs for %s/%s/%s", cell.kind, cell.delivery, cell.native)
		}
		record.Provenance.Provider = ""
		if providerProgress(record) {
			t.Fatal("unattributed record confirmed provider delivery")
		}
	}
}

func TestProviderOutcomePreservesAdmissionOnUnavailableProofOrFlush(t *testing.T) {
	t.Parallel()
	for _, cell := range []struct {
		name, state, reason string
		readErr, flushErr   error
	}{
		{"typed rejection", store.IntentFailed, "REVIVE_UNSUPPORTED", nil, nil},
		{"unavailable result", store.IntentUncertain, "REVIVE_FAILED", workersessions.ErrSessionNotFound, nil},
		{"refused settlement", store.IntentAdmitted, "REVIVE_PENDING", nil, store.ErrUnavailable},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			f, message, intent := admittedEngineFixture(t)
			f.engine.readSession = func(_ context.Context, req workersessions.GetRequest) (workersessions.Session, error) {
				return workersessions.Session{ID: req.ID, State: workersessions.StateFailed, Result: &workersessions.TerminalResult{
					Outcome: workersessions.TerminalOutcomeFailed, Cause: &workersessions.FailureCause{ProviderFailureKind: providers.ExecuteFailureKindSessionNotFound},
				}}, cell.readErr
			}
			f.ledger.commitError = cell.flushErr
			// A generic native diagnostic is not confirmation. The terminal read
			// supplies classification; no provider body enters this projection.
			records := []string{
				`{"kind":"PROGRESS","phase":"UPDATED","provenance":{"provider":"codex","delivery":"NATIVE_STREAM","nativeEventType":"progress.updated"}}`,
				`{"kind":"SESSION","phase":"FAILED"}`,
			}
			f.engine.consumeDelivery(t.Context(), intent, providerRecordSubscription(records))
			entry, _, _, _ := f.ledger.LookupMessage(message.MessageID)
			if entry.Message.Status != agentmessages.Queued || entry.Message.Reason != cell.reason || entry.Message.RevivedWorkerSessionID != "" || f.ledger.intents[intent.RequestID].State != cell.state {
				t.Fatal("unconfirmed outcome changed admission or claimed delivery")
			}
		})
	}
}

func admittedEngineFixture(t *testing.T) (*engineFixture, agentmessages.Message, store.DeliveryIntent) {
	t.Helper()
	f := newEngineFixture()
	ended := f.authority.recipients["lead"]
	ended.Observation.State, ended.Observation.Revivable = workersessions.StateCompleted, true
	f.authority.recipients["lead"] = ended
	f.engine.continueSession = func(_ context.Context, req workersessions.ContinueRequest) (workersessions.ContinueResult, error) {
		return workersessions.ContinueResult{RequestID: req.RequestID, SourceWorkerSessionID: req.SourceWorkerSessionID, SuccessorWorkerSessionID: req.SuccessorWorkerSessionID,
			Session: workersessions.Session{ID: req.SuccessorWorkerSessionID}}, nil
	}
	message, err := f.engine.Send(t.Context(), engineRequest("ask", "safe input"), f.caller, "")
	if err != nil || message.Reason != "REVIVE_PENDING" {
		t.Fatal("admission was not pending", err)
	}
	return f, message, f.ledger.intents[message.MessageID]
}

func providerRecordSubscription(records []string) events.Subscription {
	return func(context.Context) events.Delivery {
		if len(records) == 0 {
			return events.Delivery{Kind: events.DeliveryClosed}
		}
		payload := records[0]
		records = records[1:]
		return events.Delivery{Kind: events.DeliveryRecord, Record: events.Record{SchemaID: "workers.draft.v1", Payload: json.RawMessage(payload)}}
	}
}

type deliveryStreamAppender struct {
	observationAppender
	started, stopped chan struct{}
}

type recipientEndStream struct {
	observationAppender
	next    chan struct{}
	wake    chan events.Delivery
	resumed chan events.Cursor
}

func (*recipientEndStream) Read(_ context.Context, request events.ReadRequest) (events.ReadResult, error) {
	return events.ReadResult{Outcome: events.ReadOutcomeGap,
		Gap: &events.GapFacts{Topic: request.Topic, Requested: 0, EarliestRetained: 2, Head: 3}}, nil
}

func (s *recipientEndStream) Subscribe(ctx context.Context, request events.SubscribeRequest) (events.Subscription, error) {
	if request.From.Position != 0 {
		s.resumed <- request.From
	}
	return func(context.Context) events.Delivery {
		select {
		case s.next <- struct{}{}:
		case <-ctx.Done():
			return events.Delivery{Kind: events.DeliveryCanceled}
		}
		select {
		case record := <-s.wake:
			return record
		case <-ctx.Done():
			return events.Delivery{Kind: events.DeliveryCanceled}
		}
	}, nil
}

func TestRecipientObserverSurvivesRetainedGapUntilAuthoritativeEnd(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	message, err := f.engine.Send(t.Context(), engineRequest("ask", "safe input"), f.caller, "")
	if err != nil {
		t.Fatal(err)
	}
	stream := &recipientEndStream{next: make(chan struct{}), wake: make(chan events.Delivery), resumed: make(chan events.Cursor, 1)}
	f.engine.events = stream
	admitted := make(chan workersessions.ContinueRequest, 1)
	f.engine.continueSession = func(_ context.Context, req workersessions.ContinueRequest) (workersessions.ContinueResult, error) {
		admitted <- req
		return workersessions.ContinueResult{RequestID: req.RequestID, SourceWorkerSessionID: req.SourceWorkerSessionID,
			SuccessorWorkerSessionID: req.SuccessorWorkerSessionID, Session: workersessions.Session{ID: req.SuccessorWorkerSessionID}}, nil
	}
	release := f.engine.BindHost()
	defer release()
	awaitRecipientRead(t, stream.next)
	topic := workersessions.Topic("lead")
	// Position one is genuinely missing, rather than merely preceding the
	// earliest retained position. The component consumes the public gap tuple.
	stream.wake <- events.Delivery{Kind: events.DeliveryGap, Gap: &events.GapFacts{Topic: topic, Requested: 0, EarliestRetained: 2, Head: 3}}
	awaitRecipientRead(t, stream.next)
	stream.wake <- events.Delivery{Kind: events.DeliveryBackpressure}
	awaitRecipientRead(t, stream.next)
	if cursor := <-stream.resumed; cursor.Topic != topic || cursor.Position != 3 {
		t.Fatal("backpressure recovery did not resume at the retained head")
	}
	// A stale terminal record is a wakeup, never authority to revive a
	// currently running recipient. Its later authoritative end is decisive.
	stream.wake <- events.Delivery{Kind: events.DeliveryRecord, Record: events.Record{
		SchemaID: "workers.draft.v1", Payload: json.RawMessage(`{"kind":"SESSION","phase":"COMPLETED"}`)}}
	awaitRecipientRead(t, stream.next)
	select {
	case <-admitted:
		t.Fatal("a gap guessed an ended recipient")
	default:
	}
	f.engine.mu.Lock()
	ended := f.authority.recipients["lead"]
	ended.Observation.State, ended.Observation.Revivable = workersessions.StateCompleted, true
	f.authority.recipients["lead"] = ended
	f.engine.mu.Unlock()
	stream.wake <- events.Delivery{Kind: events.DeliveryClosed}
	select {
	case req := <-admitted:
		if req.RequestID != message.MessageID || req.FollowUpInput != message.Body {
			t.Fatal("gap recovery lost durable message")
		}
	case <-t.Context().Done():
		t.Fatal("retained gap lost live end delivery")
	}
	release()
	if len(f.ledger.intents) != 1 {
		t.Fatal("end replay branched after retained gap")
	}
}

func awaitRecipientRead(t *testing.T, next <-chan struct{}) {
	t.Helper()
	select {
	case <-next:
	case <-t.Context().Done():
		t.Fatal("recipient observer stopped before live end")
	}
}

func TestHostReleaseCancelsAndJoinsEndDeliveryAdmission(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	if _, err := f.engine.Send(t.Context(), engineRequest("ask", "safe input"), f.caller, ""); err != nil {
		t.Fatal(err)
	}
	ended := f.authority.recipients["lead"]
	ended.Observation.State, ended.Observation.Revivable = workersessions.StateCompleted, true
	f.authority.recipients["lead"] = ended
	f.engine.events = &recipientEndStream{next: make(chan struct{}), wake: make(chan events.Delivery)}
	started, stopped := make(chan struct{}), make(chan struct{})
	f.engine.continueSession = func(ctx context.Context, _ workersessions.ContinueRequest) (workersessions.ContinueResult, error) {
		close(started)
		<-ctx.Done()
		close(stopped)
		return workersessions.ContinueResult{}, ctx.Err()
	}
	release := f.engine.BindHost()
	defer release()
	awaitRecipientRead(t, started)
	release()
	select {
	case <-stopped:
	default:
		t.Fatal("host release did not cancel and join end admission")
	}
	if f.ledger.entries[0].Message.Status != agentmessages.Queued || f.ledger.entries[0].Message.Reason != "REVIVE_FAILED" {
		t.Fatal("canceled end admission lost the queued message or guessed delivery")
	}
}

func (s *deliveryStreamAppender) Subscribe(ctx context.Context, request events.SubscribeRequest) (events.Subscription, error) {
	close(s.started)
	return func(context.Context) events.Delivery {
		<-ctx.Done()
		close(s.stopped)
		return events.Delivery{Kind: events.DeliveryCanceled}
	}, nil
}

func TestLastHostReleaseJoinsObserversAndPreservesPendingAdmission(t *testing.T) {
	t.Parallel()
	f, message, intent := admittedEngineFixture(t)
	stream := &deliveryStreamAppender{started: make(chan struct{}), stopped: make(chan struct{})}
	f.engine.events = stream
	first, last := f.engine.BindHost(), f.engine.BindHost()
	f.engine.mu.Lock()
	f.engine.observeDelivery(t.Context(), intent)
	f.engine.mu.Unlock()
	select {
	case <-stream.started:
	case <-t.Context().Done():
		t.Fatal("observer did not start")
	}
	first()
	first() // release is idempotent and cannot cancel another live host
	select {
	case <-stream.stopped:
		t.Fatal("another live host lost its observer")
	default:
	}
	last()
	select {
	case <-stream.stopped:
	default:
		t.Fatal("last host did not join observer")
	}
	entry, _, _, _ := f.ledger.LookupMessage(message.MessageID)
	if entry.Message.Reason != "REVIVE_PENDING" || f.ledger.intents[intent.RequestID].State != store.IntentAdmitted {
		t.Fatal("shutdown guessed a provider outcome")
	}
}

func assertPendingIntent(t *testing.T, f *engineFixture, req workersessions.ContinueRequest) {
	t.Helper()
	intents, _ := f.ledger.DeliveryIntents()
	if len(intents) != 1 || intents[0].State != store.IntentPending || req.RequestID != intents[0].RequestID ||
		req.SuccessorWorkerSessionID != intents[0].SuccessorWorkerSessionID || req.FollowUpInput != "safe input" || !req.ResolveHead {
		t.Fatal("effect preceded durable immutable intent")
	}
	if f.ledger.entries[0].Message.Status != agentmessages.Queued {
		t.Fatal("delivery published before admission")
	}
}

func assertReviveMessage(t *testing.T, got agentmessages.Message, intent store.DeliveryIntent) {
	t.Helper()
	if intent.State == store.IntentAccepted {
		if got.Status != agentmessages.DeliveredRevive || got.RevivedWorkerSessionID != intent.SuccessorWorkerSessionID ||
			got.To.DeliveredWorkerSessionID != got.RevivedWorkerSessionID {
			t.Fatal("missing admitted successor facts")
		}
	} else if got.Status != agentmessages.Queued || got.RevivedWorkerSessionID != "" {
		t.Fatal("failure claimed delivery")
	}
}

func TestConcurrentSendsQueueAgainstAdmittedHead(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	recipient := f.authority.recipients["lead"]
	recipient.Observation.State, recipient.Observation.Revivable = workersessions.StateCompleted, true
	f.authority.recipients["lead"] = recipient
	calls := 0
	f.engine.continueSession = func(_ context.Context, req workersessions.ContinueRequest) (workersessions.ContinueResult, error) {
		f.engine.mu.Lock()
		defer f.engine.mu.Unlock()
		calls++
		head := recipient
		head.Observation.WorkerSessionID, head.Observation.State = req.SuccessorWorkerSessionID, workersessions.StateRunning
		f.authority.recipients[req.SuccessorWorkerSessionID] = head
		recipient.Observation.ContinuationHeadWorkerSessionID = req.SuccessorWorkerSessionID
		f.authority.recipients["lead"] = recipient
		return workersessions.ContinueResult{RequestID: req.RequestID, SourceWorkerSessionID: req.SourceWorkerSessionID,
			SuccessorWorkerSessionID: req.SuccessorWorkerSessionID, Session: workersessions.Session{ID: req.SuccessorWorkerSessionID}}, nil
	}
	results := make(chan agentmessages.Message, 2)
	errCh := make(chan error, 2)
	for _, body := range []string{"first", "second"} {
		go func() {
			m, err := f.engine.Send(t.Context(), engineRequest(body, body), f.caller, "")
			results <- m
			errCh <- err
		}()
	}
	statuses := map[agentmessages.Status]int{}
	for range 2 {
		statuses[(<-results).Status]++
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 || statuses[agentmessages.Queued] != 2 {
		t.Fatal("concurrent sends branched", calls, statuses)
	}
}

func TestReviveOutcomeFaultKeepsPendingIntent(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	recipient := f.authority.recipients["lead"]
	recipient.Observation.State, recipient.Observation.Revivable = workersessions.StateCompleted, true
	f.authority.recipients["lead"] = recipient
	f.engine.continueSession = func(_ context.Context, req workersessions.ContinueRequest) (workersessions.ContinueResult, error) {
		f.ledger.commitError = store.ErrUnavailable
		return workersessions.ContinueResult{RequestID: req.RequestID, SourceWorkerSessionID: req.SourceWorkerSessionID,
			SuccessorWorkerSessionID: req.SuccessorWorkerSessionID, Session: workersessions.Session{ID: req.SuccessorWorkerSessionID}}, nil
	}
	if _, err := f.engine.Send(t.Context(), engineRequest("ask", "safe input"), f.caller, ""); !errors.Is(err, store.ErrUnavailable) {
		t.Fatal(err)
	}
	intents, _ := f.ledger.DeliveryIntents()
	if intents[0].State != store.IntentPending || f.ledger.entries[0].Message.Status != agentmessages.Queued {
		t.Fatal("failed flush published delivery")
	}
}

func TestEngineSendFlushRetryAndAlias(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	request := engineRequest("request-1", "question")
	f.ledger.beforeCommit = func(transaction store.Transaction) {
		if len(f.ledger.entries) != 0 || f.authority.finalCalls != 1 || len(transaction.Messages) != 1 {
			t.Fatal("publication preceded final authority check and commit")
		}
	}
	first, err := f.engine.Send(context.Background(), request, f.caller, "factory")
	if err != nil || first.Status != agentmessages.Queued || first.ThreadID != first.MessageID {
		t.Fatalf("send = %+v, %v", first, err)
	}
	f.ledger.beforeCommit = nil
	for i := 0; i < 2; i++ {
		if i == 1 {
			request.RequestID = "alias"
		}
		retry, err := f.engine.Send(context.Background(), request, f.caller, "factory")
		if err != nil || !reflect.DeepEqual(retry, first) {
			t.Fatalf("retry = %+v, %v", retry, err)
		}
	}
	if len(f.ledger.entries) != 1 || len(f.ledger.requests) != 2 || f.quota.calls != 1 || f.ledger.transactions[1].Kind != store.RequestAlias {
		t.Fatal("retry or alias consumed another successful send")
	}
}

func TestEngineExactReplayRetainsArchivedRecipientFacts(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	request := engineRequest("request", "question")
	original, err := f.engine.Send(t.Context(), request, f.caller, "factory")
	if err != nil {
		t.Fatal(err)
	}
	delete(f.authority.recipients, "lead")
	replayed, err := f.engine.Send(t.Context(), request, f.caller, "")
	if err != nil || !reflect.DeepEqual(original, replayed) {
		t.Fatalf("archived exact retry = %#v, %v", replayed, err)
	}
	if _, err := f.engine.Send(t.Context(), request, f.caller, "foreign"); !errors.Is(err, agentmessages.ErrRequestConflict) {
		t.Fatalf("changed explicit owner = %v", err)
	}
	request.Body = "changed"
	if _, err := f.engine.Send(t.Context(), request, f.caller, ""); !errors.Is(err, agentmessages.ErrRequestConflict) {
		t.Fatalf("changed archived retry = %v", err)
	}
	request.Body, request.RequestID = "question", "new-request"
	if _, err := f.engine.Send(t.Context(), request, f.caller, ""); !errors.Is(err, agentmessages.ErrRecipientNotFound) {
		t.Fatalf("new send bypassed ordinary recipient resolution: %v", err)
	}
	if len(f.ledger.transactions) != 1 || f.quota.calls != 1 {
		t.Fatal("archived retry or refusal created a send")
	}
}

func TestEngineChangedRequestConflictsWithoutCommit(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	request := engineRequest("request", "question")
	if _, err := f.engine.Send(context.Background(), request, f.caller, ""); err != nil {
		t.Fatal(err)
	}
	request.RequestID = "alias"
	if _, err := f.engine.Send(context.Background(), request, f.caller, ""); err != nil {
		t.Fatal(err)
	}
	request.Body = "changed"
	if _, err := f.engine.Send(context.Background(), request, f.caller, ""); !errors.Is(err, agentmessages.ErrRequestConflict) {
		t.Fatalf("changed alias = %v", err)
	}
	if len(f.ledger.transactions) != 2 || len(f.ledger.entries) != 1 {
		t.Fatal("request conflict committed a change")
	}
}

func TestEngineReplyIsOneTransactionAndRetryUsesParentDefaults(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	request := engineRequest("question", "question")
	request.ReplyIfEnded = "HOLD"
	parent, err := f.engine.Send(context.Background(), request, f.caller, "")
	if err != nil {
		t.Fatal(err)
	}
	f.authority.sender = engineIdentity("lead")
	f.caller.WorkerSessionID = "lead"
	reply := agentmessages.SendRequest{RequestID: "reply", Body: "answer", InReplyTo: parent.MessageID}
	message, err := f.engine.Send(context.Background(), reply, f.caller, "unrelated-selection")
	if err != nil || message.To.WorkerSessionID != "worker" || message.IfEnded != "HOLD" || message.ThreadID != parent.ThreadID || message.Hop != 0 {
		t.Fatalf("reply = %+v, %v", message, err)
	}
	transaction := f.ledger.transactions[1]
	if transaction.Kind != store.Replied || len(transaction.Messages) != 2 || transaction.Messages[1].Message.RepliedByMessageID != message.MessageID || transaction.Messages[1].Message.Status != agentmessages.Replied {
		t.Fatalf("reply was not atomic: %+v", transaction)
	}
	retry, err := f.engine.Send(context.Background(), reply, f.caller, "")
	if err != nil || !reflect.DeepEqual(retry, message) || len(f.ledger.transactions) != 2 {
		t.Fatalf("reply retry = %+v, %v", retry, err)
	}
	reply.RequestID, reply.Body = "second-reply", "another answer"
	if _, err := f.engine.Send(context.Background(), reply, f.caller, ""); !errors.Is(err, agentmessages.ErrBadRequest) {
		t.Fatalf("second reply = %v", err)
	}
}

func TestEngineRepliesDoNotConsumeForwardHops(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	parent, err := f.engine.Send(context.Background(), engineRequest("root", "question"), f.caller, "")
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < DefaultThreadMessages; i++ {
		f.authority.sender = f.authority.recipients[parent.To.WorkerSessionID]
		f.caller.WorkerSessionID = parent.To.WorkerSessionID
		request := agentmessages.SendRequest{RequestID: fmt.Sprint(i), Body: fmt.Sprint(i), InReplyTo: parent.MessageID}
		message, err := f.engine.Send(context.Background(), request, f.caller, "")
		if err != nil || message.Hop != 0 || message.ThreadID != parent.ThreadID {
			t.Fatalf("reply %d = %+v, %v", i, message, err)
		}
		parent = message
	}
	if len(f.ledger.entries) != DefaultThreadMessages {
		t.Fatal("ordinary conversation ended at the forwarding limit")
	}
}

func TestEngineFailedReplyPreservesParentAndRequestAuthority(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	parent, err := f.engine.Send(context.Background(), engineRequest("question", "question"), f.caller, "")
	if err != nil {
		t.Fatal(err)
	}
	f.authority.sender = engineIdentity("lead")
	f.ledger.commitError = store.ErrUnavailable
	reply := agentmessages.SendRequest{RequestID: "reply", Body: "answer", InReplyTo: parent.MessageID}
	failed, err := f.engine.Send(context.Background(), reply, f.caller, "")
	if !errors.Is(err, store.ErrUnavailable) || failed.MessageID != "" || !reflect.DeepEqual(f.ledger.entries[0].Message, parent) || len(f.ledger.requests) != 1 {
		t.Fatal("failed reply published state or spent request authority")
	}
	f.ledger.commitError = nil
	if _, err := f.engine.Send(context.Background(), reply, f.caller, ""); err != nil {
		t.Fatalf("retry after failed commit: %v", err)
	}
}

func TestEngineFinalAuthorityLossPreventsWriteAndRetryDisclosure(t *testing.T) {
	t.Parallel()
	for _, existing := range []bool{false, true} {
		f := newEngineFixture()
		request := engineRequest("request", "question")
		if existing {
			if _, err := f.engine.Send(context.Background(), request, f.caller, ""); err != nil {
				t.Fatal(err)
			}
		}
		before := len(f.ledger.transactions)
		f.authority.finalError = agentmessages.ErrNotPermitted
		message, err := f.engine.Send(context.Background(), request, f.caller, "")
		if !errors.Is(err, agentmessages.ErrNotPermitted) || message.MessageID != "" || len(f.ledger.transactions) != before {
			t.Fatal("lost authority disclosed or committed a message")
		}
	}
}

func TestEngineConcurrentWritesShareAdmissionLimitAndRequestKey(t *testing.T) {
	t.Parallel()
	for _, identical := range []bool{true, false} {
		f := newEngineFixture()
		f.quota.limit = 1
		var group sync.WaitGroup
		errorsSeen := make(chan error, 8)
		for i := 0; i < 8; i++ {
			group.Add(1)
			go func(i int) {
				defer group.Done()
				id := "same"
				if !identical {
					id = fmt.Sprint(i)
				}
				_, err := f.engine.Send(context.Background(), engineRequest(id, id), f.caller, "")
				errorsSeen <- err
			}(i)
		}
		group.Wait()
		close(errorsSeen)
		succeeded := 0
		for err := range errorsSeen {
			if err == nil {
				succeeded++
			} else if !errors.Is(err, agentmessages.ErrLimitExceeded) {
				t.Fatal(err)
			}
		}
		want := 1
		if identical {
			want = 8
		}
		if succeeded != want || len(f.ledger.entries) != 1 || len(f.ledger.transactions) != 1 {
			t.Fatalf("succeeded=%d want=%d entries=%d transactions=%d", succeeded, want, len(f.ledger.entries), len(f.ledger.transactions))
		}
	}
}

func TestEngineDedupWindowAndEndedRecipient(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	ended := f.authority.recipients["lead"]
	ended.Observation.State = workersessions.StateCompleted
	f.authority.recipients["lead"] = ended
	first, err := f.engine.Send(context.Background(), engineRequest("first", "question"), f.caller, "")
	if err != nil || first.Reason != "REVIVE_UNSUPPORTED" {
		t.Fatalf("ended recipient = %+v, %v", first, err)
	}
	f.now = f.now.Add(10 * time.Minute)
	second, err := f.engine.Send(context.Background(), engineRequest("second", "question"), f.caller, "")
	if err != nil || first.MessageID == second.MessageID || len(f.ledger.entries) != 2 {
		t.Fatalf("dedup boundary = %+v, %v", second, err)
	}
}

func TestTerminalRecipientCapabilitySnapshotDoesNotDiscardDelivery(t *testing.T) {
	t.Parallel()
	for _, rejected := range []bool{false, true} {
		t.Run(fmt.Sprint(rejected), func(t *testing.T) {
			t.Parallel()
			f := newEngineFixture()
			ended := f.authority.recipients["lead"]
			ended.Observation.State = workersessions.StateCompleted
			ended.Observation.ProviderSessionAvailable = true
			ended.Observation.Revivable = false // publication has not finished yet
			f.authority.recipients["lead"] = ended
			calls := 0
			f.engine.continueSession = func(_ context.Context, req workersessions.ContinueRequest) (workersessions.ContinueResult, error) {
				calls++
				if rejected {
					return workersessions.ContinueResult{}, workersessions.ErrContinuationProviderSessionInvalid
				}
				return workersessions.ContinueResult{RequestID: req.RequestID, SourceWorkerSessionID: req.SourceWorkerSessionID,
					SuccessorWorkerSessionID: req.SuccessorWorkerSessionID, Session: workersessions.Session{ID: req.SuccessorWorkerSessionID}}, nil
			}
			request := engineRequest("terminal", "retained question")
			message, err := f.engine.Send(t.Context(), request, f.caller, "")
			if err != nil || calls != 1 || message.Status != agentmessages.Queued || message.RevivedWorkerSessionID != "" {
				t.Fatal("capability snapshot suppressed admission or guessed delivery", err)
			}
			want := "REVIVE_PENDING"
			if rejected {
				want = "REVIVE_UNSUPPORTED"
			}
			if message.Reason != want {
				t.Fatal("authoritative admission outcome was lost")
			}
			if retry, err := f.engine.Send(t.Context(), request, f.caller, ""); err != nil || retry != message || calls != 1 {
				t.Fatal("terminal delivery retry duplicated continuation", err)
			}
		})
	}
}

func TestEngineDeniedAndUnknownTargetsNeverCommit(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"denied", "unknown", "disabled", "no-caller", "unrelated-reply"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f := newEngineFixture()
			request, want := engineRequest("request", "question"), agentmessages.ErrNotPermitted
			switch kind {
			case "denied":
				f.authority.denied = true
			case "unknown":
				request.To.WorkerSessionID, want = "missing", agentmessages.ErrRecipientNotFound
			case "disabled":
				f.engine.enabled, want = false, agentmessages.ErrDisabled
			case "no-caller":
				f.caller = nil
			case "unrelated-reply":
				f.ledger.entries = []store.Entry{{Message: agentmessages.Message{MessageID: "parent"}, RecipientChainIdentity: "unrelated"}}
				request.InReplyTo = "parent"
			}
			if _, err := f.engine.Send(context.Background(), request, f.caller, ""); !errors.Is(err, want) {
				t.Fatalf("error=%v want=%v", err, want)
			}
			if len(f.ledger.transactions) != 0 || f.quota.calls != 0 {
				t.Fatal("denied operation wrote or spent quota")
			}
		})
	}
}

func TestEngineAdmissionExpiresDueMessagesAtomically(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	first, err := f.engine.Send(context.Background(), engineRequest("first", "question"), f.caller, "")
	if err != nil {
		t.Fatal(err)
	}
	f.now = first.ExpiresAt
	next, err := f.engine.Send(context.Background(), engineRequest("next", "new question"), f.caller, "")
	if err != nil {
		t.Fatal(err)
	}
	tx := f.ledger.transactions[1]
	if len(tx.Messages) != 2 || tx.Messages[0].Message.MessageID != next.MessageID || tx.Messages[1].Message.Status != agentmessages.Expired {
		t.Fatalf("admission did not include durable expiry: %+v", tx)
	}
	if f.ledger.entries[0].Message.Status != agentmessages.Expired || f.quota.calls != 2 {
		t.Fatal("expiry publication or successful-send accounting changed")
	}
}

func TestEngineDueRetryAndAliasReturnExpiredWithoutAnotherSend(t *testing.T) {
	t.Parallel()
	for _, alias := range []bool{false, true} {
		t.Run(fmt.Sprint(alias), func(t *testing.T) {
			t.Parallel()
			f := newEngineFixture()
			request := engineRequest("first", "question")
			seconds := 60
			request.ExpiresInSeconds = &seconds
			first, err := f.engine.Send(context.Background(), request, f.caller, "")
			if err != nil {
				t.Fatal(err)
			}
			f.now = first.ExpiresAt
			if alias {
				request.RequestID = "alias"
			}
			got, err := f.engine.Send(context.Background(), request, f.caller, "")
			if err != nil || got.MessageID != first.MessageID || got.Status != agentmessages.Expired {
				t.Fatalf("due retry = %+v, %v", got, err)
			}
			if len(f.ledger.entries) != 1 || len(f.ledger.transactions) != 2 || f.quota.calls != 1 || f.ledger.entries[0].Message.Status != agentmessages.Expired {
				t.Fatal("due retry created a send or omitted durable expiry")
			}
			got, err = f.engine.Send(context.Background(), request, f.caller, "")
			if err != nil || got.Status != agentmessages.Expired || len(f.ledger.transactions) != 2 {
				t.Fatal("expired retry was not idempotent", err)
			}
		})
	}
}

func TestEngineFailedAdmissionDoesNotPublishExpiry(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"denial", "final-authority", "store", "conflict"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			f := newEngineFixture()
			first, err := f.engine.Send(context.Background(), engineRequest("first", "question"), f.caller, "")
			if err != nil {
				t.Fatal(err)
			}
			f.now = first.ExpiresAt
			request := engineRequest("next", "new question")
			switch failure {
			case "denial":
				f.authority.denied = true
			case "final-authority":
				f.authority.finalError = agentmessages.ErrNotPermitted
			case "store":
				f.ledger.commitError = store.ErrUnavailable
			case "conflict":
				request.RequestID = "first"
			}
			if _, err := f.engine.Send(context.Background(), request, f.caller, ""); err == nil {
				t.Fatal("failed admission succeeded")
			}
			if len(f.ledger.transactions) != 1 || len(f.ledger.entries) != 1 || f.ledger.entries[0].Message.Status != agentmessages.Queued {
				t.Fatal("failed admission changed existing message expiry")
			}
		})
	}
}

func TestEngineReplyExpiryLeavesConversationTerminalStatesIntact(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	short := engineRequest("short", "short-lived question")
	seconds := 60
	short.ExpiresInSeconds = &seconds
	due, err := f.engine.Send(context.Background(), short, f.caller, "")
	if err != nil {
		t.Fatal(err)
	}
	parent, err := f.engine.Send(context.Background(), engineRequest("parent", "long-lived question"), f.caller, "")
	if err != nil {
		t.Fatal(err)
	}
	f.now = due.ExpiresAt
	f.authority.sender = engineIdentity("lead")
	f.caller.WorkerSessionID = "lead"
	reply, err := f.engine.Send(context.Background(), agentmessages.SendRequest{RequestID: "reply", Body: "answer", InReplyTo: parent.MessageID}, f.caller, "")
	if err != nil {
		t.Fatal(err)
	}
	tx := f.ledger.transactions[2]
	if tx.Kind != store.Replied || len(tx.Messages) != 3 || f.ledger.entries[0].Message.Status != agentmessages.Expired || f.ledger.entries[1].Message.Status != agentmessages.Replied {
		t.Fatalf("reply/expiry transaction = %+v", tx)
	}
	f.now = reply.ExpiresAt
	f.authority.sender = engineIdentity("worker")
	f.caller.WorkerSessionID = "worker"
	if _, err := f.engine.Send(context.Background(), engineRequest("after", "later question"), f.caller, ""); err != nil {
		t.Fatal(err)
	}
	if f.ledger.entries[0].Message.Status != agentmessages.Expired || f.ledger.entries[1].Message.Status != agentmessages.Replied || f.ledger.entries[1].Message.RepliedByMessageID != reply.MessageID || f.ledger.entries[2].Message.Status != agentmessages.Expired {
		t.Fatal("later admission regressed terminal conversation state")
	}
}
