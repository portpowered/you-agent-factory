package admission

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/agent_messages/internal/store"
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
	r = detachedDefaults(r)
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
	beforeCommit func(store.Transaction)
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
	f.engine = NewEngine(true, f.authority, engineValidator{}, f.quota, f.ledger, func() time.Time { return f.now }, func() string { id++; return fmt.Sprintf("id-%d", id) }, []byte("controlled-cursor-signing-key-32bytes"), nil, nil)
	return f
}

func engineIdentity(id string) Identity {
	return Identity{Chain: id + "-chain", Work: id + "-work", Observation: workersessions.Observation{WorkerSessionID: id, FactorySessionID: "factory", State: workersessions.StateRunning}}
}

func engineRequest(id, body string) agentmessages.SendRequest {
	return agentmessages.SendRequest{RequestID: id, To: &agentmessages.Address{WorkerSessionID: "lead"}, Body: body}
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
