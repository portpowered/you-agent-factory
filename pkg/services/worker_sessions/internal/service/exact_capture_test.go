package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type restartRecipeStore struct {
	target    recordings.WorkerControlTarget
	execution workers.WorkstationDispatchRequest
	calls     int
	err       error
}

func (store *restartRecipeStore) SaveWorkerRestartRecipe(_ context.Context, target recordings.WorkerControlTarget, execution workers.WorkstationDispatchRequest) error {
	store.calls++
	store.target = target
	store.execution = execution
	return store.err
}

// The recorder and artifact store are controlled collaborators; the component
// under test decides whether one direct execution has reconstructible input.
func TestDirectRestartRecipePreservesInputOrSkipsUnsafeInput(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"safe", "store-failure", "owner-refused", "factory", "env-override", "workflow-context", "sensitive-prompt", "fail-closed", "secret-argument", "escaped-secret-token", "secret-key", "non-json-token"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			r, plan, _ := newDurableInterruptFixture(t)
			store := &restartRecipeStore{}
			r.restart = store
			r.observations["worker"] = &observation{direct: cell != "factory"}
			plan.execution.Execution.Model = "captured-model"
			plan.execution.Execution.ReasoningEffort = "high"
			if cell != "safe" && cell != "store-failure" && cell != "factory" {
				configureUnsafeInterruptRecipe(&plan, cell)
			}
			if cell == "store-failure" {
				store.err = errors.New("sync failed")
			}
			if cell == "owner-refused" {
				store.err = recordings.ErrInvalidRecordingRedactionRequest
			}
			err := r.saveDirectRestartRecipe(t.Context(), workersessions.InvokeSessionRequest{ID: "worker", Execution: plan.execution})
			if cell == "safe" || cell == "store-failure" {
				assertDirectRestartRecipeStored(t, store, plan.dispatchID, err)
			} else if cell == "owner-refused" {
				if store.calls != 1 || err != nil {
					t.Fatalf("owner refusal blocked ordinary invocation: calls=%d error=%v", store.calls, err)
				}
			} else if err != nil || store.calls != 0 {
				t.Fatalf("unreconstructible input must remain invocable without persistence: calls=%d error=%v", store.calls, err)
			}
		})
	}
}

func assertDirectRestartRecipeStored(t *testing.T, store *restartRecipeStore, attemptID string, err error) {
	t.Helper()
	if store.calls != 1 || store.target.ExpectedAttemptID != attemptID || store.target.WorkerSessionID != "worker" || store.execution.Execution.Model != "captured-model" || store.execution.Execution.ReasoningEffort != "high" || !errors.Is(err, store.err) {
		t.Fatalf("recipe persistence: calls=%d target=%+v error=%v", store.calls, store.target, err)
	}
}

type controlCaptureReader struct {
	entry recordings.WorkerSessionCatalogEntry
	err   error
	id    string
}

func (*controlCaptureReader) ListWorkerSessionCaptures(context.Context, recordings.WorkerCapturedCatalogRequest) (recordings.WorkerCapturedCatalogPage, error) {
	panic("exact control identity must not list captured history")
}

func (f *controlCaptureReader) LookupWorkerSessionCapture(_ context.Context, id string) (recordings.WorkerSessionCatalogEntry, error) {
	f.id = id
	return f.entry, f.err
}

func (*controlCaptureReader) ReadWorkerCapturedActivity(context.Context, recordings.WorkerCapturedActivityRequest) (recordings.WorkerCapturedActivityPage, error) {
	panic("opening identity must not read activity")
}

type controlCaptureLifecycle struct {
	awaited bool
	aborted bool
	closed  bool
}

func (c *controlCaptureLifecycle) AwaitOpening(context.Context) error { c.awaited = true; return nil }
func (c *controlCaptureLifecycle) Abort(context.Context, error) error { c.aborted = true; return nil }
func (c *controlCaptureLifecycle) Close(context.Context) error        { c.closed = true; return nil }

func exactCaptureIdentity() recordings.WorkerControlTarget {
	return recordings.WorkerControlTarget{
		RecordingID: "recording", WorkerSessionID: "worker", FactorySessionID: "factory",
		RecordingGenerationID: "generation", OwnerEpoch: "owner",
	}
}

func changeCaptureIdentity(target *recordings.WorkerControlTarget, field string) {
	switch field {
	case "recording":
		target.RecordingID = "other-recording"
	case "worker":
		target.WorkerSessionID = "other-worker"
	case "scope":
		target.FactorySessionID = "other-factory"
	case "generation":
		target.RecordingGenerationID = "other-generation"
	case "epoch":
		target.OwnerEpoch = "other-owner"
	}
}

func TestControlOpeningBindsAcknowledgedCaptureIdentity(t *testing.T) {
	t.Parallel()
	for _, mismatch := range []string{"none", "recording", "worker", "scope", "missing-generation", "missing-epoch", "unavailable"} {
		t.Run(mismatch, func(t *testing.T) {
			t.Parallel()
			r := newTestRegistry(t)
			id := scopedWorkerAddress("worker", "factory")
			r.reserveIfAbsent(id)
			identity := exactCaptureIdentity()
			switch mismatch {
			case "missing-generation":
				identity.RecordingGenerationID = ""
			case "missing-epoch":
				identity.OwnerEpoch = ""
			default:
				changeCaptureIdentity(&identity, mismatch)
			}
			reader := &controlCaptureReader{entry: recordings.WorkerSessionCatalogEntry{
				WorkerSessionID: identity.WorkerSessionID, RecordingID: identity.RecordingID,
				FactorySessionID: identity.FactorySessionID, RecordingGenerationID: identity.RecordingGenerationID, OwnerEpoch: identity.OwnerEpoch,
			}}
			if mismatch == "unavailable" {
				reader.err = errors.New("private directory and secret payload")
			}
			r.logs = &LogReader{reader: reader}
			capture := &controlCaptureLifecycle{}
			err := r.publishOpeningRecord(t.Context(), id, "dispatch", workers.SessionPayload{
				Status: string(workersessions.StateStarting), RecordingID: "recording", FactorySessionID: "factory",
			}, "codex", capture)
			pub := r.publicationFor(id)
			if !capture.awaited || reader.id != "worker" {
				t.Fatal("opening did not await capture and resolve its public identity")
			}
			if mismatch != "none" {
				assertCaptureOpeningRefused(t, err, capture, pub)
				return
			}
			assertCaptureOpeningBound(t, r, id, err, capture, reader)
		})
	}
}

func assertCaptureOpeningRefused(t *testing.T, err error, capture *controlCaptureLifecycle, pub *publication) {
	t.Helper()
	if !errors.Is(err, recordings.ErrWorkerRecordingOpening) || !capture.aborted || pub.open || pub.capture != (recordings.WorkerControlTarget{}) {
		t.Fatalf("invalid capture granted admission: error %v, open %t, identity %+v", err, pub.open, pub.capture)
	}
	if strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("opening leaked storage diagnostics: %v", err)
	}
}

func assertCaptureOpeningBound(t *testing.T, r *registry, id string, err error, capture *controlCaptureLifecycle, reader *controlCaptureReader) {
	t.Helper()
	pub := r.publicationFor(id)
	if err != nil || !pub.open || capture.aborted || pub.capture != exactCaptureIdentity() {
		t.Fatalf("acknowledged opening = %v, open %t, identity %+v", err, pub.open, pub.capture)
	}
	target, err := r.freezeControlTarget(id)
	if err != nil || target.capture != exactCaptureIdentity() || target.publication != pub {
		t.Fatalf("frozen capture = %+v, %v", target, err)
	}
	reader.entry.OwnerEpoch = "later-catalog-owner"
	if target.capture.OwnerEpoch != "owner" || pub.capture.OwnerEpoch != "owner" {
		t.Fatal("later catalog identity replaced admitted control ownership")
	}
	if err := r.publishTerminalRecord(t.Context(), id, "dispatch", workersessions.StateCompleted, workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted}); err != nil {
		t.Fatal(err)
	}
	if !capture.closed || pub.open || pub.capture != target.capture {
		t.Fatal("terminal closure lost the acknowledged capture identity")
	}
}

func TestControlFrozenCaptureRefusesReplacementAfterWait(t *testing.T) {
	t.Parallel()
	for _, action := range []workersessions.ControlAction{workersessions.ControlActionCancel, workersessions.ControlActionTerminate} {
		for _, field := range []string{"recording", "worker", "scope", "generation", "epoch", "publication"} {
			t.Run(string(action)+"/"+field, func(t *testing.T) {
				t.Parallel()
				r := newTestRegistry(t)
				const id = "worker"
				s := newSupervision("dispatch", "")
				s.accepted, s.controlActive = true, true
				s.controlDone = make(chan struct{})
				close(s.controlDone)
				calls := 0
				s.installCancel(func() { calls++ })
				r.supervisions[id] = s
				r.sessions[id] = workersessions.Session{ID: id, State: workersessions.StateRunning}
				r.publications[id] = &publication{open: true, capture: exactCaptureIdentity()}
				target, err := r.freezeControlTarget(id)
				if err != nil {
					t.Fatal(err)
				}
				if _, retry, err := r.cancelControlIteration(t.Context(), workersessions.ControlRequest{ID: id}, action, true, target); !retry || err != nil {
					t.Fatalf("pending control = retry %t, %v", retry, err)
				}
				s.controlActive = false
				if field == "publication" {
					r.publications[id] = &publication{capture: target.capture}
				} else {
					changeCaptureIdentity(&r.publications[id].capture, field)
				}
				result, retry, err := r.cancelControlIteration(t.Context(), workersessions.ControlRequest{ID: id}, action, true, target)
				if retry || !errors.Is(err, workersessions.ErrInvalidState) || result.Outcome != workersessions.ControlOutcomeFailed || result.DispatchID != "dispatch" {
					t.Fatalf("stale capture stop = %+v, retry %t, %v", result, retry, err)
				}
				assertFrozenControlUnaffected(t, r, id, s, calls)
			})
		}
	}
}

func TestControlFrozenRuntimeCaptureRefusesReplacementDuringHistory(t *testing.T) {
	t.Parallel()
	for _, action := range []workersessions.ControlAction{workersessions.ControlActionCancel, workersessions.ControlActionTerminate} {
		for _, field := range []string{"recording", "worker", "scope", "generation", "epoch"} {
			t.Run(string(action)+"/"+field, func(t *testing.T) {
				t.Parallel()
				r, id, attempt, calls := newFrozenRuntimeControlFixture(t)
				r.publications[id].capture = exactCaptureIdentity()
				r.events = &controlReplacementAppender{EventsAppender: r.events, replace: func() {
					// The append callback runs inside this publication's lock.
					changeCaptureIdentity(&r.publications[id].capture, field)
				}}
				var result workersessions.ControlResult
				var err error
				if action == workersessions.ControlActionCancel {
					result, err = r.Cancel(t.Context(), workersessions.ControlRequest{ID: id})
				} else {
					result, err = r.Terminate(t.Context(), workersessions.ControlRequest{ID: id})
				}
				if !errors.Is(err, workersessions.ErrInvalidState) || result.Outcome != workersessions.ControlOutcomeFailed || *calls != 0 || attempt.controlPending || attempt.controlAction != "" {
					t.Fatalf("stale capture reached Runtime: %+v, %v, calls %d", result, err, *calls)
				}
			})
		}
	}
}

func TestControlFrozenRuntimeCaptureRefusesReplacementAfterWait(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"recording", "worker", "scope", "generation", "epoch", "publication"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			r, id, attempt, calls := newFrozenRuntimeControlFixture(t)
			r.publications[id].capture = exactCaptureIdentity()
			target, err := r.freezeControlTarget(id)
			if err != nil {
				t.Fatal(err)
			}
			attempt.controlPending = true
			attempt.controlDone = make(chan struct{})
			claimed, wait, completed, err := r.claimFrozenRuntimeControl(id, target)
			if err != nil || claimed || wait != attempt.controlDone || completed != nil {
				t.Fatalf("pending Runtime control = %t, %v, %v, %v", claimed, wait, completed, err)
			}
			if field == "publication" {
				r.publications[id] = &publication{capture: target.capture}
			} else {
				changeCaptureIdentity(&r.publications[id].capture, field)
			}
			attempt.resolveControl(workersessions.ControlActionCancel, "", errors.New("prior control failed"))
			<-wait
			result, complete, err := r.awaitRuntimeAttemptControl(id, workersessions.ControlActionTerminate, target)
			if !complete || !errors.Is(err, workersessions.ErrInvalidState) || result.Outcome != workersessions.ControlOutcomeFailed || *calls != 0 || attempt.controlPending {
				t.Fatalf("stale Runtime capture after wait = %+v, %t, %v, calls %d", result, complete, err, *calls)
			}
		})
	}
}
