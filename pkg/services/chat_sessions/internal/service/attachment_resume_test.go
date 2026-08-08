package service

import (
	"context"
	"errors"
	"testing"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
)

// TestStore_Attach_ResumeReactivatesDetachedInteractiveAttachment proves a
// reconnecting caller's Attach(Resume: true, Interactive: true) reactivates
// an existing detached interactive attachment under the new ConnectionID,
// preserving its original ID and already-advanced AfterSequence delivery
// cursor -- so a reconnect resumes exactly where the previous connection
// left off instead of replaying already-acknowledged history from zero.
func TestStore_Attach_ResumeReactivatesDetachedInteractiveAttachment(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newSequencingTestSession(t)
	last, session := sequenceAndAdvance(t, store, session, 1, 3)

	original, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-a", Interactive: true})
	if err != nil {
		t.Fatalf("Attach (original): %v", err)
	}
	acked, err := store.AcknowledgeAttachment(ctx, acknowledgeAttachmentRequest(session.ID, original.Attachment.ID, session.Version, last))
	if err != nil {
		t.Fatalf("AcknowledgeAttachment: %v", err)
	}
	if _, err := store.Detach(ctx, chatsessions.DetachRequest{SessionID: session.ID, AttachmentID: original.Attachment.ID}); err != nil {
		t.Fatalf("Detach: %v", err)
	}

	resumed, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-b", Interactive: true, Resume: true})
	if err != nil {
		t.Fatalf("Attach (resume): %v", err)
	}
	if resumed.Attachment.ID != original.Attachment.ID {
		t.Fatalf("resumed attachment ID = %q, want original ID %q (a fresh ID would replay from zero)", resumed.Attachment.ID, original.Attachment.ID)
	}
	if resumed.Attachment.AfterSequence != acked.Attachment.AfterSequence {
		t.Fatalf("resumed AfterSequence = %d, want preserved %d", resumed.Attachment.AfterSequence, acked.Attachment.AfterSequence)
	}
	if resumed.Attachment.ConnectionID != "conn-b" {
		t.Fatalf("resumed ConnectionID = %q, want conn-b", resumed.Attachment.ConnectionID)
	}
	if resumed.Attachment.Detached {
		t.Fatal("resumed attachment Detached = true, want false")
	}

	store.mu.RLock()
	count := len(store.sessions[session.ID].attachments)
	store.mu.RUnlock()
	if count != 1 {
		t.Fatalf("session has %d attachments after resume, want exactly 1 (reactivated, not duplicated)", count)
	}
}

// TestStore_Attach_ResumeSelectsOwnDetachedAttachment proves two consumers
// with different acknowledged positions cannot inherit each other's cursor:
// identity-less resume rejects the ambiguity, then each caller resumes its
// own durable Attachment identity regardless of detach or reconnect order.
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestStore_Attach_ResumeSelectsOwnDetachedAttachment(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newSequencingTestSession(t)
	last, session := sequenceAndAdvance(t, store, session, 1, 3)

	first, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-a", Interactive: true})
	if err != nil {
		t.Fatalf("Attach (first): %v", err)
	}
	second, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-b", Interactive: true})
	if err != nil {
		t.Fatalf("Attach (second): %v", err)
	}
	firstAck, err := store.AcknowledgeAttachment(ctx, acknowledgeAttachmentRequest(session.ID, first.Attachment.ID, session.Version, 1))
	if err != nil {
		t.Fatalf("AcknowledgeAttachment (first): %v", err)
	}
	current, err := store.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession after first acknowledgement: %v", err)
	}
	secondAck, err := store.AcknowledgeAttachment(ctx, acknowledgeAttachmentRequest(session.ID, second.Attachment.ID, current.Session.Version, last))
	if err != nil {
		t.Fatalf("AcknowledgeAttachment (second): %v", err)
	}
	if _, err := store.Detach(ctx, chatsessions.DetachRequest{SessionID: session.ID, AttachmentID: first.Attachment.ID}); err != nil {
		t.Fatalf("Detach (first): %v", err)
	}
	if _, err := store.Detach(ctx, chatsessions.DetachRequest{SessionID: session.ID, AttachmentID: second.Attachment.ID}); err != nil {
		t.Fatalf("Detach (second): %v", err)
	}

	_, err = store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-c", Interactive: true, Resume: true})
	var ambiguity *chatsessions.AttachmentResumeAmbiguityError
	if !errors.As(err, &ambiguity) || !errors.Is(err, chatsessions.ErrAmbiguousAttachmentResume) {
		t.Fatalf("Attach (ambiguous resume): got %v, want AttachmentResumeAmbiguityError", err)
	}
	if ambiguity.CandidateCount != 2 {
		t.Fatalf("ambiguous resume candidate count = %d, want 2", ambiguity.CandidateCount)
	}

	resumedSecond, err := store.Attach(ctx, chatsessions.AttachRequest{
		SessionID: session.ID, ConnectionID: "conn-d", Interactive: true,
		Resume: true, ResumeAttachmentID: second.Attachment.ID,
	})
	if err != nil {
		t.Fatalf("Attach (resume second): %v", err)
	}
	resumedFirst, err := store.Attach(ctx, chatsessions.AttachRequest{
		SessionID: session.ID, ConnectionID: "conn-e", Interactive: true,
		Resume: true, ResumeAttachmentID: first.Attachment.ID,
	})
	if err != nil {
		t.Fatalf("Attach (resume first): %v", err)
	}
	if resumedSecond.Attachment.ID != second.Attachment.ID || resumedSecond.Attachment.AfterSequence != secondAck.Attachment.AfterSequence {
		t.Fatalf("resumed second = %+v, want ID %q and AfterSequence %d", resumedSecond.Attachment, second.Attachment.ID, secondAck.Attachment.AfterSequence)
	}
	if resumedFirst.Attachment.ID != first.Attachment.ID || resumedFirst.Attachment.AfterSequence != firstAck.Attachment.AfterSequence {
		t.Fatalf("resumed first = %+v, want ID %q and AfterSequence %d", resumedFirst.Attachment, first.Attachment.ID, firstAck.Attachment.AfterSequence)
	}
}

// TestStore_Attach_ResumeWithoutDetachedAttachmentCreatesFresh proves
// Attach(Resume: true) has no effect -- an ordinary fresh attachment at
// AfterSequence 0 is created -- when the session has no detached interactive
// attachment: a session's first-ever attach, and a Resume request while an
// interactive attachment is already actively connected, both fall through
// to this same safe default.
func TestStore_Attach_ResumeWithoutDetachedAttachmentCreatesFresh(t *testing.T) {
	ctx := context.Background()

	t.Run("first ever attach", func(t *testing.T) {
		store, session := newAttachTestSession(t)
		result, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-a", Interactive: true, Resume: true})
		if err != nil {
			t.Fatalf("Attach (resume, no prior attachment): %v", err)
		}
		if result.Attachment.AfterSequence != 0 {
			t.Fatalf("AfterSequence = %d, want 0", result.Attachment.AfterSequence)
		}
		if result.Attachment.Detached {
			t.Fatal("freshly created attachment Detached = true, want false")
		}
	})

	t.Run("interactive attachment still connected", func(t *testing.T) {
		store, session := newAttachTestSession(t)
		live, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-a", Interactive: true})
		if err != nil {
			t.Fatalf("Attach (live): %v", err)
		}

		result, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-b", Interactive: true, Resume: true})
		if err != nil {
			t.Fatalf("Attach (resume, no detached attachment): %v", err)
		}
		if result.Attachment.ID == live.Attachment.ID {
			t.Fatal("Resume reactivated a still-connected (non-detached) attachment, want a fresh one")
		}
		if result.Attachment.AfterSequence != 0 {
			t.Fatalf("AfterSequence = %d, want 0", result.Attachment.AfterSequence)
		}
	})
}
