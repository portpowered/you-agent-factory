package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
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

	streamA := startS8LiveStream(t, scenario.fixture, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, scenario.session.id, ids.workerA, s8InterruptProviderSessionA)
	streamA.writer.waitWorkerSessionFrame(t, ids.workerA)
	streamB := startS8LiveStream(t, scenario.fixture, ctx, scenario.manager, scenario.env, scenario.repositoryB.path, scenario.serverURL, scenario.session.id, ids.workerB, s8InterruptProviderSessionB)
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

	streamSuccessor := startS8LiveStream(t, scenario.fixture, ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, scenario.session.id, ids.successor, s8InterruptProviderSessionA)
	streamSuccessor.writer.waitWorkerSessionFrame(t, ids.successor)
	active := listS8RemoteWorkers(t, ctx, scenario.manager, scenario.env, scenario.factoryDir, scenario.serverURL, "CANCELED", "STARTING", "RUNNING")
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
		if status != http.StatusBadRequest {
			t.Fatalf("successor conflict status = %d body=%s", status, body)
		}
		assertS8InterruptError(t, apiErr, "BAD_REQUEST", "VALIDATION", ids.workerA, ids.workerB)

		code, phase, cliErr := executeS8InterruptCLIError(ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.workerA, ids.interruptRequest, ids.workerB)
		if cliErr != nil {
			t.Fatalf("CLI replay transport: %v", cliErr)
		}
		if code != "BAD_REQUEST" || phase != string(factoryapi.WorkerSessionInterruptErrorPhaseValidation) {
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

func postS8Interrupt(
	t *testing.T,
	ctx context.Context,
	serverURL, sourceID, requestID, successorID, replacement string,
) factoryapi.WorkerSessionInterruptResponse {
	t.Helper()
	status, body, response, err := sendS8InterruptHTTP(ctx, serverURL, sourceID, requestID, successorID, replacement)
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
) (int, string, factoryapi.WorkerSessionInterruptResponse, error) {
	payload, err := json.Marshal(factoryapi.WorkerSessionInterruptRequest{
		RequestId: requestID, SuccessorWorkerSessionId: successorID, ReplacementMessage: replacement,
	})
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
) (string, string, error) {
	inputs := support.FakeInputs(ctx, []string{
		"you", "--remote", "--server", serverURL, "--json", "worker-sessions", "interrupt", sourceID,
		"--request-id", requestID, "--successor-worker-session-id", successorID,
		"--replacement-message", s8ReplacementMessage, "--async",
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
