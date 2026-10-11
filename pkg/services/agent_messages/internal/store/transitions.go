package store

import (
	"reflect"
	"sort"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
)

func (j *Journal) validate(t Transaction) error {
	if err := t.validate(); err != nil {
		return err
	}
	firstSnapshot := t.Kind == Snapshot && j.sequence == 0
	if (!firstSnapshot && t.Sequence != j.sequence+1) || j.records[t.RecordID] {
		return ErrInvalidTransaction
	}
	changed := make(map[string]Entry, len(t.Messages))
	for _, entry := range t.Messages {
		id := entry.Message.MessageID
		if _, duplicate := changed[id]; duplicate {
			return ErrInvalidTransaction
		}
		changed[id] = entry
	}
	if err := j.validateChanges(t, changed); err != nil {
		return err
	}
	seen := make(map[requestKey]bool, len(t.Requests))
	for _, request := range t.Requests {
		key := requestKey{request.SenderIdentity, request.RequestID}
		if seen[key] {
			return ErrInvalidTransaction
		}
		seen[key] = true
		if old, exists := j.requests[key]; exists && old != request {
			return ErrInvalidTransaction
		}
		entry, exists := j.entry(request.MessageID, changed)
		if !exists || entry.SenderChainIdentity != request.SenderIdentity {
			return ErrInvalidTransaction
		}
	}
	return nil
}

func (j *Journal) validateChanges(t Transaction, changed map[string]Entry) error {
	switch t.Kind {
	case Snapshot:
		return j.validateSnapshot(t, changed)
	case RequestAlias:
		if len(t.Requests) == 0 {
			return ErrInvalidTransaction
		}
		return j.validateAdmissionExpiry(t, changed)
	case Sent, Replied:
		return j.validateAdmission(t, changed)
	default:
		return j.validateUpdates(t, changed)
	}
}

func (j *Journal) validateSnapshot(t Transaction, changed map[string]Entry) error {
	// Snapshots retain original send sequence and are accepted only at the
	// start of a compacted journal, preserving page ordering across compaction.
	if j.sequence != 0 {
		return ErrInvalidTransaction
	}
	seen := make(map[uint64]bool, len(changed))
	for _, entry := range changed {
		if entry.Sequence > t.Sequence || seen[entry.Sequence] {
			return ErrInvalidTransaction
		}
		seen[entry.Sequence] = true
	}
	return j.validateLinks(changed)
}

func (j *Journal) validateAdmission(t Transaction, changed map[string]Entry) error {
	admitted := make(map[string]Entry, len(changed))
	expired := make(map[string]Entry)
	for id, entry := range changed {
		if entry.Message.Status == agentmessages.Expired {
			expired[id] = entry
		} else {
			admitted[id] = entry
		}
	}
	if err := j.validateAdmissionExpiry(t, expired); err != nil {
		return err
	}
	changed = admitted
	wantCount := 1
	if t.Kind == Replied {
		wantCount = 2
	}
	if len(changed) != wantCount || len(t.Requests) != 1 {
		return ErrInvalidTransaction
	}
	newCount := 0
	for id, entry := range changed {
		old, exists := j.entries[id]
		if !exists {
			newCount++
			if entry.Message.Status != agentmessages.Queued ||
				entry.Sequence != t.Sequence || entry.Message.RepliedByMessageID != "" {
				return ErrInvalidTransaction
			}
			continue
		}
		if !validTransition(old, entry, t.Kind) {
			return ErrInvalidTransaction
		}
	}
	if newCount != 1 {
		return ErrInvalidTransaction
	}
	return j.validateLinks(changed)
}

func (j *Journal) validateAdmissionExpiry(t Transaction, changed map[string]Entry) error {
	for id, entry := range changed {
		old, exists := j.entries[id]
		if !exists || entry.Message.ExpiresAt.After(t.CommittedAt) || !validTransition(old, entry, Expired) {
			return ErrInvalidTransaction
		}
	}
	return nil
}

func (j *Journal) validateUpdates(t Transaction, changed map[string]Entry) error {
	if len(changed) == 0 || len(t.Requests) != 0 {
		return ErrInvalidTransaction
	}
	for id, entry := range changed {
		old, exists := j.entries[id]
		if !exists || !validTransition(old, entry, t.Kind) {
			return ErrInvalidTransaction
		}
	}
	return j.validateLinks(changed)
}

func validTransition(old, next Entry, kind string) bool {
	if old.Message.Status == agentmessages.Expired || old.Message.Status == agentmessages.Replied {
		return false
	}
	// A page may mark live messages READ and due messages EXPIRED atomically.
	// Each entry still follows one of the original monotonic transitions.
	if kind == InboxUpdated {
		if next.Message.Status != agentmessages.Read && next.Message.Status != agentmessages.Expired {
			return false
		}
		kind = string(next.Message.Status)
	}
	expected := old
	switch kind {
	case Read:
		if old.Message.Status != agentmessages.Queued {
			return false
		}
		expected.Message.Status = agentmessages.Read
	case Replied:
		expected.Message.Status = agentmessages.Replied
		expected.Message.RepliedByMessageID = next.Message.RepliedByMessageID
		if next.Message.RepliedByMessageID == "" {
			return false
		}
	case Expired:
		expected.Message.Status = agentmessages.Expired
	default:
		return false
	}
	return reflect.DeepEqual(expected, next)
}

func (j *Journal) validateLinks(changed map[string]Entry) error {
	for _, entry := range changed {
		m := entry.Message
		root, exists := j.entry(m.ThreadID, changed)
		if !exists || root.Message.ThreadID != m.ThreadID || root.Message.InReplyTo != "" {
			return ErrInvalidTransaction
		}
		if !j.validReplyLinks(m, changed) {
			return ErrInvalidTransaction
		}
		if m.InReplyTo != "" {
			parent, _ := j.entry(m.InReplyTo, changed)
			if parent.Sequence >= entry.Sequence {
				return ErrInvalidTransaction
			}
		}
	}
	return nil
}

func (j *Journal) validReplyLinks(m agentmessages.Message, changed map[string]Entry) bool {
	if m.InReplyTo == "" {
		if m.ThreadID != m.MessageID {
			return false
		}
	} else {
		parent, exists := j.entry(m.InReplyTo, changed)
		if !exists || m.MessageID == m.InReplyTo || parent.Message.ThreadID != m.ThreadID ||
			parent.Message.RepliedByMessageID != m.MessageID || parent.Message.Status != agentmessages.Replied {
			return false
		}
	}
	if m.Status == agentmessages.Replied {
		reply, exists := j.entry(m.RepliedByMessageID, changed)
		return exists && reply.Message.InReplyTo == m.MessageID && reply.Message.ThreadID == m.ThreadID
	}
	return m.RepliedByMessageID == ""
}

func (j *Journal) entry(id string, changed map[string]Entry) (Entry, bool) {
	if entry, exists := changed[id]; exists {
		return entry, true
	}
	entry, exists := j.entries[id]
	return entry, exists
}

// Entries returns detached admission facts ordered by original send sequence.
// Read authorization belongs to the Messaging service, not this storage layer.
func (j *Journal) Entries() ([]Entry, uint64, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.opened || j.fault != nil {
		return nil, 0, j.unavailable()
	}
	result := make([]Entry, 0, len(j.entries))
	for _, entry := range j.entries {
		result = append(result, entry)
	}
	sort.Slice(result, func(a, b int) bool {
		if result[a].Sequence == result[b].Sequence {
			return result[a].Message.MessageID < result[b].Message.MessageID
		}
		return result[a].Sequence < result[b].Sequence
	})
	return result, j.sequence, nil
}

// LookupRequest recovers the original message and normalized fingerprint for a
// sender/request pair, including aliases admitted without another send.
func (j *Journal) LookupRequest(sender, request string) (Request, bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.opened || j.fault != nil {
		return Request{}, false, j.unavailable()
	}
	value, exists := j.requests[requestKey{sender, request}]
	return value, exists, nil
}
