package factory_test

import (
	"context"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

const (
	externalConsumerProofBuildMaxDuration = 60 * time.Second
	externalConsumerProofBuildCleanupTime = 5 * time.Second
)

// DEL-RUN-CKPT proof: CaptureCheckpoint, LoadCheckpoint, and RestoreCheckpoint
// are gone from the published Factory Runtime root, and every external
// consumer implementing Service compiles and behaves without them.

func TestServiceDoesNotExposeDeletedCheckpointMethods(t *testing.T) {
	t.Parallel()

	serviceType := reflect.TypeOf((*factory.Service)(nil)).Elem()
	for _, forbidden := range []string{"CaptureCheckpoint", "LoadCheckpoint", "RestoreCheckpoint"} {
		if _, ok := serviceType.MethodByName(forbidden); ok {
			t.Fatalf("Service must not expose deleted checkpoint method %s", forbidden)
		}
	}
}

// TestExternalConsumerCannotCallDeletedCheckpointMethods is the required
// external-consumer negative-compilation proof: it invokes the Go compiler
// against testdata/checkpointdeletionproof, an external-consumer fixture that
// calls svc.CaptureCheckpoint/LoadCheckpoint/RestoreCheckpoint on a
// factory.Service, and asserts the build fails with an undefined-method
// diagnostic naming each removed selector. The fixture lives under a
// directory named "testdata" specifically so `go build ./...`, `go vet
// ./...`, and normal package discovery never compile it as part of this
// module; only this test compiles it, on purpose, expecting failure.
func TestExternalConsumerCannotCallDeletedCheckpointMethods(t *testing.T) {
	// Keep the compiler proof serial with the package's other tests. The nested
	// Go build can contend with the package test binary for the Windows build
	// cache, so parallel scheduling turns a slow compile into an unbounded wait.
	buildContext, cancel := externalConsumerProofBuildContext(t)
	defer cancel()

	cmd := exec.CommandContext(buildContext, "go", "build", "./testdata/checkpointdeletionproof")
	// Give the nested build private cache and temporary-build directories. The
	// external proof may run beside other package builds, and sharing those
	// directories was the observed source of Windows lock contention.
	cmd.Env = append(os.Environ(), "GOCACHE="+t.TempDir(), "GOTMPDIR="+t.TempDir())
	// CommandContext kills the go process when the proof deadline expires.
	// WaitDelay bounds the reap and pipe cleanup so orphaned compiler output
	// cannot keep CombinedOutput blocked after cancellation.
	cmd.WaitDelay = externalConsumerProofBuildCleanupTime
	output, err := cmd.CombinedOutput()
	if contextErr := buildContext.Err(); contextErr != nil {
		childDeadline, _ := buildContext.Deadline()
		t.Fatalf("external-consumer checkpoint deletion proof build ended with %v; child was killed and reaped at its %s deadline: %v\nbuild output:\n%s", contextErr, childDeadline.Format(time.RFC3339Nano), err, output)
	}
	if err == nil {
		t.Fatalf("expected compilation of testdata/checkpointdeletionproof to fail because CaptureCheckpoint/LoadCheckpoint/RestoreCheckpoint no longer exist on factory.Service, but the build succeeded")
	}

	got := string(output)
	for _, forbidden := range []string{"CaptureCheckpoint", "LoadCheckpoint", "RestoreCheckpoint"} {
		if !strings.Contains(got, forbidden) {
			t.Errorf("expected compiler diagnostic naming removed method %s, got build output:\n%s", forbidden, got)
		}
	}
	if !strings.Contains(got, "undefined") {
		t.Errorf("expected an undefined-method compiler diagnostic, got build output:\n%s", got)
	}
}

func externalConsumerProofBuildContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()

	testDeadline, hasTestDeadline := t.Deadline()
	if !hasTestDeadline {
		return context.WithTimeout(context.Background(), externalConsumerProofBuildMaxDuration)
	}

	now := time.Now()
	childDeadline := testDeadline.Add(-externalConsumerProofBuildCleanupTime)
	maxChildDeadline := now.Add(externalConsumerProofBuildMaxDuration)
	if childDeadline.After(maxChildDeadline) {
		childDeadline = maxChildDeadline
	}
	if !childDeadline.After(now) {
		t.Fatalf("external-consumer checkpoint deletion proof has no time left for a bounded child build and cleanup before the test deadline")
	}

	return context.WithDeadline(context.Background(), childDeadline)
}

// externalConsumerPeer implements factory.Service using only the surviving
// root vocabulary, proving an external consumer can satisfy the interface
// without any checkpoint request/result/error vocabulary in scope.
type externalConsumerPeer struct{}

var _ factory.Service = (*externalConsumerPeer)(nil)

func (externalConsumerPeer) ControlPause(context.Context, factory.PauseRequest) (factory.PauseResult, error) {
	return factory.PauseResult{Outcome: factory.ControlOutcomeAccepted}, nil
}
func (externalConsumerPeer) ControlResume(context.Context, factory.ResumeRequest) (factory.ResumeResult, error) {
	return factory.ResumeResult{Outcome: factory.ControlOutcomeAccepted}, nil
}
func (externalConsumerPeer) ControlTerminate(context.Context, factory.TerminateRequest) (factory.TerminateResult, error) {
	return factory.TerminateResult{Outcome: factory.ControlOutcomeAccepted}, nil
}
func (externalConsumerPeer) ControlWaitToComplete(factory.WaitToCompleteRequest) factory.WaitToCompleteResult {
	done := make(chan struct{})
	close(done)
	return factory.WaitToCompleteResult{Done: done}
}
func (externalConsumerPeer) ControlMoveWork(_ context.Context, req factory.MoveWorkRequest) (factory.MoveWorkResult, error) {
	return factory.MoveWorkResult{WorkID: req.WorkID, ToState: req.StateName}, nil
}
func (externalConsumerPeer) Observe(context.Context, factory.ObserveRequest) (factory.ObserveResult, error) {
	return factory.ObserveResult{Observation: factory.Observation{Status: factory.ObservationStatusActive}}, nil
}
func (externalConsumerPeer) CleanInvocationSnapshot(context.Context) (factory.CleanInvocationSnapshot, error) {
	return factory.CleanInvocationSnapshot{}, nil
}
func (externalConsumerPeer) PlanDispatch(_ context.Context, req factory.PlanDispatchRequest) (factory.PlanDispatchResult, error) {
	return factory.PlanDispatchResult{Outcome: factory.DispatchPlanOutcomeAccepted, DispatchID: req.DispatchID}, nil
}
func (externalConsumerPeer) InvokeWorker(_ context.Context, _ factory.InvokeWorkerRequest) (factory.InvokeWorkerResult, error) {
	return factory.InvokeWorkerResult{}, nil
}

func (externalConsumerPeer) AcceptDispatchResult(_ context.Context, req factory.AcceptDispatchResultRequest) (factory.AcceptDispatchResultResult, error) {
	return factory.AcceptDispatchResultResult{Outcome: factory.DispatchPlanOutcomeRetired, DispatchID: req.DispatchID}, nil
}

func TestExternalConsumerSatisfiesServiceWithoutCheckpointVocabulary(t *testing.T) {
	t.Parallel()

	var runtime factory.Service = externalConsumerPeer{}
	ctx := context.Background()

	if _, err := runtime.ControlPause(ctx, factory.PauseRequest{}); err != nil {
		t.Fatalf("ControlPause() error = %v", err)
	}
	if _, err := runtime.Observe(ctx, factory.ObserveRequest{}); err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if _, err := runtime.PlanDispatch(ctx, factory.PlanDispatchRequest{DispatchID: "del-run-ckpt-proof"}); err != nil {
		t.Fatalf("PlanDispatch() error = %v", err)
	}
	if _, err := runtime.AcceptDispatchResult(ctx, factory.AcceptDispatchResultRequest{DispatchID: "del-run-ckpt-proof"}); err != nil {
		t.Fatalf("AcceptDispatchResult() error = %v", err)
	}
}
