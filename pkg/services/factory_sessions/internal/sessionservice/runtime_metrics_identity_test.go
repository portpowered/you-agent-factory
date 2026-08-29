package service

import (
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func TestSelectCompletionSessionIdentityUsesRetainedMetricIdentity(t *testing.T) {
	const canonicalID = "canonical-runtime-id"

	identity := selectCompletionSessionIdentity(
		factorysessions.DefaultSessionID,
		factoryruntime.SessionBuildSpec{
			SessionID:        factorysessions.DefaultSessionID,
			MetricsSessionID: canonicalID,
		},
	)

	if identity.id != factorysessions.DefaultSessionID {
		t.Fatalf("completion identity id = %q, want %q", identity.id, factorysessions.DefaultSessionID)
	}
	if !identity.isDefault {
		t.Fatal("completion identity isDefault = false, want true")
	}
	if identity.runtimeID != canonicalID {
		t.Fatalf("completion identity runtime ID = %q, want %q", identity.runtimeID, canonicalID)
	}
}
