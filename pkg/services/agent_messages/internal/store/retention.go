package store

import (
	"encoding/json"
	"sort"
	"strconv"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
)

const DefaultRetention = 30 * 24 * time.Hour

// Compact removes only complete, settled threads whose messages precede the
// retention cutoff. Pending messages and every member of a retained thread
// remain, including all request aliases and original admission facts. Startup
// composition must invoke this before exposing the writable profile.
func (j *Journal) Compact(now time.Time, retention time.Duration) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.compact(now, retention)
}

func (j *Journal) compact(now time.Time, retention time.Duration) error {
	if !j.opened || j.fault != nil {
		return j.unavailable()
	}
	if now.IsZero() || retention < 24*time.Hour {
		return ErrInvalidTransaction
	}
	snapshot := j.retainedSnapshot(now.UTC(), retention)
	if len(snapshot.Messages) == len(j.entries) {
		return nil
	}
	// Validate the self-contained replacement before asking the host to publish
	// it. This also rebuilds only the surviving indexes, without stale members.
	next := New(j.fs, j.path)
	next.entries = make(map[string]Entry)
	next.requests = make(map[requestKey]Request)
	next.records = make(map[string]bool)
	next.indexes = make(map[indexKey][]string)
	if err := next.validate(snapshot); err != nil {
		return err
	}
	next.apply(snapshot)
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return ErrInvalidTransaction
	}
	if err := j.fs.ReplaceDurable(j.path, append(encoded, '\n')); err != nil {
		return ErrUnavailable
	}
	j.entries, j.requests, j.records = next.entries, next.requests, next.records
	j.ordered, j.indexes = next.ordered, next.indexes
	return nil
}

func (j *Journal) retainedSnapshot(now time.Time, retention time.Duration) Transaction {
	keepThreads := make(map[string]bool)
	cutoff := now.Add(-retention)
	for _, entry := range j.entries {
		m := entry.Message
		settled := m.Status == agentmessages.Expired || m.Status == agentmessages.Replied
		if !m.SentAt.Before(cutoff) || m.ExpiresAt.After(now) || !settled {
			keepThreads[m.ThreadID] = true
		}
	}
	t := Transaction{Version: Version, RecordID: "snapshot-" + strconv.FormatUint(j.sequence, 10),
		Sequence: j.sequence, Kind: Snapshot, CommittedAt: now, Messages: []Entry{}, Requests: []Request{}}
	keepIDs := make(map[string]bool)
	for _, id := range j.ordered {
		entry := j.entries[id]
		if keepThreads[entry.Message.ThreadID] {
			t.Messages = append(t.Messages, entry)
			keepIDs[id] = true
		}
	}
	for _, request := range j.requests {
		if keepIDs[request.MessageID] {
			t.Requests = append(t.Requests, request)
		}
	}
	sort.Slice(t.Requests, func(a, b int) bool {
		left, right := t.Requests[a], t.Requests[b]
		if left.SenderIdentity != right.SenderIdentity {
			return left.SenderIdentity < right.SenderIdentity
		}
		return left.RequestID < right.RequestID
	})
	return t
}
