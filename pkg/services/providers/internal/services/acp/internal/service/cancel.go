package service

import (
	"context"
	"fmt"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// acpCancelSendTimeout bounds only the outbound session/cancel notification
// send; it does not bound waiting for the attempt to observe cancellation,
// which is instead bounded by the caller's own ctx (see tryCancel).
const acpCancelSendTimeout = 500 * time.Millisecond

// cancelableSession is the exact live session/prompt turn a session/cancel
// notification can currently target. It exists only for the span between the
// owning daemon.execute call issuing its session/prompt request and that
// call returning. cancelled is set by daemon.execute to the real recorded
// outcome (whether the response's StopReason was truly Cancelled) strictly
// before done is closed, so any tryCancel call already blocked on <-done
// observes it without its own synchronization: this is what lets tryCancel
// ground "accepted" in the attempt's actual terminal behavior instead of
// merely in "a cancel notification was sent while a session looked active".
type cancelableSession struct {
	attemptID  string
	sessionID  acpsdk.SessionId
	connection *acpsdk.ClientSideConnection
	done       chan struct{}
	cancelled  bool
}

// beginCancelable opens the cancelable window for one attempt's in-flight
// session/prompt turn.
func (daemon *daemon) beginCancelable(attemptID string, sessionID acpsdk.SessionId, connection *acpsdk.ClientSideConnection) *cancelableSession {
	session := &cancelableSession{attemptID: attemptID, sessionID: sessionID, connection: connection, done: make(chan struct{})}
	daemon.sessionMu.Lock()
	daemon.active = session
	daemon.sessionMu.Unlock()
	return session
}

// endCancelable closes the cancelable window opened by beginCancelable and
// unblocks any tryCancel call already waiting on it. Safe to call on every
// daemon.execute exit path, including an unexpected terminal unwind. The
// active field is cleared, and the lock released, strictly before done is
// closed: a concurrent tryCancel that observes daemon.active == session is
// therefore guaranteed session.done is not yet closed, and one that observes
// it cleared is guaranteed the attempt's real cancelled outcome (above) is
// already final and safe to read after <-session.done.
func (daemon *daemon) endCancelable(session *cancelableSession) {
	daemon.sessionMu.Lock()
	if daemon.active == session {
		daemon.active = nil
	}
	daemon.sessionMu.Unlock()
	close(session.done)
}

// cancelable reports, without any side effect, whether attemptID names the
// currently open cancelable window. It is a deliberately racy pre-filter;
// see acp.Service.Cancelable.
func (daemon *daemon) cancelable(attemptID string) bool {
	daemon.sessionMu.Lock()
	defer daemon.sessionMu.Unlock()
	return daemon.active != nil && daemon.active.attemptID == attemptID
}

// tryCancel atomically determines whether attemptID names the exact live
// cancelable window and, only if so, delivers a session/cancel notification
// and blocks (bounded by ctx) until the window closes. See acp.Service.
// TryCancel for the accepted/err contract.
//
// The outbound send is bounded by its own fixed acpCancelSendTimeout,
// independent of ctx: this keeps a send failure (wrapped in
// providers.ErrControlSignalFailed below, whether caused by a broken
// connection or this fixed bound firing) unambiguously distinct from the
// caller giving up during the wait phase below, which instead surfaces the
// caller's own unwrapped ctx.Err() -- deriving the send bound from ctx
// instead would make both cases produce the same context.DeadlineExceeded
// value and erase that distinction. A send failure returns promptly without
// waiting further: an already-broken connection has no reason to ever close
// session.done on its own.
func (daemon *daemon) tryCancel(ctx context.Context, attemptID string) (accepted bool, err error) {
	daemon.sessionMu.Lock()
	session := daemon.active
	daemon.sessionMu.Unlock()
	if session == nil || session.attemptID != attemptID {
		return false, nil
	}

	sendCtx, cancelSend := context.WithTimeout(context.Background(), acpCancelSendTimeout)
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

// acpControlCanceledFailure reports the established canceled ExecuteFailure
// for an ACP attempt whose session/prompt turn honored a session/cancel
// notification and returned StopReasonCancelled instead of an RPC error,
// consistent with the ExecuteFailureKindCanceled every other cancellation
// path (native and ACP request-context cancellation) already normalizes to.
func acpControlCanceledFailure(id providers.ID) error {
	return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindCanceled, Message: fmt.Sprintf("ACP provider %q attempt was canceled", id)}
}
