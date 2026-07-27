package conductor_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/conductor"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

func TestConductorCanceledRejectsLateWritesUnderConcurrentLoad(t *testing.T) {
	t.Parallel()

	const concurrency = 8
	var wait sync.WaitGroup
	errs := make([]error, concurrency)
	lateWrites := make([]error, concurrency)
	destinations := make([]*terminalDestination, concurrency)

	for index := range concurrency {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			errs[index], lateWrites[index], destinations[index] = invokeCanceledLateWrite(t, index)
		}(index)
	}
	wait.Wait()

	for index := range concurrency {
		assertCanceledLateWriteWorker(t, index, errs[index], lateWrites[index], destinations[index])
	}
}

func TestConductorConcurrentDispatchPreservesCorrelationOrderAndTerminals(t *testing.T) {
	t.Parallel()

	const concurrency = 12
	providers, recording := newLimitedCapabilityRegistry(t)
	recording.invoke = orderedSuccessfulInvoke(t, recording)
	subject := conductor.New(providers)

	var wait sync.WaitGroup
	errs := make([]error, concurrency)
	destinations := make([]*orderedDestination, concurrency)
	for index := range concurrency {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			destination := &orderedDestination{}
			destinations[index] = destination
			errs[index] = subject.Invoke(
				context.Background(),
				"conductor.fixture",
				acceptedRequest(fmt.Sprintf("inv-concurrent-%d", index)),
				destination,
			)
		}(index)
	}
	wait.Wait()

	for index := range concurrency {
		assertConcurrentDispatchWorker(t, index, errs[index], destinations[index])
	}
}

func TestConductorWriterBackpressureUnderConcurrentLoad(t *testing.T) {
	t.Parallel()

	const (
		concurrency = 10
		secret      = "sk-conductor-backpressure-secret"
	)
	providers, recording := newLimitedCapabilityRegistry(t)
	var lateCloses atomic.Int32
	recording.invoke = backpressureAwareInvoke(t, recording, secret, &lateCloses)
	subject := conductor.New(providers)
	sinkErr := errors.New("response sink backpressure")

	errs, failing, succeeding := runMixedBackpressureInvokes(subject, concurrency, sinkErr)
	if lateCloses.Load() == 0 {
		t.Fatal("expected late Close() rejections after backpressure write failures")
	}
	for index := range concurrency {
		requestID := fmt.Sprintf("inv-backpressure-%d", index)
		if index%2 == 0 {
			assertBackpressureFailureWorker(t, index, errs[index], sinkErr, failing[index])
			continue
		}
		assertBackpressureSuccessWorker(t, index, requestID, secret, errs[index], succeeding[index])
	}
}

func invokeCanceledLateWrite(t *testing.T, index int) (error, error, *terminalDestination) {
	t.Helper()
	providers, recording := newLimitedCapabilityRegistry(t)
	var lateWrite error
	recording.invoke = func(ctx context.Context, request inference.InvocationRequest, writer inference.ResponseWriter) error {
		<-ctx.Done()
		if err := writer.Close(ctx, inference.FailedCompletion(inference.NewFailure(inference.FailureInput{
			Kind:    inference.FailureCanceled,
			Message: "canceled",
			Diagnostics: map[string]string{
				"invariant": conductor.InvariantCanceled,
			},
		}))); err != nil {
			return err
		}
		lateWrite = writer.WriteEvent(
			ctx,
			messageEvent(t, request.InvocationID(), string(recording.identity), "late", "after-close"),
		)
		return lateWrite
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	destination := &terminalDestination{}
	err := conductor.New(providers).Invoke(
		ctx,
		"conductor.fixture",
		acceptedRequest(fmt.Sprintf("inv-race-cancel-late-%d", index)),
		destination,
	)
	return err, lateWrite, destination
}

func assertCanceledLateWriteWorker(t *testing.T, index int, err, lateWrite error, destination *terminalDestination) {
	t.Helper()
	if lateWrite == nil {
		t.Fatalf("worker %d late WriteEvent() error = nil, want rejection after close", index)
	}
	if err == nil {
		t.Fatalf("worker %d Invoke() error = nil, want late-write rejection", index)
	}
	assertExactlyOneTerminal(t, destination)
	assertInterruptionFailure(
		t,
		destination.completion.Failure(),
		inference.FailureCanceled,
		conductor.InvariantCanceled,
		false,
	)
	if len(destination.drafts) != 0 {
		t.Fatalf("worker %d drafts=%d, want no progress after cancel close", index, len(destination.drafts))
	}
}

func orderedSuccessfulInvoke(
	t *testing.T,
	recording *recordingIntegration,
) func(context.Context, inference.InvocationRequest, inference.ResponseWriter) error {
	t.Helper()
	return func(ctx context.Context, request inference.InvocationRequest, writer inference.ResponseWriter) error {
		provider := string(recording.identity)
		invocationID := request.InvocationID()
		for _, phase := range []workers.Phase{workers.PhaseStarted, workers.PhaseCompleted} {
			if err := writer.WriteEvent(ctx, runEvent(t, invocationID, provider, phase)); err != nil {
				return err
			}
		}
		content := invocationID + "-content"
		if err := writer.WriteEvent(ctx, messageEvent(t, invocationID, provider, invocationID+"-message", content)); err != nil {
			return err
		}
		return writer.Close(ctx, inference.SuccessfulCompletion(inference.NewResponse(inference.ResponseInput{
			Content: content,
		})))
	}
}

func assertConcurrentDispatchWorker(t *testing.T, index int, err error, destination *orderedDestination) {
	t.Helper()
	if err != nil {
		t.Fatalf("worker %d Invoke() error = %v", index, err)
	}
	invocationID := fmt.Sprintf("inv-concurrent-%d", index)
	if destination.closes != 1 || destination.completion == nil || destination.completion.Response() == nil {
		t.Fatalf("worker %d closes=%d completion=%#v, want one successful terminal", index, destination.closes, destination.completion)
	}
	if got := destination.completion.Response().Content(); got != invocationID+"-content" {
		t.Fatalf("worker %d response content = %q, want %q", index, got, invocationID+"-content")
	}
	wantOrder := []string{"RUN:STARTED", "RUN:COMPLETED", "MESSAGE:COMPLETED", "CLOSE"}
	if got := destination.order(); !equalStrings(got, wantOrder) {
		t.Fatalf("worker %d order = %v, want %v", index, got, wantOrder)
	}
	for _, draft := range destination.drafts {
		if got := draft.Draft().RunID; got != invocationID {
			t.Fatalf("worker %d draft RunID = %q, want %q", index, got, invocationID)
		}
	}
}

func backpressureAwareInvoke(
	t *testing.T,
	recording *recordingIntegration,
	secret string,
	lateCloses *atomic.Int32,
) func(context.Context, inference.InvocationRequest, inference.ResponseWriter) error {
	t.Helper()
	return func(ctx context.Context, request inference.InvocationRequest, writer inference.ResponseWriter) error {
		provider := string(recording.identity)
		writeErr := writer.WriteEvent(ctx, runEvent(t, request.InvocationID(), provider, workers.PhaseStarted))
		if writeErr != nil {
			return rejectUnsafeLateClose(ctx, writer, secret, lateCloses, writeErr)
		}
		return emitSuccessfulBackpressureProgress(ctx, t, writer, provider, request.InvocationID())
	}
}

func rejectUnsafeLateClose(
	ctx context.Context,
	writer inference.ResponseWriter,
	secret string,
	lateCloses *atomic.Int32,
	writeErr error,
) error {
	lateClose := writer.Close(ctx, inference.FailedCompletion(inference.NewFailure(inference.FailureInput{
		Kind:    inference.FailureDependency,
		Message: "should not replace writer failure token=" + secret,
		Diagnostics: map[string]string{
			"detail": "Authorization: Bearer " + secret,
		},
	})))
	if lateClose != nil {
		lateCloses.Add(1)
	}
	return writeErr
}

func emitSuccessfulBackpressureProgress(
	ctx context.Context,
	t *testing.T,
	writer inference.ResponseWriter,
	provider, invocationID string,
) error {
	t.Helper()
	if err := writer.WriteEvent(ctx, runEvent(t, invocationID, provider, workers.PhaseCompleted)); err != nil {
		return err
	}
	if err := writer.WriteEvent(ctx, messageEvent(t, invocationID, provider, invocationID+"-message", invocationID+"-ok")); err != nil {
		return err
	}
	return writer.Close(ctx, inference.SuccessfulCompletion(inference.NewResponse(inference.ResponseInput{
		Content: invocationID + "-ok",
	})))
}

func runMixedBackpressureInvokes(
	subject *conductor.Conductor,
	concurrency int,
	sinkErr error,
) ([]error, []*failingDestination, []*orderedDestination) {
	var wait sync.WaitGroup
	errs := make([]error, concurrency)
	failing := make([]*failingDestination, concurrency)
	succeeding := make([]*orderedDestination, concurrency)
	for index := range concurrency {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			request := acceptedRequest(fmt.Sprintf("inv-backpressure-%d", index))
			if index%2 == 0 {
				destination := &failingDestination{err: sinkErr}
				failing[index] = destination
				errs[index] = subject.Invoke(context.Background(), "conductor.fixture", request, destination)
				return
			}
			destination := &orderedDestination{}
			succeeding[index] = destination
			errs[index] = subject.Invoke(context.Background(), "conductor.fixture", request, destination)
		}(index)
	}
	wait.Wait()
	return errs, failing, succeeding
}

func assertBackpressureFailureWorker(
	t *testing.T,
	index int,
	err error,
	sinkErr error,
	destination *failingDestination,
) {
	t.Helper()
	if !errors.Is(err, sinkErr) {
		t.Fatalf("worker %d error = %v, want preserved sink backpressure", index, err)
	}
	if destination.closes != 0 {
		t.Fatalf("worker %d closes = %d, want writer failure as sole terminal signal", index, destination.closes)
	}
	if destination.writes != 1 {
		t.Fatalf("worker %d writes = %d, want one failed write attempt", index, destination.writes)
	}
}

func assertBackpressureSuccessWorker(
	t *testing.T,
	index int,
	requestID, secret string,
	err error,
	destination *orderedDestination,
) {
	t.Helper()
	if err != nil {
		t.Fatalf("worker %d Invoke() error = %v", index, err)
	}
	if destination.closes != 1 || destination.completion == nil || destination.completion.Response() == nil {
		t.Fatalf("worker %d closes=%d completion=%#v, want isolated successful terminal", index, destination.closes, destination.completion)
	}
	if got := destination.completion.Response().Content(); got != requestID+"-ok" {
		t.Fatalf("worker %d response content = %q, want %q", index, got, requestID+"-ok")
	}
	for _, draft := range destination.drafts {
		if got := draft.Draft().RunID; got != requestID {
			t.Fatalf("worker %d draft RunID = %q, want %q", index, got, requestID)
		}
		if strings.Contains(string(draft.Draft().Payload), secret) {
			t.Fatalf("worker %d draft leaked unsafe provider detail", index)
		}
	}
}
