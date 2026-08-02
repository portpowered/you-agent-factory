package chatsessions

import (
	"errors"
	"strings"
	"testing"
)

func TestRequestIdentityValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		identity   RequestIdentity
		wantErr    bool
		wantReason RequestIdentityInvalidReason
	}{
		{
			name:     "connection qualified string id is valid",
			identity: RequestIdentity{ConnectionID: "conn-1", JSONRPCIDKind: JSONRPCIDKindString, JSONRPCIDString: "42"},
			wantErr:  false,
		},
		{
			name:     "connection qualified numeric id is valid",
			identity: RequestIdentity{ConnectionID: "conn-1", JSONRPCIDKind: JSONRPCIDKindNumber, JSONRPCIDNumber: 42},
			wantErr:  false,
		},
		{
			name:     "connection qualified numeric id zero is valid",
			identity: RequestIdentity{ConnectionID: "conn-1", JSONRPCIDKind: JSONRPCIDKindNumber, JSONRPCIDNumber: 0},
			wantErr:  false,
		},
		{
			name:     "transport minted opaque id is valid",
			identity: RequestIdentity{OpaqueID: "opaque-1"},
			wantErr:  false,
		},
		{
			name:       "empty identity is invalid",
			identity:   RequestIdentity{},
			wantErr:    true,
			wantReason: RequestIdentityInvalidEmpty,
		},
		{
			name:       "whitespace only fields are treated as empty",
			identity:   RequestIdentity{ConnectionID: "  ", JSONRPCIDKind: JSONRPCIDKindString, JSONRPCIDString: "\t", OpaqueID: " "},
			wantErr:    true,
			wantReason: RequestIdentityInvalidEmpty,
		},
		{
			name:       "bare string json-rpc id without connection id is invalid",
			identity:   RequestIdentity{JSONRPCIDKind: JSONRPCIDKindString, JSONRPCIDString: "42"},
			wantErr:    true,
			wantReason: RequestIdentityInvalidBareJSONRPCID,
		},
		{
			name:       "bare numeric json-rpc id without connection id is invalid",
			identity:   RequestIdentity{JSONRPCIDKind: JSONRPCIDKindNumber, JSONRPCIDNumber: 42},
			wantErr:    true,
			wantReason: RequestIdentityInvalidBareJSONRPCID,
		},
		{
			name:       "connection id without json-rpc id is invalid",
			identity:   RequestIdentity{ConnectionID: "conn-1"},
			wantErr:    true,
			wantReason: RequestIdentityInvalidIncompleteConnectionPair,
		},
		{
			name:       "connection id with whitespace-only string json-rpc id is invalid",
			identity:   RequestIdentity{ConnectionID: "conn-1", JSONRPCIDKind: JSONRPCIDKindString, JSONRPCIDString: "  "},
			wantErr:    true,
			wantReason: RequestIdentityInvalidIncompleteConnectionPair,
		},
		{
			name:       "opaque id mixed with connection id is invalid",
			identity:   RequestIdentity{ConnectionID: "conn-1", OpaqueID: "opaque-1"},
			wantErr:    true,
			wantReason: RequestIdentityInvalidMixedIdentityModes,
		},
		{
			name:       "opaque id mixed with json-rpc id is invalid",
			identity:   RequestIdentity{JSONRPCIDKind: JSONRPCIDKindString, JSONRPCIDString: "42", OpaqueID: "opaque-1"},
			wantErr:    true,
			wantReason: RequestIdentityInvalidMixedIdentityModes,
		},
		{
			name: "opaque id mixed with fully connection qualified identity is invalid",
			identity: RequestIdentity{
				ConnectionID:    "conn-1",
				JSONRPCIDKind:   JSONRPCIDKindString,
				JSONRPCIDString: "42",
				OpaqueID:        "opaque-1",
			},
			wantErr:    true,
			wantReason: RequestIdentityInvalidMixedIdentityModes,
		},
		{
			name:       "unrecognized json-rpc id kind is invalid",
			identity:   RequestIdentity{ConnectionID: "conn-1", JSONRPCIDKind: JSONRPCIDKind("BOOL"), JSONRPCIDString: "42"},
			wantErr:    true,
			wantReason: RequestIdentityInvalidJSONRPCIDKind,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			before := test.identity
			err := test.identity.Validate()

			if test.identity != before {
				t.Fatalf("Validate mutated the supplied identity: got %+v, want %+v", test.identity, before)
			}

			if !test.wantErr {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("Validate() = nil, want error with reason %s", test.wantReason)
			}

			var invalid *InvalidRequestIdentityError
			if !errors.As(err, &invalid) {
				t.Fatalf("Validate() error = %v (%T), want *InvalidRequestIdentityError", err, err)
			}
			if invalid.Reason != test.wantReason {
				t.Fatalf("Validate() reason = %s, want %s", invalid.Reason, test.wantReason)
			}
		})
	}
}

func TestRequestIdentityDistinguishesNumericAndStringJSONRPCIDs(t *testing.T) {
	t.Parallel()

	numeric := RequestIdentity{ConnectionID: "conn-1", JSONRPCIDKind: JSONRPCIDKindNumber, JSONRPCIDNumber: 1}
	str := RequestIdentity{ConnectionID: "conn-1", JSONRPCIDKind: JSONRPCIDKindString, JSONRPCIDString: "1"}

	if err := numeric.Validate(); err != nil {
		t.Fatalf("numeric identity Validate() = %v, want nil", err)
	}
	if err := str.Validate(); err != nil {
		t.Fatalf("string identity Validate() = %v, want nil", err)
	}

	if numeric == str {
		t.Fatalf("numeric json-rpc id 1 and string json-rpc id %q on the same connection must not be conflated, got equal identities %+v", "1", numeric)
	}

	numericAgain := RequestIdentity{ConnectionID: "conn-1", JSONRPCIDKind: JSONRPCIDKindNumber, JSONRPCIDNumber: 1}
	if numeric != numericAgain {
		t.Fatalf("identical connection and numeric json-rpc id must produce equal identities, got %+v != %+v", numeric, numericAgain)
	}

	strAgain := RequestIdentity{ConnectionID: "conn-1", JSONRPCIDKind: JSONRPCIDKindString, JSONRPCIDString: "1"}
	if str != strAgain {
		t.Fatalf("identical connection and string json-rpc id must produce equal identities, got %+v != %+v", str, strAgain)
	}
}

func TestRequestIdentityValidateErrorDoesNotLeakSuppliedValues(t *testing.T) {
	t.Parallel()

	secretLookingConnectionID := "conn-with-secret-token-abc123"
	err := RequestIdentity{ConnectionID: secretLookingConnectionID}.Validate()
	if err == nil {
		t.Fatalf("Validate() = nil, want error")
	}
	if got := err.Error(); got == "" {
		t.Fatalf("Error() = %q, want non-empty message", got)
	} else if strings.Contains(got, secretLookingConnectionID) {
		t.Fatalf("Error() = %q leaked the supplied connection id", got)
	}
}
