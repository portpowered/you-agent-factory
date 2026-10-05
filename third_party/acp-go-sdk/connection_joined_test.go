package acp

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
)

func TestJoinedWaitsForAdmittedHandlers(t *testing.T) {
	for _, method := range []string{"notification", "request"} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			reader := newScriptedPeerReader()
			entered, release := make(chan struct{}), make(chan struct{})
			t.Cleanup(reader.closeWithEOF)
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			t.Cleanup(unblock)
			conn := NewConnection(func(context.Context, string, json.RawMessage) (any, *RequestError) {
				close(entered)
				<-release
				return nil, nil
			}, io.Discard, reader)
			id := ""
			if method == "request" {
				id = `,"id":1`
			}
			reader.deliver(`{"jsonrpc":"2.0","method":"test"` + id + `}` + "\n")
			awaitSignal(t, entered, "handler admission")
			reader.closeWithEOF()
			awaitSignal(t, conn.Done(), "disconnect")
			requireNotJoined(t, conn)
			// Drain timeout/cancellation is not callback completion either.
			conn.inboundCancel(io.EOF)
			requireNotJoined(t, conn)
			unblock()
			awaitSignal(t, conn.Joined(), "handler and shutdown completion")
		})
	}
}

func TestJoinedWaitsForReaderAfterCancellation(t *testing.T) {
	t.Parallel()
	reader := newScriptedPeerReader()
	t.Cleanup(reader.closeWithEOF)
	conn := NewConnection(nil, io.Discard, reader)
	conn.cancel(context.Canceled)
	awaitSignal(t, conn.Done(), "cancellation")
	requireNotJoined(t, conn)
	reader.closeWithEOF()
	awaitSignal(t, conn.Joined(), "reader completion")
}

type heldCancelWriter struct{ entered, release chan struct{} }

func (w *heldCancelWriter) Write(p []byte) (int, error) {
	close(w.entered)
	<-w.release
	return len(p), nil
}

func TestJoinedWaitsForCancellationWriter(t *testing.T) {
	t.Parallel()
	reader := newScriptedPeerReader()
	writer := &heldCancelWriter{make(chan struct{}), make(chan struct{})}
	t.Cleanup(reader.closeWithEOF)
	conn := NewConnection(nil, writer, reader)
	conn.sendCancelRequest("1")
	awaitSignal(t, writer.entered, "cancel write")
	reader.closeWithEOF()
	awaitSignal(t, conn.Done(), "disconnect")
	requireNotJoined(t, conn)
	close(writer.release)
	awaitSignal(t, conn.Joined(), "cancel writer completion")
}

func requireNotJoined(t *testing.T, conn *Connection) {
	t.Helper()
	select {
	case <-conn.Joined():
		t.Fatal("Joined closed with owned work outstanding")
	default:
	}
}
