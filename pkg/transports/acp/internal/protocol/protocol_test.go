package protocol

import (
	"encoding/json"
	"errors"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
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

	err := GuardEnvelope("conn-1", json.RawMessage(`{"jsonrpc":"2.0","method":"session/new"}`),
		func(envelope.Envelope) error { validateCalled = true; return nil },
		func() error { effectCalled = true; return nil },
	)

	if err == nil {
		t.Fatal("GuardEnvelope() error = nil, want a bounded rejection for a malformed envelope (missing id)")
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

	err := GuardEnvelope("conn-1", json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"session/experimental_fork"}`),
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

func TestGuardEnvelope_ValidEnvelopeCallsEffectExactlyOnceWithBoundIdentity(t *testing.T) {
	calls := 0
	var gotIdentity identity.RequestIdentity
	var gotParams string

	err := GuardEnvelope("conn-1", json.RawMessage(`{"jsonrpc":"2.0","id":42,"method":"session/new","params":{"cwd":"/a"}}`),
		func(env envelope.Envelope) error {
			gotIdentity = env.Identity
			gotParams = string(env.Params)
			return nil
		},
		func() error { calls++; return nil },
	)

	if err != nil {
		t.Fatalf("GuardEnvelope() error = %v, want nil", err)
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
	if gotParams != `{"cwd":"/a"}` {
		t.Errorf("validate saw params %q, want the raw params object", gotParams)
	}
}

func TestGuardEnvelope_SameJSONRPCIDDifferentConnectionsNeverCollide(t *testing.T) {
	raw := json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"session/cancel","params":{"sessionId":"s"}}`)
	var identities []identity.RequestIdentity

	for _, conn := range []identity.ConnectionID{"conn-a", "conn-b"} {
		err := GuardEnvelope(conn, raw,
			func(env envelope.Envelope) error { identities = append(identities, env.Identity); return nil },
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

func requireRequestError(t *testing.T, err error) *acpsdk.RequestError {
	t.Helper()
	var reqErr *acpsdk.RequestError
	if !errors.As(err, &reqErr) {
		t.Fatalf("error = %v (%T), want *acpsdk.RequestError", err, err)
	}
	return reqErr
}
