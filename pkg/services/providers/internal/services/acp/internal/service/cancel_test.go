package service

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
	return acpsdk.NewClientSideConnection(&client{}, outboundWriter, io.MultiReader())
}

// endTurn marks session's turn finished with the given real recorded
// outcome, mirroring what daemon.execute does right before calling
// endCancelable: set cancelled, then close the window.
func endTurn(d *daemon, session *cancelableSession, cancelled bool) {
	session.cancelled = cancelled
	d.endCancelable(session)
}

func TestDaemonTryCancelDeliversNotificationAndBlocksUntilWindowCloses(t *testing.T) {
	peer := newFakeSessionPeer()
	connection := newPipedConnection(t, peer)

	d := &daemon{}
	session := d.beginCancelable("attempt-1", acpsdk.SessionId("session-1"), connection)

	if !d.cancelable("attempt-1") {
		t.Fatal("cancelable(attempt-1) = false, want true once beginCancelable has run")
	}
	if d.cancelable("attempt-other") {
		t.Fatal("cancelable(attempt-other) = true, want false for a different attempt id")
	}

	type outcome struct {
		accepted bool
		err      error
	}
	tryCancelDone := make(chan outcome, 1)
	go func() {
		accepted, err := d.tryCancel(context.Background(), "attempt-1")
		tryCancelDone <- outcome{accepted: accepted, err: err}
	}()

	notification := <-peer.received
	if notification.SessionId != "session-1" {
		t.Fatalf("CancelNotification.SessionId = %q, want session-1", notification.SessionId)
	}

	select {
	case <-tryCancelDone:
		t.Fatal("tryCancel() returned before the cancelable window closed")
	default:
	}

	endTurn(d, session, true)

	result := <-tryCancelDone
	if result.err != nil {
		t.Fatalf("tryCancel() error = %v, want nil", result.err)
	}
	if !result.accepted {
		t.Fatal("tryCancel() accepted = false, want true when the turn's real outcome was cancellation")
	}
	if d.cancelable("attempt-1") {
		t.Fatal("cancelable(attempt-1) = true after endCancelable, want false")
	}
}

func TestDaemonTryCancelIsNoOpForMismatchedAttemptID(t *testing.T) {
	peer := newFakeSessionPeer()
	connection := newPipedConnection(t, peer)

	d := &daemon{}
	session := d.beginCancelable("attempt-1", acpsdk.SessionId("session-1"), connection)

	accepted, err := d.tryCancel(context.Background(), "attempt-other")
	if err != nil {
		t.Fatalf("tryCancel(mismatched) error = %v, want nil", err)
	}
	if accepted {
		t.Fatal("tryCancel(mismatched) accepted = true, want false")
	}
	select {
	case notification := <-peer.received:
		t.Fatalf("tryCancel(mismatched) sent a notification %#v, want none", notification)
	default:
	}

	endTurn(d, session, false)
}

func TestDaemonTryCancelIsNoOpBeforeWindowOpensAndAfterItCloses(t *testing.T) {
	peer := newFakeSessionPeer()
	connection := newPipedConnection(t, peer)
	d := &daemon{}

	accepted, err := d.tryCancel(context.Background(), "attempt-1")
	if err != nil {
		t.Fatalf("tryCancel(before window) error = %v, want nil", err)
	}
	if accepted {
		t.Fatal("tryCancel(before window) accepted = true, want false")
	}

	session := d.beginCancelable("attempt-1", acpsdk.SessionId("session-1"), connection)
	endTurn(d, session, false)

	accepted, err = d.tryCancel(context.Background(), "attempt-1")
	if err != nil {
		t.Fatalf("tryCancel(after window) error = %v, want nil", err)
	}
	if accepted {
		t.Fatal("tryCancel(after window) accepted = true, want false")
	}
	select {
	case notification := <-peer.received:
		t.Fatalf("tryCancel() outside the window sent a notification %#v, want none", notification)
	default:
	}
}

// TestDaemonTryCancelLosesRaceToNaturalCompletionReportsUnsupportedNotCompleted
// is the deterministic regression for the TOCTOU this file's Cancelable/
// Cancel split previously had: a tryCancel call can still observe the window
// open (daemon.active matches) and send its notification, but by the time
// the turn's real outcome is recorded the turn had already decided to finish
// normally (not via this cancellation) a moment before the notification
// arrived. tryCancel must ground "accepted" in that real recorded outcome,
// not in "a matching identity was observed a moment earlier", so it reports
// accepted=false here even though the identity check passed and the
// notification was sent. The pause point (between tryCancel observing the
// window and the turn recording its real outcome) is a real Go channel, not
// a sleep, so this interleaving is exercised on every run.
func TestDaemonTryCancelLosesRaceToNaturalCompletionReportsUnsupportedNotCompleted(t *testing.T) {
	peer := newFakeSessionPeer()
	connection := newPipedConnection(t, peer)

	d := &daemon{}
	session := d.beginCancelable("attempt-1", acpsdk.SessionId("session-1"), connection)

	type outcome struct {
		accepted bool
		err      error
	}
	tryCancelDone := make(chan outcome, 1)
	go func() {
		accepted, err := d.tryCancel(context.Background(), "attempt-1")
		tryCancelDone <- outcome{accepted: accepted, err: err}
	}()

	// Wait for tryCancel to have genuinely observed the window (its
	// notification is in flight) before the turn is recorded as finishing
	// normally -- proving this is a true race, not a sequencing assumption.
	<-peer.received

	// The turn's own goroutine now decides its real outcome was NOT
	// cancellation (for example: the peer's response already arrived over
	// the wire before it processed our notification) and closes the window
	// accordingly.
	endTurn(d, session, false)

	result := <-tryCancelDone
	if result.err != nil {
		t.Fatalf("tryCancel() error = %v, want nil", result.err)
	}
	if result.accepted {
		t.Fatal("tryCancel() accepted = true, want false: the turn's real outcome was not cancellation")
	}
}

func TestDaemonTryCancelReturnsPromptlyWhenCallerContextEndsWhileWaiting(t *testing.T) {
	peer := newFakeSessionPeer()
	connection := newPipedConnection(t, peer)

	d := &daemon{}
	session := d.beginCancelable("attempt-1", acpsdk.SessionId("session-1"), connection)
	t.Cleanup(func() { endTurn(d, session, false) })

	ctx, cancel := context.WithCancel(context.Background())
	type outcome struct {
		accepted bool
		err      error
	}
	tryCancelDone := make(chan outcome, 1)
	go func() {
		accepted, err := d.tryCancel(ctx, "attempt-1")
		tryCancelDone <- outcome{accepted: accepted, err: err}
	}()

	// The notification was sent (the send itself is not what the caller is
	// giving up on); the turn just never returns on its own within this
	// test, modeling a stuck peer.
	<-peer.received

	select {
	case <-tryCancelDone:
		t.Fatal("tryCancel() returned before the caller context ended")
	default:
	}

	cancel()

	result := <-tryCancelDone
	if result.accepted {
		t.Fatal("tryCancel() accepted = true, want false when the caller gave up waiting")
	}
	if !errors.Is(result.err, context.Canceled) {
		t.Fatalf("tryCancel() error = %v, want errors.Is context.Canceled", result.err)
	}
}

// TestDaemonTryCancelReturnsSendFailurePromptlyWithoutWaitingForDoneToClose
// proves the send-failure short-circuit: the outbound pipe is broken before
// tryCancel ever runs (modeling a dead ACP connection), so the notification
// send itself fails, and the turn's done channel is deliberately never
// closed by this test. A caller ctx with no deadline is used so the only way
// this test can pass is if tryCancel returns as soon as the send fails
// instead of also waiting unconditionally on <-session.done; if it did wait,
// this test would hang until its own timeout below fires.
func TestDaemonTryCancelReturnsSendFailurePromptlyWithoutWaitingForDoneToClose(t *testing.T) {
	outboundReader, outboundWriter := io.Pipe()
	_ = outboundReader.Close()
	connection := acpsdk.NewClientSideConnection(&client{}, outboundWriter, io.MultiReader())

	d := &daemon{}
	d.beginCancelable("attempt-1", acpsdk.SessionId("session-1"), connection)

	done := make(chan struct {
		accepted bool
		err      error
	}, 1)
	go func() {
		accepted, err := d.tryCancel(context.Background(), "attempt-1")
		done <- struct {
			accepted bool
			err      error
		}{accepted, err}
	}()

	select {
	case result := <-done:
		if result.accepted {
			t.Fatal("tryCancel() accepted = true, want false alongside a send failure")
		}
		if result.err == nil {
			t.Fatal("tryCancel() error = nil, want a non-nil send failure")
		}
		if !errors.Is(result.err, providers.ErrControlSignalFailed) {
			t.Fatalf("tryCancel() error = %v, want errors.Is ErrControlSignalFailed", result.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("tryCancel() hung instead of returning promptly after the send failed")
	}
}

func TestServiceCancelableAndTryCancelResolveAliasAndDelegateToDaemon(t *testing.T) {
	serviceValue, err := New([]providers.ACPIntegration{{
		ID: "entry-1", Name: "custom-acp", Aliases: []string{"custom"}, Transport: "stdio", Command: "agent acp",
	}}, nil, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	svc := serviceValue.(*Service)
	d := svc.daemons["custom-acp"]

	peer := newFakeSessionPeer()
	connection := newPipedConnection(t, peer)
	session := d.beginCancelable("attempt-1", acpsdk.SessionId("session-1"), connection)

	if !svc.Cancelable("custom", "attempt-1") {
		t.Fatal("Cancelable(alias) = false, want true")
	}
	if svc.Cancelable("custom", "attempt-other") {
		t.Fatal("Cancelable(alias, wrong attempt) = true, want false")
	}
	if svc.Cancelable("unknown-provider", "attempt-1") {
		t.Fatal("Cancelable(unknown provider) = true, want false")
	}

	type outcome struct {
		accepted bool
		err      error
	}
	tryCancelDone := make(chan outcome, 1)
	go func() {
		accepted, err := svc.TryCancel(context.Background(), "custom", "attempt-1")
		tryCancelDone <- outcome{accepted: accepted, err: err}
	}()
	<-peer.received
	endTurn(d, session, true)
	result := <-tryCancelDone
	if result.err != nil {
		t.Fatalf("TryCancel() error = %v, want nil", result.err)
	}
	if !result.accepted {
		t.Fatal("TryCancel() accepted = false, want true")
	}
	if svc.Cancelable("custom", "attempt-1") {
		t.Fatal("Cancelable(alias) = true after the window closed, want false")
	}

	accepted, err := svc.TryCancel(context.Background(), "unknown-provider", "attempt-1")
	if err != nil {
		t.Fatalf("TryCancel(unknown provider) error = %v, want nil", err)
	}
	if accepted {
		t.Fatal("TryCancel(unknown provider) accepted = true, want false")
	}
}
