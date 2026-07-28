package details

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func assertProviderSessionDetailIdentity(
	t *testing.T,
	detail factoryapi.ProviderSessionDetailResponse,
	wantID string,
	wantProvider factoryapi.LoadableProviderSessionProvider,
	wantKind factoryapi.LoadableProviderSessionKind,
) {
	t.Helper()

	if detail.ProviderSession.Id != wantID {
		t.Fatalf("detail provider session id = %q, want %q", detail.ProviderSession.Id, wantID)
	}
	if detail.ProviderSession.Provider != wantProvider {
		t.Fatalf("detail provider = %q, want %q", detail.ProviderSession.Provider, wantProvider)
	}
	if detail.ProviderSession.Kind != wantKind {
		t.Fatalf("detail kind = %q, want %q", detail.ProviderSession.Kind, wantKind)
	}
}
