package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/session"
)

// promptTextParams builds a raw session/prompt params payload carrying one
// text content block, the shape a client sends for a typed "/factory
// <value>" command or an ordinary chat message alike.
func promptTextParams(sessionID, text string) string {
	raw, err := json.Marshal(map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]any{{"type": "text", "text": text}},
	})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func TestParseFactoryCommandRecognizesExactFormOnly(t *testing.T) {
	tests := []struct {
		name        string
		content     []session.TextContent
		wantValue   string
		wantMatched bool
		wantErr     bool
	}{
		{name: "exact command", content: []session.TextContent{{Text: "/factory factory:@you/review"}}, wantValue: "factory:@you/review", wantMatched: true},
		{name: "missing value", content: []session.TextContent{{Text: "/factory"}}, wantMatched: true, wantErr: true},
		{name: "extra token", content: []session.TextContent{{Text: "/factory a b"}}, wantMatched: true, wantErr: true},
		{name: "unrelated prompt", content: []session.TextContent{{Text: "hello there"}}, wantMatched: false},
		{name: "similar but different command", content: []session.TextContent{{Text: "/factories factory:@you/review"}}, wantMatched: false},
		{name: "multiple content blocks", content: []session.TextContent{{Text: "/factory"}, {Text: "factory:@you/review"}}, wantMatched: false},
		{name: "leading whitespace", content: []session.TextContent{{Text: "   /factory factory:@you/review  "}}, wantValue: "factory:@you/review", wantMatched: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			value, matched, err := parseFactoryCommand(tc.content)
			if matched != tc.wantMatched {
				t.Fatalf("matched = %v, want %v", matched, tc.wantMatched)
			}
			if tc.wantErr && err == nil {
				t.Fatal("err = nil, want a malformed-command error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if value != tc.wantValue {
				t.Fatalf("value = %q, want %q", value, tc.wantValue)
			}
		})
	}
}

func TestHandleSessionPromptFactoryCommandDelegatesToChangeTarget(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "/factory factory:@you/review"))

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr != nil {
		t.Fatalf("handleSessionPrompt() error = %+v, want success", rpcErr)
	}

	var resp acpsdk.PromptResponse
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.StopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("stopReason = %q, want end_turn", resp.StopReason)
	}

	if !chatSessions.setTargetCalled {
		t.Fatal("SetTarget was not called, want exactly one target change")
	}
	wantTarget := chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/review"}
	if chatSessions.setTargetReq.Target != wantTarget {
		t.Fatalf("SetTarget Target = %+v, want %+v", chatSessions.setTargetReq.Target, wantTarget)
	}
	if chatSessions.setTargetReq.ExpectedVersion != 3 {
		t.Fatalf("SetTarget ExpectedVersion = %d, want 3", chatSessions.setTargetReq.ExpectedVersion)
	}
	if chatSessions.startTurnCalled {
		t.Fatal("StartTurn was called, want no prompt turn started for the /factory fallback command")
	}
}

func TestHandleSessionPromptFactoryCommandMissingValueRejectsWithNoMutation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "/factory"))

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a rejection for a missing /factory value")
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil on rejection", result)
	}
	if chatSessions.getSessionCalled {
		t.Fatal("GetSession was called, want no effect for a malformed /factory command")
	}
	if chatSessions.setTargetCalled {
		t.Fatal("SetTarget was called, want no mutation for a malformed /factory command")
	}
	if len(catalog.calls) != 0 {
		t.Fatalf("catalog resolved %d times, want 0 for a malformed /factory command", len(catalog.calls))
	}
}

func TestHandleSessionPromptFactoryCommandExtraTokensRejectsWithNoMutation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "/factory factory:@you/review extra"))

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a rejection for an extra-token /factory command")
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil on rejection", result)
	}
	if chatSessions.setTargetCalled {
		t.Fatal("SetTarget was called, want no mutation for an extra-token /factory command")
	}
}

func TestHandleSessionPromptFactoryCommandDisallowedTargetRejectsWithNoMutation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{err: &chatsessions.FactoryTargetCatalogError{
		Target: "factory:@you/not-allowed",
		Err:    chatsessions.ErrFactoryTargetNotAllowed,
	}}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "/factory factory:@you/not-allowed"))

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a rejection for a disallowed target")
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil on rejection", result)
	}
	if chatSessions.setTargetCalled {
		t.Fatal("SetTarget was called, want no mutation for a disallowed target")
	}
}

func TestHandleSessionPromptFactoryCommandStaleVersionRejectsWithNoMutation(t *testing.T) {
	chatSessions := &fakeChatSessionsService{
		getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project"),
		setTargetErr:     &chatsessions.ConflictError{Value: "Session", ID: "session-1", Expected: 3, Actual: 4},
	}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "/factory factory:@you/review"))

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a rejection for a stale expected version")
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil on rejection", result)
	}
}

func TestHandleSessionPromptFactoryCommandWorkingRootIncompatibleRejects(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{err: &chatsessions.FactoryTargetCatalogError{
		Target: "factory:@you/pinned",
		Err:    chatsessions.ErrFactoryTargetWorkingRootIncompatible,
	}}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "/factory factory:@you/pinned"))

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a rejection for a working-root-incompatible target")
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil on rejection", result)
	}
	if chatSessions.setTargetCalled {
		t.Fatal("SetTarget was called, want no mutation for a working-root-incompatible target")
	}
}

func TestHandleSessionPromptFactoryCommandFailureNeverLeaksRawValueOrRoot(t *testing.T) {
	sensitiveTarget := "factory:@you/sk_live_should_never_leak"
	sensitiveRoot := "/home/operator/should-never-leak"
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, sensitiveRoot)}
	catalog := &fakeFactoryTargetCatalogService{err: &chatsessions.FactoryTargetCatalogError{
		Target: sensitiveTarget,
		Err:    chatsessions.ErrFactoryTargetNotInstalled,
	}}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "/factory "+sensitiveTarget))

	_, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil {
		t.Fatal("handleSessionPrompt() error = nil, want a rejection")
	}
	encoded, err := json.Marshal(rpcErr)
	if err != nil {
		t.Fatalf("marshal rpc error: %v", err)
	}
	if strings.Contains(string(encoded), sensitiveTarget) {
		t.Fatalf("rpc error %s leaked the raw requested target", encoded)
	}
	if strings.Contains(string(encoded), sensitiveRoot) {
		t.Fatalf("rpc error %s leaked the raw working root", encoded)
	}
}

// TestHandleSessionPromptNonCommandContentFallsThroughToMethodNotFound
// proves ordinary prompt content -- anything not an exact "/factory
// <value>" command attempt -- still receives method-not-found, exactly the
// behavior "session/prompt" had before this command was recognized: this
// story adds only the "/factory" fallback, not general prompt-turn
// admission or Factory invocation.
func TestHandleSessionPromptNonCommandContentFallsThroughToMethodNotFound(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams("session-1", "hello there"))

	result, rpcErr := server.handleSessionPrompt(context.Background(), env)
	if rpcErr == nil || rpcErr.Code != -32601 {
		t.Fatalf("error = %+v, want method-not-found (-32601) for non-command prompt content", rpcErr)
	}
	if result != nil {
		t.Fatalf("handleSessionPrompt() result = %q, want nil", result)
	}
	if chatSessions.getSessionCalled || chatSessions.setTargetCalled || chatSessions.startTurnCalled {
		t.Fatal("a Chat Sessions call was made, want no effect for non-command prompt content")
	}
	if len(catalog.calls) != 0 {
		t.Fatalf("catalog resolved %d times, want 0 for non-command prompt content", len(catalog.calls))
	}
}

func TestServeDispatchesSessionPromptFactoryCommandOverRealJSONRPCFraming(t *testing.T) {
	chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	server := newTestServer(chatSessions, catalog, "/home/operator")

	input := `{"jsonrpc":"2.0","id":1,"method":"session/prompt","params":` +
		promptTextParams("session-1", "/factory factory:@you/review") + `}` + "\n"
	out := &bytes.Buffer{}
	if err := server.Serve(context.Background(), strings.NewReader(input), out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	resp := assertSingleResponseLine(t, out)
	if string(resp.ID) != "1" {
		t.Fatalf("id = %s, want 1", resp.ID)
	}
	if resp.Error != nil {
		t.Fatalf("error = %+v, want a successful result", resp.Error)
	}
	if !chatSessions.setTargetCalled {
		t.Fatal("SetTarget was not called over the real Serve path")
	}
}

// TestFactoryFallbackAndSetConfigOptionProduceEquivalentEffects is the
// parity test story 004 asks for: the same session, catalog, and requested
// target run through both entry paths -- "session/set_config_option" and
// the "/factory <value>" prompt fallback -- and produce the exact same
// SetTarget call (target, session id, expected version), proving neither
// path duplicates catalog filtering, reference parsing, expected-version
// policy, state mutation, or error translation, and that success mutates
// target exactly once per path with no prompt turn or Factory execution.
func TestFactoryFallbackAndSetConfigOptionProduceEquivalentEffects(t *testing.T) {
	buildFixtures := func() (*fakeChatSessionsService, *fakeFactoryTargetCatalogService, *Server) {
		chatSessions := &fakeChatSessionsService{getSessionResult: sessionAt("session-1", "factory:@you/factory-builder", 3, "/work/project")}
		catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
		return chatSessions, catalog, newTestServer(chatSessions, catalog, "/home/operator")
	}

	t.Run("success", func(t *testing.T) {
		viaConfigOption, _, configServer := buildFixtures()
		env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionSetConfigOption,
			setConfigOptionParams("session-1", "factory:@you/review"))
		if _, rpcErr := configServer.handleSessionSetConfigOption(context.Background(), env); rpcErr != nil {
			t.Fatalf("handleSessionSetConfigOption() error = %+v, want success", rpcErr)
		}

		viaPrompt, _, promptServer := buildFixtures()
		promptEnv := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
			promptTextParams("session-1", "/factory factory:@you/review"))
		if _, rpcErr := promptServer.handleSessionPrompt(context.Background(), promptEnv); rpcErr != nil {
			t.Fatalf("handleSessionPrompt() error = %+v, want success", rpcErr)
		}

		if viaConfigOption.setTargetReq.Target != viaPrompt.setTargetReq.Target {
			t.Fatalf("SetTarget Target differs: config_option=%+v prompt=%+v", viaConfigOption.setTargetReq.Target, viaPrompt.setTargetReq.Target)
		}
		if viaConfigOption.setTargetReq.SessionID != viaPrompt.setTargetReq.SessionID {
			t.Fatalf("SetTarget SessionID differs: config_option=%q prompt=%q", viaConfigOption.setTargetReq.SessionID, viaPrompt.setTargetReq.SessionID)
		}
		if viaConfigOption.setTargetReq.ExpectedVersion != viaPrompt.setTargetReq.ExpectedVersion {
			t.Fatalf("SetTarget ExpectedVersion differs: config_option=%d prompt=%d", viaConfigOption.setTargetReq.ExpectedVersion, viaPrompt.setTargetReq.ExpectedVersion)
		}
		if viaPrompt.startTurnCalled {
			t.Fatal("StartTurn was called via the /factory prompt fallback, want no prompt turn")
		}
	})

	failureTable := []struct {
		name  string
		value string
		err   error
	}{
		{name: "disallowed", value: "factory:@you/not-allowed", err: &chatsessions.FactoryTargetCatalogError{Target: "factory:@you/not-allowed", Err: chatsessions.ErrFactoryTargetNotAllowed}},
		{name: "working-root-incompatible", value: "factory:@you/pinned", err: &chatsessions.FactoryTargetCatalogError{Target: "factory:@you/pinned", Err: chatsessions.ErrFactoryTargetWorkingRootIncompatible}},
		{name: "not-installed", value: "factory:@you/missing", err: &chatsessions.FactoryTargetCatalogError{Target: "factory:@you/missing", Err: chatsessions.ErrFactoryTargetNotInstalled}},
	}
	for _, tc := range failureTable {
		t.Run(tc.name, func(t *testing.T) {
			viaConfigOption, catalogA, configServer := buildFixtures()
			catalogA.err = tc.err
			env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionSetConfigOption,
				setConfigOptionParams("session-1", tc.value))
			_, configErr := configServer.handleSessionSetConfigOption(context.Background(), env)
			if configErr == nil {
				t.Fatal("handleSessionSetConfigOption() error = nil, want a rejection")
			}

			viaPrompt, catalogB, promptServer := buildFixtures()
			catalogB.err = tc.err
			promptEnv := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
				promptTextParams("session-1", "/factory "+tc.value))
			_, promptErr := promptServer.handleSessionPrompt(context.Background(), promptEnv)
			if promptErr == nil {
				t.Fatal("handleSessionPrompt() error = nil, want a rejection")
			}

			if configErr.Code != promptErr.Code {
				t.Fatalf("error classification differs: config_option=%d prompt=%d", configErr.Code, promptErr.Code)
			}
			if viaConfigOption.setTargetCalled || viaPrompt.setTargetCalled {
				t.Fatal("SetTarget was called, want no mutation on either path for a rejected target")
			}
			if viaPrompt.startTurnCalled {
				t.Fatal("StartTurn was called via the /factory prompt fallback, want no prompt turn")
			}
		})
	}
}
