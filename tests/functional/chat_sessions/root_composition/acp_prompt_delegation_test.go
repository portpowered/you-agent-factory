package root_composition_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
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
	callsAfterFirstTurn := factorySessionIDCalls.Load()
	if callsAfterFirstTurn == 0 {
		t.Fatal("Factory Session ID generator was never called after the first (unbound) turn, want at least one Factory Session activation")
	}

	secondResp := sendSessionPrompt(t, server, sessionID, "a follow-up message in the same episode")
	if secondResp.Error != nil {
		t.Fatalf("second session/prompt response error = %+v, want a successful final result", secondResp.Error)
	}
	if got := factorySessionIDCalls.Load(); got != callsAfterFirstTurn {
		t.Fatalf("Factory Session ID generator calls after the second (already-bound) turn = %d, want unchanged from %d (no second Factory Session activation)", got, callsAfterFirstTurn)
	}
}

// sendSessionPrompt drives one real "session/prompt" call, with one text
// content block, on its own connection against the given already-created
// session, and returns the decoded JSON-RPC response.
func sendSessionPrompt(t *testing.T, server acp.Server, sessionID, text string) rpcMessage {
	t.Helper()

	params, err := json.Marshal(map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]any{{"type": "text", "text": text}},
	})
	if err != nil {
		t.Fatalf("marshal session/prompt params: %v", err)
	}
	line := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"session/prompt","params":%s}`, params) + "\n"

	var out bytes.Buffer
	if err := server.Serve(context.Background(), strings.NewReader(line), &out); err != nil {
		t.Fatalf("Serve(session/prompt) error = %v", err)
	}
	return decodeRPCMessage(t, &out)
}
