package envelope

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
)

func TestDecode_BindsConnectionMethodAndParams(t *testing.T) {
	env, err := Decode("conn-1", json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"session/prompt","params":{"sessionId":"sess-1"}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.Method != "session/prompt" {
		t.Fatalf("Method = %q, want session/prompt", env.Method)
	}
	if string(env.Params) != `{"sessionId":"sess-1"}` {
		t.Fatalf("Params = %s, want the raw params object", env.Params)
	}
	connID, ok := env.Identity.ConnectionID()
	if !ok || connID != "conn-1" {
		t.Fatalf("Identity.ConnectionID() = (%q, %v), want (\"conn-1\", true)", connID, ok)
	}
}

func TestDecode_SameJSONRPCIDDifferentConnectionsProduceDistinctIdentities(t *testing.T) {
	raw := json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"session/cancel","params":{}}`)

	envA, err := Decode("conn-a", raw)
	if err != nil {
		t.Fatalf("Decode(conn-a) unexpected error: %v", err)
	}
	envB, err := Decode("conn-b", raw)
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
		{"wrong jsonrpc version", `{"jsonrpc":"1.0","id":1,"method":"session/cancel"}`},
		{"missing jsonrpc version", `{"id":1,"method":"session/cancel"}`},
		{"missing method", `{"jsonrpc":"2.0","id":1}`},
		{"blank method", `{"jsonrpc":"2.0","id":1,"method":"   "}`},
		{"missing id", `{"jsonrpc":"2.0","method":"session/cancel"}`},
		{"null id", `{"jsonrpc":"2.0","id":null,"method":"session/cancel"}`},
		{"object id", `{"jsonrpc":"2.0","id":{},"method":"session/cancel"}`},
		{"array id", `{"jsonrpc":"2.0","id":[],"method":"session/cancel"}`},
		{"boolean id", `{"jsonrpc":"2.0","id":true,"method":"session/cancel"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decode("conn-1", json.RawMessage(tt.raw))
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
	_, err := Decode("", json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"session/cancel"}`))
	if !errors.Is(err, ErrMalformedEnvelope) {
		t.Fatalf("Decode() with blank connection id error = %v, want it to wrap ErrMalformedEnvelope", err)
	}
}

func TestEnvelope_JSONRoundTrip(t *testing.T) {
	env, err := Decode("conn-1", json.RawMessage(`{"jsonrpc":"2.0","id":"req-7","method":"session/new","params":{"cwd":"/a"}}`))
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
	if decoded.Method != env.Method || string(decoded.Params) != string(env.Params) {
		t.Fatalf("round-tripped envelope = %+v, want %+v", decoded, env)
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
