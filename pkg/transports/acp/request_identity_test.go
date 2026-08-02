package acp_test

import (
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/transports/acp"
)

func stringWireID(s string) acpsdk.RequestId {
	v := acpsdk.RequestIdStr(s)
	return acpsdk.RequestId{Str: &v}
}

func numberWireID(n int) acpsdk.RequestId {
	v := acpsdk.RequestIdNumber(n)
	return acpsdk.RequestId{Number: &v}
}

func nullWireID() acpsdk.RequestId {
	v := acpsdk.RequestIdNull{}
	return acpsdk.RequestId{Null: &v}
}

func TestNewRequestIdentityAcceptsStringAndNumericIDs(t *testing.T) {
	stringIdentity, err := acp.NewRequestIdentity("conn-a", stringWireID("1"))
	if err != nil {
		t.Fatalf("NewRequestIdentity(string id) unexpected error: %v", err)
	}
	if stringIdentity.Kind != acp.WireIDKindString || stringIdentity.StringID != "1" {
		t.Fatalf("NewRequestIdentity(string id) = %+v, want Kind=string StringID=1", stringIdentity)
	}

	numberIdentity, err := acp.NewRequestIdentity("conn-a", numberWireID(1))
	if err != nil {
		t.Fatalf("NewRequestIdentity(number id) unexpected error: %v", err)
	}
	if numberIdentity.Kind != acp.WireIDKindNumber || numberIdentity.NumberID != 1 {
		t.Fatalf("NewRequestIdentity(number id) = %+v, want Kind=number NumberID=1", numberIdentity)
	}

	if stringIdentity == numberIdentity {
		t.Fatalf("numeric id 1 and string id %q must not be conflated, got equal identities %+v", "1", stringIdentity)
	}
}

func TestNewRequestIdentityDistinguishesConnections(t *testing.T) {
	first, err := acp.NewRequestIdentity("conn-a", stringWireID("shared"))
	if err != nil {
		t.Fatalf("NewRequestIdentity(conn-a) unexpected error: %v", err)
	}
	second, err := acp.NewRequestIdentity("conn-b", stringWireID("shared"))
	if err != nil {
		t.Fatalf("NewRequestIdentity(conn-b) unexpected error: %v", err)
	}
	if first == second {
		t.Fatalf("equal wire ids on different connections must produce distinct identities, got %+v == %+v", first, second)
	}

	third, err := acp.NewRequestIdentity("conn-a", stringWireID("shared"))
	if err != nil {
		t.Fatalf("NewRequestIdentity(conn-a) second call unexpected error: %v", err)
	}
	if first != third {
		t.Fatalf("identical connection and wire id must produce equal identities, got %+v != %+v", first, third)
	}
}

func TestNewRequestIdentityRejectsBlankConnectionID(t *testing.T) {
	cases := []string{"", "   "}
	for _, connectionID := range cases {
		_, err := acp.NewRequestIdentity(connectionID, stringWireID("1"))
		if err == nil {
			t.Fatalf("NewRequestIdentity(%q, ...) expected error, got nil", connectionID)
		}
		identityErr, ok := err.(*acp.RequestIdentityError)
		if !ok {
			t.Fatalf("NewRequestIdentity(%q, ...) error type = %T, want *acp.RequestIdentityError", connectionID, err)
		}
		if identityErr.Code != acp.RequestIdentityErrorBlankConnectionID {
			t.Fatalf("NewRequestIdentity(%q, ...) code = %q, want %q", connectionID, identityErr.Code, acp.RequestIdentityErrorBlankConnectionID)
		}
	}
}

func TestNewRequestIdentityRejectsInvalidWireIDShapes(t *testing.T) {
	cases := map[string]acpsdk.RequestId{
		"null id":  nullWireID(),
		"empty id": {},
	}
	for name, wireID := range cases {
		t.Run(name, func(t *testing.T) {
			identity, err := acp.NewRequestIdentity("conn-a", wireID)
			if err == nil {
				t.Fatalf("NewRequestIdentity(%s) expected error, got identity %+v", name, identity)
			}
			identityErr, ok := err.(*acp.RequestIdentityError)
			if !ok {
				t.Fatalf("NewRequestIdentity(%s) error type = %T, want *acp.RequestIdentityError", name, err)
			}
			if identityErr.Code != acp.RequestIdentityErrorInvalidWireID {
				t.Fatalf("NewRequestIdentity(%s) code = %q, want %q", name, identityErr.Code, acp.RequestIdentityErrorInvalidWireID)
			}
			if identity != (acp.RequestIdentity{}) {
				t.Fatalf("NewRequestIdentity(%s) returned a non-zero identity on error: %+v", name, identity)
			}
		})
	}
}

func TestNewMintedRequestIdentity(t *testing.T) {
	first, err := acp.NewMintedRequestIdentity("conn-a", "minted-1")
	if err != nil {
		t.Fatalf("NewMintedRequestIdentity() unexpected error: %v", err)
	}
	if first.Kind != acp.WireIDKindMinted || first.MintedID != "minted-1" {
		t.Fatalf("NewMintedRequestIdentity() = %+v, want Kind=minted MintedID=minted-1", first)
	}

	second, err := acp.NewMintedRequestIdentity("conn-a", "minted-2")
	if err != nil {
		t.Fatalf("NewMintedRequestIdentity() unexpected error: %v", err)
	}
	if first == second {
		t.Fatalf("distinct minted ids must produce distinct identities, got %+v == %+v", first, second)
	}

	wireIdentity, err := acp.NewRequestIdentity("conn-a", stringWireID("minted-1"))
	if err != nil {
		t.Fatalf("NewRequestIdentity() unexpected error: %v", err)
	}
	if first == wireIdentity {
		t.Fatalf("a minted identity must never equal a wire-id identity with the same printed value, got %+v == %+v", first, wireIdentity)
	}
}

func TestNewMintedRequestIdentityRejectsBlankInputs(t *testing.T) {
	if _, err := acp.NewMintedRequestIdentity("", "minted-1"); err == nil {
		t.Fatal("NewMintedRequestIdentity(blank connection) expected error, got nil")
	} else if identityErr, ok := err.(*acp.RequestIdentityError); !ok || identityErr.Code != acp.RequestIdentityErrorBlankConnectionID {
		t.Fatalf("NewMintedRequestIdentity(blank connection) error = %v, want RequestIdentityErrorBlankConnectionID", err)
	}

	if _, err := acp.NewMintedRequestIdentity("conn-a", ""); err == nil {
		t.Fatal("NewMintedRequestIdentity(blank minted id) expected error, got nil")
	} else if identityErr, ok := err.(*acp.RequestIdentityError); !ok || identityErr.Code != acp.RequestIdentityErrorBlankMintedID {
		t.Fatalf("NewMintedRequestIdentity(blank minted id) error = %v, want RequestIdentityErrorBlankMintedID", err)
	}
}
