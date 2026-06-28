package fixtures_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/runtimepersist"
)

func TestJavaScriptRuntimeService_ResumeInterruptedSession_ReconstructsFromCheckpointSummary(t *testing.T) {
	harness := startInterruptedResumableSession(t, "req-runtime-resume-interrupted-001")

	if harness.provider.CallCount() < 2 {
		t.Fatalf("provider infer calls = %d, want at least 2 before interrupt", harness.provider.CallCount())
	}

	snapshotPath := filepath.Join(runtimepersist.DirForProjectRoot(harness.projectRoot), harness.sessionID+".json")
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Fatalf("interrupted snapshot must be durable before cross-instance resume: %v", err)
	}

	firstDispatchBeforeResume, err := harness.initial.GetDispatch(context.Background(), harness.sessionID, "dispatch-1")
	if err != nil {
		t.Fatalf("GetDispatch dispatch-1 before resume: %v", err)
	}
	if firstDispatchBeforeResume.Status != fse.DispatchStatusCompleted {
		t.Fatalf("dispatch-1 before resume = %#v, want COMPLETED", firstDispatchBeforeResume)
	}

	resumedService := newResumedRuntimeService(harness)
	resumed, err := resumedService.ResumeInterruptedSession(context.Background(), harness.sessionID, fse.ResumeSessionRequest{
		RequestID: "req-runtime-resume-interrupted-resume-001",
	})
	if err != nil {
		t.Fatalf("ResumeInterruptedSession: %v", err)
	}
	if resumed.SessionID != harness.sessionID {
		t.Fatalf("resumed sessionId = %q, want %q", resumed.SessionID, harness.sessionID)
	}
	if resumed.Status != string(fse.LifecycleStatusResuming) {
		t.Fatalf("resumed start status = %q, want RESUMING", resumed.Status)
	}

	success := waitUntilSessionStatus(t, resumedService, harness.sessionID, fse.LifecycleStatusSucceeded, 5*time.Second)
	if success.ResultSummary == nil || success.ResultSummary.ResultStatus != string(fse.ResultStatusFinal) {
		t.Fatalf("resumed resultSummary = %#v, want FINAL", success.ResultSummary)
	}

	if harness.provider.CallCount() != 3 {
		t.Fatalf("provider infer calls = %d, want 3 (step-one, blocked step-two, resumed step-two without rerunning step-one)", harness.provider.CallCount())
	}

	firstDispatchAfterResume, err := resumedService.GetDispatch(context.Background(), harness.sessionID, "dispatch-1")
	if err != nil {
		t.Fatalf("GetDispatch dispatch-1 after resume: %v", err)
	}
	if firstDispatchAfterResume.Status != fse.DispatchStatusCompleted {
		t.Fatalf("dispatch-1 after resume = %#v, want COMPLETED", firstDispatchAfterResume)
	}
	if firstDispatchAfterResume.ID != firstDispatchBeforeResume.ID {
		t.Fatalf("dispatch-1 id changed across resume: %q -> %q", firstDispatchBeforeResume.ID, firstDispatchAfterResume.ID)
	}

	secondDispatchAfterResume, err := resumedService.GetDispatch(context.Background(), harness.sessionID, "dispatch-2")
	if err != nil {
		t.Fatalf("GetDispatch dispatch-2 after resume: %v", err)
	}
	if secondDispatchAfterResume.Status != fse.DispatchStatusCompleted {
		t.Fatalf("dispatch-2 after resume = %#v, want COMPLETED", secondDispatchAfterResume)
	}

	result, err := resumedService.GetResult(context.Background(), harness.sessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != fse.ResultStatusFinal {
		t.Fatalf("result status = %q, want FINAL", result.ResultStatus)
	}
}
