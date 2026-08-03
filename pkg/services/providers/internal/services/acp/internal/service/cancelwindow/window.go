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
type Session struct {
	attemptID  string
	sessionID  acpsdk.SessionId
	connection *acpsdk.ClientSideConnection
	done       chan struct{}
	cancelled  bool
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
// unwind. The active field is cleared, and the lock released, strictly
// before done is closed: a concurrent TryCancel that observes the window
// still open is therefore guaranteed session.done is not yet closed, and one
// that observes it cleared is guaranteed the attempt's real cancelled
// outcome is already final and safe to read after <-session.done.
func (w *Window) End(session *Session, cancelled bool) {
	session.cancelled = cancelled
	w.mu.Lock()
	if w.active == session {
		w.active = nil
	}
	w.mu.Unlock()
	close(session.done)
}

// Live reports, without any side effect, whether attemptID names the
// currently open cancelable window. It is a deliberately racy pre-filter;
// see acp.Service.Cancelable.
func (w *Window) Live(attemptID string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.active != nil && w.active.attemptID == attemptID
}

// TryCancel atomically determines whether attemptID names the exact live
// cancelable window and, only if so, delivers a session/cancel notification
// and blocks (bounded by ctx) until the window closes. See acp.Service.
// TryCancel for the accepted/err contract.
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
func (w *Window) TryCancel(ctx context.Context, attemptID string) (accepted bool, err error) {
	w.mu.Lock()
	session := w.active
	w.mu.Unlock()
	if session == nil || session.attemptID != attemptID {
		return false, nil
	}

	sendCtx, cancelSend := context.WithTimeout(context.Background(), sendTimeout)
	defer cancelSend()
	if err := session.connection.Cancel(sendCtx, acpsdk.CancelNotification{SessionId: session.sessionID}); err != nil {
		return false, fmt.Errorf("%w: deliver session/cancel: %v", providers.ErrControlSignalFailed, err)
	}

	select {
	case <-session.done:
		return session.cancelled, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}
