package service

import (
	"context"
	"encoding/json"
	"fmt"

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
