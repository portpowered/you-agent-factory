package service

import (
	"context"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
)

// Attach registers one connection's delivery position against a Chat
// Session. It reports *NotFoundError for an unknown SessionID and a
// *ValidationError when ConnectionID is blank; in either failure case no
// attachment is created. A successful attachment is independent of every
// other attachment on the session and of the session's own state: it never
// reads or writes Session, episodes, turns, or control intents.
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

// Detach removes one previously registered Attachment. It reports
// *NotFoundError when SessionID does not identify an existing session or
// when AttachmentID does not identify an existing attachment on that
// session; in either failure case no attachment is removed. Removing one
// attachment never changes any other attachment or the session's own state.
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
	if _, ok := record.attachments[req.AttachmentID]; !ok {
		return chatsessions.DetachResult{}, &chatsessions.NotFoundError{Value: "Attachment", ID: req.AttachmentID}
	}

	delete(record.attachments, req.AttachmentID)
	s.sessions[req.SessionID] = record

	return chatsessions.DetachResult{}, nil
}
