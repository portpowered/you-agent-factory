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

// sendTimeout bounds only the outbound session/cancel notification send; it
// does not bound waiting for the attempt to observe cancellation, which is
// instead bounded by the caller's own ctx (see Window.TryCancel).
const sendTimeout = 500 * time.Millisecond

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
// mu arbitrates the one decision that must be atomic between TryCancel and
// End: whether this generation is still open for a notification to be sent
// at all. TryCancel holds mu across both the terminal check and the actual
// outbound send, so End cannot record completion in the gap between "not
// yet terminal" and "notification sent" - either TryCancel observes
// terminal first and sends nothing, or it claims the still-open generation
// first and End cannot record completion until that send finishes. This
// closes the check-then-send race the earlier non-blocking check left open,
// where a delayed send could reach the wire after the generation - and
// potentially its session identity - was already free for reuse.
type Session struct {
	attemptID  string
	sessionID  acpsdk.SessionId
	connection *acpsdk.ClientSideConnection
	mu         sync.Mutex
	terminal   bool
	cancelled  bool
	done       chan struct{}
}

// Window is the single-slot live-session tracker for one ACP daemon. The
// zero value is ready to use.
type Window struct {
	mu     sync.Mutex
	active *Session
}

// Begin opens the cancelable window for one attempt's in-flight session/
// prompt turn.
func (w *Window) Begin(attemptID string, sessionID acpsdk.SessionId, connection *acpsdk.ClientSideConnection) *Session {
	session := &Session{attemptID: attemptID, sessionID: sessionID, connection: connection, done: make(chan struct{})}
	w.mu.Lock()
	w.active = session
	w.mu.Unlock()
	return session
}

// End closes the window opened by Begin, recording cancelled as the turn's
// real outcome strictly before unblocking any TryCancel call already waiting
// on it, and unblocks any TryCancel call already waiting on it. Safe to call
// on every daemon.execute exit path, including an unexpected terminal
// unwind. session.terminal and session.cancelled are recorded under
// session.mu, the same lock TryCancel holds across its own terminal check
// and send: this is what makes "did completion or control win this
// generation" one atomic decision rather than two independently-timed ones.
// The active field is cleared, and the lock released, strictly before done
// is closed: a concurrent TryCancel that observes the window still open is
// therefore guaranteed session.done is not yet closed, and one that observes
// it cleared is guaranteed the attempt's real cancelled outcome is already
// final and safe to read after <-session.done.
func (w *Window) End(session *Session, cancelled bool) {
	session.mu.Lock()
	session.terminal = true
	session.cancelled = cancelled
	session.mu.Unlock()

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
// The terminal check and the send both run while holding session.mu, the
// same lock End takes to record completion. This makes "is this generation
// still open to send to" one atomic decision shared with End, not two
// independently-timed ones: End can never record completion in the window
// between TryCancel deciding "not yet terminal" and the notification
// actually reaching the wire, so a send can never land after this
// generation - and the identity it was claimed against - is already free
// for reuse. If End wins the race to lock first, TryCancel observes
// terminal=true and returns accepted=false, err=nil without sending
// anything; if TryCancel wins, End simply blocks until the send completes,
// then records the generation's real outcome as usual.
//
// The outbound send is bounded by its own fixed sendTimeout, independent of
// ctx: this keeps a send failure (wrapped in providers.ErrControlSignalFailed
// below, whether caused by a broken connection or this fixed bound firing)
// unambiguously distinct from the caller giving up during the wait phase
// below, which instead surfaces the caller's own unwrapped ctx.Err() --
// deriving the send bound from ctx instead would make both cases produce the
// same context.DeadlineExceeded value and erase that distinction. A send
// failure returns promptly without waiting further: an already-broken
// connection has no reason to ever close session.done on its own.
func (session *Session) TryCancel(ctx context.Context) (accepted bool, err error) {
	session.mu.Lock()
	if session.terminal {
		// End already won ownership of this generation: sending a
		// notification to a dead session would be meaningless, and could
		// never be misread as reaching a replacement generation, since we
		// never touch w.active.
		session.mu.Unlock()
		return false, nil
	}

	sendCtx, cancelSend := context.WithTimeout(context.Background(), sendTimeout)
	sendErr := session.connection.Cancel(sendCtx, acpsdk.CancelNotification{SessionId: session.sessionID})
	cancelSend()
	session.mu.Unlock()

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
