package identity

import (
	"encoding/json"
	"testing"
)

func TestSameJSONRPCIDDifferentConnectionsAreDistinct(t *testing.T) {
	id := NewNumberJSONRPCID(1)

	first, err := NewCorrelated(ConnectionID("conn-a"), id)
	if err != nil {
		t.Fatalf("NewCorrelated(conn-a): %v", err)
	}
	second, err := NewCorrelated(ConnectionID("conn-b"), id)
	if err != nil {
		t.Fatalf("NewCorrelated(conn-b): %v", err)
	}

	if first.Equal(second) {
		t.Fatalf("expected requests on different connections with the same json-rpc id to be distinct")
	}

	sameAgain, err := NewCorrelated(ConnectionID("conn-a"), id)
	if err != nil {
		t.Fatalf("NewCorrelated(conn-a) again: %v", err)
	}
	if !first.Equal(sameAgain) {
		t.Fatalf("expected the same connection and json-rpc id to be equal")
	}
}

func TestStringAndNumberIDsWithEqualTextAreDistinct(t *testing.T) {
	str, err := NewCorrelated(ConnectionID("conn-a"), NewStringJSONRPCID("1"))
	if err != nil {
		t.Fatalf("NewCorrelated(string): %v", err)
	}
	num, err := NewCorrelated(ConnectionID("conn-a"), NewNumberJSONRPCID(1))
	if err != nil {
		t.Fatalf("NewCorrelated(number): %v", err)
	}

	if str.Equal(num) {
		t.Fatalf("expected a string id and a number id with equal text to remain distinct")
	}
}

func TestRequestIdentityJSONRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		build func() (RequestIdentity, error)
	}{
		{
			name: "correlated string id",
			build: func() (RequestIdentity, error) {
				return NewCorrelated(ConnectionID("conn-a"), NewStringJSONRPCID("req-1"))
			},
		},
		{
			name: "correlated number id",
			build: func() (RequestIdentity, error) {
				return NewCorrelated(ConnectionID("conn-b"), NewNumberJSONRPCID(42))
			},
		},
		{
			name: "minted",
			build: func() (RequestIdentity, error) {
				return NewMinted("permission-9f3a")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original, err := tc.build()
			if err != nil {
				t.Fatalf("build: %v", err)
			}

			encoded, err := json.Marshal(original)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}

			var decoded RequestIdentity
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			if !original.Equal(decoded) {
				t.Fatalf("round trip changed identity: original=%+v decoded=%+v", original, decoded)
			}

			reencoded, err := json.Marshal(decoded)
			if err != nil {
				t.Fatalf("re-Marshal: %v", err)
			}
			if string(reencoded) != string(encoded) {
				t.Fatalf("re-encoding drifted: first=%s second=%s", encoded, reencoded)
			}
		})
	}
}

func TestRequestIdentityValidation(t *testing.T) {
	if _, err := NewCorrelated(ConnectionID(""), NewNumberJSONRPCID(1)); err == nil {
		t.Fatalf("expected empty connection id to be rejected")
	}
	if _, err := NewCorrelated(ConnectionID("conn-a"), JSONRPCID{}); err == nil {
		t.Fatalf("expected zero-value json-rpc id to be rejected")
	}
	if _, err := NewMinted(""); err == nil {
		t.Fatalf("expected empty minted id to be rejected")
	}
	if _, err := NewMinted("   "); err == nil {
		t.Fatalf("expected whitespace-only minted id to be rejected")
	}
}

func TestJSONRPCIDRejectsUnsupportedShapes(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"null", "null"},
		{"boolean", "true"},
		{"object", `{"foo":"bar"}`},
		{"array", `[1,2]`},
		{"empty", ""},
		{"malformed number", "1.2.3"},
		{"malformed string", `"unterminated`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewJSONRPCID(json.RawMessage(tc.raw)); err == nil {
				t.Fatalf("expected %q to be rejected as a json-rpc id", tc.raw)
			}
		})
	}
}

func TestJSONRPCIDAcceptsSupportedShapes(t *testing.T) {
	cases := []string{`"req-1"`, "1", "-1", "1.5", `""`}
	for _, raw := range cases {
		if _, err := NewJSONRPCID(json.RawMessage(raw)); err != nil {
			t.Fatalf("expected %q to be accepted as a json-rpc id, got %v", raw, err)
		}
	}
}

func TestConnectionIDReflectsOnlyCorrelatedIdentities(t *testing.T) {
	correlated, err := NewCorrelated(ConnectionID("conn-a"), NewNumberJSONRPCID(1))
	if err != nil {
		t.Fatalf("NewCorrelated: %v", err)
	}
	if connID, ok := correlated.ConnectionID(); !ok || connID != ConnectionID("conn-a") {
		t.Fatalf("expected correlated identity to expose its connection id, got %q ok=%v", connID, ok)
	}
	if correlated.IsMinted() {
		t.Fatalf("expected a correlated identity to not report itself as minted")
	}

	minted, err := NewMinted("permission-1")
	if err != nil {
		t.Fatalf("NewMinted: %v", err)
	}
	if _, ok := minted.ConnectionID(); ok {
		t.Fatalf("expected a minted identity to have no connection id")
	}
	if !minted.IsMinted() {
		t.Fatalf("expected a minted identity to report itself as minted")
	}
}
