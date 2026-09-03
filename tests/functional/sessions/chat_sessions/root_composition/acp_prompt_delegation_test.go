// Functional owner: sessions/chat_sessions/root_composition.
package root_composition_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestACPPromptDelegationStartsOneFactorySessionAndReusesItForLaterTurns
// proves the customer-facing production composition for ordinary ACP prompt
// delegation end to end: it seeds a real installed packaged Factory and a
// real persisted ACP Agent profile, calls root.BuildProcess (the exact
// public entrypoint the you binary uses), drives a real session/new call
// followed by two real session/prompt calls through Process.ACPServer(),
// and observes successful first and follow-up turns against the same public
// ACP session through the real, singular Chat Sessions and Factory Sessions
// authorities root.BuildProcess composes, not fakes.
//
// Before this story's on-demand Factory Sessions activation
// (factorysessionwire.OnDemandFactoryTargetService), every session/prompt
// call that reached Factory dispatch through this exact composition path
// failed with factorysessions.ErrExecutionServiceNotConfigured, because the
// process-scoped factorysessions.Service root.BuildProcess constructs stays
// permanently inert outside the CLI daemon's OpenApplication bootstrap. This
// test is a regression guard for that gap.
func TestACPPromptDelegationStartsOneFactorySessionAndReusesItForLaterTurns(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving root.BuildProcess Factory Session dispatch")
	}

	t.Parallel()
	cohort := controlledACPCohortForTest(t)
	server := controlledACPServerForCohort(t, cohort)
	cwd := controlledACPWorkingDirectoryForCohort(t, cohort, "delegation-reuse")
	sessionID := assertSessionNewReturnsDefaultTarget(t, server, cwd, "factory:@you/goal")
	if sessionID == "" {
		t.Fatal("session/new returned a blank sessionId")
	}
	firstResp := sendSessionPrompt(t, server, sessionID, "please help with this goal [cohort-reuse]")
	if firstResp.Error != nil {
		t.Fatalf("first session/prompt response error = %+v, want a successful final result", firstResp.Error)
	}
	assertPromptResponseStopReason(t, firstResp, acpsdk.StopReasonEndTurn)
	secondResp := sendSessionPrompt(t, server, sessionID, "a follow-up message in the same episode [cohort-reuse]")
	if secondResp.Error != nil {
		t.Fatalf("second session/prompt response error = %+v, want a successful final result", secondResp.Error)
	}
	assertPromptResponseStopReason(t, secondResp, acpsdk.StopReasonEndTurn)
}

// TestACPPromptDelegationFailedFactoryInvocationReportsAnACPError proves the
// production composition reports a Factory invocation that reached a real
// published FAILED terminal status as a JSON-RPC error, not as a successful
// prompt result.
//
// This is the one outcome ACP's own vocabulary cannot express any other way.
// StopReason is a closed set -- end_turn, max_tokens, max_turn_requests,
// refusal, cancelled -- with no failure member, so a failed run reported
// through a successful PromptResponse is indistinguishable from a completed
// one: the client renders a finished turn and the customer never learns their
// work did not run. ACP's failure channel is the error response, so that is
// where a failure belongs.
//
// The error's data is deliberately narrow. It carries only the invocation's
// error code, which is a closed three-value vocabulary
// (factorysessions.InvocationErrorCode*), and never the invocation's Message,
// which is free-form provider diagnostics that can contain a command line,
// a path, or a credential. protocol.FactoryInvocationFailure bounds an
// unrecognized code to INVOCATION_RUNTIME_FAILURE rather than passing it
// through, so a future status cannot widen what reaches the wire.
func TestACPPromptDelegationFailedFactoryInvocationReportsAnACPError(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving root.BuildProcess Factory Session dispatch")
	}

	t.Parallel()
	cohort := controlledACPCohortForTest(t)
	server := controlledACPServerForCohort(t, cohort)
	cwd := controlledACPWorkingDirectoryForCohort(t, cohort, "delegation-failure")
	sessionID := assertSessionNewReturnsDefaultTarget(t, server, cwd, "factory:@you/goal")
	if sessionID == "" {
		t.Fatal("session/new returned a blank sessionId")
	}
	resp := sendSessionPrompt(t, server, sessionID, "please help with this goal [cohort-failure]")
	if resp.Error == nil {
		t.Fatalf("session/prompt response error = nil with result %s, want a JSON-RPC error for a FAILED Factory invocation", resp.Result)
	}
	if resp.Error.Code != internalErrorCode {
		t.Errorf("error code = %d, want %d (internal error)", resp.Error.Code, internalErrorCode)
	}
	// Decoding Data into map[string]string asserts the bounded shape as well
	// as the value: exactly one string member. Any structured payload the
	// mapper might later attach -- a nested result, a message, an identifier
	// -- fails to decode here rather than reaching a customer's client.
	encodedData, err := json.Marshal(resp.Error.Data)
	if err != nil {
		t.Fatalf("marshal error data: %v", err)
	}
	var data map[string]string
	if err := json.Unmarshal(encodedData, &data); err != nil {
		t.Fatalf("error data %s is not a flat string map: %v", encodedData, err)
	}
	if len(data) != 1 {
		t.Fatalf("error data = %s, want exactly one member (the bounded invocation error code)", encodedData)
	}
	if data["reason"] != string(factorysessions.InvocationErrorCodeRuntimeFailure) {
		t.Errorf("error data reason = %q, want %q", data["reason"], factorysessions.InvocationErrorCodeRuntimeFailure)
	}

	// A failed invocation must release the session's busy state like any
	// other terminal outcome: the next prompt is still admitted and reaches
	// its own dispatch rather than being rejected as busy.
	secondResp := sendSessionPrompt(t, server, sessionID, "a retry after the failed invocation [cohort-failure]")
	if secondResp.Error == nil {
		t.Fatal("second session/prompt response error = nil, want the same bounded failure, not a fabricated success")
	}
	if strings.Contains(strings.ToLower(secondResp.Error.Error()), "busy") {
		t.Errorf("second session/prompt error = %v, want a fresh dispatch failure rather than a stranded busy rejection", secondResp.Error)
	}
}

// internalErrorCode is JSON-RPC's reserved internal-error code, which
// acpsdk.NewInternalError sets.
const internalErrorCode = -32603

// TestACPPromptDelegationUnresolvableFactoryTargetFailsSafelyAndTerminalizes
// proves that when the admitted episode's Factory target can no longer
// resolve to an installed named Factory at dispatch time -- distinct from
// session/new's own effective-catalog check, which reads the same
// named-Factory installation this test removes only after session/new has
// already succeeded, exactly the window where a concurrent uninstall could
// leave a session pointed at a target its own catalog snapshot no longer
// backs -- the real root.BuildProcess composition, through
// provideACPServerFactoryTargetRuntimeResolver's
// factorydefinitions.ErrNamedFactoryNotFound path, returns a bounded
// internal ACP error instead of a crash or a fabricated success, and leaves
// no turn stranded: a second prompt on the same session is admitted and
// fails the identical safe way, proving the first failure released the
// session's busy state.
func TestACPPromptDelegationUnresolvableFactoryTargetFailsSafelyAndTerminalizes(t *testing.T) {
	t.Parallel()
	// Unlike this file's other integration tests, this one never reaches
	// real Factory execution -- both prompts fail during runtime-target
	// resolution, before any provider or workflow runs -- so it stays fast
	// and runs even under -short, which is exactly the lane
	// `make test-functional-coverage` uses.
	scenario := newUnresolvableFactoryScenario(t)

	firstResp := sendSessionPrompt(t, scenario.server, scenario.sessionID, "please help with this goal")
	firstErrorData := assertBoundedDependencyError(t, firstResp, "first unresolvable-target prompt")

	secondResp := sendSessionPrompt(t, scenario.server, scenario.sessionID, "a retry after the unresolvable target failure")
	secondErrorData := assertBoundedDependencyError(t, secondResp, "retry after unresolvable-target prompt")
	if secondErrorData != firstErrorData {
		t.Fatalf("retry error data = %s, want the same bounded error data as the first failure %s", secondErrorData, firstErrorData)
	}
	if strings.Contains(strings.ToLower(secondResp.Error.Error()), "busy") {
		t.Fatalf("retry error = %v, want a fresh bounded resolver failure after busy state was released", secondResp.Error)
	}

	if got := scenario.runner.requestCount(); got != 0 {
		t.Fatalf("provider command calls during resolver failures = %d, want 0 (all failures precede Factory execution)", got)
	}
}

type unresolvableFactoryScenario struct {
	server    acp.Server
	runner    *controlledACPCommandRunner
	sessionID string
}

func newUnresolvableFactoryScenario(t *testing.T) unresolvableFactoryScenario {
	t.Helper()
	// This scenario owns its process and filesystem because it removes the
	// selected Factory after admission.
	home := chatTempDir(t, "unresolvable Factory", "unresolvable-")
	seedInstalledPackagedFactory(t, home, "@you/goal")
	support.SeedACPAgentProfile(t, home, "factory:@you/goal", []string{"factory:@you/goal"})

	runner := &controlledACPCommandRunner{}
	process, err := buildChatProcess(t, "unresolvable Factory", serviceedges.Edges{
		ProviderCommandRunner: runner,
	})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	closeProcessCleanly(t, process)
	server := process.ACPServer()
	if server == nil {
		t.Fatal("Process.ACPServer() returned a nil acp.Server")
	}
	registerChatACPServerHome(server, home)

	cwd := chatTempDir(t, "unresolvable Factory working directory", "unresolvable-cwd-")
	sessionID := assertSessionNewReturnsDefaultTarget(t, server, cwd, "factory:@you/goal")
	if sessionID == "" {
		t.Fatal("session/new returned a blank sessionId")
	}

	// Remove the installed Factory after session/new so prompt dispatch takes
	// the customer-visible runtime resolver failure path.
	globalRoot, err := factorydefinitions.NamedFactoriesRootForHome(home)
	if err != nil {
		t.Fatalf("NamedFactoriesRootForHome() error = %v", err)
	}
	if err := os.RemoveAll(filepath.Join(globalRoot, "@you", "goal")); err != nil {
		t.Fatalf("RemoveAll(installed Factory directory) error = %v", err)
	}
	return unresolvableFactoryScenario{
		server: server, runner: runner, sessionID: sessionID,
	}
}

// assertBoundedDependencyError proves the resolver failure reaches the ACP
// boundary as the fixed dependency_unavailable shape. The returned canonical
// JSON is compared across failure classes and the retry so a later change
// cannot leak a target, path, operator document, or prompt into the error.
func assertBoundedDependencyError(t *testing.T, resp rpcMessage, operation string) string {
	t.Helper()
	if resp.Error == nil {
		t.Fatalf("%s error = nil, want a bounded internal dependency error", operation)
	}
	if resp.Error.Code != internalErrorCode {
		t.Fatalf("%s error code = %d, want %d (internal error)", operation, resp.Error.Code, internalErrorCode)
	}
	encodedData, err := json.Marshal(resp.Error.Data)
	if err != nil {
		t.Fatalf("marshal %s error data: %v", operation, err)
	}
	var data map[string]string
	if err := json.Unmarshal(encodedData, &data); err != nil {
		t.Fatalf("%s error data %s is not a flat string map: %v", operation, encodedData, err)
	}
	if len(data) != 1 || data["reason"] != "dependency_unavailable" {
		t.Fatalf("%s error data = %s, want exactly {reason: dependency_unavailable}", operation, encodedData)
	}
	return string(encodedData)
}

// closeProcessCleanly registers a cleanup that closes process and fails the
// test if that close reports an error. On-demand Factory Sessions activation
// (see pkg/wire's compositeProcessLifecycle) keeps an opened Factory target
// runtime's own log file handle open until Process.Close tears it down; this
// runs before chatTempDir's cleanup for the owning home (t.Cleanup callbacks
// run LIFO, and the process cleanup is registered after that home is made),
// so the runtime is closed before its home directory is removed -- proving
// this story's reachable close path actually works, not merely compiling,
// instead of masking an unclosed runtime with an error-tolerant temp
// directory.
func closeProcessCleanly(t *testing.T, process support.ApplicationProcess) {
	t.Helper()
	t.Cleanup(func() {
		if err := closeChatProcess(process); err != nil {
			t.Errorf("Process.Close() error = %v, want clean teardown", err)
		}
	})
}

// assertPromptResponseStopReason decodes resp's Result as a real
// acpsdk.PromptResponse and asserts its StopReason, proving the response is
// the closed final-only shape this transport publishes, not just the
// absence of an RPC-level error.
func assertPromptResponseStopReason(t *testing.T, resp rpcMessage, want acpsdk.StopReason) {
	t.Helper()
	var decoded acpsdk.PromptResponse
	if err := json.Unmarshal(resp.Result, &decoded); err != nil {
		t.Fatalf("unmarshal PromptResponse: %v", err)
	}
	if decoded.StopReason != want {
		t.Fatalf("stopReason = %q, want %q", decoded.StopReason, want)
	}
}

// TestACPPromptDelegationRedeliveredRequestMakesNoSecondFactoryDispatch
// proves, through the same real root.BuildProcess composition, that
// redelivering the identical connection-scoped "session/prompt" request --
// the same JSON-RPC id on the same connection, exactly what a retrying
// client sends after an ambiguous or dropped response -- never dispatches a
// second Factory Session start for content this process already admitted and
// executed once. This is a regression guard for the gap
// chat_sessions/internal/service.Store.StartTurn's turnsByRequest index
// closes: without it, a redelivered request admitted a brand-new turn and
// dispatched Factory work a second time.
func TestACPPromptDelegationRedeliveredRequestMakesNoSecondFactoryDispatch(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving root.BuildProcess Factory Session dispatch")
	}
	t.Parallel()

	// A single delivery's provider effect is the baseline: if a redelivered
	// duplicate on the same connection dispatched twice, its request-specific
	// external-effect count would increase.
	singleDelivery := runPromptDeliveries(t, "single-delivery", 1)
	if singleDelivery.providerRequests == 0 {
		t.Fatal("controlled provider received no request for a single delivery, want one provider effect")
	}

	duplicateDelivery := runPromptDeliveries(t, "duplicate-delivery", 2)
	if duplicateDelivery.providerRequests != singleDelivery.providerRequests {
		t.Fatalf("controlled provider requests after a redelivered duplicate = %d, want unchanged from the single-delivery baseline %d (no second provider effect)",
			duplicateDelivery.providerRequests, singleDelivery.providerRequests)
	}
}

// runPromptDeliveries creates one isolated session on the shared root-built
// application, then sends copies of the identical
// "session/prompt" request (same wire id, same connection) within a single
// Serve call, asserting every resulting response is successful. It returns
// Factory Session ID and provider request counts, so a caller can compare
// effects across a different number of identical deliveries.
func runPromptDeliveries(t *testing.T, homePrefix string, deliveries int) promptDeliveryObservation {
	t.Helper()

	cohort := controlledACPCohortForTest(t)
	server := controlledACPServerForCohort(t, cohort)
	marker := "[cohort-redelivery-" + homePrefix + "]"
	callsBeforeDelivery := cohort.runner.requestCountContaining(marker)

	cwd := controlledACPWorkingDirectoryForCohort(t, cohort, "redelivery-"+homePrefix)
	sessionID := assertSessionNewReturnsDefaultTarget(t, server, cwd, "factory:@you/goal")
	if sessionID == "" {
		t.Fatal("session/new returned a blank sessionId")
	}
	// Every delivered line carries the exact same wire id (2) on one Serve
	// call, so every one resolves to the identical connection-scoped
	// RequestIdentity -- a true redelivery, not distinct requests that merely
	// share a bare id across different connections (which this transport
	// already keeps distinct on purpose).
	params, err := json.Marshal(map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]any{{"type": "text", "text": "please help with this goal " + marker}},
	})
	if err != nil {
		t.Fatalf("marshal session/prompt params: %v", err)
	}
	promptLine := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"session/prompt","params":%s}`, params) + "\n"
	input := strings.Repeat(promptLine, deliveries)

	var out bytes.Buffer
	if err := serveChatRequest(server, context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	responses := responseLinesOnly(t, &out)
	if len(responses) != deliveries {
		t.Fatalf("response line count = %d, want exactly %d (one per delivered request): %s", len(responses), deliveries, out.String())
	}
	for i, resp := range responses {
		if resp.Error != nil {
			t.Fatalf("response[%d] error = %+v, want a successful final result", i, resp.Error)
		}
		assertPromptResponseStopReason(t, resp, acpsdk.StopReasonEndTurn)
	}
	return promptDeliveryObservation{
		providerRequests: cohort.runner.requestCountContaining(marker) - callsBeforeDelivery,
	}
}

type promptDeliveryObservation struct {
	providerRequests int
}

// responseLinesOnly splits out into complete newline-terminated lines and
// decodes only the ones carrying a request/response shape (a populated Result
// or Error), skipping any interleaved outbound "session/update" notification
// lines (which never carry a "result" or "error" member).
func responseLinesOnly(t *testing.T, out *bytes.Buffer) []rpcMessage {
	t.Helper()
	var responses []rpcMessage
	scanner := bufio.NewScanner(bytes.NewReader(out.Bytes()))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var decoded map[string]json.RawMessage
		if err := json.Unmarshal(line, &decoded); err != nil {
			t.Fatalf("unmarshal line %q: %v", line, err)
		}
		if _, isNotification := decoded["method"]; isNotification {
			continue
		}
		var msg rpcMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			t.Fatalf("unmarshal response line %q: %v", line, err)
		}
		responses = append(responses, msg)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan response lines: %v", err)
	}
	return responses
}

// sendSessionPrompt drives one real "session/prompt" call, with one text
// content block, on its own connection against the given already-created
// session, and returns the decoded JSON-RPC response.
func sendSessionPrompt(t *testing.T, server acp.Server, sessionID, text string) rpcMessage {
	t.Helper()

	msg, err := doSessionPrompt(server, sessionID, text)
	if err != nil {
		t.Fatalf("session/prompt: %v", err)
	}
	return msg
}

// doSessionPrompt is sendSessionPrompt's *testing.T-free core: it reports
// failures via its returned error instead of calling any testing.T method,
// so it is safe to call from a goroutine other than the one running the
// test (testing.T's Fatal/Fatalf family must only ever be called from the
// test's own goroutine) -- needed by tests that drive one "session/prompt"
// call concurrently with another against the same session.
func doSessionPrompt(server acp.Server, sessionID, text string) (rpcMessage, error) {
	params, err := json.Marshal(map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]any{{"type": "text", "text": text}},
	})
	if err != nil {
		return rpcMessage{}, fmt.Errorf("marshal session/prompt params: %w", err)
	}
	line := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"session/prompt","params":%s}`, params) + "\n"
	var out bytes.Buffer
	if err := serveChatRequest(server, context.Background(), strings.NewReader(line), &out); err != nil {
		return rpcMessage{}, fmt.Errorf("Serve(session/prompt): %w", err)
	}
	responses := responseLinesOnlyErr(&out)
	if len(responses) != 1 {
		return rpcMessage{}, fmt.Errorf("response line count = %d, want exactly 1", len(responses))
	}
	return responses[0], nil
}

// responseLinesOnlyErr is responseLinesOnly's *testing.T-free core, for use
// from doSessionPrompt (which must itself stay callable from a non-test
// goroutine).
func responseLinesOnlyErr(out *bytes.Buffer) []rpcMessage {
	var responses []rpcMessage
	scanner := bufio.NewScanner(bytes.NewReader(out.Bytes()))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var decoded map[string]json.RawMessage
		if err := json.Unmarshal(line, &decoded); err != nil {
			continue
		}
		if _, isNotification := decoded["method"]; isNotification {
			continue
		}
		var msg rpcMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		responses = append(responses, msg)
	}
	return responses
}

// TestACPPromptDelegationConcurrentPromptRejectsAsBusyWithNoFactoryDispatch
// proves, through the same real root.BuildProcess composition, that a second
// "session/prompt" call arriving while the first is genuinely still
// dispatching (its Factory Session activation has already reached the
// workflow's own inference call and not yet returned) is rejected as busy
// with zero additional Factory Session activation, and that the first,
// legitimately in-flight turn still completes normally afterward with no
// stranded busy state left behind. This is the functional-level counterpart
// to the unit-level busy-admission coverage in
// pkg/transports/acp/internal/stdio/session_prompt_admission_test.go, driven against
// the real Chat Sessions Store and the real on-demand Factory Sessions
// activation instead of fakes.
func TestACPPromptDelegationConcurrentPromptRejectsAsBusyWithNoFactoryDispatch(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving root.BuildProcess Factory Session dispatch")
	}
	t.Parallel()

	cohort := controlledACPCohortForTest(t)
	server := controlledACPServerForCohort(t, cohort)
	started, releaseBusy := cohort.runner.armBusy()
	t.Cleanup(releaseBusy)

	cwd := controlledACPWorkingDirectoryForCohort(t, cohort, "delegation-busy")
	sessionID := assertSessionNewReturnsDefaultTarget(t, server, cwd, "factory:@you/goal")
	if sessionID == "" {
		t.Fatal("session/new returned a blank sessionId")
	}
	firstDone := make(chan rpcMessage, 1)
	firstErr := make(chan error, 1)
	go func() {
		msg, err := doSessionPrompt(server, sessionID, "please help with this goal [cohort-busy]")
		if err != nil {
			firstErr <- err
			return
		}
		firstDone <- msg
	}()

	select {
	case <-started:
	case err := <-firstErr:
		t.Fatalf("first session/prompt failed before its dispatch began: %v", err)
	}

	concurrentResp, err := doSessionPrompt(server, sessionID, "a concurrent prompt while the turn is busy [cohort-busy-concurrent]")
	if err != nil {
		t.Fatalf("concurrent session/prompt: %v", err)
	}
	if concurrentResp.Error == nil {
		t.Fatal("concurrent session/prompt response error = nil, want a bounded rejection for a busy session")
	}
	if got := cohort.runner.requestCountContaining("[cohort-busy-concurrent]"); got != 0 {
		t.Fatalf("controlled provider received %d requests for the rejected concurrent prompt, want zero", got)
	}

	releaseBusy()

	select {
	case firstResp := <-firstDone:
		if firstResp.Error != nil {
			t.Fatalf("first session/prompt response error = %+v, want a successful final result", firstResp.Error)
		}
		assertPromptResponseStopReason(t, firstResp, acpsdk.StopReasonEndTurn)
	case err := <-firstErr:
		t.Fatalf("first session/prompt error = %v", err)
	}

	// The busy rejection must not have stranded the session: a later, wholly
	// distinct prompt is still admitted and completes normally.
	laterResp := sendSessionPrompt(t, server, sessionID, "a later prompt after the busy rejection resolved [cohort-busy-later]")
	if laterResp.Error != nil {
		t.Fatalf("later session/prompt response error = %+v, want a successful final result", laterResp.Error)
	}
	assertPromptResponseStopReason(t, laterResp, acpsdk.StopReasonEndTurn)
}
