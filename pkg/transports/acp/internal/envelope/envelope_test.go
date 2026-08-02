package envelope

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
)

func TestDecode_BindsConnectionMethodAndParams(t *testing.T) {
	env, err := Decode("conn-1", 1, json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"session/prompt","params":{"sessionId":"sess-1"}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.Method != "session/prompt" {
		t.Fatalf("Method = %q, want session/prompt", env.Method)
	}
	if string(env.Params) != `{"sessionId":"sess-1"}` {
		t.Fatalf("Params = %s, want the raw params object", env.Params)
	}
	if env.IsNotification {
		t.Fatal("IsNotification = true, want false for a request method")
	}
	connID, ok := env.Identity.ConnectionID()
	if !ok || connID != "conn-1" {
		t.Fatalf("Identity.ConnectionID() = (%q, %v), want (\"conn-1\", true)", connID, ok)
	}
}

func TestDecode_SameJSONRPCIDDifferentConnectionsProduceDistinctIdentities(t *testing.T) {
	raw := json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"session/prompt","params":{}}`)

	envA, err := Decode("conn-a", 1, raw)
	if err != nil {
		t.Fatalf("Decode(conn-a) unexpected error: %v", err)
	}
	envB, err := Decode("conn-b", 1, raw)
	if err != nil {
		t.Fatalf("Decode(conn-b) unexpected error: %v", err)
	}
	if envA.Identity.Equal(envB.Identity) {
		t.Fatal("two connections sending the same JSON-RPC id produced equal identities")
	}
}

func TestDecode_RejectsMalformedInput(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"invalid JSON", `{not json`},
		{"wrong jsonrpc version", `{"jsonrpc":"1.0","id":1,"method":"session/prompt"}`},
		{"missing jsonrpc version", `{"id":1,"method":"session/prompt"}`},
		{"missing method", `{"jsonrpc":"2.0","id":1}`},
		{"blank method", `{"jsonrpc":"2.0","id":1,"method":"   "}`},
		{"missing id on a request method", `{"jsonrpc":"2.0","method":"session/prompt"}`},
		{"null id on a request method", `{"jsonrpc":"2.0","id":null,"method":"session/prompt"}`},
		{"object id", `{"jsonrpc":"2.0","id":{},"method":"session/prompt"}`},
		{"array id", `{"jsonrpc":"2.0","id":[],"method":"session/prompt"}`},
		{"boolean id", `{"jsonrpc":"2.0","id":true,"method":"session/prompt"}`},
		{"id-bearing session/cancel notification", `{"jsonrpc":"2.0","id":1,"method":"session/cancel","params":{"sessionId":"s"}}`},
		{"id-bearing session/update notification", `{"jsonrpc":"2.0","id":1,"method":"session/update","params":{"sessionId":"s","update":{}}}`},
		{"null id-bearing session/cancel notification", `{"jsonrpc":"2.0","id":null,"method":"session/cancel","params":{"sessionId":"s"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decode("conn-1", 1, json.RawMessage(tt.raw))
			if err == nil {
				t.Fatalf("Decode(%s) error = nil, want a malformed-envelope rejection", tt.raw)
			}
			if !errors.Is(err, ErrMalformedEnvelope) {
				t.Fatalf("Decode(%s) error = %v, want it to wrap ErrMalformedEnvelope", tt.raw, err)
			}
		})
	}
}

func TestDecode_RejectsBlankConnectionID(t *testing.T) {
	_, err := Decode("", 1, json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"session/prompt"}`))
	if !errors.Is(err, ErrMalformedEnvelope) {
		t.Fatalf("Decode() with blank connection id error = %v, want it to wrap ErrMalformedEnvelope", err)
	}
}

func TestDecode_AcceptsValidNotificationWithoutID(t *testing.T) {
	for _, method := range []string{"session/cancel", "session/update"} {
		t.Run(method, func(t *testing.T) {
			raw := json.RawMessage(`{"jsonrpc":"2.0","method":"` + method + `","params":{"sessionId":"s"}}`)
			env, err := Decode("conn-1", 1, raw)
			if err != nil {
				t.Fatalf("Decode() unexpected error: %v", err)
			}
			if !env.IsNotification {
				t.Fatal("IsNotification = false, want true for a notification method")
			}
			if !env.Identity.IsMinted() {
				t.Fatal("expected a notification's identity to be minted, not connection-correlated")
			}
			if _, ok := env.Identity.ConnectionID(); ok {
				t.Fatal("expected a notification's identity to have no correlated connection id")
			}
		})
	}
}

func TestDecode_NotificationIdentityIsDistinctPerConnection(t *testing.T) {
	raw := json.RawMessage(`{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":"s"}}`)

	envA, err := Decode("conn-a", 1, raw)
	if err != nil {
		t.Fatalf("Decode(conn-a) unexpected error: %v", err)
	}
	envB, err := Decode("conn-b", 1, raw)
	if err != nil {
		t.Fatalf("Decode(conn-b) unexpected error: %v", err)
	}
	if envA.Identity.Equal(envB.Identity) {
		t.Fatal("two connections sending the same notification produced equal minted identities")
	}
}

// TestDecode_NotificationIdentityIsDistinctPerSequenceOnSameConnection proves
// two same-method notifications received on the *same* connection do not
// collide: a notification carries no JSON-RPC id, so its identity is minted
// from the connection, method, and the caller-supplied notificationSeq. This
// is the collision this transport must avoid in practice -- the
// connection/framing layer sends a stream of session/update notifications on
// one connection -- unlike the cross-connection case above, which two
// different NewMinted inputs already made trivially distinct.
func TestDecode_NotificationIdentityIsDistinctPerSequenceOnSameConnection(t *testing.T) {
	raw := json.RawMessage(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s","update":{}}}`)

	first, err := Decode("conn-1", 1, raw)
	if err != nil {
		t.Fatalf("Decode(seq=1) unexpected error: %v", err)
	}
	second, err := Decode("conn-1", 2, raw)
	if err != nil {
		t.Fatalf("Decode(seq=2) unexpected error: %v", err)
	}
	if first.Identity.Equal(second.Identity) {
		t.Fatal("two notifications on the same connection with different sequence numbers produced equal identities")
	}

	firstEncoded, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("Marshal(first) error = %v", err)
	}
	var firstDecoded Envelope
	if err := json.Unmarshal(firstEncoded, &firstDecoded); err != nil {
		t.Fatalf("Unmarshal(first) error = %v", err)
	}
	if !firstDecoded.Identity.Equal(first.Identity) {
		t.Fatalf("round-tripped identity = %+v, want %+v", firstDecoded.Identity, first.Identity)
	}
	if firstDecoded.Identity.Equal(second.Identity) {
		t.Fatal("round-tripped notification identity collapsed onto a different sequence's identity")
	}
}

func TestEnvelope_JSONRoundTrip(t *testing.T) {
	env, err := Decode("conn-1", 1, json.RawMessage(`{"jsonrpc":"2.0","id":"req-7","method":"session/new","params":{"cwd":"/a"}}`))
	if err != nil {
		t.Fatalf("Decode() unexpected error: %v", err)
	}

	encoded, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded Envelope
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !decoded.Identity.Equal(env.Identity) {
		t.Fatalf("round-tripped identity = %+v, want %+v", decoded.Identity, env.Identity)
	}
	if decoded.Method != env.Method || string(decoded.Params) != string(env.Params) || decoded.IsNotification != env.IsNotification {
		t.Fatalf("round-tripped envelope = %+v, want %+v", decoded, env)
	}
}

func TestEnvelope_NotificationJSONRoundTrip(t *testing.T) {
	env, err := Decode("conn-1", 1, json.RawMessage(`{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":"s"}}`))
	if err != nil {
		t.Fatalf("Decode() unexpected error: %v", err)
	}

	encoded, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded Envelope
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !decoded.IsNotification {
		t.Fatal("round-tripped envelope lost IsNotification = true")
	}
	if !decoded.Identity.Equal(env.Identity) {
		t.Fatalf("round-tripped identity = %+v, want %+v", decoded.Identity, env.Identity)
	}
}

func TestEnvelope_MintedIdentityIsNotCorrelated(t *testing.T) {
	minted, err := identity.NewMinted("permission-req-1")
	if err != nil {
		t.Fatalf("NewMinted() unexpected error: %v", err)
	}
	env := Envelope{Identity: minted, Method: "session/request_permission"}
	if env.Method != "session/request_permission" {
		t.Fatalf("Method = %q, want session/request_permission", env.Method)
	}
	if !env.Identity.IsMinted() {
		t.Fatal("expected a minted identity to report IsMinted() = true")
	}
	if _, ok := env.Identity.ConnectionID(); ok {
		t.Fatal("expected a minted identity to have no connection id")
	}
}
