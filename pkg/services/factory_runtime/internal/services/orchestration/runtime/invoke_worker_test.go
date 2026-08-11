package runtime

import (
	"context"
	"testing"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// TestInvokeWorker_ARerunDispatchReachesWorkersUnderItsOwnIdentity pins the
// identity a Worker carries into Workers.
//
// A JavaScript workflow resumed after an interruption re-runs the child that
// was cut off under its original orchestrator-minted dispatch ID. Workers
// treats a dispatch ID as single-use for the whole life of its pool -- an
// accepted dispatch is never removed from the pool's record map -- so a re-run
// that reuses that ID is refused before it reaches an executor, and the caller
// sees START_FAILURE rather than a second Worker.
//
// The Worker Session identity is the one Runtime already mints uniquely, so it
// is the identity Workers is given. What the caller sees is unchanged: its own
// dispatch ID comes back on the result, because that is the identity its own
// records are keyed by.
func TestInvokeWorker_ARerunDispatchReachesWorkersUnderItsOwnIdentity(t *testing.T) {
	boundary := newControlledWorkstationBoundary()
	impl := newInvokeWorkerTestFactory(t, boundary)

	firstWorkersID, first := runOneInvokeWorker(t, impl, boundary, "child-1")
	secondWorkersID, second := runOneInvokeWorker(t, impl, boundary, "child-1")

	if secondWorkersID == firstWorkersID {
		t.Fatalf(
			"re-run Workers dispatch ID = %q, want an identity distinct from the first attempt's %q",
			secondWorkersID,
			firstWorkersID,
		)
	}
	if secondWorkersID != second.WorkerSessionID {
		t.Fatalf(
			"re-run Workers dispatch ID = %q, want the reserved Worker Session identity %q",
			secondWorkersID,
			second.WorkerSessionID,
		)
	}
	for _, result := range []factory.InvokeWorkerResult{first, second} {
		if result.DispatchID != "child-1" {
			t.Fatalf("result dispatch ID = %q, want the caller's own %q", result.DispatchID, "child-1")
		}
		if result.Outcome != factory.InvokeWorkerOutcomeCompleted {
			t.Fatalf("result outcome = %q, want COMPLETED", result.Outcome)
		}
	}
}

// TestInvokeWorker_FirstAttemptUsesTheCallerDispatchIdentity keeps the common
// case honest: an uncontended Worker still reaches Workers under exactly the
// identity its caller minted, so the resume suffix above is visibly the
// exception rather than the rule.
func TestInvokeWorker_FirstAttemptUsesTheCallerDispatchIdentity(t *testing.T) {
	boundary := newControlledWorkstationBoundary()
	impl := newInvokeWorkerTestFactory(t, boundary)

	workersID, result := runOneInvokeWorker(t, impl, boundary, "child-1")
	if workersID != "child-1" {
		t.Fatalf("Workers dispatch ID = %q, want the caller's own %q", workersID, "child-1")
	}
	if result.WorkerSessionID != "child-1" {
		t.Fatalf("Worker Session ID = %q, want %q", result.WorkerSessionID, "child-1")
	}
}

// TestInvokeWorker_CarriesTheAuthoredWorkerNameAndPermissionPolicy pins the
// two facts a Worker with no authored workstation can only get from its
// caller.
//
// The worker name is what --with-mock-workers matches a named preset on, at
// the subprocess boundary, so a Worker that arrives without it is never the
// mock the operator configured. The permission policy is the invocation
// -effective one the caller already resolved; dropping it runs the Worker
// under a policy its own dispatch record says it does not have.
func TestInvokeWorker_CarriesTheAuthoredWorkerNameAndPermissionPolicy(t *testing.T) {
	boundary := newControlledWorkstationBoundary()
	impl := newInvokeWorkerTestFactory(t, boundary)
	capabilities := &workers.Capabilities{NativeStreaming: true, ToolLifecycle: true}

	observed, _ := runInvokeWorker(t, impl, boundary, factory.InvokeWorkerRequest{
		DispatchID:      "child-1",
		Prompt:          "run",
		WorkerName:      "worker-a",
		SkipPermissions: true,
		RecordingID:     "recording-1",
		Capabilities:    capabilities,
	})
	if observed.Execution.WorkerType != "worker-a" {
		t.Fatalf("Workers worker type = %q, want the authored worker name %q", observed.Execution.WorkerType, "worker-a")
	}
	if observed.Execution.Dispatch.WorkerType != "worker-a" {
		t.Fatalf(
			"dispatch worker type = %q, want the authored worker name %q",
			observed.Execution.Dispatch.WorkerType,
			"worker-a",
		)
	}
	if !observed.Execution.SkipPermissions {
		t.Fatal("Workers skip-permissions = false, want the caller's resolved policy")
	}
	if observed.Execution.RecordingID != "recording-1" {
		t.Fatalf("Workers recording ID = %q, want recording-1", observed.Execution.RecordingID)
	}
	if observed.Execution.Capabilities == nil || !observed.Execution.Capabilities.NativeStreaming || !observed.Execution.Capabilities.ToolLifecycle {
		t.Fatalf("Workers capabilities = %+v, want caller-supplied capability facts", observed.Execution.Capabilities)
	}
}

// runOneInvokeWorker drives one minimal InvokeWorker to its terminal result
// and reports the dispatch identity Workers actually observed.
func runOneInvokeWorker(
	t *testing.T,
	impl *factoryImpl,
	boundary *controlledWorkstationBoundary,
	dispatchID string,
) (string, factory.InvokeWorkerResult) {
	t.Helper()
	observed, result := runInvokeWorker(t, impl, boundary, factory.InvokeWorkerRequest{
		DispatchID: dispatchID,
		Prompt:     "run",
	})
	return observed.Execution.Dispatch.DispatchID, result
}

// runInvokeWorker drives one InvokeWorker to its terminal result and reports
// the request Workers actually observed alongside it.
func runInvokeWorker(
	t *testing.T,
	impl *factoryImpl,
	boundary *controlledWorkstationBoundary,
	req factory.InvokeWorkerRequest,
) (workers.WorkstationDispatchRequest, factory.InvokeWorkerResult) {
	t.Helper()
	type outcome struct {
		result factory.InvokeWorkerResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := impl.InvokeWorker(context.Background(), req)
		done <- outcome{result: result, err: err}
	}()
	request := awaitCanonicalWorkersRequest(t, boundary.requests)
	boundary.results <- completedWorkersResult(request)
	got := <-done
	if got.err != nil {
		t.Fatalf("InvokeWorker(%q): %v", req.DispatchID, got.err)
	}
	return request, got.result
}
