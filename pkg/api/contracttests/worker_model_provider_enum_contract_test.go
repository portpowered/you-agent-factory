package apicontract_test

import (
	"sort"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestOpenAPIContract_WorkerModelProviderEnumMatchesSupportedBackendProviders(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	schemas := componentSchemas(t, doc)
	schema := schemaObject(t, schemas, "WorkerModelProvider")

	wantPublic := make([]string, 0, len(interfaces.SupportedModelProviders()))
	for _, internal := range interfaces.SupportedModelProviders() {
		public, ok := interfaces.PublicWorkerModelProviderFromInternal(internal)
		if !ok {
			t.Fatalf("PublicWorkerModelProviderFromInternal(%q) = false", internal)
		}
		wantPublic = append(wantPublic, string(public))
	}
	sort.Strings(wantPublic)
	assertEnumValues(t, schema, "WorkerModelProvider", wantPublic)

	for _, internal := range interfaces.SupportedModelProviders() {
		public, ok := interfaces.PublicWorkerModelProviderFromInternal(internal)
		if !ok {
			t.Fatalf("PublicWorkerModelProviderFromInternal(%q) = false", internal)
		}
		mapped, ok := interfaces.InternalModelProviderFromPublicWorkerModelProvider(public)
		if !ok || mapped != internal {
			t.Fatalf("InternalModelProviderFromPublicWorkerModelProvider(%q) = (%q, %v), want (%q, true)", public, mapped, ok, internal)
		}
	}
}

func TestOpenAPIContract_GeneratedWorkerModelProviderConstantsMatchOpenAPIEnum(t *testing.T) {
	want := []factoryapi.WorkerModelProvider{
		factoryapi.WorkerModelProviderClaude,
		factoryapi.WorkerModelProviderCodex,
		factoryapi.WorkerModelProviderCursor,
		factoryapi.WorkerModelProviderGemini,
		factoryapi.WorkerModelProviderKiro,
		factoryapi.WorkerModelProviderOpenCode,
	}
	if len(want) != len(interfaces.SupportedModelProviders()) {
		t.Fatalf("generated WorkerModelProvider constants = %d, supported internal providers = %d", len(want), len(interfaces.SupportedModelProviders()))
	}
	for _, public := range want {
		if _, ok := interfaces.InternalModelProviderFromPublicWorkerModelProvider(public); !ok {
			t.Fatalf("InternalModelProviderFromPublicWorkerModelProvider(%q) = false", public)
		}
	}
}
