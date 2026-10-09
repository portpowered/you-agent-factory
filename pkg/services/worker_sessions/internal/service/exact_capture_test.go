package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
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

func (store *restartRecipeStore) ValidateWorkerRestartRecipe(context.Context, string, workers.WorkstationDispatchRequest) error {
	return store.err
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

// The interrupt preflight component reads detached settings through explicit
// store/catalog collaborators; no execution or transport is assembled here.
func TestInterruptPreflightUsesCapturedRecipeBeforeIntent(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"captured", "scoped", "missing", "unsafe", "wrong-attempt", "recipe-scope", "catalog-worker", "catalog-recording", "catalog-scope", "catalog-generation", "catalog-owner", "inherited-secret"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			r, plan, journal := newDurableInterruptFixture(t)
			r.observations["worker"] = &observation{direct: true}
			capture := r.publications["worker"].capture
			reader := &controlCaptureReader{entry: recordings.WorkerSessionCatalogEntry{
				WorkerSessionID: capture.WorkerSessionID, RecordingID: capture.RecordingID,
				RecordingGenerationID: capture.RecordingGenerationID, OwnerEpoch: capture.OwnerEpoch,
			}}
			r.logs = &LogReader{reader: reader}
			store := &restartRecipeStore{execution: cloneWorkstationDispatchRequest(plan.execution)}
			store.execution.Execution.Model = "captured-model"
			store.execution.Execution.Args = []string{"--captured-option"}
			r.restart = store
			plan.execution.Execution.Model = "changed-live-model"
			plan.execution.Execution.ProcessEnvironment = []string{"API_KEY=private-live-credential"}
			configureInterruptPreflightCell(cell, r, &plan, store, reader)
			frozen, err := r.freezeInterruptInput(t.Context(), plan)
			var operation *recordings.WorkerControlOperationRecord
			if err == nil {
				operation, err = r.beginInterruptIntent(t.Context(), frozen)
			}
			if cell != "captured" && cell != "scoped" {
				if !errors.Is(err, workersessions.ErrInterruptExecutionUnavailable) || operation != nil || len(journal.input) != 0 || len(journal.records) != 0 {
					t.Fatalf("invalid captured recipe committed intent: operation=%+v err=%v", operation, err)
				}
				if strings.Contains(err.Error(), "private-") || r.sessions["worker"].State != workersessions.StateRunning {
					t.Fatal("preflight leaked storage details or changed source state")
				}
				return
			}
			assertCapturedInterruptIntent(t, plan, operation, journal.input, err)
		})
	}
}

func assertCapturedInterruptIntent(t *testing.T, plan interruptPlan, operation *recordings.WorkerControlOperationRecord, payload json.RawMessage, err error) {
	t.Helper()
	var input durableInterruptInput
	if err != nil || operation == nil || json.Unmarshal(payload, &input) != nil || input.Execution.Execution.Model != "captured-model" ||
		len(input.Execution.Execution.Args) != 1 || input.Execution.Execution.Args[0] != "--captured-option" || input.ReplacementMessage != plan.request.ReplacementMessage {
		t.Fatalf("intent lost captured settings: input=%+v err=%v", input, err)
	}
	if strings.Contains(string(payload), "private-live-credential") || strings.Contains(string(payload), "changed-live-model") {
		t.Fatal("intent used live settings or persisted inherited credentials")
	}
}

func configureInterruptPreflightCell(cell string, r *registry, plan *interruptPlan, store *restartRecipeStore, reader *controlCaptureReader) {
	switch cell {
	case "scoped":
		capture := r.publications["worker"].capture
		capture.FactorySessionID = "factory"
		r.publications["worker"].capture = capture
		plan.execution.Execution.FactorySessionID = "factory"
		store.execution.Execution.FactorySessionID = "factory"
		reader.entry.FactorySessionID = "factory"
	case "missing":
		store.err = errors.New("private-storage-detail")
	case "unsafe":
		store.execution.Execution.EnvVars = map[string]string{"CUSTOM": "private-override"}
	case "wrong-attempt":
		store.execution.Execution.Dispatch.DispatchID = "other"
	case "recipe-scope":
		store.execution.Execution.FactorySessionID = "foreign"
	case "catalog-worker":
		reader.entry.WorkerSessionID = "foreign"
	case "catalog-recording":
		reader.entry.RecordingID = "foreign"
	case "catalog-scope":
		reader.entry.FactorySessionID = "foreign"
	case "catalog-generation":
		reader.entry.RecordingGenerationID = "foreign"
	case "catalog-owner":
		reader.entry.OwnerEpoch = "foreign"
	case "inherited-secret":
		store.execution.Execution.Model = "private-live-credential"
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
	for _, cell := range []string{"captured", "captured-scoped", "missing", "wrong-attempt", "wrong-scope", "recipe-scope", "wrong-reference"} {
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
			case "captured-scoped":
				store.execution.Execution.FactorySessionID = "factory"
				r.supervisions[req.SourceWorkerSessionID].execution.Execution.FactorySessionID = "factory"
				reader.entry.FactorySessionID = "factory"
			case "missing":
				store.err = errors.New("private missing artifact")
			case "wrong-attempt":
				store.execution.Execution.Dispatch.DispatchID = "other"
			case "wrong-scope":
				reader.entry.FactorySessionID = "other"
			case "recipe-scope":
				store.execution.Execution.FactorySessionID = "foreign"
			case "wrong-reference":
				store.reference.ID = "foreign-provider-session"
			}
			replay, owner, err := r.reserveContinuation(req)
			if cell != "captured" && cell != "captured-scoped" {
				expected := workersessions.ErrContinuationExecutionUnavailable
				if cell == "wrong-reference" {
					expected = workersessions.ErrContinuationProviderSessionInvalid
				}
				if !errors.Is(err, expected) || owner || replay != nil || len(r.sessions) != 1 {
					t.Fatalf("invalid recipe reserved successor: %+v, %t, %v", replay, owner, err)
				}
				return
			}
			assertCapturedContinuationPlan(t, replay, owner, err, req, reader.entry.FactorySessionID)
		})
	}
}

func assertCapturedContinuationPlan(t *testing.T, replay *continueReplay, owner bool, err error, req workersessions.ContinueRequest, scope string) {
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
	if replay.plan.execution.Execution.FactorySessionID != scope {
		t.Fatal("continuation changed captured Factory Session scope")
	}
}

func (store *restartRecipeStore) ReadWorkerContinuationInput(context.Context, recordings.WorkerControlOperationKey) (json.RawMessage, error) {
	if store.input != nil {
		input := store.input()
		if len(input) == 0 && store.err == nil {
			return nil, os.ErrNotExist
		}
		return input, store.err
	}
	return nil, store.err
}

func TestContinuationInputBarrierPreservesTupleOrRefusesAdmission(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"exact", "sync-failure", "conflict", "secret-input", "unsafe-override", "stale-capture", "wrong-dispatch", "wrong-scope", "missing-owner"} {
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
			configureContinuationBarrierFailure(r, &plan, store, cell)
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

func configureContinuationBarrierFailure(r *registry, plan *continuePlan, store *interruptInputStore, cell string) {
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
		r.publications[plan.request.SourceWorkerSessionID].capture.RecordingGenerationID = "foreign-generation"
	case "wrong-dispatch":
		plan.execution.Execution.Dispatch.DispatchID = "foreign-dispatch"
	case "wrong-scope":
		plan.execution.Execution.FactorySessionID = "foreign-scope"
	case "missing-owner":
		r.publications[plan.request.SourceWorkerSessionID].capture.OwnerEpoch = ""
		r.logs.reader.(*controlCaptureReader).entry.OwnerEpoch = ""
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

// The decoder observes retained data, not a live execution handle. Exercise
// corruption and scope fencing before recovery can use any decoded execution.
func TestContinuationInputDecoderRejectsCorruptionAndChangedTuple(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"exact", "formatted", "duplicate", "nested-duplicate", "alias", "unknown", "trailing", "version", "missing-reference", "wrong-reference", "wrong-dispatch", "wrong-scope", "unsafe-override", "missing-target", "stale-generation", "changed-input", "changed-successor"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			req, plan, target, payload := continuationInputDecoderFixture(t)
			payload = corruptContinuationInput(t, payload, cell)
			expected := recordings.ErrWorkerRecordingPersistence
			switch cell {
			case "stale-generation":
				target.RecordingGenerationID = "changed-generation"
			case "changed-input":
				req.FollowUpInput += " changed"
				expected = workersessions.ErrContinuationRequestIDConflict
			case "changed-successor":
				req.SuccessorWorkerSessionID = "different-successor"
				expected = workersessions.ErrContinuationRequestIDConflict
			}
			input, err := decodeContinuationInput(payload, req, target)
			if cell == "exact" || cell == "formatted" {
				if err != nil || input.Execution.Execution.Model != plan.execution.Execution.Model || input.FollowUpInput != req.FollowUpInput || len(input.Execution.Execution.ProcessEnvironment) != 0 || !reflect.DeepEqual(input.Execution.Execution.Continuation, plan.execution.Execution.Continuation) {
					t.Fatalf("detached input lost settings or identity: %+v, %v", input, err)
				}
				return
			}
			if !errors.Is(err, expected) || !reflect.DeepEqual(input, durableContinuationInput{}) {
				t.Fatalf("invalid retained tuple decoded: %+v, %v", input, err)
			}
		})
	}
}

func continuationInputDecoderFixture(t *testing.T) (workersessions.ContinueRequest, continuePlan, recordings.WorkerControlTarget, []byte) {
	t.Helper()
	req := continuationReservationRequest()
	target := exactCaptureIdentity()
	target.WorkerSessionID, target.FactorySessionID, target.ExpectedAttemptID = req.SourceWorkerSessionID, "", "dispatch-1"
	ref := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "captured-native-id"}
	plan := continuePlan{
		request: req, direct: true,
		execution: continuationExecution(continuationValidExecution("dispatch-1"), continuationDispatchID(target.ExpectedAttemptID, req.SuccessorWorkerSessionID), req.FollowUpInput, ref),
		lineage:   &workers.SessionLineage{PreviousAttemptID: target.ExpectedAttemptID},
	}
	plan.execution.Execution.Model = "captured-model"
	plan.execution.Execution.ProcessEnvironment = []string{"API_KEY=private-host-credential"}
	payload, err := encodeContinuationInput(plan, target)
	if err != nil {
		t.Fatal(err)
	}
	return req, plan, target, payload
}

func corruptContinuationInput(t *testing.T, payload []byte, cell string) []byte {
	t.Helper()
	switch cell {
	case "formatted":
		return []byte("\n " + string(payload) + " \n")
	case "duplicate":
		return []byte(strings.Replace(string(payload), `"version":1`, `"version":0,"version":1`, 1))
	case "nested-duplicate":
		return []byte(strings.Replace(string(payload), `"id":"captured-native-id"`, `"id":"foreign","id":"captured-native-id"`, 1))
	case "alias":
		return []byte(strings.Replace(string(payload), `"followUpInput"`, `"FollowUpInput"`, 1))
	case "unknown":
		return []byte(strings.Replace(string(payload), `"version":1`, `"version":1,"privateHandle":"secret"`, 1))
	case "wrong-reference":
		return []byte(strings.Replace(string(payload), `"Execution":{`, `"Execution":{"Continuation":{"ProviderSessionID":"foreign-native-id"},`, 1))
	case "trailing":
		return append(payload, []byte(`{}`)...)
	}
	var input durableContinuationInput
	if err := json.Unmarshal(payload, &input); err != nil {
		t.Fatal(err)
	}
	mutateContinuationInput(&input, cell)
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func mutateContinuationInput(input *durableContinuationInput, cell string) {
	switch cell {
	case "version":
		input.Version++
	case "missing-reference":
		input.ProviderReference = interruptInputReference{}
	case "wrong-dispatch":
		input.Execution.Execution.Dispatch.DispatchID = "different-dispatch"
	case "wrong-scope":
		input.Execution.Execution.FactorySessionID = "foreign-scope"
	case "unsafe-override":
		input.Execution.Execution.EnvVars = map[string]string{"API_KEY": "private-credential"}
	case "missing-target":
		input.Target.OwnerEpoch = ""
	}
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

type interruptContinuationSupportFake struct {
	supported bool
	err       error
	reference providers.SessionRef
}

func TestInterruptFactoryOriginRefusesBeforeControlEffects(t *testing.T) {
	t.Parallel()
	for _, hasReference := range []bool{false, true} {
		t.Run(fmt.Sprint(hasReference), func(t *testing.T) {
			t.Parallel()
			r, supervision := newRunningPauseRegistry(t)
			r.runtimeAttempts["worker-1"] = struct{}{}
			if !hasReference {
				source := r.sessions["worker-1"]
				source.ProviderSessionAssociation = nil
				r.sessions["worker-1"] = source
			}
			source := r.sessions["worker-1"].Clone()
			query := &interruptContinuationSupportFake{supported: true}
			r.continuationSupport = query
			result, err := r.Interrupt(t.Context(), workersessions.InterruptRequest{
				RequestID: "factory-refusal", SourceWorkerSessionID: source.ID,
				SuccessorWorkerSessionID: "successor-session", ReplacementMessage: "replacement",
			})
			if !errors.Is(err, workersessions.ErrInterruptFactoryUnsupported) || result.Accepted || result.Phase != workersessions.InterruptPhaseValidation {
				t.Fatalf("Factory refusal result=%#v err=%v", result, err)
			}
			if !reflect.DeepEqual(r.sessions[source.ID], source) || supervision.interrupting || supervision.controlHistory != nil ||
				r.activeStarts != 0 || len(r.interruptReplays) != 0 || len(r.publications[source.ID].accepted) != 0 || query.reference != (providers.SessionRef{}) {
				t.Fatal("Factory refusal changed source, published control history or reserved execution")
			}
			if _, err := r.Get(t.Context(), workersessions.GetRequest{ID: "successor-session"}); !errors.Is(err, workersessions.ErrSessionNotFound) {
				t.Fatalf("successor reserved: %v", err)
			}
		})
	}
}

func TestInterruptDirectFactoryCorrelationPreservesEligibility(t *testing.T) {
	t.Parallel()
	r, supervision := newRunningPauseRegistry(t)
	supervision.execution.Execution.FactorySessionID = "correlated-factory-session"
	if err := r.interruptOrigin("worker-1"); err != nil {
		t.Fatalf("direct correlation changed ownership: %v", err)
	}
}

func (fake *interruptContinuationSupportFake) SupportsContinuation(_ context.Context, reference providers.SessionRef) (bool, error) {
	fake.reference = reference
	return fake.supported, fake.err
}

func TestInterruptContinuationPolicyRefusalPreservesSource(t *testing.T) {
	t.Parallel()
	for _, cause := range []error{nil, providers.ErrUnknownProvider} {
		t.Run(fmt.Sprint(cause), func(t *testing.T) {
			t.Parallel()
			service, supervision := newRunningPauseRegistry(t)
			reference := service.sessions["worker-1"].ProviderSessionAssociation.Reference
			fake := &interruptContinuationSupportFake{err: cause}
			service.continuationSupport = fake
			result, err := service.Interrupt(t.Context(), workersessions.InterruptRequest{RequestID: "policy-refusal", SourceWorkerSessionID: "worker-1", SuccessorWorkerSessionID: "successor-session", ReplacementMessage: "follow-up"})
			want := workersessions.ErrInterruptContinuationUnsupported
			if cause != nil {
				want = workersessions.ErrInterruptProviderSessionInvalid
			}
			if !errors.Is(err, want) || result.Accepted || result.Phase != workersessions.InterruptPhaseValidation || result.Source.State != workersessions.StateRunning || fake.reference != reference {
				t.Fatalf("policy refusal: result=%#v err=%v reference=%#v", result, err, fake.reference)
			}
			if supervision.interrupting || service.activeStarts != 0 || len(service.continueReplays) != 0 || len(service.interruptReplays) != 0 {
				t.Fatal("policy refusal reserved execution or continuation")
			}
			if _, err := service.Get(t.Context(), workersessions.GetRequest{ID: "successor-session"}); !errors.Is(err, workersessions.ErrSessionNotFound) {
				t.Fatalf("successor reserved: %v", err)
			}
		})
	}
}

func TestTerminalContinuationPolicyPrecedesReservation(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"unsupported", "query-error", "supported"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			req := continuationReservationRequest()
			r := newContinuationSource(t, req)
			r.supervisions[req.SourceWorkerSessionID] = newSupervision("dispatch-1", "turn-1", continuationValidExecution("dispatch-1"))
			source := r.sessions[req.SourceWorkerSessionID].Clone()
			fake := &interruptContinuationSupportFake{supported: cell == "supported"}
			if cell == "query-error" {
				fake.err = providers.ErrUnknownProvider
			}
			r.continuationSupport = fake
			replay, owner, err := r.reserveContinuation(req)
			if cell == "supported" {
				if err != nil {
					t.Fatal(err)
				}
				if replay == nil || !owner {
					t.Fatalf("supported reservation: replay=%#v owner=%v err=%v", replay, owner, err)
				}
				assertContinuationReplayRetainsOriginalPolicy(t, r, req, replay, fake)
				return
			}
			if !errors.Is(err, workersessions.ErrContinuationProviderSessionInvalid) || replay != nil || owner {
				t.Fatalf("unsupported reservation: replay=%#v owner=%v err=%v", replay, owner, err)
			}
			assertTerminalContinuationPolicyNoEffects(t, r, req, source, fake.reference)
		})
	}
}

func assertContinuationReplayRetainsOriginalPolicy(t *testing.T, r *registry, req workersessions.ContinueRequest, original *continueReplay, support *interruptContinuationSupportFake) {
	t.Helper()
	support.supported = false
	support.err = providers.ErrUnknownProvider
	support.reference = providers.SessionRef{}
	replay, owner, err := r.reserveContinuation(req)
	if err != nil || owner || replay != original || support.reference != (providers.SessionRef{}) {
		t.Fatalf("exact replay queried changed policy: replay=%p owner=%v err=%v", replay, owner, err)
	}
}

func assertTerminalContinuationPolicyNoEffects(t *testing.T, r *registry, req workersessions.ContinueRequest, source workersessions.Session, reference providers.SessionRef) {
	t.Helper()
	if reference != source.ProviderSessionAssociation.Reference || !reflect.DeepEqual(r.sessions[source.ID], source) {
		t.Fatal("policy query lost exact reference or changed terminal source")
	}
	if r.activeStarts != 0 || len(r.continueReplays) != 0 || len(r.continuationSources) != 0 {
		t.Fatal("policy refusal reserved continuation")
	}
	if _, err := r.Get(t.Context(), workersessions.GetRequest{ID: req.SuccessorWorkerSessionID}); !errors.Is(err, workersessions.ErrSessionNotFound) {
		t.Fatalf("successor reserved: %v", err)
	}
}

func TestT7ValidatedStructuredOutputSurvivesStreamedText(t *testing.T) {
	t.Parallel()
	for _, cell := range []struct {
		name     string
		streamed bool
		value    any
	}{{"buffered-object", false, map[string]any{"answer": "validated"}}, {"streamed-object", true, map[string]any{"answer": "validated"}}, {"streamed-null", true, nil}} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			r := newTestRegistry(t)
			sink := &perRuntimeAppendCapture{EventsAppender: newInternalTestEventsService()}
			r.events = sink
			r.publications["worker"] = &publication{open: true, provider: "codex", hasMessage: cell.streamed}
			r.publishBufferedWorkerOutput(t.Context(), "worker", "attempt", workers.WorkResult{
				Output: "raw output", StructuredResult: cell.value, StructuredResultPresent: true,
			})
			requests := sink.requestsFor("")
			if len(requests) != 1 {
				t.Fatalf("validated output records = %d, want one", len(requests))
			}
			var draft workers.Draft
			if err := json.Unmarshal(requests[0].Payload, &draft); err != nil {
				t.Fatal(err)
			}
			var message workers.MessagePayload
			if err := json.Unmarshal(draft.Payload, &message); err != nil {
				t.Fatal(err)
			}
			wantBlocks := 2
			if cell.streamed {
				wantBlocks = 1
			}
			if len(message.ContentBlocks) != wantBlocks {
				t.Fatalf("structured delivery duplicated streamed text: %#v", message)
			}
			block := message.ContentBlocks[len(message.ContentBlocks)-1]
			want, _ := json.Marshal(cell.value)
			if block.Kind != workers.ContentBlockStructuredOutput || string(block.StructuredOutput) != string(want) {
				t.Fatalf("validated native value = %#v, want %s", block, want)
			}
		})
	}
}

type originatingArtifactRecorder struct {
	recordings.WorkerSessionRecordingService
	request recordings.WorkerSessionRecordingRequest
}

func (r *originatingArtifactRecorder) StartWorkerSessionRecording(_ context.Context, request recordings.WorkerSessionRecordingRequest) (recordings.WorkerSessionRecording, error) {
	r.request = request
	return nil, nil
}

func TestWorkerWorkAttributionCaptureRequestCarriesOriginatingArtifact(t *testing.T) {
	t.Parallel()
	recorder := &originatingArtifactRecorder{}
	registry := &registry{recording: recorder}
	request := workersessions.InvokeSessionRequest{ID: "worker-origin"}
	request.Execution.Execution.RecordingID = "recording-origin"
	request.Execution.Execution.FactorySessionID = "factory-origin"
	request.Execution.Execution.OriginatingArtifact = "selected-factory.jsonl"
	if _, err := registry.startWorkerRecording(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	if recorder.request.OriginatingArtifact != request.Execution.Execution.OriginatingArtifact || recorder.request.RecordingID != "recording-origin" || recorder.request.FactorySessionID != "factory-origin" {
		t.Fatalf("capture request = %+v", recorder.request)
	}
}

func TestContinuationTerminalPublicationWaitHonorsCallerCancellation(t *testing.T) {
	t.Parallel()
	req := continuationReservationRequest()
	r := newContinuationSource(t, req)
	supervision := newSupervision("dispatch-1", "turn-1", continuationValidExecution("dispatch-1"))
	supervision.accepted = true
	r.supervisions[req.SourceWorkerSessionID] = supervision
	r.observations[req.SourceWorkerSessionID] = &observation{direct: true}
	r.logs = &LogReader{reader: &controlCaptureReader{}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := r.readContinuationRecipe(req, ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("terminal publication wait = %v, want caller cancellation", err)
	}
	if len(r.continueReplays) != 0 || len(r.sessions) != 1 {
		t.Fatal("canceled publication wait reserved a successor")
	}
}

func TestWorkNameCaptureUsesPrimaryDispatchedWork(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"named", "missing-primary", "nameless", "resource"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			recorder := &originatingArtifactRecorder{}
			registry := &registry{recording: recorder}
			request := workersessions.InvokeSessionRequest{ID: "worker"}
			request.Execution.Execution.RecordingID = "recording"
			dispatch := work.WorkDispatch{Execution: work.ExecutionMetadata{WorkIDs: []string{"primary", "secondary"}}}
			primary := workers.Token{ID: "token-primary", Color: workers.Color{DataType: workers.DataTypeWork, WorkID: "primary", Name: "Primary"}}
			secondary := workers.Token{ID: "token-secondary", Color: workers.Color{DataType: workers.DataTypeWork, WorkID: "secondary", Name: "Secondary"}}
			want := "Primary"
			switch scenario {
			case "missing-primary":
				primary.Color.WorkID = "foreign"
				want = ""
			case "nameless":
				primary.Color.Name = ""
				want = ""
			case "resource":
				primary.Color.DataType = workers.DataTypeResource
				want = ""
			}
			dispatch.InputTokens = workers.InputTokens(secondary, primary)
			request.Execution.Execution.Dispatch = dispatch
			if _, err := registry.startWorkerRecording(t.Context(), request); err != nil {
				t.Fatal(err)
			}
			if recorder.request.WorkName != want {
				t.Fatalf("primary capture name=%q want=%q", recorder.request.WorkName, want)
			}
		})
	}
}
