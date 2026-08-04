package service

import (
	"context"
	"fmt"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
)

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
// retained by Events -- confirmed with one bounded Read rather than assumed
// (*AttachmentRetentionGapError). A successful advancement mutates only the
// named Attachment's AfterSequence: it never reads or writes any other
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
	switch readResult.Outcome {
	case events.ReadOutcomeProgress:
		// The full range up to the requested position is still retained.
	case events.ReadOutcomeGap:
		return chatsessions.AcknowledgeAttachmentResult{}, &chatsessions.AttachmentRetentionGapError{
			SessionID:        req.SessionID,
			AttachmentID:     req.AttachmentID,
			Requested:        uint64(req.AfterSequence),
			EarliestRetained: uint64(readResult.Gap.EarliestRetained),
			Head:             uint64(readResult.Gap.Head),
		}
	default:
		return chatsessions.AcknowledgeAttachmentResult{}, fmt.Errorf("chat sessions: events read returned unexpected outcome %d for attachment acknowledgement", readResult.Outcome)
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
