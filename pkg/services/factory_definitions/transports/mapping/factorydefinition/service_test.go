package factorydefinition

import (
	"context"
	"errors"
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

	owner := &snapshotImportOwner{}
	mapped, err := New(owner).Save(t.Context(), "selected-session", factoryapi.FactorySaveModeReplaceCurrent, request)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	editable := owner.saved
	if editable.Name != "alpha" || editable.Version == nil || editable.Version.Logical != 41 || !editable.Version.Physical.Equal(physical) {
		t.Fatalf("editable Factory = %#v, want alpha at version 41/%s", editable, physical)
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

// The mapping component receives a completed snapshot import operation and
// forwards its detached result without assembling the snapshot implementation.
type snapshotImportOwner struct {
	factorydefinitions.Service
	saved     factorydefinitions.EditableFactory
	importErr error
}

func (owner *snapshotImportOwner) PrepareFactorySnapshotImport(_ context.Context, request factorydefinitions.PrepareFactorySnapshotImportRequest) (factorydefinitions.PrepareFactorySnapshotImportResult, error) {
	if owner.importErr != nil {
		return factorydefinitions.PrepareFactorySnapshotImportResult{}, owner.importErr
	}
	var snapshot factorydefinitions.FactorySnapshot
	if err := snapshot.UnmarshalJSON(request.Payload); err != nil {
		return factorydefinitions.PrepareFactorySnapshotImportResult{}, err
	}
	return factorydefinitions.PrepareFactorySnapshotImportResult{Snapshot: &snapshot}, nil
}
func (owner *snapshotImportOwner) Save(_ context.Context, _ string, _ factorydefinitions.SaveMode, request factorydefinitions.EditableFactory) (factorydefinitions.EditableFactory, error) {
	owner.saved = request
	return request, nil
}
func TestSavePreservesSnapshotImportCauseWithoutSaving(t *testing.T) {
	t.Parallel()
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded, factorydefinitions.ErrInvalidFactorySnapshotPayload} {
		t.Run(cause.Error(), func(t *testing.T) {
			t.Parallel()
			owner := &snapshotImportOwner{importErr: cause}
			_, err := New(owner).Save(t.Context(), "selected-session", factoryapi.FactorySaveModeReplaceCurrent, factoryapi.Factory{Name: "alpha"})
			if !errors.Is(err, cause) || owner.saved.Snapshot != nil {
				t.Fatalf("Save = %v, saved = %#v", err, owner.saved)
			}
		})
	}
}
