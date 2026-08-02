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
			name:     "connection qualified is valid",
			identity: RequestIdentity{ConnectionID: "conn-1", JSONRPCID: "42"},
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
			identity:   RequestIdentity{ConnectionID: "  ", JSONRPCID: "\t", OpaqueID: " "},
			wantErr:    true,
			wantReason: RequestIdentityInvalidEmpty,
		},
		{
			name:       "bare json-rpc id without connection id is invalid",
			identity:   RequestIdentity{JSONRPCID: "42"},
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
			name:       "opaque id mixed with connection id is invalid",
			identity:   RequestIdentity{ConnectionID: "conn-1", OpaqueID: "opaque-1"},
			wantErr:    true,
			wantReason: RequestIdentityInvalidMixedIdentityModes,
		},
		{
			name:       "opaque id mixed with json-rpc id is invalid",
			identity:   RequestIdentity{JSONRPCID: "42", OpaqueID: "opaque-1"},
			wantErr:    true,
			wantReason: RequestIdentityInvalidMixedIdentityModes,
		},
		{
			name: "opaque id mixed with fully connection qualified identity is invalid",
			identity: RequestIdentity{
				ConnectionID: "conn-1",
				JSONRPCID:    "42",
				OpaqueID:     "opaque-1",
			},
			wantErr:    true,
			wantReason: RequestIdentityInvalidMixedIdentityModes,
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
