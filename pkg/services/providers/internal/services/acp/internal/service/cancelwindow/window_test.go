package cancelwindow

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
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
}

func newFakeSessionPeer() *fakeSessionPeer {
	return &fakeSessionPeer{received: make(chan acpsdk.CancelNotification, 8)}
}

func (peer *fakeSessionPeer) run(from io.Reader) {
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
	t.Cleanup(func() { _ = outboundWriter.Close() })
	return acpsdk.NewClientSideConnection(noopClient{}, outboundWriter, io.MultiReader())
}

func TestWindowTryCancelDeliversNotificationAndBlocksUntilWindowCloses(t *testing.T) {
	peer := newFakeSessionPeer()
	connection := newPipedConnection(t, peer)

	w := &Window{}
	session := w.Begin("attempt-1", acpsdk.SessionId("session-1"), connection)

	if !w.Live("attempt-1") {
		t.Fatal("Live(attempt-1) = false, want true once Begin has run")
	}
	if w.Live("attempt-other") {
		t.Fatal("Live(attempt-other) = true, want false for a different attempt id")
	}

	type outcome struct {
		accepted bool
		err      error
	}
	tryCancelDone := make(chan outcome, 1)
	go func() {
		accepted, err := w.TryCancel(context.Background(), "attempt-1")
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
	if w.Live("attempt-1") {
		t.Fatal("Live(attempt-1) = true after End, want false")
	}
}

func TestWindowTryCancelIsNoOpForMismatchedAttemptID(t *testing.T) {
	peer := newFakeSessionPeer()
	connection := newPipedConnection(t, peer)

	w := &Window{}
	session := w.Begin("attempt-1", acpsdk.SessionId("session-1"), connection)

	accepted, err := w.TryCancel(context.Background(), "attempt-other")
	if err != nil {
		t.Fatalf("TryCancel(mismatched) error = %v, want nil", err)
	}
	if accepted {
		t.Fatal("TryCancel(mismatched) accepted = true, want false")
	}
	select {
	case notification := <-peer.received:
		t.Fatalf("TryCancel(mismatched) sent a notification %#v, want none", notification)
	default:
	}

	w.End(session, false)
}

func TestWindowTryCancelIsNoOpBeforeWindowOpensAndAfterItCloses(t *testing.T) {
	peer := newFakeSessionPeer()
	connection := newPipedConnection(t, peer)
	w := &Window{}

	accepted, err := w.TryCancel(context.Background(), "attempt-1")
	if err != nil {
		t.Fatalf("TryCancel(before window) error = %v, want nil", err)
	}
	if accepted {
		t.Fatal("TryCancel(before window) accepted = true, want false")
	}

	session := w.Begin("attempt-1", acpsdk.SessionId("session-1"), connection)
	w.End(session, false)

	accepted, err = w.TryCancel(context.Background(), "attempt-1")
	if err != nil {
		t.Fatalf("TryCancel(after window) error = %v, want nil", err)
	}
	if accepted {
		t.Fatal("TryCancel(after window) accepted = true, want false")
	}
	select {
	case notification := <-peer.received:
		t.Fatalf("TryCancel() outside the window sent a notification %#v, want none", notification)
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

	type outcome struct {
		accepted bool
		err      error
	}
	tryCancelDone := make(chan outcome, 1)
	go func() {
		accepted, err := w.TryCancel(context.Background(), "attempt-1")
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

	ctx, cancel := context.WithCancel(context.Background())
	type outcome struct {
		accepted bool
		err      error
	}
	tryCancelDone := make(chan outcome, 1)
	go func() {
		accepted, err := w.TryCancel(ctx, "attempt-1")
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

	done := make(chan struct {
		accepted bool
		err      error
	}, 1)
	go func() {
		accepted, err := w.TryCancel(context.Background(), "attempt-1")
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
