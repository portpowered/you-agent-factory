package response_events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestInjectedInvocationScopedTimeoutAndCancellationKeepPeerUsable(t *testing.T) {
	t.Parallel()
	gates := map[string]*isolatedCommandGate{}
	for _, prompt := range []string{"injected-timeout-a", "injected-timeout-b", "timeout-disabled", "caller-cancellation", "provider-failure", "success"} {
		gates[prompt] = &isolatedCommandGate{entered: make(chan context.Context, 4), release: make(chan struct{}), returned: make(chan error, 4), output: prompt + " COMPLETE"}
		gates[prompt+"-peer"] = &isolatedCommandGate{entered: make(chan context.Context, 4), release: make(chan struct{}), returned: make(chan error, 4), output: prompt + " peer COMPLETE"}
	}
	runner := &fourScopeCodexRunner{gates: gates}
	baseURL, sessions, diagnostics := startInjectedInvocationHost(t, runner)
	assertInjectedTimeoutAndMissingPeer(t, baseURL, sessions, diagnostics, runner, gates)

	for _, test := range []struct {
		name         string
		timeout      int64
		cancelCaller bool
		failure      error
		status       factorysessions.InvocationTerminalStatus
	}{
		{name: "success", status: factorysessions.InvocationTerminalStatusCompleted},
		{name: "timeout-disabled", timeout: 2000, status: factorysessions.InvocationTerminalStatusTimedOut},
		{name: "caller-cancellation", cancelCaller: true, status: factorysessions.InvocationTerminalStatusCanceled},
		{name: "provider-failure", failure: errors.New("owned provider failure"), status: factorysessions.InvocationTerminalStatusFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertInjectedInvocationWaitOutcome(t, baseURL, sessions, diagnostics, gates[test.name], gates[test.name+"-peer"], test.name, test.timeout, test.cancelCaller, test.failure, test.status)
		})
	}
}

func assertInjectedTimeoutAndMissingPeer(t *testing.T, baseURL string, sessions factorysessions.Service, diagnostics *observer.ObservedLogs, runner *fourScopeCodexRunner, gates map[string]*isolatedCommandGate) {
	t.Helper()
	ids := openInjectedTimeoutSessions(t, baseURL)

	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := sessions.Invoke(canceled, injectedInvocationRequest(ids[0], 0, false)); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-admission cancellation=%v", err)
	}
	peer := make(chan concurrentIsolationInvocation, 1)
	go func() {
		response, err := postConcurrentIsolationInvocation(t.Context(), baseURL, ids[1], "injected-timeout-b")
		peer <- concurrentIsolationInvocation{response: response, err: err}
	}()
	peerCtx := awaitInjectedProviderEntry(t, gates["injected-timeout-b"])
	assertInjectedMissingSelector(t, baseURL, sessions, runner)
	result := make(chan injectedInvocationOutcome, 1)
	go func() {
		value, err := sessions.Invoke(t.Context(), injectedInvocationRequest(ids[0], 2000, true))
		result <- injectedInvocationOutcome{value, err}
	}()
	awaitInjectedProviderEntry(t, gates["injected-timeout-a"])
	select {
	case outcome := <-result:
		assertInjectedOutcome(t, outcome, ids[0], factorysessions.InvocationTerminalStatusTimedOut)
		assertInjectedAttribution(t, baseURL, diagnostics, ids[0], outcome.value)
		assertInjectedControlHistory(t, diagnostics, ids[0], outcome.value.RequestID, 1)
	case <-time.After(concurrentIsolationTimeout):
		t.Fatal("public invocation did not time out")
	}
	select {
	case err := <-gates["injected-timeout-a"].returned:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("selected provider stop=%v", err)
		}
	case <-time.After(concurrentIsolationTimeout):
		t.Fatal("timeout did not join selected provider")
	}
	support.WaitForSessionStopped(t, baseURL, ids[0], concurrentIsolationTimeout)
	if peerCtx.Err() != nil {
		t.Fatalf("peer canceled=%v", peerCtx.Err())
	}
	close(gates["injected-timeout-b"].release)
	assertConcurrentIsolationInvocationCompleted(t, awaitConcurrentIsolationInvocation(t, peer), "injected-timeout-b")
	next, err := postConcurrentIsolationInvocation(t.Context(), baseURL, ids[1], "injected-timeout-b-next")
	if err != nil {
		t.Fatal(err)
	}
	assertConcurrentIsolationInvocationCompleted(t, concurrentIsolationInvocation{response: next}, "injected-timeout-b")
	assertInjectedControlHistory(t, diagnostics, ids[1], "", 0)

}

type injectedInvocationOutcome struct {
	value factorysessions.InvocationResult
	err   error
}

func injectedInvocationRequest(id string, timeout int64, cancel bool) factorysessions.SessionInvokeRequest {
	return factorysessions.SessionInvokeRequest{SessionID: id, ContentProvided: true, Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "owned invocation secret-value-fi-t13"}}, Wait: factorysessions.SessionOperationWait{TimeoutMillis: timeout, CancelOnTimeout: cancel}}
}

func assertInjectedAttribution(t *testing.T, baseURL string, diagnostics *observer.ObservedLogs, id string, result factorysessions.InvocationResult) {
	t.Helper()
	submitted, terminal := 0, 0
	for _, entry := range diagnostics.All() {
		if !strings.HasPrefix(entry.Message, "factory session invocation ") {
			continue
		}
		fields := entry.ContextMap()
		encoded, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "secret-value-fi-t13") || strings.Contains(string(encoded), "owned provider failure") {
			t.Fatalf("invocation diagnostic leaked input/provider payload: %s", entry.Message)
		}
		if fields["request_id"] != result.RequestID {
			continue
		}
		if fields["session_id"] != id || fields["trace_id"] != result.TraceID {
			t.Fatalf("invocation log correlation=%#v", fields)
		}
		switch entry.Message {
		case "factory session invocation submitted":
			submitted++
		case "factory session invocation completed", "factory session invocation failed":
			terminal++
			if fields["status"] != string(result.Status) {
				t.Fatalf("invocation terminal diagnostic=%#v", fields)
			}
		}
	}
	if submitted != 1 || terminal != 1 {
		t.Fatalf("request %s submitted/terminal diagnostics=%d/%d", result.RequestID, submitted, terminal)
	}
	assertInjectedWorkAttribution(t, baseURL, id, result)

}

func injectedString[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func assertInjectedOutcome(t *testing.T, result injectedInvocationOutcome, id string, status factorysessions.InvocationTerminalStatus) {
	t.Helper()
	if result.err != nil || result.value.Status != status || result.value.RequestID == "" || result.value.TraceID == "" {
		t.Fatalf("invocation=%#v error=%v want=%s", result.value, result.err, status)
	}
	if status == factorysessions.InvocationTerminalStatusCompleted {
		if result.value.ErrorCode != "" {
			t.Fatalf("successful invocation has error=%#v", result.value)
		}
		return // Success intentionally omits the non-success session/work context.
	}
	codes := map[factorysessions.InvocationTerminalStatus]string{
		factorysessions.InvocationTerminalStatusTimedOut: "INVOCATION_TIMED_OUT",
		factorysessions.InvocationTerminalStatusCanceled: "INVOCATION_CANCELED",
		factorysessions.InvocationTerminalStatusFailed:   "INVOCATION_RUNTIME_FAILURE",
	}
	// The current live invocation result omits SessionID; addressed session
	// attribution is carried by its canonical events and diagnostics.
	if result.value.SessionID != "" || result.value.WorkID == "" || result.value.ErrorCode != codes[status] {
		t.Fatalf("terminal identity/code=%#v", result.value)
	}
}

func assertInjectedControlHistory(t *testing.T, diagnostics *observer.ObservedLogs, id, requestID string, want int) {
	t.Helper()
	count := 0
	// Live cancel records its intent in diagnostics and worker observations;
	// SESSION_LIFECYCLE_CONTROL is currently emitted only for pause/resume.
	for _, entry := range diagnostics.FilterMessage("factory session lifecycle control").All() {
		fields := entry.ContextMap()
		if fields["session_id"] != id {
			continue
		}
		count++
		if fields["request_id"] != requestID || fields["operation"] != "CANCEL" || fields["outcome"] != "ACCEPTED" || fields["lifecycle_control_status"] != "SUCCEEDED" {
			t.Fatalf("cancel correlation/transition=%#v", fields)
		}
	}
	if count != want {
		t.Fatalf("session %s lifecycle controls=%d want=%d", id, count, want)
	}
}

func postInjectedRESTInvocation(ctx context.Context, baseURL, id string, timeout int64) (factoryapi.InvocationResponse, error) {
	payload := fmt.Sprintf(`{"sourceKind":"text","content":[{"type":"text","text":"owned invocation secret-value-fi-t13"}],"timeoutMillis":%d}`, timeout)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/factory-sessions/"+url.PathEscape(id)+"/invocations", strings.NewReader(payload))
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	defer response.Body.Close()
	var value factoryapi.InvocationResponse
	err = json.NewDecoder(response.Body).Decode(&value)
	if response.StatusCode != http.StatusOK {
		return value, fmt.Errorf("invocation HTTP status=%d result=%#v", response.StatusCode, value)
	}
	return value, err
}

func assertInjectedMissingSelector(t *testing.T, baseURL string, sessions factorysessions.Service, runner *fourScopeCodexRunner) {
	t.Helper()
	// B is already admitted and held at its command edge; A has not started.
	// The stable observation window proves no command dispatch for either read.
	before := runner.calls.Load()
	const missingID = "b26c8a2c-4b12-43f7-9a11-12cb69f20271"
	if _, err := sessions.Invoke(t.Context(), injectedInvocationRequest(missingID, 0, false)); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("missing-session service error=%v", err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, baseURL+"/factory-sessions/"+missingID+"/invocations", strings.NewReader(`{"content":[{"type":"text","text":"missing selector"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusNotFound || !strings.Contains(string(body), "NOT_FOUND") {
		t.Fatalf("missing selector HTTP=%d body=%s error=%v", response.StatusCode, body, err)
	}
	if after := runner.calls.Load(); after != before {
		t.Fatalf("missing selector dispatched provider work: %d -> %d", before, after)
	}
}
func awaitInjectedProviderEntry(t *testing.T, gate *isolatedCommandGate) context.Context {
	t.Helper()
	select {
	case ctx := <-gate.entered:
		return ctx
	case <-time.After(concurrentIsolationTimeout):
		t.Fatal("provider did not enter owned invocation")
		return nil
	}
}

func startInjectedInvocationHost(t *testing.T, runner *fourScopeCodexRunner) (string, factorysessions.Service, *observer.ObservedLogs) {
	t.Helper()
	api := support.NewProcessAPIServer()
	core, diagnostics := observer.New(zapcore.DebugLevel)
	process, err := root.BuildProcess(context.Background(), serviceedges.Edges{ProviderCommandRunner: runner, APIServerStarter: api.Start, ProcessLogger: zap.New(core)})
	if err != nil {
		t.Fatal(err)
	}
	support.CleanupProcess(t, process)
	host := scaffoldConcurrentIsolationFactory(t, "injected-host")
	home := t.TempDir()
	inputs := support.FakeInputs(context.Background(), []string{"you", "run", "--factory", filepath.Join(host, "factory.json"), "--continuously", "--with-server", "--quiet", "--no-record"})
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = host
	support.InitializeCustomerHomeWithProcess(t, process, inputs.Input.Env, host)
	command := support.StartProcessCommand(t, process, inputs.Input)
	t.Cleanup(func() { command.Stop(t) })
	return api.WaitForURL(t), process.FactorySessions().FactorySessions().(factorysessions.Service), diagnostics
}

func assertInjectedInvocationWaitOutcome(t *testing.T, baseURL string, sessions factorysessions.Service, diagnostics *observer.ObservedLogs, gate, peerGate *isolatedCommandGate, prompt string, timeout int64, cancelCaller bool, failure error, status factorysessions.InvocationTerminalStatus) {
	t.Helper()
	gate.failure = failure
	id := support.OpenFactorySessionAt(t, baseURL, scaffoldConcurrentIsolationFactory(t, prompt)).Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, id) })
	warmup, err := postConcurrentIsolationInvocation(t.Context(), baseURL, id, prompt+"-warmup")
	if err != nil {
		t.Fatal(err)
	}
	assertConcurrentIsolationInvocationCompleted(t, concurrentIsolationInvocation{response: warmup}, "warmup "+prompt)
	peerID, peerCtx, peer := startInjectedPeer(t, baseURL, peerGate, prompt)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan injectedInvocationOutcome, 1)
	go func() {
		if timeout > 0 {
			response, err := postInjectedRESTInvocation(ctx, baseURL, id, timeout)
			done <- injectedInvocationOutcome{value: factorysessions.InvocationResult{RequestID: response.RequestId, TraceID: response.TraceId, Status: factorysessions.InvocationTerminalStatus(response.Status), SessionID: injectedString(response.SessionId), WorkID: injectedString(response.WorkId), ErrorCode: injectedString(response.ErrorCode)}, err: err}
			return
		}
		value, err := sessions.Invoke(ctx, injectedInvocationRequest(id, timeout, false))
		done <- injectedInvocationOutcome{value, err}
	}()
	providerCtx := awaitInjectedProviderEntry(t, gate)
	if cancelCaller {
		cancel()
	}
	if failure != nil || status == factorysessions.InvocationTerminalStatusCompleted {
		close(gate.release)
	}
	select {
	case result := <-done:
		assertInjectedOutcome(t, result, id, status)
		assertInjectedAttribution(t, baseURL, diagnostics, id, result.value)
		assertInjectedWaitArtifacts(t, result.value, gate, failure, status)
	case <-time.After(concurrentIsolationTimeout):
		t.Fatal("invocation wait did not return")
	}
	finishInjectedOwnedEffect(t, baseURL, sessions, diagnostics, id, gate, peerGate, providerCtx, timeout, cancelCaller, failure, status)

	if peerCtx.Err() != nil {
		t.Fatalf("peer provider canceled: %v", peerCtx.Err())
	}
	close(peerGate.release)
	assertInjectedPeerOutcome(t, baseURL, diagnostics, peerID, awaitConcurrentIsolationInvocation(t, peer), peerGate.output)
	next, err := postConcurrentIsolationInvocation(t.Context(), baseURL, peerID, "next peer invocation")
	assertInjectedPeerOutcome(t, baseURL, diagnostics, peerID, concurrentIsolationInvocation{response: next, err: err}, peerGate.output)
	assertInjectedControlHistory(t, diagnostics, peerID, "", 0)
}

func assertInjectedPeerOutcome(t *testing.T, baseURL string, diagnostics *observer.ObservedLogs, id string, result concurrentIsolationInvocation, output string) {
	t.Helper()
	assertConcurrentIsolationInvocationCompleted(t, result, output)
	if len(*result.response.PrimaryResult) != 1 {
		t.Fatalf("peer primary content=%#v", result.response.PrimaryResult)
	}
	part, err := (*result.response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil || part.Text != output {
		t.Fatalf("peer primary text=%q want=%q error=%v", part.Text, output, err)
	}
	assertInjectedAttribution(t, baseURL, diagnostics, id, factorysessions.InvocationResult{RequestID: result.response.RequestId, TraceID: result.response.TraceId, Status: factorysessions.InvocationTerminalStatusCompleted})
}

func startInjectedPeer(t *testing.T, baseURL string, peerGate *isolatedCommandGate, prompt string) (string, context.Context, chan concurrentIsolationInvocation) {
	t.Helper()
	peerID := support.OpenFactorySessionAt(t, baseURL, scaffoldConcurrentIsolationFactory(t, prompt+"-peer")).Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, peerID) })
	peerWarmup, err := postConcurrentIsolationInvocation(t.Context(), baseURL, peerID, "peer warmup")
	if err != nil {
		t.Fatal(err)
	}
	assertConcurrentIsolationInvocationCompleted(t, concurrentIsolationInvocation{response: peerWarmup}, "warmup "+prompt+"-peer")
	peer := make(chan concurrentIsolationInvocation, 1)
	go func() {
		response, err := postConcurrentIsolationInvocation(t.Context(), baseURL, peerID, "peer invocation")
		peer <- concurrentIsolationInvocation{response: response, err: err}
	}()
	peerCtx := awaitInjectedProviderEntry(t, peerGate)
	return peerID, peerCtx, peer
}

func openInjectedTimeoutSessions(t *testing.T, baseURL string) [2]string {
	t.Helper()
	var ids [2]string
	for i, prompt := range []string{"injected-timeout-a", "injected-timeout-b"} {
		ids[i] = support.OpenFactorySessionAt(t, baseURL, scaffoldConcurrentIsolationFactory(t, prompt)).Session.Id
		id := ids[i]
		t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, id) })
		warmup, err := postConcurrentIsolationInvocation(t.Context(), baseURL, id, prompt+"-warmup")
		if err != nil {
			t.Fatal(err)
		}
		assertConcurrentIsolationInvocationCompleted(t, concurrentIsolationInvocation{response: warmup}, "warmup "+prompt)
	}
	return ids
}

func finishInjectedOwnedEffect(t *testing.T, baseURL string, sessions factorysessions.Service, diagnostics *observer.ObservedLogs, id string, gate, peerGate *isolatedCommandGate, providerCtx context.Context, timeout int64, cancelCaller bool, failure error, status factorysessions.InvocationTerminalStatus) {
	t.Helper()
	if failure == nil && status != factorysessions.InvocationTerminalStatusCompleted && providerCtx.Err() != nil {
		t.Fatalf("caller wait canceled provider without timeout control policy: %v", providerCtx.Err())
	}
	assertInjectedControlHistory(t, diagnostics, id, "", 0)
	var continued *support.FactoryEventStream
	if timeout > 0 {
		// The invocation response stream closes with its timed-out wait. The
		// canonical Factory stream owns Work continuing beyond that wait.
		continued = support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(baseURL, id))
		t.Cleanup(continued.Close)
	}
	if cancelCaller {
		control, err := sessions.Control(t.Context(), factorysessions.SessionControlRequest{SessionID: id, Mode: factorysessions.SessionOperationModeLive, Operation: factorysessions.SessionControlCancel, Control: factorysessions.ControlRequest{RequestID: "caller-owned-cancel"}})
		if err != nil {
			t.Fatalf("explicit caller cleanup=%#v error=%v", control, err)
		}
		assertInjectedControlHistory(t, diagnostics, id, "caller-owned-cancel", 1)
	} else if failure == nil && status != factorysessions.InvocationTerminalStatusCompleted {
		close(gate.release)
	}
	select {
	case err := <-gate.returned:
		if cancelCaller && !errors.Is(err, context.Canceled) {
			t.Fatalf("explicit control did not cancel owned provider: %v", err)
		}
	case <-time.After(concurrentIsolationTimeout):
		t.Fatal("owned provider did not return")
	}
	if continued != nil {
		assertInjectedContinuedWork(t, continued, gate.output)
	}
}

func assertInjectedContinuedWork(t *testing.T, stream *support.FactoryEventStream, output string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), concurrentIsolationTimeout)
	defer cancel()
	for {
		event := stream.NextEventContext(ctx)
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		encoded, err := json.Marshal(event.Payload)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), output) {
			return
		}
	}
}

func assertInjectedWorkAttribution(t *testing.T, baseURL, id string, result factorysessions.InvocationResult) {
	t.Helper()
	workSeen := false
	for _, event := range support.GetFactoryEventsForSessionAt(t, baseURL, id) {
		if injectedString(event.Context.RequestId) != result.RequestID || event.Context.WorkIds == nil {
			continue
		}
		for _, workID := range *event.Context.WorkIds {
			workSeen = workSeen || (workID != "" && (result.WorkID == "" || workID == result.WorkID))
		}
	}
	if !workSeen {
		t.Fatalf("canonical history omitted invocation request/Work correlation: %#v", result)
	}
}

func assertInjectedWaitArtifacts(t *testing.T, result factorysessions.InvocationResult, gate *isolatedCommandGate, failure error, status factorysessions.InvocationTerminalStatus) {
	t.Helper()
	if status == factorysessions.InvocationTerminalStatusCompleted && (len(result.PrimaryResult) != 1 || result.PrimaryResult[0].Text != gate.output) {
		t.Fatalf("success primary result=%#v", result.PrimaryResult)
	}
	if failure != nil && (result.FailureReason != "unknown" || result.WorkState != "task:failed" || len(result.PrimaryResult) != 0) {
		t.Fatalf("failure classification/artifact=%#v", result)
	}
}
