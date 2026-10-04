package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	acpsdk "github.com/portpowered/infinite-you/third_party/acp-go-sdk"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp/internal/service/cancelwindow"
)

// No OS process participates: controlled exit observations prove owner wait
// policy and stream release; delivered process-tree cleanup belongs to IR01.
func TestAttemptTeardownUsesInjectedScheduler(t *testing.T) {
	for _, outcome := range []string{"graceful", "forced-exit", "kill-timeout", "caller-cancel"} {
		t.Run(outcome, func(t *testing.T) {
			t.Parallel()
			scheduler := newTeardownScheduler()
			stdin, stdout := &teardownStream{}, &teardownStream{}
			logger := &teardownLogger{}
			handles := &attemptHandles{
				stdin: stdin, stdout: stdout, finished: make(chan error, 1), scheduler: scheduler, logger: logger,
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- handles.terminate(ctx) }()
			grace := awaitTeardownTimer(t, scheduler, 500*time.Millisecond)
			timers := []*teardownTimer{grace}
			if outcome == "graceful" {
				handles.finished <- nil
			} else {
				if outcome == "caller-cancel" {
					cancel()
				} else {
					scheduler.clock.SetTick(500)
				}
				timers = append(timers, awaitTeardownTimer(t, scheduler, 2*time.Second))
				if outcome == "kill-timeout" {
					scheduler.clock.SetTick(2500)
				} else {
					handles.finished <- nil
				}
			}
			assertTeardownOutcome(t, outcome, <-done, stdin, stdout, timers)
			assertTeardownLogs(t, outcome, logger)
		})
	}
}

func assertTeardownOutcome(t *testing.T, outcome string, err error, stdin, stdout *teardownStream, timers []*teardownTimer) {
	t.Helper()
	switch outcome {
	case "caller-cancel":
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("teardown = %v, want caller cancellation", err)
		}
	case "kill-timeout":
		if err == nil || err.Error() != "ACP process did not exit after termination" {
			t.Fatalf("teardown = %v, want kill-wait failure", err)
		}
	default:
		if err != nil {
			t.Fatalf("teardown = %v, want successful exit", err)
		}
	}
	if stdin.closes.Load() != 1 || stdout.closes.Load() != 1 {
		t.Fatal("teardown did not release both streams once")
	}
	for _, timer := range timers {
		if !timer.stopped.Load() {
			t.Fatal("teardown left its scheduler timer running")
		}
	}
}

type teardownStream struct{ closes atomic.Int32 }

func (*teardownStream) Read([]byte) (int, error)    { return 0, io.EOF }
func (*teardownStream) Write(p []byte) (int, error) { return len(p), nil }
func (stream *teardownStream) Close() error         { stream.closes.Add(1); return nil }

type teardownTimer struct {
	platformclock.Timer
	duration time.Duration
	stopped  atomic.Bool
}

func (timer *teardownTimer) Stop() bool { timer.stopped.Store(true); return timer.Timer.Stop() }

type teardownScheduler struct {
	clock   *platformclock.Deterministic
	created chan *teardownTimer
}

func newTeardownScheduler() *teardownScheduler {
	return &teardownScheduler{clock: platformclock.NewDeterministic(time.Unix(0, 0), time.Millisecond), created: make(chan *teardownTimer, 4)}
}
func (scheduler *teardownScheduler) Now() time.Time { return scheduler.clock.Now() }
func (scheduler *teardownScheduler) NewTimer(duration time.Duration) platformclock.Timer {
	timer := &teardownTimer{Timer: scheduler.clock.NewTimer(duration), duration: duration}
	scheduler.created <- timer
	return timer
}
func awaitTeardownTimer(t *testing.T, scheduler *teardownScheduler, duration time.Duration) *teardownTimer {
	t.Helper()
	select {
	case timer := <-scheduler.created:
		if timer.duration != duration {
			t.Fatalf("timer duration = %v, want %v", timer.duration, duration)
		}
		return timer
	case <-time.After(5 * time.Second):
		t.Fatal("teardown did not reach its scheduler wait")
		return nil
	}
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
	}}, nil, nil, platformprocess.NewParentOwnedStdio, platformclock.Real{}, logging.NoopLogger{})
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

// Each attempt owns its diagnostic sink; reading follows the joined teardown.
type teardownLogger struct {
	logging.NoopLogger
	reasons []string
}

func (logger *teardownLogger) Verbose(_ string, fields ...any) {
	for index := 0; index+1 < len(fields); index += 2 {
		if fields[index] == "cleanup_reason" {
			logger.reasons = append(logger.reasons, fields[index+1].(string))
		}
	}
}

func assertTeardownLogs(t *testing.T, outcome string, logger *teardownLogger) {
	t.Helper()
	want := []string{"cancel", "post_run"}
	if outcome == "graceful" {
		want = []string{"post_run"}
	}
	if !slices.Equal(logger.reasons, want) {
		t.Fatalf("cleanup diagnostics = %v, want %v", logger.reasons, want)
	}
}
