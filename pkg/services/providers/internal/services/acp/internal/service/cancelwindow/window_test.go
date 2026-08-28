package cancelwindow

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// noopClient satisfies acpsdk.Client with no behavior; these tests only
// exercise the outbound session/cancel notification path, never an inbound
// client-facing RPC.
type noopClient struct{}

func (noopClient) ReadTextFile(context.Context, acpsdk.ReadTextFileRequest) (acpsdk.ReadTextFileResponse, error) {
	return acpsdk.ReadTextFileResponse{}, nil
}
func (noopClient) WriteTextFile(context.Context, acpsdk.WriteTextFileRequest) (acpsdk.WriteTextFileResponse, error) {
	return acpsdk.WriteTextFileResponse{}, nil
}
func (noopClient) RequestPermission(context.Context, acpsdk.RequestPermissionRequest) (acpsdk.RequestPermissionResponse, error) {
	return acpsdk.RequestPermissionResponse{}, nil
}
func (noopClient) SessionUpdate(context.Context, acpsdk.SessionNotification) error { return nil }
func (noopClient) CreateTerminal(context.Context, acpsdk.CreateTerminalRequest) (acpsdk.CreateTerminalResponse, error) {
	return acpsdk.CreateTerminalResponse{}, nil
}
func (noopClient) KillTerminal(context.Context, acpsdk.KillTerminalRequest) (acpsdk.KillTerminalResponse, error) {
	return acpsdk.KillTerminalResponse{}, nil
}
func (noopClient) TerminalOutput(context.Context, acpsdk.TerminalOutputRequest) (acpsdk.TerminalOutputResponse, error) {
	return acpsdk.TerminalOutputResponse{}, nil
}
func (noopClient) ReleaseTerminal(context.Context, acpsdk.ReleaseTerminalRequest) (acpsdk.ReleaseTerminalResponse, error) {
	return acpsdk.ReleaseTerminalResponse{}, nil
}
func (noopClient) WaitForTerminalExit(context.Context, acpsdk.WaitForTerminalExitRequest) (acpsdk.WaitForTerminalExitResponse, error) {
	return acpsdk.WaitForTerminalExitResponse{}, nil
}

// fakeSessionPeer stands in for a real ACP agent process for cancel-seam
// tests: it reads JSON-RPC lines directly over an in-process io.Pipe (no
// subprocess) and records every session/cancel notification it receives, so
// tests can wait on a real Go channel instead of any sleep-based timing.
type fakeSessionPeer struct {
	received chan acpsdk.CancelNotification
	done     chan struct{}
}

func newFakeSessionPeer() *fakeSessionPeer {
	return &fakeSessionPeer{
		received: make(chan acpsdk.CancelNotification, 8),
		done:     make(chan struct{}),
	}
}

func (peer *fakeSessionPeer) run(from io.Reader) {
	defer close(peer.done)
	scanner := bufio.NewScanner(from)
	for scanner.Scan() {
		var message struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(scanner.Bytes(), &message) != nil {
			continue
		}
		if message.Method != "session/cancel" {
			continue
		}
		var params acpsdk.CancelNotification
		if json.Unmarshal(message.Params, &params) == nil {
			peer.received <- params
		}
	}
}

// newPipedConnection wires a real acpsdk.ClientSideConnection to an
// in-process fake peer over io.Pipe, exercising the real ACP protocol
// encoding/decoding without spawning a subprocess.
func newPipedConnection(t *testing.T, peer *fakeSessionPeer) *acpsdk.ClientSideConnection {
	t.Helper()
	outboundReader, outboundWriter := io.Pipe()
	go peer.run(outboundReader)
	t.Cleanup(func() {
		_ = outboundWriter.Close()
		_ = outboundReader.Close()
		<-peer.done
	})
	return acpsdk.NewClientSideConnection(noopClient{}, outboundWriter, io.MultiReader())
}

func TestWindowTryCancelDeliversNotificationAndBlocksUntilWindowCloses(t *testing.T) {
	peer := newFakeSessionPeer()
	connection := newPipedConnection(t, peer)

	w := &Window{}
	session := w.Begin("attempt-1", acpsdk.SessionId("session-1"), connection)
	if session.sendTimeout != defaultSendTimeout {
		t.Fatalf("zero-value Window session sendTimeout = %s, want %s", session.sendTimeout, defaultSendTimeout)
	}

	if _, ok := w.Claim("attempt-other"); ok {
		t.Fatal("Claim(attempt-other) ok = true, want false for a different attempt id")
	}

	claimed, ok := w.Claim("attempt-1")
	if !ok || claimed != session {
		t.Fatalf("Claim(attempt-1) = (%v, %v), want the open session and true", claimed, ok)
	}

	type outcome struct {
		accepted bool
		err      error
	}
	tryCancelDone := make(chan outcome, 1)
	go func() {
		accepted, err := claimed.TryCancel(context.Background())
		tryCancelDone <- outcome{accepted: accepted, err: err}
	}()

	notification := <-peer.received
	if notification.SessionId != "session-1" {
		t.Fatalf("CancelNotification.SessionId = %q, want session-1", notification.SessionId)
	}

	select {
	case <-tryCancelDone:
		t.Fatal("TryCancel() returned before the cancelable window closed")
	default:
	}

	w.End(session, true)

	result := <-tryCancelDone
	if result.err != nil {
		t.Fatalf("TryCancel() error = %v, want nil", result.err)
	}
	if !result.accepted {
		t.Fatal("TryCancel() accepted = false, want true when the turn's real outcome was cancellation")
	}
	if _, ok := w.Claim("attempt-1"); ok {
		t.Fatal("Claim(attempt-1) ok = true after End, want false")
	}
}

func TestWindowClaimIsNoOpForMismatchedAttemptID(t *testing.T) {
	peer := newFakeSessionPeer()
	connection := newPipedConnection(t, peer)

	w := &Window{}
	session := w.Begin("attempt-1", acpsdk.SessionId("session-1"), connection)

	claimed, ok := w.Claim("attempt-other")
	if ok || claimed != nil {
		t.Fatalf("Claim(mismatched) = (%v, %v), want (nil, false)", claimed, ok)
	}
	select {
	case notification := <-peer.received:
		t.Fatalf("Claim(mismatched) sent a notification %#v, want none", notification)
	default:
	}

	w.End(session, false)
}

func TestWindowClaimIsNoOpBeforeWindowOpensAndAfterItCloses(t *testing.T) {
	peer := newFakeSessionPeer()
	connection := newPipedConnection(t, peer)
	w := &Window{}

	claimed, ok := w.Claim("attempt-1")
	if ok || claimed != nil {
		t.Fatalf("Claim(before window) = (%v, %v), want (nil, false)", claimed, ok)
	}

	session := w.Begin("attempt-1", acpsdk.SessionId("session-1"), connection)
	w.End(session, false)

	claimed, ok = w.Claim("attempt-1")
	if ok || claimed != nil {
		t.Fatalf("Claim(after window) = (%v, %v), want (nil, false)", claimed, ok)
	}
	select {
	case notification := <-peer.received:
		t.Fatalf("Claim() outside the window sent a notification %#v, want none", notification)
	default:
	}
}

// TestWindowClaimPinsExactGenerationSoReusedAttemptIDCannotRedirectDelivery is
// the deterministic ABA regression: claim generation A's session, let A
// complete and fully close its window, open generation B with the identical
// attemptID, then resume A's already-captured claim. Because TryCancel
// operates only on the *Session pinned by Claim (never re-resolving via
// Window.active/attemptID), A's stale delivery must report accepted=false,
// err=nil and send nothing to B - B must receive zero notifications and stay
// independently cancelable.
func TestWindowClaimPinsExactGenerationSoReusedAttemptIDCannotRedirectDelivery(t *testing.T) {
	peer := newFakeSessionPeer()
	connection := newPipedConnection(t, peer)
	w := &Window{}

	sessionA := w.Begin("attempt-1", acpsdk.SessionId("session-A"), connection)
	claimA, ok := w.Claim("attempt-1")
	if !ok || claimA != sessionA {
		t.Fatalf("Claim(A) = (%v, %v), want (sessionA, true)", claimA, ok)
	}

	// A completes normally (not via cancellation) and its window fully closes.
	w.End(sessionA, false)
	if _, ok := w.Claim("attempt-1"); ok {
		t.Fatal("Claim(attempt-1) ok = true after A's End, want false: identity must be free for reuse")
	}

	// B opens, reusing the exact same attemptID string.
	sessionB := w.Begin("attempt-1", acpsdk.SessionId("session-B"), connection)
	t.Cleanup(func() { w.End(sessionB, false) })

	// A's delayed signal now resumes against its already-captured claim.
	accepted, err := claimA.TryCancel(context.Background())
	if err != nil {
		t.Fatalf("claimA.TryCancel() error = %v, want nil", err)
	}
	if accepted {
		t.Fatal("claimA.TryCancel() accepted = true, want false: A already ended, must not reach B")
	}
	select {
	case notification := <-peer.received:
		t.Fatalf("claimA.TryCancel() sent a notification %#v, want none delivered to B", notification)
	default:
	}

	// B must remain independently, correctly claimable/cancelable.
	claimB, ok := w.Claim("attempt-1")
	if !ok || claimB != sessionB {
		t.Fatalf("Claim(B) = (%v, %v), want (sessionB, true)", claimB, ok)
	}
}

// gatedWriter blocks the first Write call until release is closed, letting a
// test pause a TryCancel goroutine at the exact instant it is inside its
// outbound send - after it has already decided the generation was not yet
// terminal - so a concurrent End can be attempted against that same window.
// entered is closed once the blocked Write is reached, giving the test a
// real synchronization point instead of a sleep.
type gatedWriter struct {
	w           io.Writer
	entered     chan struct{}
	release     chan struct{}
	once        sync.Once
	releaseOnce sync.Once
}

func (g *gatedWriter) Write(p []byte) (int, error) {
	g.once.Do(func() { close(g.entered) })
	<-g.release
	return g.w.Write(p)
}

func (g *gatedWriter) releaseWrite() {
	g.releaseOnce.Do(func() { close(g.release) })
}

func stopAndDrainTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

// waitForSignal bounds only a failed witness's diagnostic path; successful
// ordering is driven by the supplied channel rather than by the timeout.
func waitForSignal(t *testing.T, signal <-chan struct{}, timeout time.Duration, message string) {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer stopAndDrainTimer(timer)
	select {
	case <-signal:
	case <-timer.C:
		t.Fatal(message)
	}
}

// TestWindowEndReachesTerminalResultAndReleasesIdentityWhileATryCancelSendIsStuckPastTheSendBound
// is the deterministic regression for the blocked-write liveness bug: an
// earlier fix held session.mu across the outbound send itself, so a peer
// that stopped reading mid-write (the acp-go-sdk connection does not honor
// ctx once inside its synchronous io.Writer.Write call) left that mutex held
// forever, and End - which must acquire the same mutex to record completion
// - blocked indefinitely. That meant Window's active slot, the identity
// reservation, and everything upstream that waits on daemon.execute
// returning could hang forever on a single unresponsive peer. This test
// pins TryCancel deep inside a send that is held open well past sendTimeout
// (via gatedWriter, a real blocking primitive, not a sleep) and proves End
// still reaches a defined terminal result and fully releases the window
// promptly - including letting a same-identity Begin proceed immediately -
// without ever waiting on that stuck send. It also proves that once the
// stuck send is finally released, its late notification reaches only its
// own session ID and never the replacement generation that reused the
// identity in the meantime.
func TestWindowEndReachesTerminalResultAndReleasesIdentityWhileATryCancelSendIsStuckPastTheSendBound(t *testing.T) {
	const testSendTimeout = 10 * time.Millisecond

	peer := newFakeSessionPeer()
	outboundReader, outboundWriter := io.Pipe()
	gate := &gatedWriter{w: outboundWriter, entered: make(chan struct{}), release: make(chan struct{})}
	go peer.run(outboundReader)
	connection := acpsdk.NewClientSideConnection(noopClient{}, gate, io.MultiReader())

	w := &Window{sendTimeout: testSendTimeout}
	sessionA := w.Begin("attempt-1", acpsdk.SessionId("session-A"), connection)
	type outcome struct {
		accepted bool
		err      error
	}
	tryCancelDone := make(chan outcome, 1)
	var tryCancelResult outcome
	var receiveTryCancelOnce sync.Once
	waitForTryCancel := func() outcome {
		receiveTryCancelOnce.Do(func() { tryCancelResult = <-tryCancelDone })
		return tryCancelResult
	}

	var endAOnce sync.Once
	endA := func() { endAOnce.Do(func() { w.End(sessionA, false) }) }
	var sessionB *Session
	var endBOnce sync.Once
	endB := func() {
		if sessionB != nil {
			endBOnce.Do(func() { w.End(sessionB, false) })
		}
	}
	var endDone chan struct{}
	tryCancelStarted := false
	t.Cleanup(func() {
		// Release the real blocked Write before joining TryCancel. End must
		// also run on every failure path so the waiting call can observe its
		// terminal outcome and all goroutines can finish before the pipes close.
		gate.releaseWrite()
		endA()
		endB()
		if endDone != nil {
			<-endDone
		}
		if tryCancelStarted {
			_ = waitForTryCancel()
		}
		_ = outboundWriter.Close()
		_ = outboundReader.Close()
		<-peer.done
	})
	if sessionA.sendTimeout != testSendTimeout {
		t.Fatalf("test Window session sendTimeout = %s, want %s", sessionA.sendTimeout, testSendTimeout)
	}

	claimedA, ok := w.Claim("attempt-1")
	if !ok || claimedA != sessionA {
		t.Fatalf("Claim(attempt-1) = (%v, %v), want the open session and true", claimedA, ok)
	}

	tryCancelStarted = true
	go func() {
		accepted, err := claimedA.TryCancel(context.Background())
		tryCancelDone <- outcome{accepted: accepted, err: err}
	}()

	// TryCancel has won ownership of A and is now blocked mid-send, well past
	// what the session-local send bound would limit if the SDK honored it.
	waitForSignal(t, gate.entered, 2*time.Second, "TryCancel() did not enter the gated real SDK write")
	boundTimer := time.NewTimer(2 * sessionA.sendTimeout)
	t.Cleanup(func() { stopAndDrainTimer(boundTimer) })
	// This timer starts only after writer entry, making its channel an
	// explicit post-entry elapsed signal instead of a scheduling sleep.
	<-boundTimer.C

	endDone = make(chan struct{})
	go func() {
		// A's real Execute() outcome was not a cancellation - the daemon's
		// own response raced ahead of the (still in-flight) cancel send.
		endA()
		close(endDone)
	}()

	waitForSignal(t, endDone, 2*time.Second, "End() did not reach a terminal result while TryCancel's send was stuck; a blocked peer write must never block End")

	// The identity must be free for reuse immediately - not just eventually.
	sessionB = w.Begin("attempt-1", acpsdk.SessionId("session-B"), connection)
	if sessionB.sendTimeout != testSendTimeout {
		t.Fatalf("replacement session sendTimeout = %s, want %s", sessionB.sendTimeout, testSendTimeout)
	}
	claimedB, ok := w.Claim("attempt-1")
	if !ok || claimedB != sessionB {
		t.Fatalf("Claim(attempt-1) after A's End = (%v, %v), want the fresh session B and true", claimedB, ok)
	}
	endB()

	// Now let A's long-stuck send finally land.
	gate.releaseWrite()

	result := waitForTryCancel()
	if result.err != nil {
		t.Fatalf("TryCancel() error = %v, want nil", result.err)
	}
	if result.accepted {
		t.Fatal("TryCancel() accepted = true, want false: End already recorded A's outcome before this call could observe cancellation")
	}

	notification := <-peer.received
	if notification.SessionId != "session-A" {
		t.Fatalf("late notification SessionId = %q, want session-A (A's own identity), never B's", notification.SessionId)
	}
	select {
	case extra := <-peer.received:
		t.Fatalf("received an unexpected second notification %#v, want exactly one (A's stale send, never one addressed to B)", extra)
	default:
	}
}

// TestWindowTryCancelLosesRaceToNaturalCompletionReportsUnsupportedNotCompleted
// is the deterministic regression for the TOCTOU the original Cancelable/
// Cancel split had: a TryCancel call can still observe the window open
// (active matches) and send its notification, but by the time the turn's
// real outcome is recorded the turn had already decided to finish normally
// (not via this cancellation) a moment before the notification arrived.
// TryCancel must ground "accepted" in that real recorded outcome, not in "a
// matching identity was observed a moment earlier", so it reports
// accepted=false here even though the identity check passed and the
// notification was sent. The pause point (between TryCancel observing the
// window and the turn recording its real outcome) is a real Go channel, not
// a sleep, so this interleaving is exercised on every run.
func TestWindowTryCancelLosesRaceToNaturalCompletionReportsUnsupportedNotCompleted(t *testing.T) {
	peer := newFakeSessionPeer()
	connection := newPipedConnection(t, peer)

	w := &Window{}
	session := w.Begin("attempt-1", acpsdk.SessionId("session-1"), connection)
	claimed, ok := w.Claim("attempt-1")
	if !ok {
		t.Fatal("Claim(attempt-1) ok = false, want true")
	}

	type outcome struct {
		accepted bool
		err      error
	}
	tryCancelDone := make(chan outcome, 1)
	go func() {
		accepted, err := claimed.TryCancel(context.Background())
		tryCancelDone <- outcome{accepted: accepted, err: err}
	}()

	// Wait for TryCancel to have genuinely observed the window (its
	// notification is in flight) before the turn is recorded as finishing
	// normally -- proving this is a true race, not a sequencing assumption.
	<-peer.received

	// The turn's own goroutine now decides its real outcome was NOT
	// cancellation (for example: the peer's response already arrived over
	// the wire before it processed our notification) and closes the window
	// accordingly.
	w.End(session, false)

	result := <-tryCancelDone
	if result.err != nil {
		t.Fatalf("TryCancel() error = %v, want nil", result.err)
	}
	if result.accepted {
		t.Fatal("TryCancel() accepted = true, want false: the turn's real outcome was not cancellation")
	}
}

func TestWindowTryCancelReturnsPromptlyWhenCallerContextEndsWhileWaiting(t *testing.T) {
	peer := newFakeSessionPeer()
	connection := newPipedConnection(t, peer)

	w := &Window{}
	session := w.Begin("attempt-1", acpsdk.SessionId("session-1"), connection)
	t.Cleanup(func() { w.End(session, false) })
	claimed, ok := w.Claim("attempt-1")
	if !ok {
		t.Fatal("Claim(attempt-1) ok = false, want true")
	}

	ctx, cancel := context.WithCancel(context.Background())
	type outcome struct {
		accepted bool
		err      error
	}
	tryCancelDone := make(chan outcome, 1)
	go func() {
		accepted, err := claimed.TryCancel(ctx)
		tryCancelDone <- outcome{accepted: accepted, err: err}
	}()

	// The notification was sent (the send itself is not what the caller is
	// giving up on); the turn just never returns on its own within this
	// test, modeling a stuck peer.
	<-peer.received

	select {
	case <-tryCancelDone:
		t.Fatal("TryCancel() returned before the caller context ended")
	default:
	}

	cancel()

	result := <-tryCancelDone
	if result.accepted {
		t.Fatal("TryCancel() accepted = true, want false when the caller gave up waiting")
	}
	if !errors.Is(result.err, context.Canceled) {
		t.Fatalf("TryCancel() error = %v, want errors.Is context.Canceled", result.err)
	}
}

// TestWindowTryCancelReturnsSendFailurePromptlyWithoutWaitingForDoneToClose
// proves the send-failure short-circuit: the outbound pipe is broken before
// TryCancel ever runs (modeling a dead ACP connection), so the notification
// send itself fails, and the turn's done channel is deliberately never
// closed by this test. A caller ctx with no deadline is used so the only way
// this test can pass is if TryCancel returns as soon as the send fails
// instead of also waiting unconditionally on <-session.done; if it did wait,
// this test would hang until its own timeout below fires.
func TestWindowTryCancelReturnsSendFailurePromptlyWithoutWaitingForDoneToClose(t *testing.T) {
	outboundReader, outboundWriter := io.Pipe()
	_ = outboundReader.Close()
	connection := acpsdk.NewClientSideConnection(noopClient{}, outboundWriter, io.MultiReader())

	w := &Window{}
	w.Begin("attempt-1", acpsdk.SessionId("session-1"), connection)
	claimed, ok := w.Claim("attempt-1")
	if !ok {
		t.Fatal("Claim(attempt-1) ok = false, want true")
	}

	done := make(chan struct {
		accepted bool
		err      error
	}, 1)
	go func() {
		accepted, err := claimed.TryCancel(context.Background())
		done <- struct {
			accepted bool
			err      error
		}{accepted, err}
	}()

	select {
	case result := <-done:
		if result.accepted {
			t.Fatal("TryCancel() accepted = true, want false alongside a send failure")
		}
		if result.err == nil {
			t.Fatal("TryCancel() error = nil, want a non-nil send failure")
		}
		if !errors.Is(result.err, providers.ErrControlSignalFailed) {
			t.Fatalf("TryCancel() error = %v, want errors.Is ErrControlSignalFailed", result.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("TryCancel() hung instead of returning promptly after the send failed")
	}
}
