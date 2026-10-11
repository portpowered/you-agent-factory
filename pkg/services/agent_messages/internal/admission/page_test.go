package admission

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/agent_messages/internal/store"
)

// Page tests isolate the admission coordinator with controlled authority,
// journal, clock and signing key. Public CLI/HTTP proof remains functional work.
func (f *engineFixture) seedInbox(count int) {
	for i := 1; i <= count; i++ {
		id := fmt.Sprintf("message-%d", i)
		f.ledger.entries = append(f.ledger.entries, store.Entry{
			Sequence: uint64(i), SenderChainIdentity: "worker-chain", SenderWorkIdentity: "worker-work",
			RecipientChainIdentity: "lead-chain", RecipientWorkIdentity: "lead-work",
			Message: agentmessages.Message{MessageID: id, ThreadID: id,
				From:   agentmessages.Sender{WorkerSessionID: "worker", FactorySessionID: "factory"},
				To:     agentmessages.Recipient{WorkerSessionID: "lead", FactorySessionID: "factory"},
				Status: agentmessages.Queued, Body: id, SentAt: f.now, ExpiresAt: f.now.Add(time.Hour),
				Correlation: agentmessages.Correlation{WorkID: "work", FactorySessionID: "factory"}},
		})
	}
	f.ledger.sequence = uint64(count)
}

func TestListAuthorizedANDFiltersAndEmptyArray(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	f.seedInbox(5)
	f.ledger.entries[0].Message.Correlation.WorkID = "other-work"
	f.ledger.entries[1].Message.Status = agentmessages.Read
	f.ledger.entries[2].Message.Correlation.FactorySessionID = "other-factory"
	f.ledger.entries[3].SenderChainIdentity = "hidden-sender"
	f.ledger.entries[3].SenderWorkIdentity = "hidden-work"
	f.ledger.entries[3].RecipientChainIdentity = "hidden-recipient"
	f.ledger.entries[3].RecipientWorkIdentity = "hidden-recipient-work"
	r := agentmessages.ListRequest{Caller: f.caller, ToWorkerSessionID: "lead", FromWorkerSessionID: "worker",
		Correlation: agentmessages.Correlation{WorkID: "work", FactorySessionID: "factory"},
		Statuses:    []agentmessages.Status{agentmessages.Queued}}
	page, err := f.engine.List(context.Background(), r)
	if err != nil || len(page.Messages) != 1 || page.Messages[0].MessageID != "message-5" {
		t.Fatalf("filtered page = %+v, %v", page, err)
	}
	r.ThreadID = "message-1"
	page, err = f.engine.List(context.Background(), r)
	if err != nil || page.Messages == nil || len(page.Messages) != 0 || page.NextToken != "" {
		t.Fatal("empty intersection was not an empty array", err)
	}
	operator, err := f.engine.List(context.Background(), agentmessages.ListRequest{})
	if err != nil || len(operator.Messages) != 5 || len(f.ledger.transactions) != 0 {
		t.Fatal("operator observation was filtered or marked another recipient's messages", err)
	}
}

func TestListPagesPreserveSequenceCeilingAndQueryNormalization(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	f.seedInbox(3)
	r := agentmessages.ListRequest{Caller: f.caller, MaxResults: 1,
		Statuses: []agentmessages.Status{agentmessages.Read, agentmessages.Queued, agentmessages.Read}}
	first, err := f.engine.List(context.Background(), r)
	if err != nil || len(first.Messages) != 1 || first.Messages[0].MessageID != "message-1" || first.NextToken == "" {
		t.Fatal("first page", first, err)
	}
	// A later admission is visible to a new query, but outside this walk.
	later := f.ledger.entries[0]
	later.Sequence, later.Message.MessageID = 4, "later"
	f.ledger.entries = append(f.ledger.entries, later)
	f.ledger.sequence = 4
	r.Statuses = []agentmessages.Status{agentmessages.Queued, agentmessages.Read}
	r.NextToken = first.NextToken
	second, err := f.engine.List(context.Background(), r)
	if err != nil || len(second.Messages) != 1 || second.Messages[0].MessageID != "message-2" {
		t.Fatal("second page", second, err)
	}
	repeated, err := f.engine.List(context.Background(), r)
	if err != nil || !reflect.DeepEqual(second, repeated) {
		t.Fatal("cursor retry changed the page", err)
	}
	r.NextToken = second.NextToken
	third, err := f.engine.List(context.Background(), r)
	if err != nil || len(third.Messages) != 1 || third.Messages[0].MessageID != "message-3" || third.NextToken != "" {
		t.Fatal("last page included a concurrent send", third, err)
	}
}

func TestListRejectsTamperedForeignAndMalformedCursors(t *testing.T) {
	t.Parallel()
	for _, fault := range []string{"tampered", "malformed", "oversized", "query", "caller", "key", "future", "backwards"} {
		t.Run(fault, func(t *testing.T) {
			t.Parallel()
			f := newEngineFixture()
			f.seedInbox(3)
			r := agentmessages.ListRequest{Caller: f.caller, MaxResults: 1}
			first, err := f.engine.List(context.Background(), r)
			if err != nil {
				t.Fatal(err)
			}
			r.NextToken = first.NextToken
			r = corruptPageRequest(t, f, r, fault)
			page, err := f.engine.List(context.Background(), r)
			if !errors.Is(err, agentmessages.ErrCursorInvalid) || len(page.Messages) != 0 || len(f.ledger.transactions) != 0 {
				t.Fatal("invalid cursor disclosed content or changed state", err)
			}
		})
	}
}

func corruptPageRequest(t *testing.T, f *engineFixture, r agentmessages.ListRequest, fault string) agentmessages.ListRequest {
	t.Helper()
	switch fault {
	case "tampered":
		r.NextToken = "X" + r.NextToken[1:]
	case "malformed":
		r.NextToken = "garbage"
	case "oversized":
		r.NextToken = strings.Repeat("x", maxCursorBytes+1)
	case "query":
		r.ThreadID = "message-1"
	case "caller":
		f.authority.sender = engineIdentity("lead")
	case "key":
		f.engine.cursorKey = []byte(strings.Repeat("z", minCursorKeyBytes))
	case "future", "backwards":
		position, err := f.engine.decodeCursor(r.NextToken)
		if err != nil {
			t.Fatal(err)
		}
		if fault == "future" {
			position.Through++
		} else {
			position.After = position.Through
		}
		r.NextToken, err = f.engine.encodeCursor(position)
		if err != nil {
			t.Fatal(err)
		}
	}
	return r
}

func TestListToMeUsesVerifiedContinuationAndOwnWork(t *testing.T) {
	t.Parallel()
	for _, identity := range []Identity{
		{Chain: "lead-chain", Work: "new-work"},
		{Chain: "new-chain", Work: "lead-work"},
		{Chain: "unrelated", Work: "other-work"},
	} {
		t.Run(identity.Chain+identity.Work, func(t *testing.T) {
			t.Parallel()
			f := newEngineFixture()
			f.seedInbox(1)
			f.authority.sender = identity
			r := agentmessages.ListRequest{Caller: f.caller, ToMe: true, MarkRead: true}
			page, err := f.engine.List(context.Background(), r)
			if err != nil {
				t.Fatal(err)
			}
			if identity.Chain == "unrelated" {
				if len(page.Messages) != 0 || len(f.ledger.transactions) != 0 {
					t.Fatal("unrelated identity read or marked a message")
				}
			} else if len(page.Messages) != 1 || page.Messages[0].Status != agentmessages.Read || f.ledger.entries[0].Message.Status != agentmessages.Read {
				t.Fatal("verified recipient did not receive/mark its inbox")
			}
		})
	}
}

func TestListMarksAndExpiresOnePageAtomically(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	f.seedInbox(5)
	f.authority.sender = engineIdentity("lead")
	f.ledger.entries[1].Message.ExpiresAt = f.now
	f.ledger.entries[2].Message.Status = agentmessages.Replied
	f.ledger.entries[3].Message.Status = agentmessages.Expired
	r := agentmessages.ListRequest{Caller: f.caller, ToMe: true, MarkRead: true, MaxResults: 4}
	page, err := f.engine.List(context.Background(), r)
	if err != nil || len(page.Messages) != 4 || page.NextToken == "" {
		t.Fatal("page", page, err)
	}
	want := []agentmessages.Status{agentmessages.Read, agentmessages.Expired, agentmessages.Replied, agentmessages.Expired}
	for i, status := range want {
		if page.Messages[i].Status != status || f.ledger.entries[i].Message.Status != status {
			t.Fatal("non-monotonic or uncommitted status", page)
		}
	}
	if len(f.ledger.transactions) != 1 || len(f.ledger.transactions[0].Messages) != 2 || f.ledger.entries[4].Message.Status != agentmessages.Queued {
		t.Fatal("page changes were split or changed an unreturned entry")
	}
	_, err = f.engine.List(context.Background(), r)
	if err != nil || len(f.ledger.transactions) != 1 {
		t.Fatal("repeated mark-read was not idempotent", err)
	}
}

func TestListFailureReturnsNoPageOrStatusChange(t *testing.T) {
	t.Parallel()
	for _, failure := range []error{store.ErrUnavailable, agentmessages.ErrNotPermitted, context.Canceled} {
		t.Run(failure.Error(), func(t *testing.T) {
			t.Parallel()
			f := newEngineFixture()
			f.seedInbox(2)
			f.authority.sender = engineIdentity("lead")
			before := append([]store.Entry{}, f.ledger.entries...)
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
			page, err := f.engine.List(ctx, agentmessages.ListRequest{Caller: f.caller, ToMe: true, MarkRead: true})
			if !errors.Is(err, failure) || len(page.Messages) != 0 || page.NextToken != "" || !reflect.DeepEqual(before, f.ledger.entries) {
				t.Fatal("failed page was published or mutated inbox state", err)
			}
		})
	}
}

func TestListQueuedFilterMarksOnlyReturnedRecipientMessages(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	f.seedInbox(3)
	f.ledger.entries[0].Message.ExpiresAt = f.now
	f.ledger.entries[2].RecipientChainIdentity = "other-recipient"
	f.ledger.entries[2].RecipientWorkIdentity = "other-work"
	f.ledger.entries[2].SenderChainIdentity = "lead-chain"
	f.authority.sender = engineIdentity("lead")
	r := agentmessages.ListRequest{Caller: f.caller, MarkRead: true, Statuses: []agentmessages.Status{agentmessages.Queued}}
	page, err := f.engine.List(context.Background(), r)
	if err != nil || len(page.Messages) != 2 || page.Messages[0].Status != agentmessages.Read || page.Messages[1].Status != agentmessages.Queued {
		t.Fatal("queued filter marked an expired or sender-only message", page, err)
	}
	if len(f.ledger.transactions) != 1 || len(f.ledger.transactions[0].Messages) != 1 {
		t.Fatal("mark-read changed more than returned recipient entries")
	}
}

func TestListRejectsInvalidOrUnauthenticatedInboxOptions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		request agentmessages.ListRequest
		want    error
	}{
		{"operator to-me", agentmessages.ListRequest{ToMe: true}, agentmessages.ErrNotPermitted},
		{"operator mark-read", agentmessages.ListRequest{MarkRead: true}, agentmessages.ErrNotPermitted},
		{"negative size", agentmessages.ListRequest{MaxResults: -1}, agentmessages.ErrBadRequest},
		{"oversized page", agentmessages.ListRequest{MaxResults: maxPageSize + 1}, agentmessages.ErrBadRequest},
		{"blank recipient", agentmessages.ListRequest{ToWorkerSessionID: " "}, agentmessages.ErrBadRequest},
		{"invalid UTF-8 filter", agentmessages.ListRequest{ThreadID: string([]byte{0xff})}, agentmessages.ErrBadRequest},
		{"invalid status", agentmessages.ListRequest{Statuses: []agentmessages.Status{"UNKNOWN"}}, agentmessages.ErrBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newEngineFixture()
			f.seedInbox(1)
			page, err := f.engine.List(context.Background(), tc.request)
			if !errors.Is(err, tc.want) || len(page.Messages) != 0 || len(f.ledger.transactions) != 0 {
				t.Fatal("invalid request disclosed or mutated a message", err)
			}
		})
	}
}

func TestListExpiryStatusFilterPersistsAndDisabledFailsClosed(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	f.seedInbox(1)
	f.now = f.ledger.entries[0].Message.ExpiresAt
	r := agentmessages.ListRequest{Statuses: []agentmessages.Status{agentmessages.Expired}}
	page, err := f.engine.List(context.Background(), r)
	if err != nil || len(page.Messages) != 1 || page.Messages[0].Status != agentmessages.Expired ||
		f.ledger.entries[0].Message.Status != agentmessages.Expired || len(f.ledger.transactions) != 1 {
		t.Fatal("expiry filter returned an uncommitted expiry", page, err)
	}
	f.engine.enabled = false
	page, err = f.engine.List(context.Background(), r)
	if !errors.Is(err, agentmessages.ErrDisabled) || len(page.Messages) != 0 || len(f.ledger.transactions) != 1 {
		t.Fatal("disabled messaging disclosed or changed state", err)
	}
}

func TestListNeverFingerprintsCredentialBearingFilters(t *testing.T) {
	t.Parallel()
	f := newEngineFixture()
	f.seedInbox(2)
	r := agentmessages.ListRequest{Caller: f.caller, MaxResults: 1, Correlation: agentmessages.Correlation{WorkID: f.caller.Token}}
	page, err := f.engine.List(context.Background(), r)
	if !errors.Is(err, agentmessages.ErrBadRequest) || page.NextToken != "" || len(page.Messages) != 0 || len(f.ledger.transactions) != 0 {
		t.Fatal("credential-bearing filter produced a fingerprint or effect", err)
	}
}
