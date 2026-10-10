package lifecycle_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/lifecycle"
)

// Only the selected component ports are active. Unselected operations retain the
// disabled contract, so none of these cases assembles another service owner.
type delegatedOwner struct {
	definitions.UnimplementedService
	ctx           context.Context
	request       any
	err           error
	validation    definitions.ValidateStructuralFactoryDefinitionResult
	layout        definitions.PrepareFactoryLayoutResult
	flattened     definitions.FlattenFactoryLayoutResult
	expanded      definitions.ExpandFactoryLayoutResult
	created       definitions.CreateNamedFactoryResult
	replaced      definitions.ReplaceNamedFactoryResult
	packaged      definitions.ListBuiltInPackagedFactoriesResult
	snapshot      definitions.ResolveRuntimeSnapshotResult
	capture       definitions.CaptureFactorySnapshotResult
	prepareImport definitions.PrepareFactorySnapshotImportResult
	materialize   definitions.MaterializeFactorySnapshotResult
}

func (o *delegatedOwner) ValidateStructuralFactoryDefinition(ctx context.Context, request definitions.ValidateStructuralFactoryDefinitionRequest) (definitions.ValidateStructuralFactoryDefinitionResult, error) {
	o.ctx, o.request = ctx, request
	return o.validation, o.err
}
func (o *delegatedOwner) PrepareFactoryLayout(ctx context.Context, request definitions.PrepareFactoryLayoutRequest) (definitions.PrepareFactoryLayoutResult, error) {
	o.ctx, o.request = ctx, request
	return o.layout, o.err
}
func (o *delegatedOwner) ListBuiltInPackagedFactories(ctx context.Context, request definitions.ListBuiltInPackagedFactoriesRequest) (definitions.ListBuiltInPackagedFactoriesResult, error) {
	o.ctx, o.request = ctx, request
	return o.packaged, o.err
}
func (o *delegatedOwner) ResolveRuntimeSnapshot(ctx context.Context, request definitions.ResolveRuntimeSnapshotRequest) (definitions.ResolveRuntimeSnapshotResult, error) {
	o.ctx, o.request = ctx, request
	return o.snapshot, o.err
}

func TestInjectedDelegationPreservesResultsRequestsAndErrorIdentity(t *testing.T) {
	t.Parallel()
	for _, failure := range []struct {
		name string
		err  error
	}{
		{"success", nil}, {"failure", errors.New("selected owner failed")}, {"cancellation", context.Canceled},
	} {
		t.Run(failure.name, func(t *testing.T) {
			t.Parallel()
			for _, operation := range []string{"validation", "authoring", "flatten", "expand", "create", "replace", "distribution", "snapshot", "capture", "prepare-import", "materialize"} {
				t.Run(operation, func(t *testing.T) {
					t.Parallel()
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					if failure.err == context.Canceled {
						cancel()
					}
					owner := &delegatedOwner{err: failure.err,
						validation:    definitions.ValidateStructuralFactoryDefinitionResult{Validation: definitions.ValidationResult{Targets: []definitions.ValidationTarget{{}}}},
						layout:        definitions.PrepareFactoryLayoutResult{Prepared: definitions.PreparedFactoryLayoutPayload{Canonical: []byte(`{"name":"alpha"}`)}},
						flattened:     definitions.FlattenFactoryLayoutResult{Canonical: []byte("flat-alpha")},
						expanded:      definitions.ExpandFactoryLayoutResult{FactoryDir: "/factories/alpha"},
						created:       definitions.CreateNamedFactoryResult{Name: "alpha", FactoryDir: "/factories/alpha"},
						replaced:      definitions.ReplaceNamedFactoryResult{Name: "alpha", FactoryDir: "/factories/alpha"},
						packaged:      definitions.ListBuiltInPackagedFactoriesResult{Entries: []definitions.BuiltInPackagedFactoryEntry{{Name: "alpha"}}},
						snapshot:      definitions.ResolveRuntimeSnapshotResult{Snapshot: definitions.RuntimeSnapshot{FactoryDir: "alpha"}},
						capture:       definitions.CaptureFactorySnapshotResult{Snapshot: snapshotPayload()},
						prepareImport: definitions.PrepareFactorySnapshotImportResult{Name: "alpha"},
						materialize:   definitions.MaterializeFactorySnapshotResult{TargetDir: "alpha"},
					}
					disabled := definitions.UnimplementedService{}
					service := lifecycle.NewWithCatalogPackagesValidationDistributionAndAuthoring(
						nil, lifecycle.StubActivationGateway(), disabled, owner, owner, owner, owner, disabled, nil, disabled.ListEffectiveFactories,
						owner,
					)
					if owner.ctx != nil {
						t.Fatal("construction called a collaborator")
					}
					assertDelegation(t, ctx, service, owner, operation, failure.err)
				})
			}
		})
	}
}

func assertDelegation(t *testing.T, ctx context.Context, service *lifecycle.Service, owner *delegatedOwner, operation string, cause error) {
	t.Helper()
	var request, result, expected any
	var err error
	switch operation {
	case "capture":
		r := definitions.CaptureFactorySnapshotRequest{FactoryDir: "alpha", Canonical: []byte(`{"name":"alpha"}`), Name: "alpha"}
		request, expected = r, owner.capture
		result, err = service.CaptureFactorySnapshot(ctx, r)
	case "prepare-import":
		r := definitions.PrepareFactorySnapshotImportRequest{Payload: []byte(`{"name":"alpha"}`)}
		request, expected = r, owner.prepareImport
		result, err = service.PrepareFactorySnapshotImport(ctx, r)
	case "materialize":
		r := definitions.MaterializeFactorySnapshotRequest{TargetDir: "alpha", Snapshot: snapshotPayload()}
		request, expected = r, owner.materialize
		result, err = service.MaterializeFactorySnapshot(ctx, r)
	case "validation":
		r := definitions.ValidateStructuralFactoryDefinitionRequest{Canonical: []byte(`{"name":"alpha"}`)}
		request, expected = r, owner.validation
		result, err = service.ValidateStructuralFactoryDefinition(ctx, r)
	case "authoring":
		r := definitions.PrepareFactoryLayoutRequest{Name: "alpha", Payload: []byte(`{"name":"alpha"}`)}
		request, expected = r, owner.layout
		result, err = service.PrepareFactoryLayout(ctx, r)
	case "flatten":
		r := definitions.FlattenFactoryLayoutRequest{Path: "/factories/alpha"}
		request, expected = r, owner.flattened
		result, err = service.FlattenFactoryLayout(ctx, r)
	case "expand":
		r := definitions.ExpandFactoryLayoutRequest{Path: "/factories/alpha"}
		request, expected = r, owner.expanded
		result, err = service.ExpandFactoryLayout(ctx, r)
	case "create":
		r := definitions.CreateNamedFactoryRequest{RootDir: "/factories", Name: "alpha", Prepared: owner.layout.Prepared}
		request, expected = r, owner.created
		result, err = service.CreateNamedFactory(ctx, r)
	case "replace":
		r := definitions.ReplaceNamedFactoryRequest{RootDir: "/factories", Name: "alpha", Prepared: owner.layout.Prepared}
		request, expected = r, owner.replaced
		result, err = service.ReplaceNamedFactory(ctx, r)
	case "distribution":
		r := definitions.ListBuiltInPackagedFactoriesRequest{}
		request, expected = r, owner.packaged
		result, err = service.ListBuiltInPackagedFactories(ctx, r)
	case "snapshot":
		r := definitions.ResolveRuntimeSnapshotRequest{FactoryDir: "alpha"}
		request, expected = r, owner.snapshot
		result, err = service.ResolveRuntimeSnapshot(ctx, r)
	}
	if owner.ctx != ctx || !reflect.DeepEqual(owner.request, request) || !reflect.DeepEqual(result, expected) || err != cause || !errors.Is(err, cause) {
		t.Fatalf("delegation request=%#v result=%#v error=%v; want request=%#v result=%#v error=%v", owner.request, result, err, request, expected, cause)
	}
}

func (o *delegatedOwner) CaptureFactorySnapshot(ctx context.Context, request definitions.CaptureFactorySnapshotRequest) (definitions.CaptureFactorySnapshotResult, error) {
	o.ctx, o.request = ctx, request
	return o.capture, o.err
}

func (o *delegatedOwner) PrepareFactorySnapshotImport(ctx context.Context, request definitions.PrepareFactorySnapshotImportRequest) (definitions.PrepareFactorySnapshotImportResult, error) {
	o.ctx, o.request = ctx, request
	return o.prepareImport, o.err
}

func (o *delegatedOwner) MaterializeFactorySnapshot(ctx context.Context, request definitions.MaterializeFactorySnapshotRequest) (definitions.MaterializeFactorySnapshotResult, error) {
	o.ctx, o.request = ctx, request
	return o.materialize, o.err
}

func TestExplicitlyDisabledSnapshotOwnerRetainsDisabledResults(t *testing.T) {
	t.Parallel()
	disabled := definitions.UnimplementedService{}
	service := lifecycle.NewWithCatalogPackagesValidationDistributionAndAuthoring(nil, lifecycle.StubActivationGateway(), disabled, disabled, disabled, disabled, nil, disabled, nil, disabled.ListEffectiveFactories, disabled)
	for _, subject := range []*lifecycle.Service{service} {
		captured, captureErr := subject.CaptureFactorySnapshot(t.Context(), definitions.CaptureFactorySnapshotRequest{})
		expectedCapture, expectedCaptureErr := disabled.CaptureFactorySnapshot(t.Context(), definitions.CaptureFactorySnapshotRequest{})
		imported, importErr := subject.PrepareFactorySnapshotImport(t.Context(), definitions.PrepareFactorySnapshotImportRequest{})
		expectedImport, expectedImportErr := disabled.PrepareFactorySnapshotImport(t.Context(), definitions.PrepareFactorySnapshotImportRequest{})
		materialized, materializeErr := subject.MaterializeFactorySnapshot(t.Context(), definitions.MaterializeFactorySnapshotRequest{})
		expectedMaterialize, expectedMaterializeErr := disabled.MaterializeFactorySnapshot(t.Context(), definitions.MaterializeFactorySnapshotRequest{})
		if !reflect.DeepEqual(captured, expectedCapture) || !errors.Is(captureErr, expectedCaptureErr) || !reflect.DeepEqual(imported, expectedImport) || !errors.Is(importErr, expectedImportErr) || !reflect.DeepEqual(materialized, expectedMaterialize) || !errors.Is(materializeErr, expectedMaterializeErr) {
			t.Fatalf("disabled snapshot owner changed contract: %v, %v, %v", captureErr, importErr, materializeErr)
		}
	}
}

func snapshotPayload() *definitions.FactorySnapshot {
	payload := definitions.FactorySnapshot(`{"name":"alpha"}`)
	return &payload
}

func (o *delegatedOwner) FlattenFactoryLayout(ctx context.Context, request definitions.FlattenFactoryLayoutRequest) (definitions.FlattenFactoryLayoutResult, error) {
	o.ctx, o.request = ctx, request
	return o.flattened, o.err
}
func (o *delegatedOwner) ExpandFactoryLayout(ctx context.Context, request definitions.ExpandFactoryLayoutRequest) (definitions.ExpandFactoryLayoutResult, error) {
	o.ctx, o.request = ctx, request
	return o.expanded, o.err
}
func (o *delegatedOwner) CreateNamedFactory(ctx context.Context, request definitions.CreateNamedFactoryRequest) (definitions.CreateNamedFactoryResult, error) {
	o.ctx, o.request = ctx, request
	return o.created, o.err
}
func (o *delegatedOwner) ReplaceNamedFactory(ctx context.Context, request definitions.ReplaceNamedFactoryRequest) (definitions.ReplaceNamedFactoryResult, error) {
	o.ctx, o.request = ctx, request
	return o.replaced, o.err
}
