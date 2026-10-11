package admission

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/agent_messages/internal/store"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestReviveLimitCountsOnlyAcceptedAdmissionsAndReleasesAtHour(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	recipient := f.authority.recipients["lead"]
	recipient.Observation.State, recipient.Observation.Revivable = workersessions.StateCompleted, true
	f.authority.recipients["lead"] = recipient
	calls := 0
	f.engine.continueSession = func(_ context.Context, request workersessions.ContinueRequest) (workersessions.ContinueResult, error) {
		calls++
		result := workersessions.ContinueResult{RequestID: request.RequestID, SourceWorkerSessionID: request.SourceWorkerSessionID,
			SuccessorWorkerSessionID: request.SuccessorWorkerSessionID, Session: workersessions.Session{ID: request.SuccessorWorkerSessionID}}
		if calls == 1 {
			return result, workersessions.ErrContinuationNotAccepted
		}
		return result, nil
	}
	failed, err := f.engine.Send(t.Context(), engineRequest("failed", "failed attempt"), f.caller, "")
	if err != nil || failed.Status != agentmessages.Queued || failed.Reason != "REVIVE_FAILED" {
		t.Fatal("rejection lost queued message", err)
	}
	assertSixRevivesAndRetries(t, f, &calls)
	limited, err := f.engine.Send(t.Context(), engineRequest("seventh", "limited ask"), f.caller, "")
	if err != nil || limited.Status != agentmessages.Queued || limited.Reason != "REVIVE_LIMIT" || calls != 7 {
		t.Fatal("seventh accepted revive was not limited", err)
	}
	// The oldest accepted facts leave the open rolling window exactly at an hour.
	f.now = f.now.Add(time.Hour)
	got, err := f.engine.Send(t.Context(), engineRequest("next-hour", "next hour ask"), f.caller, "")
	got = confirmEngineDelivery(t, f, got)
	if err != nil || got.Status != agentmessages.DeliveredRevive || calls != 8 {
		t.Fatal("rolling window did not release quota", err)
	}
	queued, _, _, err := f.ledger.LookupMessage(limited.MessageID)
	if err != nil || queued.Message.Status != agentmessages.DeliveredRevive || queued.Message.RevivedWorkerSessionID != got.RevivedWorkerSessionID {
		t.Fatal("released quota did not deliver eligible queued ask in the same batch", err)
	}
}

func TestAdmittedUnconfirmedReviveKeepsChainReserved(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	ended := f.authority.recipients["lead"]
	ended.Observation.State, ended.Observation.Revivable = workersessions.StateCompleted, true
	f.authority.recipients["lead"] = ended
	f.ledger.intents = map[string]store.DeliveryIntent{"pending-provider": {
		RequestID: "pending-provider", RecipientChainIdentity: ended.Chain,
		State: store.IntentAdmitted, AcceptedAt: f.now, Reason: "REVIVE_PENDING",
	}}
	calls := 0
	f.engine.continueSession = func(context.Context, workersessions.ContinueRequest) (workersessions.ContinueResult, error) {
		calls++
		return workersessions.ContinueResult{}, nil
	}
	got, err := f.engine.Send(t.Context(), engineRequest("later", "later question"), f.caller, "")
	if err != nil || got.Status != agentmessages.Queued || calls != 0 {
		t.Fatal("pending provider confirmation allowed another continuation", err)
	}
	if len(f.ledger.entries) != 1 || len(f.ledger.transactions) != 1 {
		t.Fatal("pending confirmation lost the later queued ask or invented an effect")
	}
}

func assertSixRevivesAndRetries(t *testing.T, f *engineFixture, calls *int) {
	t.Helper()
	for i := range 6 {
		request := engineRequest(fmt.Sprint(i), fmt.Sprintf("accepted %d", i))
		got, err := f.engine.Send(t.Context(), request, f.caller, "")
		got = confirmEngineDelivery(t, f, got)
		if err != nil || got.Status != agentmessages.DeliveredRevive {
			t.Fatalf("revive %d was not admitted: %v", i+1, err)
		}
		if retry, err := f.engine.Send(t.Context(), request, f.caller, ""); err != nil || retry != got || *calls != i+2 {
			t.Fatal("retry changed delivery or consumed another revive", err)
		}
	}
}

func TestEndedRecipientBatchesOnlyEligibleQueuedBodiesInSendOrder(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	requests := []agentmessages.SendRequest{
		engineRequest("first", "first eligible"), engineRequest("held", "held body"),
		engineRequest("read", "read body"), engineRequest("replied", "replied body"),
		engineRequest("expired", "expired body"), engineRequest("last", "last eligible"),
	}
	requests[1].IfEnded = "HOLD"
	for _, request := range requests {
		if _, err := f.engine.Send(t.Context(), request, f.caller, ""); err != nil {
			t.Fatal(err)
		}
	}
	// Explicit ledger facts isolate selection policy from read/reply/expiry owners.
	f.ledger.entries[2].Message.Status = agentmessages.Read
	f.ledger.entries[3].Message.Status = agentmessages.Replied
	f.ledger.entries[4].Message.ExpiresAt = f.now
	recipient := f.authority.recipients["lead"]
	recipient.Observation.State, recipient.Observation.Revivable = workersessions.StateCompleted, true
	f.authority.recipients["lead"] = recipient
	calls := 0
	f.engine.continueSession = func(_ context.Context, request workersessions.ContinueRequest) (workersessions.ContinueResult, error) {
		calls++
		if request.FollowUpInput != "first eligible\n\nlast eligible\n\nnew eligible" || request.RequestID != f.ledger.entries[0].Message.MessageID || !request.ResolveHead {
			t.Fatal("delivery batch changed input order, eligibility or stable request identity")
		}
		return workersessions.ContinueResult{RequestID: request.RequestID, SourceWorkerSessionID: request.SourceWorkerSessionID,
			SuccessorWorkerSessionID: request.SuccessorWorkerSessionID, Session: workersessions.Session{ID: request.SuccessorWorkerSessionID}}, nil
	}
	got, err := f.engine.Send(t.Context(), engineRequest("new", "new eligible"), f.caller, "")
	got = confirmEngineDelivery(t, f, got)
	if err != nil || got.Status != agentmessages.DeliveredRevive || calls != 1 {
		t.Fatal("ordered batch was not admitted once", err)
	}
	assertEligibleBatchOutcome(t, f, got)
}

func assertEligibleBatchOutcome(t *testing.T, f *engineFixture, got agentmessages.Message) {
	t.Helper()
	for _, index := range []int{0, 5, 6} {
		m := f.ledger.entries[index].Message
		if m.Status != agentmessages.DeliveredRevive || m.RevivedWorkerSessionID != got.RevivedWorkerSessionID {
			t.Fatal("batch lost shared successor")
		}
	}
	if len(f.ledger.entries) != 7 || f.ledger.entries[1].Message.Status != agentmessages.Queued || f.ledger.entries[2].Message.Status != agentmessages.Read ||
		f.ledger.entries[3].Message.Status != agentmessages.Replied || f.ledger.entries[4].Message.Status != agentmessages.Expired {
		t.Fatal("batch changed excluded messages or invented a public message")
	}
}

// This component witness explicitly supplies confirmation. Public functional
// journeys own proof that a provider record actually produces this transition.
func confirmEngineDelivery(t *testing.T, f *engineFixture, message agentmessages.Message) agentmessages.Message {
	t.Helper()
	for _, intent := range f.ledger.intents {
		if intent.State == store.IntentAdmitted {
			f.engine.settleDelivery(t.Context(), intent, store.IntentAccepted, "")
		}
	}
	entry, found, _, err := f.ledger.LookupMessage(message.MessageID)
	if err != nil || !found {
		t.Fatal("confirmation lost admitted message", err)
	}
	return entry.Message
}

func TestLimitsCountsSuccessfulChainSendsAcrossStatuses(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	entries := make([]store.Entry, DefaultSendsPerHour)
	statuses := []agentmessages.Status{agentmessages.Queued, agentmessages.Read, agentmessages.Replied, agentmessages.Expired}
	for i := range entries {
		entries[i] = store.Entry{SenderChainIdentity: "chain", Message: agentmessages.Message{
			From:   agentmessages.Sender{WorkerSessionID: "successor"},
			Status: statuses[i%len(statuses)], SentAt: now.Add(-30 * time.Minute),
		}}
	}
	if err := (Limits{}).Check(now, "chain", "", 0, entries[:9]); err != nil {
		t.Fatalf("tenth send: %v", err)
	}
	err := (Limits{}).Check(now, "chain", "", 0, entries)
	assertLimit(t, err, "sender", 1800)
	if err := (Limits{}).Check(now, "another-chain", "", 0, entries); err != nil {
		t.Fatalf("other chain affected: %v", err)
	}
	if err := (Limits{}).Check(now.Add(30*time.Minute), "chain", "", 0, entries); err != nil {
		t.Fatalf("exact window boundary did not release quota: %v", err)
	}
}

func TestLimitsRoundsWindowRetryUpAndHonorsConfiguration(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	entries := []store.Entry{{SenderChainIdentity: "chain", Message: agentmessages.Message{
		SentAt: now.Add(-time.Hour + time.Nanosecond),
	}}}
	l := Limits{SendsPerHour: 1}
	assertLimit(t, l.Check(now, "chain", "", 0, entries), "sender", 1)
	if err := l.Check(now.Add(time.Nanosecond), "chain", "", 0, entries); err != nil {
		t.Fatalf("nanosecond boundary: %v", err)
	}
	// A backward clock must not erase a previously committed send.
	entries[0].Message.SentAt = now.Add(time.Minute)
	assertLimit(t, l.Check(now, "chain", "", 0, entries), "sender", 3660)
	entries = append(entries, store.Entry{SenderChainIdentity: "chain", Message: agentmessages.Message{SentAt: now.Add(-time.Minute)}})
	assertLimit(t, l.Check(now, "chain", "", 0, entries), "sender", 3660)
}

func TestLimitsThreadAndHopBoundaries(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	entries := make([]store.Entry, DefaultThreadMessages)
	for i := range entries {
		entries[i].Message.ThreadID = "thread"
	}
	for _, hop := range []int{0, 3} {
		if err := (Limits{}).Check(now, "chain", "thread", hop, entries[:7]); err != nil {
			t.Fatalf("eighth message / hop=%d: %v", hop, err)
		}
	}
	assertLimit(t, (Limits{}).Check(now, "chain", "thread", 0, entries), "thread", 0)
	assertLimit(t, (Limits{ThreadMessages: 1}).Check(now, "chain", "thread", 0, entries[:1]), "thread", 0)
	assertLimit(t, (Limits{}).Check(now, "chain", "other-thread", 4, nil), "hop", 0)
	if err := (Limits{}).Check(now, "chain", "other-thread", 0, entries); err != nil {
		t.Fatalf("other thread affected: %v", err)
	}
	for _, l := range []Limits{{SendsPerHour: -1}, {ThreadMessages: -1}} {
		if err := l.Check(now, "chain", "", 0, nil); !errors.Is(err, agentmessages.ErrBadRequest) {
			t.Fatalf("invalid configuration: %v", err)
		}
	}
}

func assertLimit(t *testing.T, err error, dimension string, retry int) {
	t.Helper()
	var limit *agentmessages.LimitError
	if !errors.Is(err, agentmessages.ErrLimitExceeded) || !errors.As(err, &limit) ||
		limit.Dimension != dimension || limit.RetryAfterSeconds != retry {
		t.Fatalf("got %v, want %s limit retry=%d", err, dimension, retry)
	}
}

func TestLimitsConfiguredHopIncludesZero(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	for _, maximum := range []int{0, 1, MaxHop} {
		limit := Limits{HopLimit: &maximum}
		if err := limit.Check(now, "chain", "thread", maximum, nil); err != nil {
			t.Fatalf("allowed configured hop %d: %v", maximum, err)
		}
		assertLimit(t, limit.Check(now, "chain", "thread", maximum+1, nil), "hop", 0)
	}
	for _, maximum := range []int{-1, MaxHop + 1} {
		err := (Limits{HopLimit: &maximum}).Check(now, "chain", "thread", 0, nil)
		if !errors.Is(err, agentmessages.ErrBadRequest) {
			t.Fatalf("invalid configured hop %d accepted: %v", maximum, err)
		}
	}
}
