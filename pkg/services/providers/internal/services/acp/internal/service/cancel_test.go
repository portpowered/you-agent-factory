package service

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"testing"

	acpsdk "github.com/portpowered/infinite-you/third_party/acp-go-sdk"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp/internal/service/cancelwindow"
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

// TestServiceClaimAndTryCancelResolveAliasAndDelegateToAttempt proves the
// Service-level Claim/TryCancel wrappers resolve aliases and unknown
// providers correctly and delegate to the exact canonical provider's live
// attempt cancel window. The window's own accept/reject/race semantics are
// covered directly and exhaustively by the cancelwindow package tests; this
// file only proves the resolution/delegation wiring on top of it, and that the
// captured generation is bound to the one attempt that opened it.
func TestServiceClaimAndTryCancelResolveAliasAndDelegateToAttempt(t *testing.T) {
	serviceValue, err := New([]providers.ACPIntegration{{
		ID: "entry-1", Name: "custom-acp", Aliases: []string{"custom"}, Transport: "stdio", Command: "agent acp",
	}}, nil, nil, platformprocess.NewParentOwnedStdio)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	svc := serviceValue.(*Service)
	target := svc.providers["custom-acp"]
	attempt := target.newAttempt(providers.ExecuteRequest{AttemptID: "attempt-1"})
	t.Cleanup(attempt.release)

	peer := newFakeSessionPeer()
	connection := newPipedConnection(t, peer)
	session := attempt.window.Begin("attempt-1", acpsdk.SessionId("session-1"), connection)

	generation, ok := svc.Claim("custom", "attempt-1")
	if !ok || generation == nil {
		t.Fatalf("Claim(alias) = (%v, %v), want a non-nil generation and true", generation, ok)
	}
	assertClaimResolution(t, svc)

	tryCancelDone := make(chan cancelOutcome, 1)
	go func() {
		accepted, err := svc.TryCancel(context.Background(), generation)
		tryCancelDone <- cancelOutcome{accepted: accepted, err: err}
	}()
	<-peer.received
	attempt.window.End(session, true)
	result := <-tryCancelDone
	if result.err != nil {
		t.Fatalf("TryCancel() error = %v, want nil", result.err)
	}
	if !result.accepted {
		t.Fatal("TryCancel() accepted = false, want true")
	}
	if _, ok := svc.Claim("custom", "attempt-1"); ok {
		t.Fatal("Claim(alias) ok = true after the window closed, want false")
	}

	// The generation captured before its window ended is stale now. Delivering
	// through that same handle must reach nothing at all - not the ended
	// session, and not a replacement attempt that reuses the identical
	// canonical provider and attempt ID while the control is still in flight.
	replacement := target.newAttempt(providers.ExecuteRequest{AttemptID: "attempt-1"})
	t.Cleanup(replacement.release)
	replacementPeer := newFakeSessionPeer()
	replacementConnection := newPipedConnection(t, replacementPeer)
	replacementSession := replacement.window.Begin("attempt-1", acpsdk.SessionId("session-2"), replacementConnection)
	assertStaleGenerationDeliversNothing(t, svc, replacement, replacementPeer, replacementSession, generation)

	// A nil generation is not a control this service can deliver at all.
	accepted, err := svc.TryCancel(context.Background(), nil)
	if err != nil {
		t.Fatalf("TryCancel(nil generation) error = %v, want nil", err)
	}
	if accepted {
		t.Fatal("TryCancel(nil generation) accepted = true, want false")
	}
}

// cancelOutcome is one delivered TryCancel result, so a test can observe a
// delivery that is still in flight without blocking the goroutine performing it.
type cancelOutcome struct {
	accepted bool
	err      error
}

// assertClaimResolution proves Claim resolves the accepted alias and refuses an
// attempt that is not live for it, as well as an unregistered provider, without
// disturbing the generation it already captured.
func assertClaimResolution(t *testing.T, svc *Service) {
	t.Helper()
	if _, ok := svc.Claim("custom", "attempt-other"); ok {
		t.Fatal("Claim(alias, wrong attempt) ok = true, want false")
	}
	if _, ok := svc.Claim("unknown-provider", "attempt-1"); ok {
		t.Fatal("Claim(unknown provider) ok = true, want false")
	}
}

// assertStaleGenerationDeliversNothing proves an ended generation reaches no
// session at all - neither the ended one nor the replacement attempt that
// reuses the identical canonical provider and attempt ID - and that the
// replacement's own live turn is still independently claimable.
func assertStaleGenerationDeliversNothing(
	t *testing.T,
	svc *Service,
	replacement *attempt,
	replacementPeer *fakeSessionPeer,
	replacementSession *cancelwindow.Session,
	generation acp.Generation,
) {
	t.Helper()
	accepted, err := svc.TryCancel(context.Background(), generation)
	if err != nil || accepted {
		t.Fatalf("TryCancel(ended generation) = (%v, %v), want (false, nil): a stale generation must deliver nothing", accepted, err)
	}
	select {
	case notification := <-replacementPeer.received:
		t.Fatalf("stale generation delivered a session/cancel to a replacement attempt's session: %#v", notification)
	default:
	}
	// The replacement's own live turn is untouched and independently claimable,
	// so a control that claims it now reaches it and nothing else.
	current, ok := svc.Claim("custom", "attempt-1")
	if !ok {
		t.Fatal("Claim(alias) on the replacement attempt ok = false, want true")
	}
	replacementCancel := make(chan cancelOutcome, 1)
	go func() {
		accepted, err := svc.TryCancel(context.Background(), current)
		replacementCancel <- cancelOutcome{accepted: accepted, err: err}
	}()
	if _, ok := <-replacementPeer.received; !ok {
		t.Fatal("replacement session received no session/cancel notification")
	}
	replacement.window.End(replacementSession, true)
	if result := <-replacementCancel; result.err != nil || !result.accepted {
		t.Fatalf("TryCancel(replacement generation) = (%v, %v), want (true, nil)", result.accepted, result.err)
	}
}
