package service

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"testing"

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

func TestDaemonCancelDeliversNotificationAndBlocksUntilWindowCloses(t *testing.T) {
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

	cancelDone := make(chan error, 1)
	go func() {
		cancelDone <- d.cancel(context.Background(), "attempt-1")
	}()

	notification := <-peer.received
	if notification.SessionId != "session-1" {
		t.Fatalf("CancelNotification.SessionId = %q, want session-1", notification.SessionId)
	}

	select {
	case <-cancelDone:
		t.Fatal("cancel() returned before the cancelable window closed")
	default:
	}

	d.endCancelable(session)

	if err := <-cancelDone; err != nil {
		t.Fatalf("cancel() error = %v, want nil", err)
	}
	if d.cancelable("attempt-1") {
		t.Fatal("cancelable(attempt-1) = true after endCancelable, want false")
	}
}

func TestDaemonCancelIsNoOpForMismatchedAttemptID(t *testing.T) {
	peer := newFakeSessionPeer()
	connection := newPipedConnection(t, peer)

	d := &daemon{}
	session := d.beginCancelable("attempt-1", acpsdk.SessionId("session-1"), connection)

	if err := d.cancel(context.Background(), "attempt-other"); err != nil {
		t.Fatalf("cancel(mismatched) error = %v, want nil", err)
	}
	select {
	case notification := <-peer.received:
		t.Fatalf("cancel(mismatched) sent a notification %#v, want none", notification)
	default:
	}

	d.endCancelable(session)
}

func TestDaemonCancelIsNoOpBeforeWindowOpensAndAfterItCloses(t *testing.T) {
	peer := newFakeSessionPeer()
	connection := newPipedConnection(t, peer)
	d := &daemon{}

	if err := d.cancel(context.Background(), "attempt-1"); err != nil {
		t.Fatalf("cancel(before window) error = %v, want nil", err)
	}

	session := d.beginCancelable("attempt-1", acpsdk.SessionId("session-1"), connection)
	d.endCancelable(session)

	if err := d.cancel(context.Background(), "attempt-1"); err != nil {
		t.Fatalf("cancel(after window) error = %v, want nil", err)
	}
	select {
	case notification := <-peer.received:
		t.Fatalf("cancel() outside the window sent a notification %#v, want none", notification)
	default:
	}
}

func TestServiceCancelableAndCancelResolveAliasAndDelegateToDaemon(t *testing.T) {
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

	cancelDone := make(chan error, 1)
	go func() { cancelDone <- svc.Cancel(context.Background(), "custom", "attempt-1") }()
	<-peer.received
	d.endCancelable(session)
	if err := <-cancelDone; err != nil {
		t.Fatalf("Cancel() error = %v, want nil", err)
	}
	if svc.Cancelable("custom", "attempt-1") {
		t.Fatal("Cancelable(alias) = true after the window closed, want false")
	}

	if err := svc.Cancel(context.Background(), "unknown-provider", "attempt-1"); err != nil {
		t.Fatalf("Cancel(unknown provider) error = %v, want nil", err)
	}
}
