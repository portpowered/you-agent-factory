package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
)

// Sequence commits req onto EventsTopic(req.SessionID) through the injected
// EventsAppender, assigning a stable ItemID before append. The entire
// operation -- session lookup, ParentItemID validation, Events append, and
// the session's own sequenced-item index update -- runs under s.mu, which is
// this Store's one serialization point: no two Sequence (or any other Store)
// calls ever interleave, so commit order can never depend on source
// timestamps or goroutine start order. On any failure (including one
// reported by the EventsAppender itself), no partial state is left behind:
// req.SessionID's sequencedItemIDs index is only ever updated after a
// successful, newly accepted Events append.
func (s *Store) Sequence(ctx context.Context, req chatsessions.SequenceRequest) (result chatsessions.SequenceResult, err error) {
	s.logStart("Sequence", req.SessionID)
	defer func() {
		s.logOutcome("Sequence", req.SessionID, err,
			"item_id", result.ItemID,
			"parent_item_id", result.ParentItemID,
			"aggregate_sequence", uint64(result.AggregateSequence),
			"outcome", sequenceOutcomeLabel(result.Outcome))
	}()

	if err := ctx.Err(); err != nil {
		return chatsessions.SequenceResult{}, err
	}
	if err := req.Validate(); err != nil {
		return chatsessions.SequenceResult{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.sessions[req.SessionID]
	if !ok {
		return chatsessions.SequenceResult{}, &chatsessions.NotFoundError{Value: "Session", ID: req.SessionID}
	}
	if req.ParentItemID != "" {
		if _, known := record.sequencedItemIDs[req.ParentItemID]; !known {
			return chatsessions.SequenceResult{}, &chatsessions.NotFoundError{Value: "ParentItem", ID: req.ParentItemID}
		}
	}

	item := chatsessions.SequencedItem{
		ItemID:       s.newID(),
		ParentItemID: req.ParentItemID,
		Kind:         req.Kind,
		Payload:      req.Payload,
	}
	if err := item.Validate(); err != nil {
		return chatsessions.SequenceResult{}, err
	}
	envelope, marshalErr := json.Marshal(item)
	if marshalErr != nil {
		return chatsessions.SequenceResult{}, fmt.Errorf("chat sessions: marshal sequenced item envelope: %w", marshalErr)
	}

	appendResult, appendErr := s.eventsAppender.Append(ctx, events.AppendRequest{
		Topic:          chatsessions.EventsTopic(req.SessionID),
		SourceType:     req.SourceType,
		SourceID:       req.SourceID,
		SourceSequence: req.SourceSequence,
		SourceEventID:  req.SourceEventID,
		SchemaID:       req.SchemaID,
		Payload:        envelope,
	})
	if appendErr != nil {
		return chatsessions.SequenceResult{}, appendErr
	}

	switch appendResult.Outcome {
	case events.AppendOutcomeAccepted:
		record.sequencedItemIDs[item.ItemID] = struct{}{}
		s.sessions[req.SessionID] = record
		return chatsessions.SequenceResult{
			SessionID:         req.SessionID,
			ItemID:            item.ItemID,
			ParentItemID:      item.ParentItemID,
			AggregateSequence: appendResult.Record.ID.Position,
			Outcome:           chatsessions.SequenceOutcomeAccepted,
		}, nil
	case events.AppendOutcomeDuplicate:
		var original chatsessions.SequencedItem
		if unmarshalErr := json.Unmarshal(appendResult.Record.Payload, &original); unmarshalErr != nil {
			return chatsessions.SequenceResult{}, fmt.Errorf("chat sessions: unmarshal original sequenced item envelope: %w", unmarshalErr)
		}
		if field, contradicted := contradictedField(req, appendResult.Record.SchemaID, original); contradicted {
			return chatsessions.SequenceResult{}, &chatsessions.SequencedIdentityConflictError{
				SessionID:      req.SessionID,
				SourceType:     string(req.SourceType),
				SourceID:       string(req.SourceID),
				SourceSequence: uint64(req.SourceSequence),
				SourceEventID:  string(req.SourceEventID),
				Field:          field,
			}
		}
		return chatsessions.SequenceResult{
			SessionID:         req.SessionID,
			ItemID:            original.ItemID,
			ParentItemID:      original.ParentItemID,
			AggregateSequence: appendResult.Record.ID.Position,
			Outcome:           chatsessions.SequenceOutcomeDuplicate,
		}, nil
	default:
		return chatsessions.SequenceResult{}, fmt.Errorf("chat sessions: events append returned unexpected outcome %d", appendResult.Outcome)
	}
}

// contradictedField reports the first field of req that disagrees with
// original, the already-committed record resolved for req's reused
// (SourceType, SourceID, SourceSequence, SourceEventID) identity tuple.
// originalSchemaID is the SchemaID Events stored alongside original (the
// SequencedItem envelope itself never carries SchemaID -- see SequencedItem's
// doc comment). An empty field name with contradicted=false means req is a
// safe, identity-preserving repeat of the exact same request.
func contradictedField(req chatsessions.SequenceRequest, originalSchemaID events.SchemaID, original chatsessions.SequencedItem) (field string, contradicted bool) {
	switch {
	case req.ParentItemID != original.ParentItemID:
		return "ParentItemID", true
	case req.Kind != original.Kind:
		return "Kind", true
	case req.SchemaID != originalSchemaID:
		return "SchemaID", true
	case !equalJSON(req.Payload, original.Payload):
		return "Payload", true
	default:
		return "", false
	}
}

// equalJSON reports whether a and b are structurally equivalent JSON values
// (so key ordering or whitespace differences between two encodings of the
// same source-native payload are never mistaken for a contradiction). Either
// value failing to unmarshal falls back to an exact byte comparison rather
// than treating malformed JSON as automatically equal or unequal.
func equalJSON(a, b json.RawMessage) bool {
	var av, bv any
	if err := json.Unmarshal(a, &av); err != nil {
		return bytes.Equal(a, b)
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return bytes.Equal(a, b)
	}
	return reflect.DeepEqual(av, bv)
}

func sequenceOutcomeLabel(outcome chatsessions.SequenceOutcome) string {
	switch outcome {
	case chatsessions.SequenceOutcomeAccepted:
		return "accepted"
	case chatsessions.SequenceOutcomeDuplicate:
		return "duplicate"
	default:
		return "unspecified"
	}
}
