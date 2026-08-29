package service

import (
	"reflect"
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

func TestRetainedRuntimeMetricsSessionIDsDeduplicatesSuccessorAndSource(t *testing.T) {
	const canonicalID = "canonical-runtime-id"

	got := retainedRuntimeMetricsSessionIDs(canonicalID, canonicalID)
	if len(got) != 1 || got[0] != canonicalID {
		t.Fatalf("retained metrics IDs = %#v, want one canonical identity", got)
	}

	got = retainedRuntimeMetricsSessionIDs(canonicalID, "source-runtime-id")
	want := []string{canonicalID, "source-runtime-id"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("retained metrics IDs = %#v, want %#v", got, want)
	}
}
