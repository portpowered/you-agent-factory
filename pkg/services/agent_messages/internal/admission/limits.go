package admission

import (
	"sort"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/agent_messages/internal/store"
)

const (
	DefaultSendsPerHour   = 10
	DefaultThreadMessages = 8
	senderWindow          = time.Hour
)

// Limits evaluates committed admission facts. The owner holds its admission
// lock across this check and flush; retries and denied/failed writes never
// contribute a new Entry and therefore never spend another successful send.
type Limits struct {
	SendsPerHour   int
	ThreadMessages int
}

func (l Limits) Check(now time.Time, senderChain, thread string, hop int, entries []store.Entry) error {
	if now.IsZero() || senderChain == "" || hop < 0 || l.SendsPerHour < 0 || l.ThreadMessages < 0 {
		return agentmessages.ErrBadRequest
	}
	if hop > MaxHop {
		return &agentmessages.LimitError{Dimension: "hop"}
	}
	sendsLimit, threadLimit := l.SendsPerHour, l.ThreadMessages
	if sendsLimit == 0 {
		sendsLimit = DefaultSendsPerHour
	}
	if threadLimit == 0 {
		threadLimit = DefaultThreadMessages
	}
	sends, messages := committedWindow(now, senderChain, thread, entries)
	if messages >= threadLimit {
		return &agentmessages.LimitError{Dimension: "thread"}
	}
	if len(sends) >= sendsLimit {
		// If configuration reduced the quota, enough sends must leave the
		// window to admit one more; the oldest alone may not release capacity.
		wait := sends[len(sends)-sendsLimit].Add(senderWindow).Sub(now)
		seconds := int(wait / time.Second)
		if wait%time.Second != 0 {
			seconds++
		}
		return &agentmessages.LimitError{Dimension: "sender", RetryAfterSeconds: seconds}
	}
	return nil
}

func committedWindow(now time.Time, senderChain, thread string, entries []store.Entry) ([]time.Time, int) {
	var sends []time.Time
	messages := 0
	cutoff := now.Add(-senderWindow)
	for _, entry := range entries {
		if thread != "" && entry.Message.ThreadID == thread {
			messages++
		}
		if entry.SenderChainIdentity == senderChain && entry.Message.SentAt.After(cutoff) {
			sends = append(sends, entry.Message.SentAt)
		}
	}
	sort.Slice(sends, func(i, j int) bool { return sends[i].Before(sends[j]) })
	return sends, messages
}
