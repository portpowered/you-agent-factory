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
	assertInterruptedResumePreconditions(t, harness)

	firstDispatchBeforeResume := getCompletedDispatch(t, harness.initial, harness.sessionID, "dispatch-1")
	resumedService := resumeInterruptedHarness(t, harness, "req-runtime-resume-interrupted-resume-001")
	waitUntilSessionStatus(t, resumedService, harness.sessionID, fse.LifecycleStatusSucceeded, 5*time.Second)

	assertProviderCallCount(t, harness.provider, 3)
	assertResumedDispatchParity(t, resumedService, harness.sessionID, firstDispatchBeforeResume)
	assertFinalResult(t, resumedService, harness.sessionID)
}

func assertInterruptedResumePreconditions(t *testing.T, harness interruptedResumableHarness) {
	t.Helper()
	if harness.provider.CallCount() < 2 {
		t.Fatalf("provider infer calls = %d, want at least 2 before interrupt", harness.provider.CallCount())
	}
	snapshotPath := filepath.Join(runtimepersist.DirForProjectRoot(harness.projectRoot), harness.sessionID+".json")
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Fatalf("interrupted snapshot must be durable before cross-instance resume: %v", err)
	}
	getCompletedDispatch(t, harness.initial, harness.sessionID, "dispatch-1")
}

func getCompletedDispatch(t *testing.T, service fse.Service, sessionID, dispatchID string) fse.DispatchDetail {
	t.Helper()
	dispatch, err := service.GetDispatch(context.Background(), sessionID, dispatchID)
	if err != nil {
		t.Fatalf("GetDispatch %s: %v", dispatchID, err)
	}
	if dispatch.Status != fse.DispatchStatusCompleted {
		t.Fatalf("dispatch %s = %#v, want COMPLETED", dispatchID, dispatch)
	}
	return dispatch
}

func assertProviderCallCount(t *testing.T, provider *sequentialBlockingProvider, want int) {
	t.Helper()
	if provider.CallCount() != want {
		t.Fatalf("provider infer calls = %d, want %d", provider.CallCount(), want)
	}
}

func assertResumedDispatchParity(
	t *testing.T,
	service *fse.JavaScriptRuntimeService,
	sessionID string,
	firstDispatchBeforeResume fse.DispatchDetail,
) {
	t.Helper()
	firstDispatchAfterResume := getCompletedDispatch(t, service, sessionID, "dispatch-1")
	if firstDispatchAfterResume.ID != firstDispatchBeforeResume.ID {
		t.Fatalf("dispatch-1 id changed across resume: %q -> %q", firstDispatchBeforeResume.ID, firstDispatchAfterResume.ID)
	}
	getCompletedDispatch(t, service, sessionID, "dispatch-2")
}

func assertFinalResult(t *testing.T, service *fse.JavaScriptRuntimeService, sessionID string) {
	t.Helper()
	result, err := service.GetResult(context.Background(), sessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != fse.ResultStatusFinal {
		t.Fatalf("result status = %q, want FINAL", result.ResultStatus)
	}
}
