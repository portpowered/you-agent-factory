package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
)

// newAttachTestSession constructs a Store and one created Session ready for
// Attach/Detach calls.
func newAttachTestSession(t *testing.T) (*Store, chatsessions.Session) {
	t.Helper()
	store := NewStore(sequentialIDs("session"), fixedClock(time.Now()))
	created, err := store.CreateSession(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return store, created.Session
}

// TestStore_Attach_ReturnsDetachedValueWithUniqueID proves a successful
// Attach retains its own session ID, connection ID, zero initial
// AfterSequence, and interactive flag as a detached value.
func TestStore_Attach_ReturnsDetachedValueWithUniqueID(t *testing.T) {
	ctx := context.Background()
	store, session := newAttachTestSession(t)

	result, err := store.Attach(ctx, chatsessions.AttachRequest{
		SessionID:    session.ID,
		ConnectionID: "conn-a",
		Interactive:  true,
	})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	attachment := result.Attachment
	if attachment.ID == "" {
		t.Fatal("Attach: expected a non-blank attachment ID")
	}
	if attachment.SessionID != session.ID {
		t.Fatalf("SessionID = %q, want %q", attachment.SessionID, session.ID)
	}
	if attachment.ConnectionID != "conn-a" {
		t.Fatalf("ConnectionID = %q, want conn-a", attachment.ConnectionID)
	}
	if attachment.AfterSequence != 0 {
		t.Fatalf("AfterSequence = %d, want 0", attachment.AfterSequence)
	}
	if !attachment.Interactive {
		t.Fatal("Interactive = false, want true")
	}

	attachment.ConnectionID = "mutated"
	store.mu.RLock()
	stored := store.sessions[session.ID].attachments[result.Attachment.ID]
	store.mu.RUnlock()
	if stored.ConnectionID != "conn-a" {
		t.Fatalf("mutating returned value affected stored attachment: got %q", stored.ConnectionID)
	}
}

// TestStore_Attach_TwoAttachmentsRemainIndependent proves two attachments to
// the same session with matching other fields remain distinct, and attaching
// or detaching one does not change the other attachment or the session's
// stream head, episode history, turns, or version.
func TestStore_Attach_TwoAttachmentsRemainIndependent(t *testing.T) {
	ctx := context.Background()
	store, session := newAttachTestSession(t)

	first, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-a", Interactive: true})
	if err != nil {
		t.Fatalf("Attach first: %v", err)
	}
	second, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-a", Interactive: true})
	if err != nil {
		t.Fatalf("Attach second: %v", err)
	}
	if first.Attachment.ID == second.Attachment.ID {
		t.Fatalf("two attachments shared ID %q, want distinct", first.Attachment.ID)
	}

	if _, err := store.Detach(ctx, chatsessions.DetachRequest{SessionID: session.ID, AttachmentID: first.Attachment.ID}); err != nil {
		t.Fatalf("Detach first: %v", err)
	}

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if _, ok := record.attachments[first.Attachment.ID]; ok {
		t.Fatal("detached attachment still present")
	}
	remaining, ok := record.attachments[second.Attachment.ID]
	if !ok {
		t.Fatal("second attachment was removed by detaching the first")
	}
	if remaining != second.Attachment {
		t.Fatalf("second attachment changed: got %+v, want %+v", remaining, second.Attachment)
	}
	if record.session != session {
		t.Fatalf("attach/detach mutated Session: got %+v, want %+v", record.session, session)
	}
	if len(record.episodes) != 1 {
		t.Fatalf("attach/detach created %d episodes, want 1", len(record.episodes))
	}
	if len(record.turns) != 0 {
		t.Fatalf("attach/detach created %d turns, want 0", len(record.turns))
	}
}

// TestStore_Detach_UnknownOrRemovedIsTypedNotFound proves detaching an
// unknown or already-removed attachment reports *NotFoundError and performs
// no additional mutation.
func TestStore_Detach_UnknownOrRemovedIsTypedNotFound(t *testing.T) {
	ctx := context.Background()
	store, session := newAttachTestSession(t)

	_, err := store.Detach(ctx, chatsessions.DetachRequest{SessionID: session.ID, AttachmentID: "does-not-exist"})
	var notFound *chatsessions.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("Detach unknown attachment: got %v, want *NotFoundError", err)
	}
	if notFound.Value != "Attachment" || notFound.ID != "does-not-exist" {
		t.Fatalf("NotFoundError = %+v, want Value=Attachment ID=does-not-exist", notFound)
	}

	attached, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-a"})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if _, err := store.Detach(ctx, chatsessions.DetachRequest{SessionID: session.ID, AttachmentID: attached.Attachment.ID}); err != nil {
		t.Fatalf("Detach: %v", err)
	}

	_, err = store.Detach(ctx, chatsessions.DetachRequest{SessionID: session.ID, AttachmentID: attached.Attachment.ID})
	if !errors.As(err, &notFound) {
		t.Fatalf("Detach already-removed attachment: got %v, want *NotFoundError", err)
	}
	if notFound.Value != "Attachment" || notFound.ID != attached.Attachment.ID {
		t.Fatalf("NotFoundError = %+v, want Value=Attachment ID=%s", notFound, attached.Attachment.ID)
	}
}

// TestStore_Attach_UnknownSessionOrInvalidInputCreatesNoAttachment proves
// attaching to an unknown session or with a blank ConnectionID reports the
// applicable typed error and creates no attachment.
func TestStore_Attach_UnknownSessionOrInvalidInputCreatesNoAttachment(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown session", func(t *testing.T) {
		store := NewStore(sequentialIDs("session"), fixedClock(time.Now()))
		_, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: "does-not-exist", ConnectionID: "conn-a"})
		var notFound *chatsessions.NotFoundError
		if !errors.As(err, &notFound) {
			t.Fatalf("Attach unknown session: got %v, want *NotFoundError", err)
		}
		if notFound.Value != "Session" || notFound.ID != "does-not-exist" {
			t.Fatalf("NotFoundError = %+v, want Value=Session ID=does-not-exist", notFound)
		}
	})

	t.Run("blank connection id", func(t *testing.T) {
		store, session := newAttachTestSession(t)
		_, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: ""})
		if !errors.Is(err, chatsessions.ErrRequiredValue) {
			t.Fatalf("Attach blank ConnectionID: got %v, want ErrRequiredValue", err)
		}
		store.mu.RLock()
		count := len(store.sessions[session.ID].attachments)
		store.mu.RUnlock()
		if count != 0 {
			t.Fatalf("Attach with invalid input created %d attachments, want 0", count)
		}
	})
}

// TestStore_Detach_UnknownSessionIsTypedNotFound proves Detach against an
// unknown SessionID reports *NotFoundError naming Session.
func TestStore_Detach_UnknownSessionIsTypedNotFound(t *testing.T) {
	ctx := context.Background()
	store := NewStore(sequentialIDs("session"), fixedClock(time.Now()))

	_, err := store.Detach(ctx, chatsessions.DetachRequest{SessionID: "does-not-exist", AttachmentID: "attachment-1"})
	var notFound *chatsessions.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("Detach unknown session: got %v, want *NotFoundError", err)
	}
	if notFound.Value != "Session" || notFound.ID != "does-not-exist" {
		t.Fatalf("NotFoundError = %+v, want Value=Session ID=does-not-exist", notFound)
	}
}

// TestStore_Attach_ConcurrentAttachAndDetachRaceFree proves concurrent
// attach/detach operations against one session are race-free, produce
// unique accepted identities, and settle on exactly the attachments that
// were never detached.
func TestStore_Attach_ConcurrentAttachAndDetachRaceFree(t *testing.T) {
	ctx := context.Background()
	store, session := newAttachTestSession(t)

	const n = 25
	ids := make([]string, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			result, err := store.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-a"})
			if err != nil {
				t.Errorf("Attach[%d]: %v", i, err)
				return
			}
			ids[i] = result.Attachment.ID
		}(i)
	}
	wg.Wait()

	seen := make(map[string]bool, n)
	for _, id := range ids {
		if id == "" {
			t.Fatal("Attach produced a blank attachment ID")
		}
		if seen[id] {
			t.Fatalf("Attach produced duplicate ID %q", id)
		}
		seen[id] = true
	}

	toDetach := ids[:n/2]
	var detachWG sync.WaitGroup
	for _, id := range toDetach {
		detachWG.Add(1)
		go func(id string) {
			defer detachWG.Done()
			if _, err := store.Detach(ctx, chatsessions.DetachRequest{SessionID: session.ID, AttachmentID: id}); err != nil {
				t.Errorf("Detach(%s): %v", id, err)
			}
		}(id)
	}
	detachWG.Wait()

	store.mu.RLock()
	record := store.sessions[session.ID]
	store.mu.RUnlock()
	if len(record.attachments) != n-len(toDetach) {
		t.Fatalf("remaining attachments = %d, want %d", len(record.attachments), n-len(toDetach))
	}
	for _, id := range toDetach {
		if _, ok := record.attachments[id]; ok {
			t.Fatalf("detached attachment %q still present", id)
		}
	}
	for _, id := range ids[n/2:] {
		if _, ok := record.attachments[id]; !ok {
			t.Fatalf("non-detached attachment %q missing", id)
		}
	}
}
