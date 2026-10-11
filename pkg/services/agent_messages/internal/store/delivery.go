package store

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"sort"
	"strings"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
)

const (
	IntentPending   = "PENDING"
	IntentAccepted  = "ACCEPTED"
	IntentFailed    = "FAILED"
	IntentUncertain = "UNCERTAIN"
)

// DeliveryIntent freezes the normalized input and successor before the effect.
// It is data, never execution authority, and is not published to Events.
type DeliveryIntent struct {
	RequestID                string    `json:"requestId"`
	RecipientChainIdentity   string    `json:"recipientChainIdentity"`
	FactorySessionID         string    `json:"factorySessionId,omitempty"`
	SourceWorkerSessionID    string    `json:"sourceWorkerSessionId"`
	SuccessorWorkerSessionID string    `json:"successorWorkerSessionId"`
	MessageIDs               []string  `json:"messageIds"`
	FollowUpInput            string    `json:"followUpInput"`
	InputSHA256              string    `json:"inputSha256"`
	ResolveHead              bool      `json:"resolveHead"`
	State                    string    `json:"state"`
	AcceptedAt               time.Time `json:"acceptedAt,omitempty"`
	Reason                   string    `json:"reason,omitempty"`
}

func (i DeliveryIntent) clone() DeliveryIntent {
	i.MessageIDs = append([]string{}, i.MessageIDs...)
	return i
}

func validReason(reason string) bool {
	return reason == "" || reason == "REVIVE_UNSUPPORTED" || reason == "REVIVE_FAILED" || reason == "REVIVE_LIMIT"
}

// DeliveryIntents returns detached, deterministic recovery and quota facts.
func (j *Journal) DeliveryIntents() ([]DeliveryIntent, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.opened || j.fault != nil {
		return nil, j.unavailable()
	}
	return sortedIntents(j.intents), nil
}

func sortedIntents(intents map[string]DeliveryIntent) []DeliveryIntent {
	result := make([]DeliveryIntent, 0, len(intents))
	for _, intent := range intents {
		result = append(result, intent.clone())
	}
	sort.Slice(result, func(a, b int) bool { return result[a].RequestID < result[b].RequestID })
	return result
}

func (j *Journal) validateIntents(t Transaction, changed map[string]Entry) error {
	if t.Kind != ReviveIntent && t.Kind != ReviveOutcome && t.Kind != Snapshot {
		if len(t.DeliveryIntents) != 0 {
			return ErrInvalidTransaction
		}
		return nil
	}
	if t.Kind != Snapshot && (len(t.DeliveryIntents) != 1 || len(t.Requests) != 0) {
		return ErrInvalidTransaction
	}
	seen := map[string]bool{}
	for _, intent := range t.DeliveryIntents {
		if seen[intent.RequestID] || !intent.valid() {
			return ErrInvalidTransaction
		}
		seen[intent.RequestID] = true
		if err := j.validateIntentChange(t, intent, changed); err != nil {
			return err
		}
	}
	if t.Kind == Snapshot {
		return validateSnapshotDeliveries(t, changed)
	}
	return nil
}

func validateSnapshotDeliveries(t Transaction, changed map[string]Entry) error {
	delivered := map[string]string{}
	for _, intent := range t.DeliveryIntents {
		if intent.State != IntentAccepted {
			continue
		}
		for _, id := range intent.MessageIDs {
			if delivered[id] != "" || changed[id].Message.RevivedWorkerSessionID != intent.SuccessorWorkerSessionID {
				return ErrInvalidTransaction
			}
			delivered[id] = intent.SuccessorWorkerSessionID
		}
	}
	for id, entry := range changed {
		if entry.Message.RevivedWorkerSessionID != delivered[id] {
			return ErrInvalidTransaction
		}
	}
	return nil
}

func (i DeliveryIntent) valid() bool {
	if !i.validTuple() {
		return false
	}
	switch i.State {
	case IntentPending:
		return i.AcceptedAt.IsZero() && i.Reason == ""
	case IntentAccepted:
		return !i.AcceptedAt.IsZero() && i.Reason == ""
	case IntentFailed, IntentUncertain:
		return i.AcceptedAt.IsZero() && (i.Reason == "REVIVE_FAILED" || i.Reason == "REVIVE_UNSUPPORTED")
	}
	return false
}

func (i DeliveryIntent) validTuple() bool {
	if !nonempty(i.RequestID) || !nonempty(i.RecipientChainIdentity) ||
		!nonempty(i.SourceWorkerSessionID) || !nonempty(i.SuccessorWorkerSessionID) ||
		i.SourceWorkerSessionID == i.SuccessorWorkerSessionID || !i.ResolveHead ||
		len(i.MessageIDs) == 0 || i.RequestID != i.MessageIDs[0] || !nonempty(i.FollowUpInput) {
		return false
	}
	hash := sha256.Sum256([]byte(i.FollowUpInput))
	return i.InputSHA256 == hex.EncodeToString(hash[:])
}

func (j *Journal) validateIntentChange(t Transaction, intent DeliveryIntent, changed map[string]Entry) error {
	if t.Kind == Snapshot {
		return j.validateIntentInput(intent, changed, false, t.CommittedAt)
	}
	prior, exists := j.intents[intent.RequestID]
	if t.Kind == ReviveIntent {
		return j.validateNewIntent(t, intent, changed, exists)
	}
	if !exists || (prior.State != IntentPending && prior.State != IntentUncertain) || intent.State == IntentPending {
		return ErrInvalidTransaction
	}
	expected := prior
	expected.State, expected.AcceptedAt, expected.Reason = intent.State, intent.AcceptedAt, intent.Reason
	if !reflect.DeepEqual(expected, intent) || intent.AcceptedAt.After(t.CommittedAt) {
		return ErrInvalidTransaction
	}
	return j.validateIntentOutcome(intent, changed)
}

func (j *Journal) validateNewIntent(t Transaction, intent DeliveryIntent, changed map[string]Entry, exists bool) error {
	if exists || intent.State != IntentPending || len(changed) != 0 {
		return ErrInvalidTransaction
	}
	assigned := map[string]bool{}
	for _, other := range j.intents {
		if other.RecipientChainIdentity == intent.RecipientChainIdentity &&
			(other.State == IntentPending || other.State == IntentUncertain) {
			return ErrInvalidTransaction
		}
		for _, id := range other.MessageIDs {
			assigned[id] = true
		}
	}
	for _, id := range intent.MessageIDs {
		if assigned[id] {
			return ErrInvalidTransaction
		}
	}
	return j.validateIntentInput(intent, changed, true, t.CommittedAt)
}

func (j *Journal) validateIntentInput(intent DeliveryIntent, changed map[string]Entry, pending bool, now time.Time) error {
	seen, bodies := map[string]bool{}, []string{}
	var sequence uint64
	for _, id := range intent.MessageIDs {
		entry, exists := j.entry(id, changed)
		if !exists || seen[id] || entry.Sequence <= sequence ||
			entry.RecipientChainIdentity != intent.RecipientChainIdentity || entry.Message.IfEnded != "REVIVE" ||
			entry.Message.To.FactorySessionID != intent.FactorySessionID {
			return ErrInvalidTransaction
		}
		if pending && (entry.Message.Status != agentmessages.Queued || !entry.Message.ExpiresAt.After(now)) {
			return ErrInvalidTransaction
		}
		seen[id], sequence = true, entry.Sequence
		bodies = append(bodies, entry.Message.Body)
	}
	if strings.Join(bodies, "\n\n") != intent.FollowUpInput {
		return ErrInvalidTransaction
	}
	return nil
}

func (j *Journal) validateIntentOutcome(intent DeliveryIntent, changed map[string]Entry) error {
	if len(changed) != len(intent.MessageIDs) {
		return ErrInvalidTransaction
	}
	for _, id := range intent.MessageIDs {
		old, exists := j.entries[id]
		next, updated := changed[id]
		if !exists || !updated {
			return ErrInvalidTransaction
		}
		expected := old
		if intent.State == IntentAccepted {
			expected.Message.Status = agentmessages.DeliveredRevive
			expected.Message.Reason = ""
			expected.Message.RevivedWorkerSessionID = intent.SuccessorWorkerSessionID
			expected.Message.To.DeliveredWorkerSessionID = intent.SuccessorWorkerSessionID
		} else {
			expected.Message.Reason = intent.Reason
		}
		if old.Message.Status != agentmessages.Queued || !reflect.DeepEqual(expected, next) {
			return ErrInvalidTransaction
		}
	}
	return nil
}
