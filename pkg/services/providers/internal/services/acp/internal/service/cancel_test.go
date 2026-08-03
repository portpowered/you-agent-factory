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

// TestServiceCancelableAndTryCancelResolveAliasAndDelegateToDaemon proves the
// Service-level Cancelable/TryCancel wrappers resolve aliases and unknown
// providers correctly and delegate to the exact resolved daemon's
// cancelwindow.Window. The window's own accept/reject/race semantics are
// covered directly and exhaustively by the cancelwindow package tests; this
// file only proves the resolution/delegation wiring on top of it.
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
	session := d.window.Begin("attempt-1", acpsdk.SessionId("session-1"), connection)

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
	d.window.End(session, true)
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
