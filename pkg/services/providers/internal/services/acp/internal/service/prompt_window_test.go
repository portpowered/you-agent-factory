package service

import (
	"context"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
)

// TestDaemonPromptWithWindow_PanicStillClosesWindowForLaterIdentityReuse
// proves the panic/unexpected-unwind fix: previously Window.End ran as a
// plain statement after connection.Prompt, so a panic during Prompt skipped
// it entirely and left the window's single active slot pointing at a dead
// session forever. promptWithWindow now defers window.End, so an unexpected
// unwind still closes it - and a later execution that reuses the same
// attempt ID can bind and have a control reach its own fresh session instead
// of racing a stale one that would never unblock TryCancel.
func TestDaemonPromptWithWindow_PanicStillClosesWindowForLaterIdentityReuse(t *testing.T) {
	t.Parallel()

	d := &daemon{}
	attemptID := "attempt-1"

	func() {
		defer func() { _ = recover() }()
		_, _ = d.promptWithWindow(attemptID, acpsdk.SessionId("stale-session"), nil, func() (acpsdk.PromptResponse, error) {
			panic("simulated unexpected Prompt unwind")
		})
	}()

	if d.window.Live(attemptID) {
		t.Fatal("window.Live() = true after a panicking prompt, want the window closed on unexpected unwind")
	}

	// A control racing in before the next execution's own Begin must observe
	// no live window for this identity - not hang on the dead stale session.
	accepted, err := d.window.TryCancel(context.Background(), attemptID)
	if err != nil {
		t.Fatalf("TryCancel() on stale identity error = %v, want nil", err)
	}
	if accepted {
		t.Fatal("TryCancel() on stale identity accepted = true, want false: no live window remains for a closed session")
	}

	// A later execution reusing the same attempt ID must be able to bind and
	// have controls reach its own fresh session, not the stale one.
	peer := newFakeSessionPeer()
	connection := newPipedConnection(t, peer)
	freshSession := d.window.Begin(attemptID, acpsdk.SessionId("fresh-session"), connection)

	type outcome struct {
		accepted bool
		err      error
	}
	tryCancelDone := make(chan outcome, 1)
	go func() {
		accepted, err := d.window.TryCancel(context.Background(), attemptID)
		tryCancelDone <- outcome{accepted: accepted, err: err}
	}()
	<-peer.received
	d.window.End(freshSession, true)
	result := <-tryCancelDone
	if result.err != nil {
		t.Fatalf("TryCancel() on reused identity error = %v, want nil", result.err)
	}
	if !result.accepted {
		t.Fatal("TryCancel() on reused identity accepted = false, want true: the fresh session must be reachable")
	}
}

// TestDaemonPromptWithWindow_NormalReturnRecordsOutcomeAndClosesWindow proves
// promptWithWindow's normal (non-panic) path is unchanged by the refactor:
// the window closes and the real StopReason still grounds the recorded
// outcome a concurrent TryCancel would observe.
func TestDaemonPromptWithWindow_NormalReturnRecordsOutcomeAndClosesWindow(t *testing.T) {
	t.Parallel()

	d := &daemon{}
	attemptID := "attempt-1"

	response, err := d.promptWithWindow(attemptID, acpsdk.SessionId("session-1"), nil, func() (acpsdk.PromptResponse, error) {
		return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, nil
	})
	if err != nil {
		t.Fatalf("promptWithWindow() error = %v, want nil", err)
	}
	if response.StopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("promptWithWindow() response = %#v, want the prompt func's own response", response)
	}
	if d.window.Live(attemptID) {
		t.Fatal("window.Live() = true after a normal return, want the window closed")
	}
}
