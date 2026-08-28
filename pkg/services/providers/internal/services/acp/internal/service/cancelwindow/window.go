// Package cancelwindow tracks the one live ACP session/prompt turn a
// session/cancel notification can currently target. It is self-contained
// (it depends on no other daemon state) so the exact-attempt cancellation
// seam can be reasoned about and tested independent of daemon's broader
// process lifecycle.
package cancelwindow

import (
	"context"
	"fmt"
	"sync"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// defaultSendTimeout is passed to the outbound session/cancel notification send as
// a best-effort deadline. The underlying acp-go-sdk connection does not
// observe ctx once the send has entered its synchronous, unbuffered
// io.Writer.Write call (SendNotification only checks ctx.Done() before that
// point), so a peer that stops reading can block the send indefinitely
// regardless of this value. Session ownership is therefore never decided by
// waiting for the send to finish (see the ownership field below) - only the
// send itself, and whichever goroutine is waiting on its result, can be left
// blocked by a stuck peer.
const defaultSendTimeout = 500 * time.Millisecond

// Session is the exact live session/prompt turn a session/cancel
// notification can currently target. It exists only for the span between
// the owning Window.Begin call and the matching Window.End call. cancelled
// is set by End to the real recorded outcome (whether the response's
// StopReason was truly Cancelled) strictly before done is closed, so any
// TryCancel call already blocked on <-done observes it without its own
// synchronization: this is what lets TryCancel ground "accepted" in the
// attempt's actual terminal behavior instead of merely in "a cancel
// notification was sent while a session looked active".
//
// mu arbitrates ownership, not delivery: it decides, in one atomic step,
// whether TryCancel or End is the first to act on this still-open
// generation, but it is never held across the outbound send or any other
// I/O. TryCancel claims ownership (open -> sending) before it sends anything
// and End claims ownership (open -> ended) before it records completion;
// exactly one of those two transitions can win a given generation, so a
// notification can never be sent after End has already claimed the
// generation. Because mu's critical sections are memory-only, End can always
// complete promptly - including releasing Window's active slot for identity
// reuse - even while a TryCancel it lost the race to is still blocked deep
// inside a stuck peer write; nothing in this package waits for that write to
// return.
type Session struct {
	attemptID   string
	sessionID   acpsdk.SessionId
	connection  *acpsdk.ClientSideConnection
	sendTimeout time.Duration
	mu          sync.Mutex
	ownership   sessionOwnership
	cancelled   bool
	done        chan struct{}
}

// sessionOwnership tracks which side - TryCancel or End - first acted on a
// still-open generation. Exactly one transition out of sessionOpen can ever
// succeed for a given Session.
type sessionOwnership int

const (
	sessionOpen sessionOwnership = iota
	sessionSending
	sessionEnded
)

// Window is the single-slot live-session tracker for one ACP daemon. The
// zero value is ready to use.
type Window struct {
	mu          sync.Mutex
	active      *Session
	sendTimeout time.Duration
}

// Begin opens the cancelable window for one attempt's in-flight session/
// prompt turn.
func (w *Window) Begin(attemptID string, sessionID acpsdk.SessionId, connection *acpsdk.ClientSideConnection) *Session {
	sendTimeout := w.sendTimeout
	if sendTimeout == 0 {
		sendTimeout = defaultSendTimeout
	}
	session := &Session{
		attemptID:   attemptID,
		sessionID:   sessionID,
		connection:  connection,
		sendTimeout: sendTimeout,
		done:        make(chan struct{}),
	}
	w.mu.Lock()
	w.active = session
	w.mu.Unlock()
	return session
}

// End closes the window opened by Begin, recording cancelled as the turn's
// real outcome strictly before unblocking any TryCancel call already waiting
// on it. Safe to call on every daemon.execute exit path, including an
// unexpected terminal unwind - and safe to call regardless of whether a
// TryCancel for this exact session is concurrently blocked inside a stuck
// outbound send: End's own work here is memory-only (a mutex-guarded
// ownership transition plus a few field writes) and never waits on that
// send, so a peer that never reads its pipe can delay TryCancel's own return
// without ever delaying End, Window's active-slot release, or a later Begin
// reusing this identity. The ownership transition (attempted, not required
// to succeed - see sessionOwnership) is what stops a TryCancel that has not
// yet started sending from ever doing so once End has run; it does not gate
// End's own completion in either direction. The active field is cleared, and
// the lock released, strictly before done is closed: a concurrent TryCancel
// that observes the window still open is therefore guaranteed session.done
// is not yet closed, and one that observes it cleared is guaranteed the
// attempt's real cancelled outcome is already final and safe to read after
// <-session.done.
func (w *Window) End(session *Session, cancelled bool) {
	session.mu.Lock()
	if session.ownership == sessionOpen {
		session.ownership = sessionEnded
	}
	session.mu.Unlock()
	session.cancelled = cancelled

	w.mu.Lock()
	if w.active == session {
		w.active = nil
	}
	w.mu.Unlock()
	close(session.done)
}

// Claim atomically captures the exact Session currently open for attemptID,
// if any, without altering window state (it does not clear w.active - only
// End does that). The returned Session is an opaque generation handle: its
// own TryCancel method is thereafter the only way to deliver to it, and it
// never re-resolves "whatever the window's active session is now". This is
// what lets a caller pin the exact generation a control was claimed against
// at claim time, so a later Begin call that reuses attemptID for a
// replacement generation cannot be substituted in when the claimed control's
// delivery finally runs.
func (w *Window) Claim(attemptID string) (*Session, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.active == nil || w.active.attemptID != attemptID {
		return nil, false
	}
	return w.active, true
}

// TryCancel delivers a session/cancel notification to this exact session -
// the one Window.Claim captured - and blocks (bounded by ctx) until it
// observes the session's real recorded terminal outcome. Because it operates
// only on the Session pinned at claim time, never on whatever Window.active
// happens to be right now, a later Begin call that reuses this session's
// attemptID for a different generation cannot redirect delivery to that
// replacement: this exact Session either delivers to itself, or - if it has
// already ended - reports accepted=false, err=nil without sending anything,
// so a stale claim can never observe a replacement generation's outcome.
//
// Ownership of "may this generation still be sent to" is claimed with a
// single mutex-guarded transition (open -> sending) before anything is sent,
// the same way End claims (open -> ended) before it records completion.
// Exactly one of those two transitions can win: if End already ran, this
// transition fails and TryCancel returns accepted=false, err=nil without
// sending anything, so a send can never land after this generation - and the
// identity it was claimed against - is already free for reuse. If TryCancel
// wins instead, End is not blocked by that outcome in any way: it still
// records the generation's real outcome and releases Window's active slot
// immediately, on its own schedule, regardless of whether this call's send
// has completed, is still in flight, or never returns at all. The mutex is
// only ever held for these instantaneous bookkeeping transitions, never
// across the outbound send itself.
//
// The outbound send passes its session-local sendTimeout as a best-effort ctx
// deadline, independent of the caller's ctx: this keeps a genuine send
// failure (wrapped in providers.ErrControlSignalFailed below) unambiguously
// distinct from the caller giving up during the wait phase below, which
// instead surfaces the caller's own unwrapped ctx.Err(). It does not,
// however, guarantee the send returns within sendTimeout - the underlying
// SDK write is not itself ctx-aware once started (see defaultSendTimeout's
// doc) -
// so a stuck peer can still leave this specific TryCancel call, and whatever
// goroutine is waiting on it, blocked past that bound. Nothing else in this
// package waits on it. A send failure returns promptly without waiting
// further: an already-broken connection has no reason to ever close
// session.done on its own.
func (session *Session) TryCancel(ctx context.Context) (accepted bool, err error) {
	session.mu.Lock()
	won := session.ownership == sessionOpen
	if won {
		session.ownership = sessionSending
	}
	session.mu.Unlock()
	if !won {
		// End already won ownership of this generation: sending a
		// notification to a dead session would be meaningless, and could
		// never be misread as reaching a replacement generation, since we
		// never touch w.active.
		return false, nil
	}

	sendCtx, cancelSend := context.WithTimeout(context.Background(), session.sendTimeout)
	sendErr := session.connection.Cancel(sendCtx, acpsdk.CancelNotification{SessionId: session.sessionID})
	cancelSend()

	if sendErr != nil {
		return false, fmt.Errorf("%w: deliver session/cancel: %v", providers.ErrControlSignalFailed, sendErr)
	}

	select {
	case <-session.done:
		return session.cancelled, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}
