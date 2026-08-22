package service

import (
	"context"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
)

// Attach registers one connection's delivery position against a Chat
// Session, or -- when req.Resume and req.Interactive are both true --
// reactivates the exact detached attachment named by ResumeAttachmentID. An
// identity-less resume is allowed only when one detached interactive
// attachment exists; several candidates return
// *AttachmentResumeAmbiguityError rather than leaking one consumer's cursor
// to another. Resume has no effect (an ordinary fresh attachment is created)
// when no detached interactive attachment exists, so it is safe to request
// for a session's first-ever attachment. It reports *NotFoundError for an
// unknown SessionID and a *ValidationError when ConnectionID is blank; in
// either failure case no attachment is created or reactivated. A successful
// attachment is independent of every other attachment on the session and of
// the session's own state: it never reads or writes Session, episodes, turns,
// or control intents.
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
		resumed, found, resumeErr := resumableInteractiveAttachment(record.attachments, req.SessionID, req.ResumeAttachmentID)
		if resumeErr != nil {
			return chatsessions.AttachResult{}, resumeErr
		}
		if found {
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

// resumableInteractiveAttachment selects a detached interactive attachment
// by its durable identity. Without an explicit identity, it returns the sole
// detached candidate only; it rejects more than one candidate rather than
// relying on Go map iteration order to choose a foreign delivery cursor.
func resumableInteractiveAttachment(
	attachments map[string]chatsessions.Attachment,
	sessionID, attachmentID string,
) (chatsessions.Attachment, bool, error) {
	if attachmentID != "" {
		attachment, found := attachments[attachmentID]
		if !found || !attachment.Detached || !attachment.Interactive {
			return chatsessions.Attachment{}, false, &chatsessions.NotFoundError{Value: "Attachment", ID: attachmentID}
		}
		return attachment, true, nil
	}

	var resumed chatsessions.Attachment
	candidates := 0
	for _, attachment := range attachments {
		if attachment.Detached && attachment.Interactive {
			resumed = attachment
			candidates++
		}
	}
	if candidates > 1 {
		return chatsessions.Attachment{}, false, &chatsessions.AttachmentResumeAmbiguityError{SessionID: sessionID, CandidateCount: candidates}
	}
	return resumed, candidates == 1, nil
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
