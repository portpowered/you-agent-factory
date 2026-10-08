package details

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// These read-only GETs own the API's safe error contract. One idle host shares
// immutable effects; each parallel leaf owns its provider ID and fault route.
// No Factory Session is needed for standalone Provider Session inspection.
func assertStorageFailureResponse(t *testing.T, body, home string) {
	t.Helper()
	var failure factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(body), &failure); err != nil {
		t.Fatalf("decode storage error: %v", err)
	}
	if failure.Code != factoryapi.ErrorResponseCodeINTERNALERROR || failure.Message != "failed to load provider session details" {
		t.Fatalf("storage failure = %#v, want safe INTERNAL_ERROR", failure)
	}
	for _, forbidden := range []string{home, "/private/operator/session-store", "private-storage-fault", `"transcript"`, `"providerSession"`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("storage failure leaked private data or fabricated detail: %s", body)
		}
	}
}

func privateStorageFault() error {
	return errors.New("private-storage-fault: /private/operator/session-store")
}
