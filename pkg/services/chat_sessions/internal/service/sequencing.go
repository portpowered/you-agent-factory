// Package service's sequencer operations (Sequence, AdvanceStreamHead, and
// AcknowledgeAttachment) live together in this one file rather than split
// one-per-file: they are the three tightly related halves of one durable
// responsibility -- committing a Chat Session's aggregate record stream and
// advancing the two independent positions ("what has been committed" and
// "what one attachment has observed") derived from it -- and consolidating
// them keeps this already file-count-constrained package from growing by
// one file per sequencer operation.
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
// req.SessionID's sequencedItemIDs/sequencedPositions indexes are only ever
// updated after a successful, newly accepted Events append.
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
	envelope, marshalErr := marshalSequencedItemEnvelope(item)
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
		record.sequencedPositions[appendResult.Record.ID.Position] = sequencedSourceIdentity{
			SourceType:     req.SourceType,
			SourceID:       req.SourceID,
			SourceSequence: req.SourceSequence,
			SourceEventID:  req.SourceEventID,
		}
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

// marshalSequencedItemEnvelope encodes item as the SequencedItem envelope
// Sequence commits as an Events record Payload, splicing item.Payload's
// exact bytes into the result rather than routing them through
// json.Marshal(item) directly: Go's stdlib marshaler runs any nested
// json.Marshaler output -- including a json.RawMessage field -- through its
// own compaction (and, by default, HTML-escaping) step, which can silently
// alter a source-native payload's insignificant whitespace or
// HTML-sensitive characters before Events ever sees it. That would break
// the "stores the assigned identities and source-native payload verbatim"
// contract. The scalar fields (ItemID, ParentItemID, Kind) still go through
// json.Marshal individually, so their own string escaping stays standard;
// only the payload's bytes are required -- and guaranteed here -- to survive
// byte-for-byte unchanged.
func marshalSequencedItemEnvelope(item chatsessions.SequencedItem) ([]byte, error) {
	itemID, err := json.Marshal(item.ItemID)
	if err != nil {
		return nil, fmt.Errorf("itemId: %w", err)
	}
	kind, err := json.Marshal(item.Kind)
	if err != nil {
		return nil, fmt.Errorf("kind: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString(`{"itemId":`)
	buf.Write(itemID)
	if item.ParentItemID != "" {
		parentItemID, marshalErr := json.Marshal(item.ParentItemID)
		if marshalErr != nil {
			return nil, fmt.Errorf("parentItemId: %w", marshalErr)
		}
		buf.WriteString(`,"parentItemId":`)
		buf.Write(parentItemID)
	}
	buf.WriteString(`,"kind":`)
	buf.Write(kind)
	buf.WriteString(`,"payload":`)
	buf.Write(item.Payload)
	buf.WriteByte('}')
	return buf.Bytes(), nil
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

// AdvanceStreamHead advances req.SessionID's StreamHead to
// req.AggregateSequence under an optimistic-version guard. It first requires
// that req.AggregateSequence was actually committed by this exact session's
// sequencer for the exact stated (SourceType, SourceID, SourceSequence,
// SourceEventID) tuple -- checked against sessionRecord.sequencedPositions,
// which Sequence alone populates on a newly accepted commit -- and rejects
// with *UncommittedStreamPositionError, mutating nothing, when the position
// was never sequenced for this session at all, belongs to a different
// session's own topic, or was committed under a different source identity.
// Without this check a caller could advance StreamHead to fabricated or
// cross-session state, which AcknowledgeAttachment would then trust as
// proof of a range no attachment ever actually observed.
//
// When StreamHead already stands at or beyond req.AggregateSequence --
// because this is a retry of an advancement that already committed, or a
// stale call for a position an intervening advancement already passed --
// AdvanceStreamHead reconciles idempotently: it reports
// AdvanceStreamHeadOutcomeAlreadyCurrent and leaves the stored session,
// including its Version, byte-for-byte unchanged, without consulting
// ExpectedVersion at all. This mirrors BindFactorySession's "already at
// target value" precedent: an already-satisfied request converges on the
// committed result instead of depending on a version that may have already
// moved past ExpectedVersion precisely because the prior call succeeded.
// Only when the position is genuinely new does AdvanceStreamHead consult
// ExpectedVersion, reporting *ConflictError on a stale mismatch and exposing
// no partially committed head update in that case: the session is either
// left completely unchanged (conflict) or fully updated (StreamHead,
// Version, UpdatedAt together), never one without the others. A successful
// advancement mutates only those three fields, leaving SelectedTarget,
// TargetEpisode, ActiveTurnID, and every attachment/control/episode record
// untouched.
func (s *Store) AdvanceStreamHead(ctx context.Context, req chatsessions.AdvanceStreamHeadRequest) (result chatsessions.AdvanceStreamHeadResult, err error) {
	s.logStart("AdvanceStreamHead", req.SessionID)
	defer func() {
		s.logOutcome("AdvanceStreamHead", req.SessionID, err,
			"source_type", string(req.SourceType),
			"source_id", string(req.SourceID),
			"source_sequence", uint64(req.SourceSequence),
			"source_event_id", string(req.SourceEventID),
			"aggregate_sequence", uint64(req.AggregateSequence),
			"version", result.Session.Version,
			"outcome", advanceStreamHeadOutcomeLabel(result.Outcome))
	}()

	if err := ctx.Err(); err != nil {
		return chatsessions.AdvanceStreamHeadResult{}, err
	}
	if err := req.Validate(); err != nil {
		return chatsessions.AdvanceStreamHeadResult{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.sessions[req.SessionID]
	if !ok {
		return chatsessions.AdvanceStreamHeadResult{}, &chatsessions.NotFoundError{Value: "Session", ID: req.SessionID}
	}

	wantIdentity := sequencedSourceIdentity{
		SourceType:     req.SourceType,
		SourceID:       req.SourceID,
		SourceSequence: req.SourceSequence,
		SourceEventID:  req.SourceEventID,
	}
	if committed, known := record.sequencedPositions[req.AggregateSequence]; !known || committed != wantIdentity {
		return chatsessions.AdvanceStreamHeadResult{}, &chatsessions.UncommittedStreamPositionError{
			SessionID:         req.SessionID,
			AggregateSequence: uint64(req.AggregateSequence),
			SourceType:        string(req.SourceType),
			SourceID:          string(req.SourceID),
			SourceSequence:    uint64(req.SourceSequence),
			SourceEventID:     string(req.SourceEventID),
		}
	}

	if record.session.StreamHead >= uint64(req.AggregateSequence) {
		return chatsessions.AdvanceStreamHeadResult{
			Session: record.session,
			Outcome: chatsessions.AdvanceStreamHeadOutcomeAlreadyCurrent,
		}, nil
	}

	if req.ExpectedVersion != record.session.Version {
		return chatsessions.AdvanceStreamHeadResult{}, &chatsessions.ConflictError{
			Value: "Session", ID: req.SessionID,
			Expected: req.ExpectedVersion, Actual: record.session.Version,
		}
	}

	updated := record.session
	updated.StreamHead = uint64(req.AggregateSequence)
	updated.Version++
	updated.UpdatedAt = s.now()
	if err := updated.Validate(); err != nil {
		return chatsessions.AdvanceStreamHeadResult{}, err
	}

	record.session = updated
	s.sessions[req.SessionID] = record

	return chatsessions.AdvanceStreamHeadResult{
		Session: updated,
		Outcome: chatsessions.AdvanceStreamHeadOutcomeAdvanced,
	}, nil
}

func advanceStreamHeadOutcomeLabel(outcome chatsessions.AdvanceStreamHeadOutcome) string {
	switch outcome {
	case chatsessions.AdvanceStreamHeadOutcomeAdvanced:
		return "advanced"
	case chatsessions.AdvanceStreamHeadOutcomeAlreadyCurrent:
		return "already_current"
	default:
		return "unspecified"
	}
}

// AcknowledgeAttachment advances req.AttachmentID's own AfterSequence
// delivery cursor to req.AfterSequence. When AfterSequence already stands at
// or beyond the requested position -- because this is a retry of an
// acknowledgement that already committed, or a stale/backward-moving
// request -- AcknowledgeAttachment reconciles idempotently: it reports
// AcknowledgeAttachmentOutcomeAlreadyCurrent and leaves the attachment
// byte-for-byte unchanged without consulting ExpectedVersion or Events at
// all, mirroring AdvanceStreamHead's own idempotent-reconcile precedent.
// Only when the position is genuinely new does AcknowledgeAttachment check,
// in order: the requested position must not exceed the session's current
// StreamHead (*AttachmentPositionError), ExpectedVersion must match the
// session's current version (*ConflictError), and the range between the
// attachment's current position and the requested one must still be fully
// retained by Events -- confirmed with one bounded Read rather than assumed.
// AcknowledgeAttachment never trusts a ReadOutcomeProgress result on its
// own as proof that the full requested range was retained: it additionally
// requires the read's own Next cursor to land exactly on the requested
// position (*AttachmentRetentionGapError otherwise), since Events reports
// Progress -- not Gap -- when a caller asks for more records than currently
// exist, and StreamHead is caller-supplied context rather than an
// Events-verified fact. A successful advancement mutates only the named
// Attachment's AfterSequence: it never reads or writes any other
// attachment, Session.StreamHead, Session.Version, or any ControlIntent.
func (s *Store) AcknowledgeAttachment(ctx context.Context, req chatsessions.AcknowledgeAttachmentRequest) (result chatsessions.AcknowledgeAttachmentResult, err error) {
	s.logStart("AcknowledgeAttachment", req.SessionID)
	defer func() {
		s.logOutcome("AcknowledgeAttachment", req.SessionID, err,
			"attachment_id", req.AttachmentID,
			"after_sequence", uint64(req.AfterSequence),
			"version", req.ExpectedVersion,
			"outcome", acknowledgeAttachmentOutcomeLabel(result.Outcome))
	}()

	if err := ctx.Err(); err != nil {
		return chatsessions.AcknowledgeAttachmentResult{}, err
	}
	if err := req.Validate(); err != nil {
		return chatsessions.AcknowledgeAttachmentResult{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.sessions[req.SessionID]
	if !ok {
		return chatsessions.AcknowledgeAttachmentResult{}, &chatsessions.NotFoundError{Value: "Session", ID: req.SessionID}
	}
	attachment, ok := record.attachments[req.AttachmentID]
	if !ok {
		return chatsessions.AcknowledgeAttachmentResult{}, &chatsessions.NotFoundError{Value: "Attachment", ID: req.AttachmentID}
	}

	if attachment.AfterSequence >= uint64(req.AfterSequence) {
		return chatsessions.AcknowledgeAttachmentResult{
			Attachment: attachment,
			Outcome:    chatsessions.AcknowledgeAttachmentOutcomeAlreadyCurrent,
		}, nil
	}

	if uint64(req.AfterSequence) > record.session.StreamHead {
		return chatsessions.AcknowledgeAttachmentResult{}, &chatsessions.AttachmentPositionError{
			SessionID:    req.SessionID,
			AttachmentID: req.AttachmentID,
			Requested:    uint64(req.AfterSequence),
			StreamHead:   record.session.StreamHead,
		}
	}

	if req.ExpectedVersion != record.session.Version {
		return chatsessions.AcknowledgeAttachmentResult{}, &chatsessions.ConflictError{
			Value: "Session", ID: req.SessionID,
			Expected: req.ExpectedVersion, Actual: record.session.Version,
		}
	}

	topic := chatsessions.EventsTopic(req.SessionID)
	readResult, readErr := s.eventsReader.Read(ctx, events.ReadRequest{
		Topic: topic,
		From:  events.Cursor{Topic: topic, Position: events.AggregateSequence(attachment.AfterSequence)},
		Limit: int(uint64(req.AfterSequence) - attachment.AfterSequence),
	})
	if readErr != nil {
		return chatsessions.AcknowledgeAttachmentResult{}, readErr
	}
	if err := verifyAcknowledgeAttachmentReadReachedRequestedPosition(req, readResult); err != nil {
		return chatsessions.AcknowledgeAttachmentResult{}, err
	}

	updated := attachment
	updated.AfterSequence = uint64(req.AfterSequence)
	if err := updated.Validate(); err != nil {
		return chatsessions.AcknowledgeAttachmentResult{}, err
	}

	record.attachments[req.AttachmentID] = updated
	s.sessions[req.SessionID] = record

	return chatsessions.AcknowledgeAttachmentResult{
		Attachment: updated,
		Outcome:    chatsessions.AcknowledgeAttachmentOutcomeAdvanced,
	}, nil
}

// verifyAcknowledgeAttachmentReadReachedRequestedPosition reports a non-nil
// error unless readResult proves the full range between an attachment's
// current position and req.AfterSequence is retained. A bare
// ReadOutcomeProgress is not itself sufficient proof: Events reports
// Progress -- not Gap -- when a caller asks for more records than currently
// exist, so this also requires the read's own Next cursor to land exactly
// on req.AfterSequence; a short read is treated the same as a retention gap,
// since the attachment cannot have genuinely observed a range that does not
// exist (for example, one an unvalidated/fabricated StreamHead claimed but
// Events never actually committed).
func verifyAcknowledgeAttachmentReadReachedRequestedPosition(req chatsessions.AcknowledgeAttachmentRequest, readResult events.ReadResult) error {
	switch readResult.Outcome {
	case events.ReadOutcomeProgress:
		if readResult.Next.Position == events.AggregateSequence(req.AfterSequence) {
			return nil
		}
		return &chatsessions.AttachmentRetentionGapError{
			SessionID:        req.SessionID,
			AttachmentID:     req.AttachmentID,
			Requested:        uint64(req.AfterSequence),
			EarliestRetained: uint64(readResult.Retained.Earliest),
			Head:             uint64(readResult.Retained.Head),
		}
	case events.ReadOutcomeGap:
		return &chatsessions.AttachmentRetentionGapError{
			SessionID:        req.SessionID,
			AttachmentID:     req.AttachmentID,
			Requested:        uint64(req.AfterSequence),
			EarliestRetained: uint64(readResult.Gap.EarliestRetained),
			Head:             uint64(readResult.Gap.Head),
		}
	default:
		return fmt.Errorf("chat sessions: events read returned unexpected outcome %d for attachment acknowledgement", readResult.Outcome)
	}
}

func acknowledgeAttachmentOutcomeLabel(outcome chatsessions.AcknowledgeAttachmentOutcome) string {
	switch outcome {
	case chatsessions.AcknowledgeAttachmentOutcomeAdvanced:
		return "advanced"
	case chatsessions.AcknowledgeAttachmentOutcomeAlreadyCurrent:
		return "already_current"
	default:
		return "unspecified"
	}
}
