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
	"sync"
	"sync/atomic"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
)

// TestACPPromptDelegationStartsOneFactorySessionAndReusesItForLaterTurns
// proves the customer-facing production composition for ordinary ACP prompt
// delegation end to end: it seeds a real installed packaged Factory and a
// real persisted ACP Agent profile, calls root.BuildProcess (the exact
// public entrypoint the you binary uses), drives a real session/new call
// followed by two real session/prompt calls through Process.ACPServer(),
// and observes exactly one Factory Session start (the first, unbound turn)
// followed by exactly one reuse (the second, already-bound turn) -- with no
// second start -- against the real, singular Chat Sessions and Factory
// Sessions authorities root.BuildProcess composes, not fakes.
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

	// This story's on-demand activation deliberately keeps an opened Factory
	// target runtime alive across turns (see OnDemandFactoryTargetService's
	// own doc comment) rather than closing it after each dispatch, since
	// nothing in this ACP prompt-delegation slice's scope observes an
	// episode/session close signal yet. That means this test's runtime log
	// file handle is still open when the test function returns, which on
	// Windows can make t.TempDir()'s own automatic RemoveAll cleanup fail
	// with "file in use" -- a real, already-known, documented limitation of
	// this slice, not a bug this test should mask by suppressing it, but
	// also not what this test is about proving. A manually managed home
	// directory with a best-effort (error-tolerant) cleanup avoids that
	// unrelated flake without hiding the underlying limitation.
	home, err := os.MkdirTemp("", "acp-prompt-delegation-home-*")
	if err != nil {
		t.Fatalf("MkdirTemp(home) error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	seedInstalledPackagedFactory(t, home, "@you/goal")
	seedACPAgentProfile(t, home, "factory:@you/goal", []string{"factory:@you/goal"})

	// ProviderOverride (an in-process workers.Provider fake) is used here
	// instead of the standards-preferred ProviderCommandRunner
	// (platformprocess.CommandRunner, a subprocess-launch-shaped fake).
	// ProviderCommandRunner *can* drive this exact @you/goal fixture --
	// tests/functional/cli/named_invocation/named_invocation_test.go already
	// does, via testutil.NewProviderCommandRunner -- so the substitution here
	// is not technical necessity, it is deliberately matching the simplest
	// fake that reproduces the one outcome this test asserts on: a genuine
	// published FAILED status from @you/goal's own goal loop, which needs a
	// real child worker dispatch neither fake alone satisfies (see this
	// file's doc comment). Swapping to ProviderCommandRunner would require
	// discovering and queuing the exact raw subprocess Stdout bytes this
	// Factory's execution adapter expects across however many dispatch
	// rounds precede the real failure, coupling this test to that adapter's
	// wire format for no additional coverage of the on-demand-activation
	// behavior this test exists to prove; ProviderOverride's higher-level
	// workers.InferenceResponse shape isolates this test from that coupling.
	// Both edges are equally real external-effect ports serviceedges.Edges
	// accepts.
	provider := testutil.NewMockProvider(workers.InferenceResponse{Content: "acknowledged\n<COMPLETE>"})
	// Factory Session runtime activations are counted through the shared
	// FactorySessionIDGenerator edge. That generator is also consumed for
	// other identifiers the opened runtime mints internally while
	// dispatching (e.g. child work/dispatch IDs), so its count is not a
	// clean absolute "one activation" signal on its own -- but a second,
	// independent runtime activation for the second turn would consume the
	// generator again for that same internal bookkeeping, so the count
	// staying exactly unchanged across the second turn still proves no
	// second activation happened.
	var factorySessionIDCalls atomic.Int32
	process, err := root.BuildProcess(context.Background(), serviceedges.Edges{
		ProviderOverride: provider,
		FactorySessionIDGenerator: func() string {
			n := factorySessionIDCalls.Add(1)
			return fmt.Sprintf("acp-factory-session-id-%d", n)
		},
	})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	server := process.ACPServer()
	if server == nil {
		t.Fatal("Process.ACPServer() returned a nil acp.Server")
	}

	cwd := t.TempDir()
	sessionID := assertSessionNewReturnsDefaultTarget(t, server, cwd, "factory:@you/goal")
	if sessionID == "" {
		t.Fatal("session/new returned a blank sessionId")
	}

	firstResp := sendSessionPrompt(t, server, sessionID, "please help with this goal")
	if firstResp.Error != nil {
		t.Fatalf("first session/prompt response error = %+v, want a successful final result", firstResp.Error)
	}
	// The seeded @you/goal packaged Factory's own business workflow reaches a
	// real published FAILED terminal status against this bare mock provider
	// (its goal loop needs an actual child worker dispatch a mock alone can't
	// satisfy -- see this file's doc comment) -- a genuine downstream Factory
	// failure this production composition must still map to the documented
	// end_turn safe fallback rather than an ACP-level error, exactly like
	// this transport's already-unit-tested mapping does.
	assertPromptResponseStopReason(t, firstResp, acpsdk.StopReasonEndTurn)
	callsAfterFirstTurn := factorySessionIDCalls.Load()
	if callsAfterFirstTurn == 0 {
		t.Fatal("Factory Session ID generator was never called after the first (unbound) turn, want at least one Factory Session activation")
	}

	secondResp := sendSessionPrompt(t, server, sessionID, "a follow-up message in the same episode")
	if secondResp.Error != nil {
		t.Fatalf("second session/prompt response error = %+v, want a successful final result", secondResp.Error)
	}
	assertPromptResponseStopReason(t, secondResp, acpsdk.StopReasonEndTurn)
	if got := factorySessionIDCalls.Load(); got != callsAfterFirstTurn {
		t.Fatalf("Factory Session ID generator calls after the second (already-bound) turn = %d, want unchanged from %d (no second Factory Session activation)", got, callsAfterFirstTurn)
	}
}

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
	// Unlike this file's other integration tests, this one never reaches
	// real Factory execution -- both prompts fail during runtime-target
	// resolution, before any provider or workflow runs -- so it stays fast
	// and runs even under -short, which is exactly the lane
	// `make test-functional-coverage` uses.
	home, err := os.MkdirTemp("", "acp-prompt-delegation-unresolvable-home-*")
	if err != nil {
		t.Fatalf("MkdirTemp(home) error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	seedInstalledPackagedFactory(t, home, "@you/goal")
	seedACPAgentProfile(t, home, "factory:@you/goal", []string{"factory:@you/goal"})

	process, err := root.BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	server := process.ACPServer()
	if server == nil {
		t.Fatal("Process.ACPServer() returned a nil acp.Server")
	}

	cwd := t.TempDir()
	sessionID := assertSessionNewReturnsDefaultTarget(t, server, cwd, "factory:@you/goal")
	if sessionID == "" {
		t.Fatal("session/new returned a blank sessionId")
	}
	// Two more session/new calls, while the installed Factory, home
	// directory, and Operator Settings document are all still intact, admit
	// a third and fourth episode this test reuses below only after breaking
	// home-directory resolution and Operator Settings resolution
	// respectively -- session admission itself must not depend on either.
	thirdSessionID := assertSessionNewReturnsDefaultTarget(t, server, cwd, "factory:@you/goal")
	if thirdSessionID == "" {
		t.Fatal("session/new (third episode) returned a blank sessionId")
	}
	fourthSessionID := assertSessionNewReturnsDefaultTarget(t, server, cwd, "factory:@you/goal")
	if fourthSessionID == "" {
		t.Fatal("session/new (fourth episode) returned a blank sessionId")
	}

	// Remove the installed Factory's own directory only after session/new
	// already resolved and admitted it, so the runtime resolver's
	// named-Factory cross-root lookup -- reached only from Factory dispatch,
	// never from session/new's separate effective-catalog check -- fails at
	// prompt-dispatch time.
	globalRoot, err := factorydefinitions.NamedFactoriesRootForHome(home)
	if err != nil {
		t.Fatalf("NamedFactoriesRootForHome() error = %v", err)
	}
	if err := os.RemoveAll(filepath.Join(globalRoot, "@you", "goal")); err != nil {
		t.Fatalf("RemoveAll(installed Factory directory) error = %v", err)
	}

	firstResp := sendSessionPrompt(t, server, sessionID, "please help with this goal")
	if firstResp.Error == nil {
		t.Fatal("first session/prompt response error = nil, want a bounded internal error for an unresolvable Factory target")
	}

	secondResp := sendSessionPrompt(t, server, sessionID, "a retry after the unresolvable target failure")
	if secondResp.Error == nil {
		t.Fatal("second session/prompt response error = nil, want the same bounded rejection, not a stranded busy session")
	}

	// The third episode's prompt proves the resolver's own earlier
	// home-directory lookup failure (reached before any catalog lookup)
	// fails exactly as safely: unset both home-directory environment
	// variables only for this call.
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	thirdResp := sendSessionPrompt(t, server, thirdSessionID, "a prompt with no resolvable home directory")
	if thirdResp.Error == nil {
		t.Fatal("third session/prompt response error = nil, want a bounded internal error when the home directory cannot be resolved")
	}

	// The fourth episode's prompt proves the resolver's Operator Defaults
	// resolution failure classifies the same safe way: restore a resolvable
	// home directory and the installed Factory this episode's own target
	// still needs to resolve past the earlier catalog-lookup branch, then
	// corrupt only the persisted Operator Settings document.
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	seedInstalledPackagedFactory(t, home, "@you/goal")
	if err := os.WriteFile(operatorsettings.DefaultConfigPath(home), []byte("not valid json"), 0o644); err != nil {
		t.Fatalf("WriteFile(corrupt Operator Settings document) error = %v", err)
	}
	fourthResp := sendSessionPrompt(t, server, fourthSessionID, "a prompt with no resolvable Operator Defaults")
	if fourthResp.Error == nil {
		t.Fatal("fourth session/prompt response error = nil, want a bounded internal error when Operator Defaults cannot be resolved")
	}
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

	// A single delivery's own Factory Session ID generator call count is the
	// baseline: if a redelivered duplicate on the same connection dispatched
	// a second time, sending it twice would produce a strictly larger count
	// than sending it once, since each real activation consumes this
	// generator at least once (see the sibling test's own comment on why
	// this shared edge is an indirect but sufficient activation-count proxy).
	singleDeliveryCalls := runPromptDeliveries(t, "single-delivery", 1)
	if singleDeliveryCalls == 0 {
		t.Fatal("Factory Session ID generator was never called for a single delivery, want at least one Factory Session activation")
	}

	duplicateDeliveryCalls := runPromptDeliveries(t, "duplicate-delivery", 2)
	if duplicateDeliveryCalls != singleDeliveryCalls {
		t.Fatalf("Factory Session ID generator calls after a redelivered duplicate = %d, want unchanged from the single-delivery baseline %d (no second Factory Session activation)",
			duplicateDeliveryCalls, singleDeliveryCalls)
	}
}

// runPromptDeliveries builds one fresh, isolated root.BuildProcess
// composition, creates one session, then sends copies of the identical
// "session/prompt" request (same wire id, same connection) within a single
// Serve call, asserting every resulting response is successful. It returns
// the final Factory Session ID generator call count, so a caller can compare
// counts across a different number of identical deliveries.
func runPromptDeliveries(t *testing.T, homePrefix string, deliveries int) int32 {
	t.Helper()

	home, err := os.MkdirTemp("", "acp-prompt-delegation-"+homePrefix+"-home-*")
	if err != nil {
		t.Fatalf("MkdirTemp(home) error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	seedInstalledPackagedFactory(t, home, "@you/goal")
	seedACPAgentProfile(t, home, "factory:@you/goal", []string{"factory:@you/goal"})

	provider := testutil.NewMockProvider(workers.InferenceResponse{Content: "acknowledged\n<COMPLETE>"})
	var factorySessionIDCalls atomic.Int32
	process, err := root.BuildProcess(context.Background(), serviceedges.Edges{
		ProviderOverride: provider,
		FactorySessionIDGenerator: func() string {
			n := factorySessionIDCalls.Add(1)
			return fmt.Sprintf("acp-%s-factory-session-id-%d", homePrefix, n)
		},
	})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	server := process.ACPServer()
	if server == nil {
		t.Fatal("Process.ACPServer() returned a nil acp.Server")
	}

	cwd := t.TempDir()
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
		"prompt":    []map[string]any{{"type": "text", "text": "please help with this goal"}},
	})
	if err != nil {
		t.Fatalf("marshal session/prompt params: %v", err)
	}
	promptLine := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"session/prompt","params":%s}`, params) + "\n"
	input := strings.Repeat(promptLine, deliveries)

	var out bytes.Buffer
	if err := server.Serve(context.Background(), strings.NewReader(input), &out); err != nil {
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

	return factorySessionIDCalls.Load()
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
	if err := server.Serve(context.Background(), strings.NewReader(line), &out); err != nil {
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

// blockingProvider wraps a workers.Provider and blocks its first Infer call
// until release is closed, closing started exactly once when that call
// begins. A test uses this to deterministically observe that a Factory
// dispatch's own inference call is genuinely in flight -- proving the
// admitted turn is actually RUNNING, not merely scheduled on a goroutine --
// before sending a concurrent request, with no sleep-based synchronization.
type blockingProvider struct {
	inner   workers.Provider
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingProvider(inner workers.Provider) *blockingProvider {
	return &blockingProvider{inner: inner, started: make(chan struct{}), release: make(chan struct{})}
}

func (b *blockingProvider) Infer(ctx context.Context, req workers.ProviderInferenceRequest) (workers.InferenceResponse, error) {
	b.once.Do(func() { close(b.started) })
	<-b.release
	return b.inner.Infer(ctx, req)
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
// pkg/transports/acp/internal/stdio/session_prompt_test.go, driven against
// the real Chat Sessions Store and the real on-demand Factory Sessions
// activation instead of fakes.
func TestACPPromptDelegationConcurrentPromptRejectsAsBusyWithNoFactoryDispatch(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving root.BuildProcess Factory Session dispatch")
	}

	home, err := os.MkdirTemp("", "acp-prompt-delegation-busy-home-*")
	if err != nil {
		t.Fatalf("MkdirTemp(home) error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	seedInstalledPackagedFactory(t, home, "@you/goal")
	seedACPAgentProfile(t, home, "factory:@you/goal", []string{"factory:@you/goal"})

	provider := newBlockingProvider(testutil.NewMockProvider(workers.InferenceResponse{Content: "acknowledged\n<COMPLETE>"}))
	var factorySessionIDCalls atomic.Int32
	process, err := root.BuildProcess(context.Background(), serviceedges.Edges{
		ProviderOverride: provider,
		FactorySessionIDGenerator: func() string {
			n := factorySessionIDCalls.Add(1)
			return fmt.Sprintf("acp-busy-factory-session-id-%d", n)
		},
	})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	server := process.ACPServer()
	if server == nil {
		t.Fatal("Process.ACPServer() returned a nil acp.Server")
	}

	cwd := t.TempDir()
	sessionID := assertSessionNewReturnsDefaultTarget(t, server, cwd, "factory:@you/goal")
	if sessionID == "" {
		t.Fatal("session/new returned a blank sessionId")
	}

	firstDone := make(chan rpcMessage, 1)
	firstErr := make(chan error, 1)
	go func() {
		msg, err := doSessionPrompt(server, sessionID, "please help with this goal")
		if err != nil {
			firstErr <- err
			return
		}
		firstDone <- msg
	}()

	select {
	case <-provider.started:
	case err := <-firstErr:
		t.Fatalf("first session/prompt failed before its dispatch began: %v", err)
	}

	// Snapshot the generator count immediately before and after the
	// concurrent busy request, while the first turn's own dispatch is still
	// parked inside the blocked Infer call and so cannot itself be
	// consuming the generator concurrently. The first turn keeps consuming
	// this same generator for its own internal bookkeeping once unblocked
	// below, so "unchanged across the whole test" is not the right
	// invariant -- "unchanged across exactly the concurrent busy request" is.
	callsBeforeConcurrent := factorySessionIDCalls.Load()
	if callsBeforeConcurrent == 0 {
		t.Fatal("Factory Session ID generator was never called for the in-flight first turn")
	}

	concurrentResp, err := doSessionPrompt(server, sessionID, "a concurrent prompt while the turn is busy")
	if err != nil {
		t.Fatalf("concurrent session/prompt: %v", err)
	}
	if concurrentResp.Error == nil {
		t.Fatal("concurrent session/prompt response error = nil, want a bounded rejection for a busy session")
	}
	if got := factorySessionIDCalls.Load(); got != callsBeforeConcurrent {
		t.Fatalf("Factory Session ID generator calls changed by %d during the concurrent busy request, want unchanged from %d (the busy rejection must make zero Factory effect)",
			got-callsBeforeConcurrent, callsBeforeConcurrent)
	}

	close(provider.release)

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
	laterResp := sendSessionPrompt(t, server, sessionID, "a later prompt after the busy rejection resolved")
	if laterResp.Error != nil {
		t.Fatalf("later session/prompt response error = %+v, want a successful final result", laterResp.Error)
	}
	assertPromptResponseStopReason(t, laterResp, acpsdk.StopReasonEndTurn)
}
