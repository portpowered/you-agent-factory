package factorydefinition

import (
	"context"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/transports/http/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestEditableFactoryRoundTripPreservesSnapshotAndVersion(t *testing.T) {
	t.Parallel()

	physical := time.Date(2026, time.July, 16, 12, 30, 0, 0, time.UTC)
	request := factoryapi.Factory{
		Name: "alpha",
		Id:   stringPointer("runtime-alpha"),
		Version: &factoryapi.HybridLogicalTimestamp{
			Logical:  apitypes.Int64String(41),
			Physical: physical,
		},
	}

	editable, err := editableFactoryFromAPI(request)
	if err != nil {
		t.Fatalf("editableFactoryFromAPI: %v", err)
	}
	if editable.Name != "alpha" || editable.Version == nil || editable.Version.Logical != 41 || !editable.Version.Physical.Equal(physical) {
		t.Fatalf("editable Factory = %#v, want alpha at version 41/%s", editable, physical)
	}

	mapped, err := editableFactoryToAPI(editable)
	if err != nil {
		t.Fatalf("editableFactoryToAPI: %v", err)
	}
	if mapped.Name != request.Name || mapped.Id == nil || *mapped.Id != "runtime-alpha" {
		t.Fatalf("mapped Factory = %#v, want original identity", mapped)
	}
	if mapped.Version == nil || mapped.Version.Logical.Int64() != 41 || !mapped.Version.Physical.Equal(physical) {
		t.Fatalf("mapped version = %#v, want 41/%s", mapped.Version, physical)
	}
}

func TestSaveModeFromAPIPreservesPolicySelection(t *testing.T) {
	t.Parallel()

	if got := saveModeFromAPI(factoryapi.FactorySaveModeUpsertNamedAndActivate); got != factorydefinitions.SaveModeUpsertNamedAndActivate {
		t.Fatalf("upsert mode = %q, want %q", got, factorydefinitions.SaveModeUpsertNamedAndActivate)
	}
	if got := saveModeFromAPI(factoryapi.FactorySaveModeReplaceCurrent); got != factorydefinitions.SaveModeReplaceCurrent {
		t.Fatalf("replace mode = %q, want %q", got, factorydefinitions.SaveModeReplaceCurrent)
	}
}

func TestServiceRequiresDomainOwner(t *testing.T) {
	t.Parallel()

	svc := New(nil)
	ctx := context.Background()
	if err := svc.ActivateNamedFactory(ctx, "alpha"); err == nil {
		t.Fatal("ActivateNamedFactory error = nil, want missing owner error")
	}
	if _, err := svc.Save(ctx, "session", factoryapi.FactorySaveModeReplaceCurrent, factoryapi.Factory{}); err == nil {
		t.Fatal("Save error = nil, want missing owner error")
	}
	if _, err := svc.GetCurrentNamedFactory(ctx); err == nil {
		t.Fatal("GetCurrentNamedFactory error = nil, want missing owner error")
	}
	if _, err := svc.GetCurrentFactoryForSession(ctx, "session"); err == nil {
		t.Fatal("GetCurrentFactoryForSession error = nil, want missing owner error")
	}
	if _, err := svc.CurrentFactoryDefinitionVersionAtRoot("root", "alpha"); err == nil {
		t.Fatal("CurrentFactoryDefinitionVersionAtRoot error = nil, want missing owner error")
	}
}

func TestEditableFactoryToAPIRequiresSnapshot(t *testing.T) {
	t.Parallel()

	if _, err := editableFactoryToAPI(factorydefinitions.EditableFactory{}); err == nil {
		t.Fatal("editableFactoryToAPI error = nil, want missing snapshot error")
	}
}

func stringPointer(value string) *string { return &value }
