package admission

import (
	"errors"
	"testing"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/agent_messages/internal/store"
)

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
