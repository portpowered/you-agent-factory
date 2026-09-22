//go:build windows && managed_process_integration

package effects

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestASRLiveCorrelationOrdersDecodedResponseAgainstOwnedWait(t *testing.T) {
	t.Parallel()

	responseFirst := runASRLiveCorrelationOrder(t, true)
	exitFirst := runASRLiveCorrelationOrder(t, false)
	if responseFirst.RequestSemanticSHA256 != exitFirst.RequestSemanticSHA256 ||
		responseFirst.ResponseSemanticSHA256 != exitFirst.ResponseSemanticSHA256 {
		t.Fatalf("semantic digests differ across schedules: response-first=%#v exit-first=%#v", responseFirst, exitFirst)
	}
	if responseFirst.ReleaseSequence >= responseFirst.WaitSequence ||
		exitFirst.WaitSequence >= exitFirst.FailureSequence ||
		exitFirst.FailureSequence >= exitFirst.ReleaseSequence {
		t.Fatalf("release/wait order response-first=%#v exit-first=%#v", responseFirst, exitFirst)
	}
}

func runASRLiveCorrelationOrder(t *testing.T, responseFirst bool) ASRLiveCorrelationOrderFacts {
	t.Helper()
	controller, requestDigest, responseDigest := newASRLiveCorrelationOrder(t)
	responseDone := awaitASRLiveCorrelationResponse(controller, requestDigest, responseDigest)
	terminal, err := controller.WaitForSignal(testSignalContext(t), ASRLiveCorrelationRPCTerminal)
	if err != nil || terminal.RequestSemanticSHA256 != requestDigest || terminal.ResponseSemanticSHA256 != responseDigest {
		t.Fatalf("decoded RPC terminal = %#v, error = %v", terminal, err)
	}
	if responseFirst {
		releaseASRLiveCorrelationResponse(t, controller, responseDone)
	}
	if err := controller.RecordChildWaited(8123, "NONZERO_EXIT", true, 1); err != nil {
		t.Fatalf("record owned Wait: %v", err)
	}
	if err := controller.RecordHostFailureObserved(8123); err != nil {
		t.Fatalf("record host lease revocation: %v", err)
	}
	if !responseFirst {
		releaseASRLiveCorrelationResponse(t, controller, responseDone)
	}
	return asrLiveCorrelationOrderFacts(controller)
}

func newASRLiveCorrelationOrder(
	t *testing.T,
) (*ASRLiveCorrelationController, string, string) {
	t.Helper()
	controller, err := NewASRLiveCorrelationController(func(_ context.Context, host string, port int) (int, error) {
		if host != "127.0.0.1" || port != 49152 {
			return 0, ErrASRLiveCorrelationOwnership
		}
		return 8123, nil
	})
	if err != nil {
		t.Fatalf("construct controller: %v", err)
	}
	if err := controller.RecordManagedChildStarted(8123, "grpc://127.0.0.1:49152"); err != nil {
		t.Fatalf("record child start: %v", err)
	}
	requestDigest := asrCorrelationTestDigest("same controlled request")
	responseDigest := asrCorrelationTestDigest("same decoded transcript and segments")
	if err := controller.RecordRequestSemanticSHA256(requestDigest); err != nil {
		t.Fatalf("record request digest: %v", err)
	}
	if err := controller.ObserveEndpoint(context.Background(), "grpc://127.0.0.1:49152"); err != nil {
		t.Fatalf("observe endpoint: %v", err)
	}
	return controller, requestDigest, responseDigest
}

func awaitASRLiveCorrelationResponse(
	controller *ASRLiveCorrelationController,
	requestDigest, responseDigest string,
) <-chan error {
	responseDone := make(chan error, 1)
	go func() {
		responseDone <- controller.AwaitResponseRelease(context.Background(), requestDigest, responseDigest)
	}()
	return responseDone
}

func releaseASRLiveCorrelationResponse(
	t *testing.T,
	controller *ASRLiveCorrelationController,
	responseDone <-chan error,
) {
	t.Helper()
	if err := controller.ReleaseResponse(); err != nil {
		t.Fatalf("release decoded response: %v", err)
	}
	if err := <-responseDone; err != nil {
		t.Fatalf("decoded response release result: %v", err)
	}
}

func asrLiveCorrelationOrderFacts(controller *ASRLiveCorrelationController) ASRLiveCorrelationOrderFacts {
	var facts ASRLiveCorrelationOrderFacts
	for _, event := range controller.Snapshot() {
		switch event.Kind {
		case ASRLiveCorrelationRPCTerminal:
			facts.TerminalSequence = event.Sequence
			facts.RequestSemanticSHA256 = event.RequestSemanticSHA256
			facts.ResponseSemanticSHA256 = event.ResponseSemanticSHA256
		case ASRLiveCorrelationResponseSent:
			facts.ReleaseSequence = event.Sequence
		case ASRLiveCorrelationChildWaited:
			facts.WaitSequence = event.Sequence
		case ASRLiveCorrelationHostFailureSeen:
			facts.FailureSequence = event.Sequence
		}
	}
	return facts
}

type ASRLiveCorrelationOrderFacts struct {
	TerminalSequence       uint64
	ReleaseSequence        uint64
	WaitSequence           uint64
	FailureSequence        uint64
	RequestSemanticSHA256  string
	ResponseSemanticSHA256 string
}

func TestASRLiveCorrelationRejectsMalformedAndUnownedEndpoints(t *testing.T) {
	t.Parallel()

	for _, address := range []string{
		"grpc://127.0.0.1",
		" grpc://127.0.0.1:49152",
		"grpc://127.0.0.1:7437",
		"grpc://127.0.0.1:+49152",
		"grpc://0.0.0.0:49152",
		"grpc://localhost:49152",
		"grpc://127.0.0.1:49152/path",
		"grpc://127.0.0.1:49152?token=private",
		"grpc://[::1]:70000",
	} {
		t.Run(address, func(t *testing.T) {
			t.Parallel()
			controller := testASRLiveCorrelationController(t, 8123)
			if err := controller.RecordManagedChildStarted(8123, "grpc://127.0.0.1:49152"); err != nil {
				t.Fatalf("record child start: %v", err)
			}
			if err := controller.ObserveEndpoint(context.Background(), address); !errors.Is(err, ErrASRLiveCorrelationOwnership) {
				t.Fatalf("ObserveEndpoint(%q) = %v, want ownership rejection", address, err)
			}
			if events := controller.Snapshot(); len(events) != 1 || events[0].Kind != ASRLiveCorrelationChildStarted {
				t.Fatalf("malformed endpoint emitted evidence: %#v", events)
			}
		})
	}

	controller, err := NewASRLiveCorrelationController(func(context.Context, string, int) (int, error) {
		return 9876, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.RecordManagedChildStarted(8123, "grpc://127.0.0.1:49152"); err != nil {
		t.Fatal(err)
	}
	if err := controller.ObserveEndpoint(context.Background(), "grpc://127.0.0.1:49152"); !errors.Is(err, ErrASRLiveCorrelationOwnership) {
		t.Fatalf("foreign listener PID accepted: %v", err)
	}
	if len(controller.Snapshot()) != 1 {
		t.Fatalf("foreign listener PID was recorded: %#v", controller.Snapshot())
	}
	if err := controller.RecordChildWaited(9876, "NONZERO_EXIT", true, 1); !errors.Is(err, ErrASRLiveCorrelationOwnership) {
		t.Fatalf("foreign child Wait accepted: %v", err)
	}
}

func TestASRLiveCorrelationClassifiesNaturalAndHarnessRequestedExits(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		stopFirst bool
		want      string
	}{
		{name: "natural", want: ASRLiveCorrelationExitNatural},
		{name: "harness requested", stopFirst: true, want: ASRLiveCorrelationExitHarnessRequested},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			controller := testASRLiveCorrelationReadyController(t)
			if test.stopFirst {
				if err := controller.RecordChildStopRequested(8123); err != nil {
					t.Fatalf("record harness stop request: %v", err)
				}
			}
			if err := controller.RecordChildWaited(8123, "NONZERO_EXIT", true, 1); err != nil {
				t.Fatalf("record child Wait: %v", err)
			}
			event, err := controller.WaitForSignal(testSignalContext(t), ASRLiveCorrelationChildWaited)
			if err != nil || event.ExitTrigger != test.want {
				t.Fatalf("child exit trigger = %q, error = %v; want %q", event.ExitTrigger, err, test.want)
			}
		})
	}
}

func TestASRLiveCorrelationCancellationDuplicateAndMissingSignalsAreTyped(t *testing.T) {
	t.Parallel()

	controller := testASRLiveCorrelationReadyController(t)
	requestDigest := asrCorrelationTestDigest("request")
	responseDigest := asrCorrelationTestDigest("response")
	if err := controller.RecordRequestSemanticSHA256(requestDigest); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	responseDone := make(chan error, 1)
	go func() {
		responseDone <- controller.AwaitResponseRelease(ctx, requestDigest, responseDigest)
	}()
	if _, err := controller.WaitForSignal(testSignalContext(t), ASRLiveCorrelationRPCTerminal); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := <-responseDone; !errors.Is(err, ErrASRLiveCorrelationCancelled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled response = %v, want typed cancellation and context cause", err)
	}
	if err := controller.ReleaseResponse(); !errors.Is(err, ErrASRLiveCorrelationOutOfOrder) {
		t.Fatalf("release after cancellation = %v, want rejected signal", err)
	}
	if err := controller.RecordHostFailureObserved(8123); !errors.Is(err, ErrASRLiveCorrelationOutOfOrder) {
		t.Fatalf("host failure before child Wait = %v, want rejected signal", err)
	}
	if err := controller.RecordChildWaited(8123, "NONZERO_EXIT", true, 1); err != nil {
		t.Fatal(err)
	}
	if err := controller.RecordHostFailureObserved(8123); err != nil {
		t.Fatal(err)
	}
	if err := controller.RecordHostFailureObserved(8123); !errors.Is(err, ErrASRLiveCorrelationDuplicate) {
		t.Fatalf("duplicate host failure signal = %v", err)
	}
	if err := controller.RecordChildWaited(8123, "NONZERO_EXIT", true, 1); !errors.Is(err, ErrASRLiveCorrelationDuplicate) {
		t.Fatalf("duplicate Wait = %v", err)
	}
	if err := controller.RecordRequestSemanticSHA256(requestDigest); !errors.Is(err, ErrASRLiveCorrelationDuplicate) {
		t.Fatalf("duplicate request digest = %v", err)
	}

	missing := testASRLiveCorrelationReadyController(t)
	missingCtx, missingCancel := context.WithCancel(context.Background())
	missingCancel()
	if _, err := missing.WaitForSignal(missingCtx, ASRLiveCorrelationRPCTerminal); !errors.Is(err, ErrASRLiveCorrelationMissing) || !errors.Is(err, context.Canceled) {
		t.Fatalf("missing terminal = %v, want typed missing signal and context cause", err)
	}
}

func TestASRLiveCorrelationConcurrentDuplicateSignalsAppendOnce(t *testing.T) {
	t.Parallel()

	controller := testASRLiveCorrelationReadyController(t)
	requestDigest := asrCorrelationTestDigest("request")
	responseDigest := asrCorrelationTestDigest("response")
	if err := controller.RecordRequestSemanticSHA256(requestDigest); err != nil {
		t.Fatal(err)
	}
	responseDone := make(chan error, 1)
	go func() {
		responseDone <- controller.AwaitResponseRelease(context.Background(), requestDigest, responseDigest)
	}()
	if _, err := controller.WaitForSignal(testSignalContext(t), ASRLiveCorrelationRPCTerminal); err != nil {
		t.Fatal(err)
	}

	var wait sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results <- controller.ReleaseResponse()
		}()
	}
	wait.Wait()
	close(results)
	var successes, duplicates int
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrASRLiveCorrelationDuplicate):
			duplicates++
		default:
			t.Fatalf("concurrent release result = %v", err)
		}
	}
	if successes != 1 || duplicates != 1 {
		t.Fatalf("concurrent release successes=%d duplicates=%d", successes, duplicates)
	}
	if err := <-responseDone; err != nil {
		t.Fatal(err)
	}
	var releaseCount int
	for _, event := range controller.Snapshot() {
		if event.Kind == ASRLiveCorrelationResponseSent {
			releaseCount++
		}
	}
	if releaseCount != 1 {
		t.Fatalf("response release event count = %d, want one; events=%#v", releaseCount, controller.Snapshot())
	}
}

func testASRLiveCorrelationReadyController(t *testing.T) *ASRLiveCorrelationController {
	t.Helper()
	controller := testASRLiveCorrelationController(t, 8123)
	if err := controller.RecordManagedChildStarted(8123, "grpc://127.0.0.1:49152"); err != nil {
		t.Fatal(err)
	}
	if err := controller.ObserveEndpoint(context.Background(), "grpc://127.0.0.1:49152"); err != nil {
		t.Fatal(err)
	}
	return controller
}

func testASRLiveCorrelationController(t *testing.T, ownerPID int) *ASRLiveCorrelationController {
	t.Helper()
	controller, err := NewASRLiveCorrelationController(func(_ context.Context, host string, port int) (int, error) {
		if host != "127.0.0.1" || port != 49152 {
			return 0, fmt.Errorf("unsafe test endpoint")
		}
		return ownerPID, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return controller
}

func testSignalContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func asrCorrelationTestDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
