package service

import (
	"context"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
)

// Attach registers one connection's delivery position against a Chat
// Session, or -- when req.Resume and req.Interactive are both true and this
// session already has a detached interactive attachment (left behind by an
// earlier connection's Detach; see Detach's own doc comment) -- reactivates
// that same attachment under req.ConnectionID instead of minting a fresh
// one, preserving its ID and already-advanced AfterSequence delivery
// cursor. Resume has no effect (an ordinary fresh attachment is created)
// when no detached interactive attachment exists, so it is always safe to
// request. It reports *NotFoundError for an unknown SessionID and a
// *ValidationError when ConnectionID is blank; in either failure case no
// attachment is created or reactivated. A successful attachment is
// independent of every other attachment on the session and of the session's
// own state: it never reads or writes Session, episodes, turns, or control
// intents.
func (s *Store) Attach(_ context.Context, req chatsessions.AttachRequest) (result chatsessions.AttachResult, err error) {
	s.logStart("Attach", req.SessionID)
	defer func() {
		s.logOutcome("Attach", req.SessionID, err, "attachment_id", result.Attachment.ID)
	}()
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.sessions[req.SessionID]
	if !ok {
		return chatsessions.AttachResult{}, &chatsessions.NotFoundError{Value: "Session", ID: req.SessionID}
	}

	if req.Resume && req.Interactive {
		if resumed, found := resumableInteractiveAttachment(record.attachments); found {
			resumed.ConnectionID = req.ConnectionID
			resumed.Detached = false
			if err := resumed.Validate(); err != nil {
				return chatsessions.AttachResult{}, err
			}
			record.attachments[resumed.ID] = resumed
			s.sessions[req.SessionID] = record
			return chatsessions.AttachResult{Attachment: resumed}, nil
		}
	}

	attachment := chatsessions.Attachment{
		ID:           s.newID(),
		SessionID:    req.SessionID,
		ConnectionID: req.ConnectionID,
		Interactive:  req.Interactive,
	}
	if err := attachment.Validate(); err != nil {
		return chatsessions.AttachResult{}, err
	}

	record.attachments[attachment.ID] = attachment
	s.sessions[req.SessionID] = record

	return chatsessions.AttachResult{Attachment: attachment}, nil
}

// resumableInteractiveAttachment returns the one detached, interactive
// attachment in attachments, if any. In ordinary use at most one exists at a
// time for a given session: Attach's own Resume path is the only way to
// reactivate one, and a still-connected interactive attachment is never
// marked detached.
func resumableInteractiveAttachment(attachments map[string]chatsessions.Attachment) (chatsessions.Attachment, bool) {
	for _, attachment := range attachments {
		if attachment.Detached && attachment.Interactive {
			return attachment, true
		}
	}
	return chatsessions.Attachment{}, false
}

// Detach marks one previously registered Attachment as detached rather than
// removing it, preserving its ID and already-advanced AfterSequence delivery
// cursor so a later Attach carrying Resume can reactivate it (see Attach's
// own doc comment) instead of a reconnecting client silently replaying
// already-delivered history from position zero. Detaching an
// already-detached attachment is idempotent. It reports *NotFoundError when
// SessionID does not identify an existing session or when AttachmentID does
// not identify an attachment ever registered on that session; in either
// failure case no attachment is changed. Detaching one attachment never
// changes any other attachment or the session's own state.
func (s *Store) Detach(_ context.Context, req chatsessions.DetachRequest) (result chatsessions.DetachResult, err error) {
	s.logStart("Detach", req.SessionID)
	defer func() {
		s.logOutcome("Detach", req.SessionID, err, "attachment_id", req.AttachmentID)
	}()
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.sessions[req.SessionID]
	if !ok {
		return chatsessions.DetachResult{}, &chatsessions.NotFoundError{Value: "Session", ID: req.SessionID}
	}
	attachment, ok := record.attachments[req.AttachmentID]
	if !ok {
		return chatsessions.DetachResult{}, &chatsessions.NotFoundError{Value: "Attachment", ID: req.AttachmentID}
	}

	attachment.Detached = true
	record.attachments[req.AttachmentID] = attachment
	s.sessions[req.SessionID] = record

	return chatsessions.DetachResult{}, nil
}
