package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestInterruptSingleSuccessor promotes the deterministic S8 provider edge to
// the public Worker Session, Event, and Work observations. The source and
// sibling overlap before the CLI interrupt; the exact HTTP replay is made
// after admission and before either provider edge is released.
func TestInterruptSingleSuccessor(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	scenario := newS8InterruptScenario(t, ctx, "manager-interrupt-single-successor")
	defer scenario.runner.releaseAll()
	ids := scenario.ids

	invokeS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, s8RemoteWorkerInvocation{
		requestID: ids.requestA, workerSessionID: ids.workerA, dispatchID: ids.dispatchA,
		factorySessionID: scenario.session.id, repository: scenario.repositoryA.path, workID: ids.workA, message: s8MessageA,
	})
	scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallAInitial, scenario.fixture.router.requests)
	invokeS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryB.path, scenario.serverURL, s8RemoteWorkerInvocation{
		requestID: ids.requestB, workerSessionID: ids.workerB, dispatchID: ids.dispatchB,
		factorySessionID: scenario.session.id, repository: scenario.repositoryB.path, workID: ids.workB, message: s8MessageB,
	})
	scenario.runner.waitStarted(t, scenario.repositoryB.path, s8InterruptCallBInitial, scenario.fixture.router.requests)

	streamA := startS8LiveStream(t, scenario.fixture, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.workerA, s8InterruptProviderSessionA)
	streamA.writer.waitWorkerSessionFrame(t, ids.workerA)
	streamB := startS8LiveStream(t, scenario.fixture, ctx, scenario.manager, scenario.env, scenario.repositoryB.path, scenario.serverURL, ids.workerB, s8InterruptProviderSessionB)
	streamB.writer.waitWorkerSessionFrame(t, ids.workerB)

	first := interruptS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor)
	assertS8InterruptAdmission(t, first, ids)
	scenario.runner.waitCanceled(t, scenario.repositoryA.path, s8InterruptCallAInitial)
	scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallASuccessor, scenario.fixture.router.requests)
	scenario.runner.assertOrder(t, "start:"+s8InterruptCallAInitial, "cancel:"+s8InterruptCallAInitial, "start:"+s8InterruptCallASuccessor)
	beforeReplay := scenario.runner.CallCount()
	replayed := postS8Interrupt(t, ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, s8ReplacementMessage)
	if got := s8InterruptResultFromAPI(replayed); !reflect.DeepEqual(got, first) {
		t.Fatalf("HTTP replay = %#v, want exact CLI admission result %#v", got, first)
	}
	if got := scenario.runner.CallCount(); got != beforeReplay {
		t.Fatalf("provider calls after exact interrupt replay = %d, want unchanged %d", got, beforeReplay)
	}

	streamSuccessor := startS8LiveStream(t, scenario.fixture, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.successor, s8InterruptProviderSessionA)
	streamSuccessor.writer.waitWorkerSessionFrame(t, ids.successor)
	// These fixture Work IDs are execution associations, not materialized Work rows.
	// Read the direct list without server-side state filters and select this run's stable IDs.
	listed := listS8RemoteWorkers(t, ctx, scenario.manager, scenario.env, scenario.factoryDir, scenario.serverURL)
	active := make([]s8WorkerObservation, 0, 3)
	for _, observation := range listed {
		switch observation.WorkerSessionID {
		case ids.workerA, ids.successor, ids.workerB:
			active = append(active, observation)
		}
	}
	if len(active) != 3 {
		t.Fatalf("post-interrupt direct Worker Session list contained %d scenario identities, want 3: %#v", len(active), active)
	}
	assertS8Observation(t, findS8Observation(t, active, ids.workerA), ids.workerA, scenario.session.id, ids.workA, "CANCELED", s8InterruptProviderSessionA, ids.dispatchA)
	assertS8Observation(t, findS8Observation(t, active, ids.successor), ids.successor, scenario.session.id, ids.workA, "RUNNING", s8InterruptProviderSessionA, ids.dispatchA+"/continue/"+ids.successor)
	assertS8Observation(t, findS8Observation(t, active, ids.workerB), ids.workerB, scenario.session.id, ids.workB, "RUNNING", s8InterruptProviderSessionB, ids.dispatchB)

	finishS8InterruptScenario(t, scenario, streamA, streamB, streamSuccessor, findS8Observation(t, active, ids.successor))
	assertS8WorkNotAdvanced(t, scenario.fixture, scenario.session.id, ids.workA, ids.workB)
	scenario.close(t)
}

// TestInterruptParity sends the first operation through HTTP and then replays
// it through the remote CLI. Both public boundaries must expose the same
// identities and admission snapshots, while the provider edge is called only
// once for the source and once for the admitted successor.
func TestInterruptParity(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	scenario := newS8InterruptScenario(t, ctx, "manager-interrupt-parity")
	fixture := scenario.fixture
	defer scenario.runner.releaseAll()
	ids := scenario.ids

	invokeS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, s8RemoteWorkerInvocation{
		requestID: ids.requestA, workerSessionID: ids.workerA, dispatchID: ids.dispatchA,
		factorySessionID: scenario.session.id, repository: scenario.repositoryA.path, workID: ids.workA, message: s8MessageA,
	})
	scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallAInitial, fixture.router.requests)

	first := postS8Interrupt(t, ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, s8ReplacementMessage)
	assertS8APIInterruptAdmission(t, first, ids)
	scenario.runner.waitCanceled(t, scenario.repositoryA.path, s8InterruptCallAInitial)
	scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallASuccessor, fixture.router.requests)
	cliResult := interruptS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor)
	if got := s8InterruptResultFromAPI(first); !reflect.DeepEqual(got, cliResult) {
		t.Fatalf("HTTP-first/CLI-replay mismatch: HTTP=%#v CLI=%#v", first, cliResult)
	}
	replayed := postS8Interrupt(t, ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, s8ReplacementMessage)
	if !reflect.DeepEqual(first, replayed) {
		t.Fatalf("HTTP replay = %#v, want original HTTP response %#v", replayed, first)
	}

	scenario.runner.release(t, scenario.repositoryA.path, s8InterruptCallASuccessor)
	_ = replayS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.successor)
	if got := scenario.runner.CallCount(); got != 2 {
		t.Fatalf("provider calls after CLI/HTTP parity replay = %d, want source plus one successor", got)
	}
	assertS8WorkNotAdvanced(t, fixture, scenario.session.id, ids.workA)
	scenario.close(t)
}

// TestInterruptRace exercises the same request through concurrent HTTP and
// CLI callers. The public result is absorbing: exactly one source cancellation
// and one successor provider admission survive the race.
func TestInterruptRace(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	scenario := newS8InterruptScenario(t, ctx, "manager-interrupt-race")
	fixture := scenario.fixture
	defer scenario.runner.releaseAll()
	ids := scenario.ids
	env := scenario.env

	invokeS8RemoteWorker(t, ctx, scenario.manager, env, scenario.repositoryA.path, scenario.serverURL, s8RemoteWorkerInvocation{
		requestID: ids.requestA, workerSessionID: ids.workerA, dispatchID: ids.dispatchA,
		factorySessionID: scenario.session.id, repository: scenario.repositoryA.path, workID: ids.workA, message: s8MessageA,
	})
	scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallAInitial, fixture.router.requests)

	start := make(chan struct{})
	outcomes := make(chan publicInterruptOutcome, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		status, body, response, err := sendS8InterruptHTTP(ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, s8ReplacementMessage)
		outcomes <- publicInterruptOutcome{api: response, status: status, body: body, err: err}
	}()
	go func() {
		defer wait.Done()
		<-start
		result, err := executeS8InterruptCLI(ctx, scenario.manager, env, scenario.repositoryA.path, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor)
		outcomes <- publicInterruptOutcome{cli: result, err: err}
	}()
	close(start)
	wait.Wait()
	close(outcomes)

	var accepted []s8InterruptResult
	for outcome := range outcomes {
		if outcome.err != nil {
			t.Fatalf("concurrent interrupt outcome: %v", outcome.err)
		}
		if outcome.status != 0 && outcome.status != http.StatusAccepted {
			t.Fatalf("concurrent HTTP interrupt status = %d body=%s", outcome.status, outcome.body)
		}
		if outcome.status == http.StatusAccepted {
			accepted = append(accepted, s8InterruptResultFromAPI(outcome.api))
		} else {
			accepted = append(accepted, outcome.cli)
		}
	}
	if len(accepted) != 2 || !reflect.DeepEqual(accepted[0], accepted[1]) || !accepted[0].Accepted {
		t.Fatalf("concurrent interrupt results = %#v, want two identical accepted results", accepted)
	}
	scenario.runner.waitCanceled(t, scenario.repositoryA.path, s8InterruptCallAInitial)
	scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallASuccessor, fixture.router.requests)
	if got := scenario.runner.cancellationCount(s8InterruptCallAInitial); got != 1 {
		t.Fatalf("concurrent interrupt cancellation count = %d, want one", got)
	}
	if got := scenario.runner.CallCount(); got != 2 {
		t.Fatalf("concurrent interrupt provider calls = %d, want source plus one successor", got)
	}
	scenario.runner.release(t, scenario.repositoryA.path, s8InterruptCallASuccessor)
	_ = replayS8RemoteWorker(t, ctx, scenario.manager, env, scenario.repositoryA.path, scenario.serverURL, ids.successor)
	assertS8WorkNotAdvanced(t, fixture, scenario.session.id, ids.workA)
	scenario.close(t)
}

// The real HTTP host retains its synced operation after the caller disconnects.
// A scenario-owned command gate separates cancellation from callback return,
// making the interrupted wait observable without sleeps or shared host locks.
func TestInterruptCallerDisconnectRetainsOneHostOwnedSuccessor(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	scenario := newS8InterruptScenario(t, ctx, "manager-interrupt-disconnect")
	defer scenario.runner.releaseAll()
	ids := scenario.ids
	callbackReturn := make(chan struct{})
	var releaseOnce sync.Once
	releaseCallback := func() { releaseOnce.Do(func() { close(callbackReturn) }) }
	t.Cleanup(releaseCallback)
	call := scenario.runner.callFor(scenario.repositoryA.path, s8InterruptCallAInitial)
	call.cancellationReturn = callbackReturn
	invokeS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, s8RemoteWorkerInvocation{
		requestID: ids.requestA, workerSessionID: ids.workerA, dispatchID: ids.dispatchA,
		factorySessionID: scenario.session.id, repository: scenario.repositoryA.path, workID: ids.workA, message: s8MessageA,
	})
	scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallAInitial, scenario.fixture.router.requests)
	callerCtx, disconnect := context.WithCancel(ctx)
	defer disconnect()
	callerDone := make(chan error, 1)
	go func() {
		_, _, _, err := sendS8InterruptHTTP(callerCtx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, s8ReplacementMessage, "recorded")
		callerDone <- err
	}()
	scenario.runner.waitCanceled(t, scenario.repositoryA.path, s8InterruptCallAInitial)
	disconnect()
	select {
	case err := <-callerDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("disconnected caller error=%v, want context cancellation", err)
		}
	case <-ctx.Done():
		t.Fatal("disconnected HTTP caller did not return")
	}
	if scenario.runner.CallCount() != 1 {
		t.Fatal("successor admitted before source callback joined")
	}
	releaseCallback()
	first := postS8Interrupt(t, ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, s8ReplacementMessage, "recorded")
	assertS8APIInterruptAdmission(t, first, ids)
	scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallASuccessor, scenario.fixture.router.requests)
	cli := interruptS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, "recorded")
	if !reflect.DeepEqual(cli, s8InterruptResultFromAPI(first)) || scenario.runner.CallCount() != 2 || scenario.runner.cancellationCount(s8InterruptCallAInitial) != 1 {
		t.Fatalf("disconnected retry=%#v, want one cancellation and one successor matching %#v", cli, first)
	}
	scenario.runner.release(t, scenario.repositoryA.path, s8InterruptCallASuccessor)
	_ = replayS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.successor)
	assertS8WorkNotAdvanced(t, scenario.fixture, scenario.session.id, ids.workA)
	scenario.close(t)
}

// TestInterruptFailure proves the stable validation failure through the
// root-composed HTTP server for a conflicting successor. The source-cancellation
// response is also sent through the configured remote HTTP edge to keep the
// CLI and REST error projections exact without pretending a provider command
// can inject a Workers cancellation-gateway error.
func TestInterruptFailure(t *testing.T) {
	t.Parallel()
	t.Run("successor-conflict", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		scenario := newS8InterruptScenario(t, ctx, "manager-interrupt-failure")
		defer scenario.runner.releaseAll()
		ids := scenario.ids

		invokeS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, s8RemoteWorkerInvocation{
			requestID: ids.requestA, workerSessionID: ids.workerA, dispatchID: ids.dispatchA,
			factorySessionID: scenario.session.id, repository: scenario.repositoryA.path, workID: ids.workA, message: s8MessageA,
		})
		scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallAInitial, scenario.fixture.router.requests)
		invokeS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryB.path, scenario.serverURL, s8RemoteWorkerInvocation{
			requestID: ids.requestB, workerSessionID: ids.workerB, dispatchID: ids.dispatchB,
			factorySessionID: scenario.session.id, repository: scenario.repositoryB.path, workID: ids.workB, message: s8MessageB,
		})
		scenario.runner.waitStarted(t, scenario.repositoryB.path, s8InterruptCallBInitial, scenario.fixture.router.requests)
		status, body, apiErr := postS8InterruptError(t, ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.workerB, s8ReplacementMessage)
		if status != http.StatusConflict {
			t.Fatalf("successor conflict status = %d body=%s", status, body)
		}
		assertS8InterruptError(t, apiErr, "WORKER_SESSION_INTERRUPT_CONFLICT", "VALIDATION", ids.workerA, ids.workerB)

		code, phase, cliErr := executeS8InterruptCLIError(ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.workerB)
		if cliErr != nil {
			t.Fatalf("CLI replay transport: %v", cliErr)
		}
		if code != "WORKER_SESSION_INTERRUPT_CONFLICT" || phase != string(factoryapi.WorkerSessionInterruptErrorPhaseValidation) {
			t.Fatalf("CLI successor conflict error = %s/%s, want typed code/phase", code, phase)
		}
		if got := scenario.runner.CallCount(); got != 2 {
			t.Fatalf("successor admission conflict provider calls = %d, want the two unchanged source/sibling calls", got)
		}
		source := showS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.workerA)
		if source.State != "RUNNING" {
			t.Fatalf("source after successor conflict = %#v, want unchanged RUNNING", source)
		}
		sibling := showS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryB.path, scenario.serverURL, ids.workerB)
		if sibling.State != "RUNNING" {
			t.Fatalf("conflicting successor sibling after conflict = %#v, want unchanged RUNNING", sibling)
		}
		assertS8WorkNotAdvanced(t, scenario.fixture, scenario.session.id, ids.workA, ids.workB)
		scenario.close(t)
	})

	t.Run("source-cancellation-transport-parity", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		fixture := ensureInvokeContinuePackageFixture(t)
		scenario := fixture.scenario(t, "remote-interrupt-failure")
		defer scenario.close(t)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/worker-sessions/source-session/interrupt" {
				http.Error(w, "unexpected interrupt route", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":                     string(factoryapi.ErrorResponseCodeWORKERSESSIONINTERRUPTSOURCECANCELLATIONFAILED),
				"family":                   "WORKER_SESSION",
				"message":                  "Workers could not cancel the Worker Session interrupt source",
				"phase":                    string(factoryapi.WorkerSessionInterruptErrorPhaseSourceCancellation),
				"requestId":                "source-failure-request",
				"sourceWorkerSessionId":    "source-session",
				"successorWorkerSessionId": "successor-session",
			})
		}))
		defer server.Close()

		status, body, apiErr := postS8InterruptError(t, ctx, server.URL, "source-session", "source-failure-request", "successor-session", "replacement")
		if status != http.StatusServiceUnavailable {
			t.Fatalf("source cancellation failure status = %d body=%s", status, body)
		}
		assertS8InterruptError(t, apiErr, string(factoryapi.ErrorResponseCodeWORKERSESSIONINTERRUPTSOURCECANCELLATIONFAILED), string(factoryapi.WorkerSessionInterruptErrorPhaseSourceCancellation), "source-session", "successor-session")
		code, phase, cliErr := executeS8InterruptCLIError(ctx, fixture.process, scenario.environment(), scenario.workingDirectory, server.URL, "source-session", "source-failure-request", "successor-session")
		if cliErr != nil {
			t.Fatalf("source-cancellation CLI transport: %v", cliErr)
		}
		if code != string(factoryapi.ErrorResponseCodeWORKERSESSIONINTERRUPTSOURCECANCELLATIONFAILED) || phase != string(factoryapi.WorkerSessionInterruptErrorPhaseSourceCancellation) {
			t.Fatalf("CLI source cancellation error = %s/%s, want typed code/phase", code, phase)
		}
	})
}

type publicInterruptOutcome struct {
	api    factoryapi.WorkerSessionInterruptResponse
	cli    s8InterruptResult
	status int
	body   string
	err    error
}

// Explicit environment overrides cannot be reconstructed from a configuration
// reference. Refuse replacement before stopping; exact termination still works.
func TestInterruptUnsafeRecipeLeavesSourceControllable(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	scenario := newS8InterruptScenario(t, ctx, "unsafe-recipe")
	t.Cleanup(scenario.runner.releaseAll)
	ids := scenario.ids
	invokeS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, s8RemoteWorkerInvocation{
		requestID: ids.requestA, workerSessionID: ids.workerA, dispatchID: ids.dispatchA,
		factorySessionID: scenario.session.id, repository: scenario.repositoryA.path, workID: ids.workA, message: s8MessageA,
		envVars: map[string]string{"CUSTOM_TOKEN": "private-explicit-override"},
	})
	scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallAInitial, scenario.fixture.router.requests)
	status, body, response := postS8InterruptError(t, ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, s8ReplacementMessage)
	if status < 400 || response.Code == "" || string(response.Phase) != "VALIDATION" || strings.Contains(body, "private-explicit-override") {
		t.Fatalf("unsafe recipe HTTP status=%d response=%#v", status, response)
	}
	code, phase, err := executeS8InterruptCLIError(ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor)
	if err != nil || code != string(response.Code) || phase != "VALIDATION" {
		t.Fatalf("unsafe recipe CLI code=%s phase=%s err=%v", code, phase, err)
	}
	if scenario.runner.CallCount() != 1 || scenario.runner.cancellationCount(s8InterruptCallAInitial) != 0 {
		t.Fatal("refused interruption stopped the source or admitted a successor")
	}
	inputs := support.FakeInputs(ctx, []string{"you", "--remote", "--server", scenario.serverURL, "--json", "worker-sessions", "terminate", ids.workerA})
	inputs.Input.Env = append([]string(nil), scenario.env...)
	inputs.Input.WorkingDirectory = scenario.repositoryA.path
	if err := scenario.manager.Execute(inputs.Input); err != nil {
		t.Fatalf("exact terminate after unsafe recipe: %v stderr=%s", err, inputs.Stderr())
	}
	scenario.runner.waitCanceled(t, scenario.repositoryA.path, s8InterruptCallAInitial)
	listed := listS8RemoteWorkers(t, ctx, scenario.manager, scenario.env, scenario.factoryDir, scenario.serverURL)
	if source := findS8Observation(t, listed, ids.workerA); source.State != "TERMINATED" || scenario.runner.CallCount() != 1 {
		t.Fatalf("plain termination source=%#v provider calls=%d", source, scenario.runner.CallCount())
	}
}

// Direct captured restart input must be usable before cancellation. These
// CLI/HTTP parity cells fault only the recording-store read edge; normal
// stopping remains available through the same public host.
func TestInterruptCapturedRecipeReadRefusalLeavesSourceControllable(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"interrupt-recipe-read-failure", "interrupt-recipe-unsafe-read"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			scenario := newS8InterruptScenario(t, ctx, cell)
			t.Cleanup(scenario.runner.releaseAll)
			ids := scenario.ids
			ids.workerA = cell + "-" + ids.workerA
			invokeS8RemoteWorker(t, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, s8RemoteWorkerInvocation{
				requestID: ids.requestA, workerSessionID: ids.workerA, dispatchID: ids.dispatchA,
				factorySessionID: scenario.session.id, repository: scenario.repositoryA.path, workID: ids.workA, message: s8MessageA,
			})
			scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallAInitial, scenario.fixture.router.requests)
			status, body, response := postS8InterruptError(t, ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, s8ReplacementMessage)
			if status != http.StatusServiceUnavailable || string(response.Code) != "WORKER_SESSION_INTERRUPT_ADMISSION_FAILED" || string(response.Phase) != "VALIDATION" || strings.Contains(body, "private-recipe-read-detail") {
				t.Fatalf("recipe preflight HTTP status=%d response=%#v body=%s", status, response, body)
			}
			code, phase, err := executeS8InterruptCLIError(ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor)
			if err != nil || code != string(response.Code) || phase != "VALIDATION" {
				t.Fatalf("recipe preflight CLI code=%s phase=%s err=%v", code, phase, err)
			}
			assertRecipeRefusalStillAllowsTermination(t, scenario, ids)
		})
	}
}

func assertRecipeRefusalStillAllowsTermination(t *testing.T, scenario s8InterruptScenario, ids s8ScenarioIdentities) {
	t.Helper()
	ctx := scenario.ctx
	listed := listS8RemoteWorkers(t, ctx, scenario.manager, scenario.env, scenario.factoryDir, scenario.serverURL)
	if findS8Observation(t, listed, ids.workerA).State != "RUNNING" || scenario.runner.CallCount() != 1 || scenario.runner.cancellationCount(s8InterruptCallAInitial) != 0 {
		t.Fatal("failed recipe read stopped source or admitted successor")
	}
	for _, item := range listed {
		if item.WorkerSessionID == ids.successor {
			t.Fatal("recipe preflight reserved a successor")
		}
	}
	inputs := support.FakeInputs(ctx, []string{"you", "--remote", "--server", scenario.serverURL, "--json", "worker-sessions", "terminate", ids.workerA})
	inputs.Input.Env, inputs.Input.WorkingDirectory = scenario.env, scenario.repositoryA.path
	if err := scenario.manager.Execute(inputs.Input); err != nil {
		t.Fatalf("terminate after recipe preflight: %v %s", err, inputs.Stderr())
	}
	scenario.runner.waitCanceled(t, scenario.repositoryA.path, s8InterruptCallAInitial)
	listed = listS8RemoteWorkers(t, ctx, scenario.manager, scenario.env, scenario.factoryDir, scenario.serverURL)
	if findS8Observation(t, listed, ids.workerA).State != "TERMINATED" || scenario.runner.CallCount() != 1 {
		t.Fatal("recipe refusal disabled exact termination")
	}
}

// The shared host uses real capture persistence and scenario-owned command
// gates. Refusal must retain live control, independently of artifact storage.
func TestInterruptInputByteRefusalLeavesSourceControllable(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"serialized-overflow", "successor-artifact-overflow", "invalid-utf8"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			scenario := newS8InterruptScenario(t, t.Context(), cell)
			t.Cleanup(scenario.runner.releaseAll)
			ids := scenario.ids
			invokeS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, s8RemoteWorkerInvocation{
				requestID: ids.requestA, workerSessionID: ids.workerA, dispatchID: ids.dispatchA,
				factorySessionID: scenario.session.id, repository: scenario.repositoryA.path, workID: ids.workA, message: s8MessageA,
			})
			scenario.runner.waitStarted(t, scenario.repositoryA.path, s8InterruptCallAInitial, scenario.fixture.router.requests)
			replacement, want := "replacement\xff", "WORKER_SESSION_INTERRUPT_INVALID"
			if cell == "successor-artifact-overflow" {
				// The frozen source input fits, but the successor identity occurs
				// in both its recipe target and its new dispatch identity.
				ids.successor = strings.Repeat("s", recordings.WorkerControlInputMaxBytes/2)
				replacement, want = "complete replacement", "BAD_REQUEST"
			}
			if cell == "serialized-overflow" || cell == "successor-artifact-overflow" {
				// The message fits as raw text; its JSON escaping exceeds the
				// entire input budget even before execution settings are added.
				if cell == "serialized-overflow" {
					replacement = strings.Repeat("<", recordings.WorkerControlInputMaxBytes/6)
				}
				want = "BAD_REQUEST"
				status, _, response := postS8InterruptError(t, scenario.ctx, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, replacement)
				if status != http.StatusBadRequest || string(response.Code) != want || string(response.Phase) != "VALIDATION" {
					t.Fatalf("overflow HTTP status=%d code=%s phase=%s", status, response.Code, response.Phase)
				}
			}
			code, phase, err := executeS8InterruptCLIError(scenario.ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.successor, replacement)
			if err != nil || code != want || phase != "VALIDATION" {
				t.Fatalf("byte refusal CLI code=%s phase=%s err=%v", code, phase, err)
			}
			assertRecipeRefusalStillAllowsTermination(t, scenario, ids)
		})
	}
}

func postS8Interrupt(
	t *testing.T,
	ctx context.Context,
	serverURL, sourceID, requestID, successorID, replacement string,
	modes ...string,
) factoryapi.WorkerSessionInterruptResponse {
	t.Helper()
	status, body, response, err := sendS8InterruptHTTP(ctx, serverURL, sourceID, requestID, successorID, replacement, modes...)
	if err != nil {
		t.Fatalf("HTTP interrupt: %v", err)
	}
	if status != http.StatusAccepted {
		t.Fatalf("HTTP interrupt status = %d body=%s", status, body)
	}
	return response
}

func postS8InterruptError(
	t *testing.T,
	ctx context.Context,
	serverURL, sourceID, requestID, successorID, replacement string,
) (int, string, factoryapi.WorkerSessionInterruptError) {
	t.Helper()
	status, body, _, err := sendS8InterruptHTTP(ctx, serverURL, sourceID, requestID, successorID, replacement)
	if err != nil {
		t.Fatalf("HTTP interrupt error request: %v", err)
	}
	var response factoryapi.WorkerSessionInterruptError
	if decodeErr := json.Unmarshal([]byte(body), &response); decodeErr != nil {
		t.Fatalf("decode HTTP interrupt error: %v body=%s", decodeErr, body)
	}
	return status, body, response
}

func sendS8InterruptHTTP(
	ctx context.Context,
	serverURL, sourceID, requestID, successorID, replacement string,
	modes ...string,
) (int, string, factoryapi.WorkerSessionInterruptResponse, error) {
	input := factoryapi.WorkerSessionInterruptRequest{RequestId: requestID, SuccessorWorkerSessionId: successorID, ReplacementMessage: replacement}
	if len(modes) != 0 {
		mode := factoryapi.WorkerSessionInterruptRequestResumeMode(modes[0])
		input.ResumeMode = &mode
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return 0, "", factoryapi.WorkerSessionInterruptResponse{}, err
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/worker-sessions/" + url.PathEscape(sourceID) + "/interrupt"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return 0, "", factoryapi.WorkerSessionInterruptResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, "", factoryapi.WorkerSessionInterruptResponse{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return response.StatusCode, "", factoryapi.WorkerSessionInterruptResponse{}, err
	}
	if response.StatusCode != http.StatusAccepted {
		return response.StatusCode, string(body), factoryapi.WorkerSessionInterruptResponse{}, nil
	}
	var decoded factoryapi.WorkerSessionInterruptResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return response.StatusCode, string(body), factoryapi.WorkerSessionInterruptResponse{}, err
	}
	return response.StatusCode, string(body), decoded, nil
}

func s8InterruptResultFromAPI(response factoryapi.WorkerSessionInterruptResponse) s8InterruptResult {
	return s8InterruptResult{
		RequestID: response.RequestId, SourceWorkerSessionID: response.SourceWorkerSessionId,
		SuccessorWorkerSessionID: response.SuccessorWorkerSessionId, Phase: string(response.Phase), Accepted: response.Accepted,
		Source:    s8InterruptSnapshot{WorkerSessionID: response.Source.WorkerSessionId, State: string(response.Source.State), EventTopic: response.Source.EventTopic},
		Successor: s8InterruptSnapshot{WorkerSessionID: response.Successor.WorkerSessionId, State: string(response.Successor.State), EventTopic: response.Successor.EventTopic},
	}
}

func assertS8APIInterruptAdmission(t *testing.T, response factoryapi.WorkerSessionInterruptResponse, ids s8ScenarioIdentities) {
	t.Helper()
	assertS8InterruptAdmission(t, s8InterruptResultFromAPI(response), ids)
}

func assertS8InterruptError(
	t *testing.T,
	response factoryapi.WorkerSessionInterruptError,
	wantCode, wantPhase, sourceID, successorID string,
) {
	t.Helper()
	if response.Code != wantCode || string(response.Phase) != wantPhase || response.SourceWorkerSessionId == nil || *response.SourceWorkerSessionId != sourceID || response.SuccessorWorkerSessionId == nil || *response.SuccessorWorkerSessionId != successorID {
		t.Fatalf("interrupt error = %#v, want code=%q phase=%q source=%q successor=%q", response, wantCode, wantPhase, sourceID, successorID)
	}
	if response.Message == "" || strings.Contains(response.Message, "controlled") {
		t.Fatalf("interrupt error message = %q, want safe actionable message", response.Message)
	}
}

func executeS8InterruptCLI(
	ctx context.Context,
	process support.Process,
	env []string,
	workingDirectory, serverURL, sourceID, requestID, successorID string,
) (s8InterruptResult, error) {
	inputs := support.FakeInputs(ctx, []string{
		"you", "--remote", "--server", serverURL, "--json", "worker-sessions", "interrupt", sourceID,
		"--request-id", requestID, "--successor-worker-session-id", successorID,
		"--replacement-message", s8ReplacementMessage, "--async",
	})
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(inputs.Input); err != nil {
		return s8InterruptResult{}, fmt.Errorf("CLI interrupt: %w stdout=%s stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var result s8InterruptResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &result); err != nil {
		return s8InterruptResult{}, err
	}
	return result, nil
}

func executeS8InterruptCLIError(
	ctx context.Context,
	process support.Process,
	env []string,
	workingDirectory, serverURL, sourceID, requestID, successorID string,
	replacement ...string,
) (string, string, error) {
	message := s8ReplacementMessage
	if len(replacement) != 0 {
		message = replacement[0]
	}
	inputs := support.FakeInputs(ctx, []string{
		"you", "--remote", "--server", serverURL, "--json", "worker-sessions", "interrupt", sourceID,
		"--request-id", requestID, "--successor-worker-session-id", successorID,
		"--replacement-message", message, "--async",
	})
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(inputs.Input); err == nil {
		return "", "", fmt.Errorf("CLI interrupt unexpectedly succeeded stdout=%s", inputs.Stdout())
	}
	for _, output := range []string{inputs.Stdout(), inputs.Stderr()} {
		var response struct {
			Code  string `json:"code"`
			Phase string `json:"phase"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &response); err == nil && response.Code != "" {
			return response.Code, response.Phase, nil
		}
	}
	return "", "", fmt.Errorf("CLI interrupt emitted no typed error stdout=%s stderr=%s", inputs.Stdout(), inputs.Stderr())
}

func assertS8WorkNotAdvanced(t *testing.T, fixture *invokeContinuePackageFixture, sessionID string, workIDs ...string) {
	t.Helper()
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, strings.TrimSuffix(fixture.baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID)+"/work")
	for _, item := range listed.Results {
		if item.WorkId == nil {
			continue
		}
		for _, workID := range workIDs {
			if *item.WorkId == workID {
				t.Fatalf("interrupt exposed Work %q in public Work list: %#v", workID, listed.Results)
			}
		}
	}
}
