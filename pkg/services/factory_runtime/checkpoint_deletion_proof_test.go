package factory_test

import (
	"context"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
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
	t.Parallel()

	cmd := exec.Command("go", "build", "./testdata/checkpointdeletionproof")
	output, err := cmd.CombinedOutput()
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
func (externalConsumerPeer) PlanDispatch(_ context.Context, req factory.PlanDispatchRequest) (factory.PlanDispatchResult, error) {
	return factory.PlanDispatchResult{Outcome: factory.DispatchPlanOutcomeAccepted, DispatchID: req.DispatchID}, nil
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
