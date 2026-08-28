package cancelwindow

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
)

const cancelWindowCleanupTimeout = 2 * time.Second

type cancelOutcome struct {
	accepted bool
	err      error
}

type cancelAttempt struct {
	done   chan struct{}
	result cancelOutcome
}

type blockedSendFixture struct {
	peer           *fakeSessionPeer
	outboundReader *io.PipeReader
	outboundWriter *io.PipeWriter
	gate           *gatedWriter
	connection     *acpsdk.ClientSideConnection
	window         *Window
	sessionA       *Session
	sessionB       *Session
	tryCancel      *cancelAttempt
	endADone       chan struct{}
	releaseOnce    sync.Once
	endAOnce       sync.Once
	endBOnce       sync.Once
}

func newBlockedSendFixture(t *testing.T, window *Window) *blockedSendFixture {
	t.Helper()
	peer := newFakeSessionPeer()
	outboundReader, outboundWriter := io.Pipe()
	gate := &gatedWriter{
		w:       outboundWriter,
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	go peer.run(outboundReader)

	fixture := &blockedSendFixture{
		peer:           peer,
		outboundReader: outboundReader,
		outboundWriter: outboundWriter,
		gate:           gate,
		connection:     acpsdk.NewClientSideConnection(noopClient{}, gate, io.MultiReader()),
		window:         window,
	}
	t.Cleanup(func() { fixture.cleanup(t) })
	return fixture
}

func (fixture *blockedSendFixture) beginA(attemptID string, sessionID acpsdk.SessionId) *Session {
	fixture.sessionA = fixture.window.Begin(attemptID, sessionID, fixture.connection)
	return fixture.sessionA
}

func (fixture *blockedSendFixture) beginB(attemptID string, sessionID acpsdk.SessionId) *Session {
	fixture.sessionB = fixture.window.Begin(attemptID, sessionID, fixture.connection)
	return fixture.sessionB
}

func (fixture *blockedSendFixture) startTryCancel(claimed *Session) {
	fixture.tryCancel = &cancelAttempt{done: make(chan struct{})}
	attempt := fixture.tryCancel
	go func() {
		attempt.result.accepted, attempt.result.err = claimed.TryCancel(context.Background())
		close(attempt.done)
	}()
}

func (fixture *blockedSendFixture) waitForTryCancel(t *testing.T) cancelOutcome {
	t.Helper()
	if fixture.tryCancel == nil {
		t.Fatal("TryCancel() was not started")
	}
	<-fixture.tryCancel.done
	return fixture.tryCancel.result
}

func (fixture *blockedSendFixture) endA() <-chan struct{} {
	if fixture.sessionA == nil {
		return nil
	}
	fixture.endAOnce.Do(func() {
		fixture.endADone = make(chan struct{})
		go func() {
			fixture.window.End(fixture.sessionA, false)
			close(fixture.endADone)
		}()
	})
	return fixture.endADone
}

func (fixture *blockedSendFixture) endB() {
	if fixture.sessionB == nil {
		return
	}
	fixture.endBOnce.Do(func() { fixture.window.End(fixture.sessionB, false) })
}

func (fixture *blockedSendFixture) releaseSend() {
	fixture.releaseOnce.Do(func() { close(fixture.gate.release) })
}

func (fixture *blockedSendFixture) waitPastSendBound(t *testing.T, sendBound time.Duration) {
	waitForCancelWindowSignal(
		t,
		fixture.gate.entered,
		cancelWindowCleanupTimeout,
		"TryCancel() did not enter the gated real SDK write",
	)
	timer := time.NewTimer(2 * sendBound)
	defer stopAndDrainCancelWindowTimer(timer)
	<-timer.C
}

func (fixture *blockedSendFixture) cleanup(t *testing.T) {
	fixture.releaseSend()
	endDone := fixture.endA()
	fixture.endB()
	waitForCancelWindowCleanup(t, "End(session-A)", endDone)
	if fixture.tryCancel != nil {
		waitForCancelWindowCleanup(t, "TryCancel(session-A)", fixture.tryCancel.done)
	}
	_ = fixture.outboundWriter.Close()
	_ = fixture.outboundReader.Close()
	waitForCancelWindowCleanup(t, "fake session peer", fixture.peer.done)
}

func requireCancelWindowClaim(t *testing.T, window *Window, attemptID string, want *Session, label string, wantDescription string) *Session {
	t.Helper()
	claimed, ok := window.Claim(attemptID)
	if !ok || claimed != want {
		t.Fatalf("Claim(%s)%s = (%v, %v), want %s", attemptID, label, claimed, ok, wantDescription)
	}
	return claimed
}

func requireCancelWindowOutcome(t *testing.T, result cancelOutcome, wantAccepted bool, message string) {
	t.Helper()
	if result.err != nil {
		t.Fatalf("TryCancel() error = %v, want nil", result.err)
	}
	if result.accepted != wantAccepted {
		t.Fatal(message)
	}
}

func requireCancelWindowNotification(t *testing.T, peer *fakeSessionPeer, want acpsdk.SessionId, wantDescription string) {
	t.Helper()
	notification := <-peer.received
	if notification.SessionId != want {
		t.Fatalf("late notification SessionId = %q, want %s", notification.SessionId, wantDescription)
	}
}

func requireNoCancelWindowNotification(t *testing.T, peer *fakeSessionPeer, message string) {
	t.Helper()
	select {
	case extra := <-peer.received:
		t.Fatalf("%s: %#v", message, extra)
	default:
	}
}

func waitForCancelWindowSignal(t *testing.T, signal <-chan struct{}, timeout time.Duration, message string) {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer stopAndDrainCancelWindowTimer(timer)
	select {
	case <-signal:
	case <-timer.C:
		t.Fatal(message)
	}
}

func waitForCancelWindowCleanup(t *testing.T, stage string, signal <-chan struct{}) {
	t.Helper()
	if signal == nil {
		return
	}
	timer := time.NewTimer(cancelWindowCleanupTimeout)
	defer stopAndDrainCancelWindowTimer(timer)
	select {
	case <-signal:
	case <-timer.C:
		t.Errorf("cleanup stage %q did not complete within %s", stage, cancelWindowCleanupTimeout)
	}
}

func stopAndDrainCancelWindowTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}
