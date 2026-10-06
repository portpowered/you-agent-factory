package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestTerminalCaptureKeepsExactAttemptReferenceWithoutCredentials(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"exact", "foreign-attempt", "inherited-secret", "override-secret", "missing"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			req := continuationReservationRequest()
			r := newContinuationSource(t, req)
			supervision := newSupervision("dispatch-1", "turn-1", continuationValidExecution("dispatch-1"))
			r.supervisions[req.SourceWorkerSessionID] = supervision
			attemptID := "dispatch-1"
			if cell == "foreign-attempt" {
				attemptID = "other"
			}
			if cell == "inherited-secret" {
				supervision.execution.Execution.ProcessEnvironment = []string{"API_KEY=provider-session-1"}
			}
			if cell == "override-secret" {
				supervision.execution.Execution.EnvVars = map[string]string{"API_KEY": "provider-session-1"}
			}
			if cell == "missing" {
				source := r.sessions[req.SourceWorkerSessionID]
				source.ProviderSessionAssociation = nil
				r.sessions[source.ID] = source
			}
			draft, err := terminalDraft(workersessions.StateCanceled, workersessions.TerminalResult{}, attemptID)
			if err != nil {
				t.Fatal(err)
			}
			r.captureTerminalProviderReference(req.SourceWorkerSessionID, attemptID, &draft)
			var payload workers.SessionPayload
			if err := json.Unmarshal(draft.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Status != "CANCELED" {
				t.Fatal("reference capture changed terminal outcome")
			}
			if cell == "exact" {
				if payload.Continuation == nil || payload.Continuation.ID != "provider-session-1" || payload.Continuation.Provider != "codex" || draft.DispatchID != "dispatch-1" {
					t.Fatalf("terminal lost exact native reference: %+v", draft)
				}
			} else if payload.Continuation != nil {
				t.Fatal("terminal captured an unproved or secret-bearing reference")
			}
		})
	}
}

type restartRecipeStore struct {
	target    recordings.WorkerControlTarget
	execution workers.WorkstationDispatchRequest
	calls     int
	err       error
	reference providers.SessionRef
	input     func() json.RawMessage
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

func (store *restartRecipeStore) ReadWorkerRestartRecipe(context.Context, recordings.WorkerControlTarget) (workers.WorkstationDispatchRequest, error) {
	return store.execution, store.err
}

func (store *restartRecipeStore) ReadWorkerContinuationSource(context.Context, recordings.WorkerControlTarget) (recordings.WorkerContinuationSource, error) {
	return recordings.WorkerContinuationSource{Execution: store.execution, Reference: store.reference, Terminal: recordings.WorkerRecordingTerminal{Status: "COMPLETED"}}, store.err
}

// Reservation uses detached captured settings with controlled storage and
// capture lookup collaborators, without opening a provider execution.
func TestContinuationReservationUsesCapturedRecipe(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"captured", "missing", "wrong-attempt", "wrong-scope", "wrong-reference"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			req := continuationReservationRequest()
			r := newContinuationSource(t, req)
			r.supervisions[req.SourceWorkerSessionID] = newSupervision("dispatch-1", "turn-1", continuationValidExecution("dispatch-1"))
			r.supervisions[req.SourceWorkerSessionID].execution.Execution.ProcessEnvironment = []string{"API_KEY=live-host-secret"}
			r.observations[req.SourceWorkerSessionID] = &observation{direct: true}
			store := &restartRecipeStore{execution: continuationValidExecution("dispatch-1")}
			store.reference = r.sessions[req.SourceWorkerSessionID].ProviderSessionAssociation.Reference
			store.execution.Execution.Model = "captured-model"
			store.execution.Execution.WorkingDirectory = "captured-workspace"
			reader := &controlCaptureReader{entry: recordings.WorkerSessionCatalogEntry{
				WorkerSessionID: req.SourceWorkerSessionID, RecordingID: "recording", Origin: "direct",
				RecordingGenerationID: "generation", OwnerEpoch: "owner",
			}}
			r.logs = &LogReader{reader: reader}
			r.restart = store
			switch cell {
			case "missing":
				store.err = errors.New("private missing artifact")
			case "wrong-attempt":
				store.execution.Execution.Dispatch.DispatchID = "other"
			case "wrong-scope":
				reader.entry.FactorySessionID = "other"
			case "wrong-reference":
				store.reference.ID = "foreign-provider-session"
			}
			replay, owner, err := r.reserveContinuation(req)
			if cell != "captured" {
				expected := workersessions.ErrContinuationExecutionUnavailable
				if cell == "wrong-reference" {
					expected = workersessions.ErrContinuationProviderSessionInvalid
				}
				if !errors.Is(err, expected) || owner || replay != nil || len(r.sessions) != 1 {
					t.Fatalf("invalid recipe reserved successor: %+v, %t, %v", replay, owner, err)
				}
				return
			}
			assertCapturedContinuationPlan(t, replay, owner, err, req)
		})
	}
}

func assertCapturedContinuationPlan(t *testing.T, replay *continueReplay, owner bool, err error, req workersessions.ContinueRequest) {
	t.Helper()
	if err != nil || !owner || replay.plan.execution.Execution.Model != "captured-model" || replay.plan.execution.Execution.WorkingDirectory != "captured-workspace" || replay.plan.execution.Execution.UserMessage != req.FollowUpInput {
		t.Fatalf("captured settings lost: %+v, %t, %v", replay, owner, err)
	}
	if reference := replay.plan.execution.Execution.Continuation; reference == nil || reference.ProviderSessionID != "provider-session-1" {
		t.Fatalf("captured execution lost exact native identity: %+v", reference)
	}
	if environment := replay.plan.execution.Execution.ProcessEnvironment; len(environment) != 1 || environment[0] != "API_KEY=live-host-secret" {
		t.Fatal("continuation lost the live host environment")
	}
}

func (store *restartRecipeStore) ReadWorkerContinuationInput(context.Context, recordings.WorkerControlOperationKey) (json.RawMessage, error) {
	if store.input != nil {
		return store.input(), store.err
	}
	return nil, store.err
}

func TestContinuationInputBarrierPreservesTupleOrRefusesAdmission(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"exact", "sync-failure", "conflict", "secret-input", "unsafe-override", "stale-capture"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			req := continuationReservationRequest()
			r := newContinuationSource(t, req)
			base := continuationValidExecution("dispatch-1")
			ref := r.sessions[req.SourceWorkerSessionID].ProviderSessionAssociation.Reference
			plan := continuePlan{
				request: req, direct: true,
				execution: continuationExecution(base, "dispatch-1/continue/successor-1", req.FollowUpInput, ref),
				lineage:   &workers.SessionLineage{PreviousAttemptID: "dispatch-1"},
			}
			store := &interruptInputStore{}
			r.operations = store
			r.restart = &restartRecipeStore{input: func() json.RawMessage { return store.input }}
			r.publications[req.SourceWorkerSessionID] = &publication{capture: recordings.WorkerControlTarget{
				WorkerSessionID: req.SourceWorkerSessionID, RecordingID: "recording", RecordingGenerationID: "generation", OwnerEpoch: "owner",
			}}
			r.logs = &LogReader{reader: &controlCaptureReader{entry: recordings.WorkerSessionCatalogEntry{
				WorkerSessionID: req.SourceWorkerSessionID, RecordingID: "recording", RecordingGenerationID: "generation", OwnerEpoch: "owner",
			}}}
			switch cell {
			case "sync-failure":
				store.writeErr = errors.New("private persistence detail")
			case "conflict":
				store.writeErr = recordings.ErrWorkerControlConflict
			case "secret-input":
				plan.execution.Execution.ProcessEnvironment = []string{"API_KEY=follow up"}
			case "unsafe-override":
				plan.execution.Execution.EnvVars = map[string]string{"API_KEY": "private"}
			case "stale-capture":
				r.publications[req.SourceWorkerSessionID].capture.RecordingGenerationID = "foreign-generation"
			}
			err := r.persistContinuationInput(plan)
			if cell == "exact" {
				assertContinuationInputTuple(t, store.input, req, ref.ID, err)
				return
			}
			if err == nil || len(store.input) != 0 {
				t.Fatalf("unsafe/unacknowledged input persisted: %s, %v", store.input, err)
			}
			if cell == "conflict" && !errors.Is(err, workersessions.ErrContinuationRequestIDConflict) {
				t.Fatalf("tuple conflict = %v", err)
			}
			// A failing persistence barrier returns before invocation preparation,
			// even with no executor; no opening or provider call is possible.
			if _, err := r.continueReserved(plan); err == nil {
				t.Fatal("failed barrier admitted continuation")
			}
		})
	}
}

func assertContinuationInputTuple(t *testing.T, payload []byte, req workersessions.ContinueRequest, referenceID string, err error) {
	t.Helper()
	var input durableContinuationInput
	if err != nil || json.Unmarshal(payload, &input) != nil || input.RequestID != req.RequestID || input.FollowUpInput != req.FollowUpInput || input.SuccessorWorkerSessionID != req.SuccessorWorkerSessionID || input.ProviderReference.ID != referenceID || input.Target.ExpectedAttemptID != "dispatch-1" || input.Execution.Execution.Dispatch.DispatchID != "dispatch-1/continue/successor-1" {
		t.Fatalf("captured tuple lost identity: %+v, %v", input, err)
	}
	assertContinuationInputSchema(t, payload)
}

// Contract proof for the encoder's actual detached output, including the
// source target and successor execution, rather than a hand-authored fixture.
func assertContinuationInputSchema(t *testing.T, payload []byte) {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	for _, name := range []string{"continuation-input.v1.schema.json", "control-envelope.v2.schema.json", "control-operation.v1.schema.json"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "..", "recordings", "internal", "services", "worker_capture", "schemas", name))
		if err != nil {
			t.Fatal(err)
		}
		var document any
		if err := json.Unmarshal(data, &document); err != nil {
			t.Fatal(err)
		}
		if err := compiler.AddResource(name, document); err != nil {
			t.Fatal(err)
		}
	}
	schema, err := compiler.Compile("continuation-input.v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("continuation tuple violates published schema: %v", err)
	}
}
