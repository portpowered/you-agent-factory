package protocol

import (
	"encoding/json"
	"errors"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/session"
)

func TestSupportedMethods(t *testing.T) {
	want := []string{
		"initialize",
		"session/new",
		"session/load",
		"session/resume",
		"session/cancel",
		"session/set_config_option",
		"session/prompt",
		"session/update",
		"session/request_permission",
	}
	for _, method := range want {
		if !SupportedMethods[method] {
			t.Errorf("SupportedMethods[%q] = false, want true", method)
		}
	}
	if SupportedMethods["session/experimental_fork"] {
		t.Error("SupportedMethods[\"session/experimental_fork\"] = true, want false (deferred behavior)")
	}
}

func TestGuard_UnsupportedMethodNeverCallsValidateOrEffect(t *testing.T) {
	validateCalled, effectCalled := false, false

	err := Guard("session/experimental_fork",
		func() error { validateCalled = true; return nil },
		func() error { effectCalled = true; return nil },
	)

	if err == nil {
		t.Fatal("Guard() error = nil, want method-not-found")
	}
	reqErr := requireRequestError(t, err)
	if reqErr.Code != -32601 {
		t.Errorf("Guard() code = %d, want -32601 (method not found)", reqErr.Code)
	}
	if validateCalled {
		t.Error("Guard() called validate for an unsupported method")
	}
	if effectCalled {
		t.Error("Guard() called effect for an unsupported method")
	}
}

func TestGuard_InvalidSupportedRequestNeverCallsEffect(t *testing.T) {
	effectCalled := false
	cause := errors.New("acp: cwd is required")

	err := Guard("session/new",
		func() error { return cause },
		func() error { effectCalled = true; return nil },
	)

	if err == nil {
		t.Fatal("Guard() error = nil, want a bounded validation error")
	}
	reqErr := requireRequestError(t, err)
	if reqErr.Code != -32602 {
		t.Errorf("Guard() code = %d, want -32602 (invalid params)", reqErr.Code)
	}
	if effectCalled {
		t.Error("Guard() called effect after validate failed")
	}
}

func TestGuard_SupportedValidRequestCallsEffectExactlyOnce(t *testing.T) {
	calls := 0

	err := Guard("session/new",
		func() error { return nil },
		func() error { calls++; return nil },
	)

	if err != nil {
		t.Fatalf("Guard() error = %v, want nil", err)
	}
	if calls != 1 {
		t.Errorf("Guard() called effect %d times, want 1", calls)
	}
}

func TestGuard_RepeatedInvalidInputIsDeterministic(t *testing.T) {
	cause := errors.New("acp: cwd is required")
	validate := func() error { return cause }
	effect := func() error { t.Fatal("effect must not be called"); return nil }

	first := requireRequestError(t, Guard("session/new", validate, effect))
	second := requireRequestError(t, Guard("session/new", validate, effect))

	if first.Code != second.Code || first.Message != second.Message {
		t.Fatalf("Guard() classification drifted across repeated evaluation: %+v vs %+v", first, second)
	}
}

func TestGuardEnvelope_MalformedEnvelopeNeverCallsValidateOrEffect(t *testing.T) {
	validateCalled, effectCalled := false, false

	env, err := GuardEnvelope("conn-1", json.RawMessage(`{"jsonrpc":"2.0","method":"session/new"}`),
		func(envelope.Envelope) error { validateCalled = true; return nil },
		func() error { effectCalled = true; return nil },
	)

	if err == nil {
		t.Fatal("GuardEnvelope() error = nil, want a bounded rejection for a malformed envelope (missing id on a request method)")
	}
	if env.IsNotification {
		t.Error("GuardEnvelope() returned IsNotification = true for a decode failure, want false")
	}
	if validateCalled {
		t.Error("GuardEnvelope() called validate for a malformed envelope")
	}
	if effectCalled {
		t.Error("GuardEnvelope() called effect for a malformed envelope")
	}
}

func TestGuardEnvelope_UnsupportedMethodNeverCallsValidateOrEffect(t *testing.T) {
	validateCalled, effectCalled := false, false

	_, err := GuardEnvelope("conn-1", json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"session/experimental_fork"}`),
		func(envelope.Envelope) error { validateCalled = true; return nil },
		func() error { effectCalled = true; return nil },
	)

	if err == nil {
		t.Fatal("GuardEnvelope() error = nil, want method-not-found")
	}
	reqErr := requireRequestError(t, err)
	if reqErr.Code != -32601 {
		t.Errorf("GuardEnvelope() code = %d, want -32601 (method not found)", reqErr.Code)
	}
	if validateCalled {
		t.Error("GuardEnvelope() called validate for an unsupported method")
	}
	if effectCalled {
		t.Error("GuardEnvelope() called effect for an unsupported method")
	}
}

// TestGuardEnvelope_ValidEnvelopeCallsEffectExactlyOnceWithBoundIdentity wires
// the real session.ValidateNewSession validator into the identity-bound
// envelope path, proving the combined method + request + method-specific
// identity round trip: params that satisfy the real validator (including
// the required mcpServers field) flow from the raw JSON-RPC bytes through
// Decode into the exact value the validator returns, under the identity
// Decode bound to this connection and id.
func TestGuardEnvelope_ValidEnvelopeCallsEffectExactlyOnceWithBoundIdentity(t *testing.T) {
	calls := 0
	var gotIdentity identity.RequestIdentity
	var gotParams session.NewSessionParams

	env, err := GuardEnvelope("conn-1", json.RawMessage(`{"jsonrpc":"2.0","id":42,"method":"session/new","params":{"cwd":"/a","mcpServers":[]}}`),
		func(env envelope.Envelope) error {
			gotIdentity = env.Identity
			v, verr := session.ValidateNewSession(env.Params)
			gotParams = v
			return verr
		},
		func() error { calls++; return nil },
	)

	if err != nil {
		t.Fatalf("GuardEnvelope() error = %v, want nil", err)
	}
	if env.IsNotification {
		t.Error("GuardEnvelope() IsNotification = true, want false for session/new")
	}
	if calls != 1 {
		t.Errorf("GuardEnvelope() called effect %d times, want 1", calls)
	}
	wantIdentity, idErr := identity.NewCorrelated("conn-1", identity.NewNumberJSONRPCID(42))
	if idErr != nil {
		t.Fatalf("identity.NewCorrelated() unexpected error: %v", idErr)
	}
	if !gotIdentity.Equal(wantIdentity) {
		t.Errorf("validate saw identity %+v, want %+v", gotIdentity, wantIdentity)
	}
	wantParams := session.NewSessionParams{Cwd: "/a"}
	if gotParams.Cwd != wantParams.Cwd || len(gotParams.AdditionalDirectories) != 0 {
		t.Errorf("validate produced %+v via the real validator, want %+v", gotParams, wantParams)
	}
}

// TestGuardEnvelope_SameJSONRPCIDDifferentConnectionsNeverCollide wires the
// real session.ValidatePrompt validator (an id-bearing request method) into
// the envelope path to prove distinct same-id connections stay distinct end
// to end, not merely at the bare identity.RequestIdentity layer.
func TestGuardEnvelope_SameJSONRPCIDDifferentConnectionsNeverCollide(t *testing.T) {
	raw := json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"session/prompt","params":{"sessionId":"s","prompt":[{"type":"text","text":"hi"}]}}`)
	var identities []identity.RequestIdentity

	for _, conn := range []identity.ConnectionID{"conn-a", "conn-b"} {
		_, err := GuardEnvelope(conn, raw,
			func(env envelope.Envelope) error {
				identities = append(identities, env.Identity)
				var req acpsdk.PromptRequest
				if uerr := json.Unmarshal(env.Params, &req); uerr != nil {
					return uerr
				}
				_, verr := session.ValidatePrompt(req)
				return verr
			},
			func() error { return nil },
		)
		if err != nil {
			t.Fatalf("GuardEnvelope(%s) error = %v, want nil", conn, err)
		}
	}

	if identities[0].Equal(identities[1]) {
		t.Fatal("two connections sending the same JSON-RPC id produced equal identities through GuardEnvelope")
	}
}

// TestGuardEnvelope_ValidNotificationSignalsNoResponseOwed wires the real
// session.ValidateCancel validator into an id-less session/cancel envelope
// and proves the resulting Envelope reports IsNotification true, so a
// caller knows never to send a response for it.
func TestGuardEnvelope_ValidNotificationSignalsNoResponseOwed(t *testing.T) {
	env, err := GuardEnvelope("conn-1", json.RawMessage(`{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":"s"}}`),
		func(env envelope.Envelope) error {
			var notif acpsdk.CancelNotification
			if uerr := json.Unmarshal(env.Params, &notif); uerr != nil {
				return uerr
			}
			_, verr := session.ValidateCancel(notif)
			return verr
		},
		func() error { return nil },
	)
	if err != nil {
		t.Fatalf("GuardEnvelope() error = %v, want nil for a valid cancel notification", err)
	}
	if !env.IsNotification {
		t.Fatal("GuardEnvelope() IsNotification = false, want true for session/cancel")
	}
	if !env.Identity.IsMinted() {
		t.Fatal("GuardEnvelope() identity is not minted, want a minted identity for a notification")
	}
}

// TestGuardEnvelope_InvalidNotificationStillSignalsNoResponseOwed proves a
// notification that fails method-specific validation still reports
// IsNotification true: the caller must discard the returned error rather
// than ever serializing it back as a JSON-RPC response, even though the
// error itself is still produced (e.g. for internal logging).
func TestGuardEnvelope_InvalidNotificationStillSignalsNoResponseOwed(t *testing.T) {
	env, err := GuardEnvelope("conn-1", json.RawMessage(`{"jsonrpc":"2.0","method":"session/cancel","params":{}}`),
		func(env envelope.Envelope) error {
			var notif acpsdk.CancelNotification
			if uerr := json.Unmarshal(env.Params, &notif); uerr != nil {
				return uerr
			}
			_, verr := session.ValidateCancel(notif)
			return verr
		},
		func() error { t.Fatal("effect must not run when validate fails"); return nil },
	)
	if err == nil {
		t.Fatal("GuardEnvelope() error = nil, want a bounded validation error for a missing sessionId")
	}
	if !env.IsNotification {
		t.Fatal("GuardEnvelope() IsNotification = false, want true even when validation fails")
	}
}

// TestGuardEnvelope_NotificationWithIDIsMalformed proves an id-bearing
// session/cancel is rejected as a malformed envelope before validate or
// effect ever runs, and that a decode failure reports IsNotification false
// (a message this ambiguous still owes an ordinary error response).
func TestGuardEnvelope_NotificationWithIDIsMalformed(t *testing.T) {
	env, err := GuardEnvelope("conn-1", json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"session/cancel","params":{"sessionId":"s"}}`),
		func(envelope.Envelope) error {
			t.Fatal("validate must not run for an id-bearing notification")
			return nil
		},
		func() error { t.Fatal("effect must not run for an id-bearing notification"); return nil },
	)
	if err == nil {
		t.Fatal("GuardEnvelope() error = nil, want a malformed-envelope rejection for an id-bearing notification")
	}
	if env.IsNotification {
		t.Error("GuardEnvelope() IsNotification = true for a decode failure, want false")
	}
}

func requireRequestError(t *testing.T, err error) *acpsdk.RequestError {
	t.Helper()
	var reqErr *acpsdk.RequestError
	if !errors.As(err, &reqErr) {
		t.Fatalf("error = %v (%T), want *acpsdk.RequestError", err, err)
	}
	return reqErr
}
