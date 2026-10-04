package lifecycle_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/lifecycle"
)

// Only these four component ports are active. Unselected operations retain the
// disabled contract, so none of these cases assembles another service owner.
type delegatedOwner struct {
	definitions.UnimplementedService
	ctx        context.Context
	request    any
	err        error
	validation definitions.ValidateStructuralFactoryDefinitionResult
	layout     definitions.PrepareFactoryLayoutResult
	packaged   definitions.ListBuiltInPackagedFactoriesResult
	snapshot   definitions.ResolveRuntimeSnapshotResult
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
			for _, operation := range []string{"validation", "authoring", "distribution", "snapshot"} {
				t.Run(operation, func(t *testing.T) {
					t.Parallel()
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					if failure.err == context.Canceled {
						cancel()
					}
					owner := &delegatedOwner{err: failure.err,
						validation: definitions.ValidateStructuralFactoryDefinitionResult{Validation: definitions.ValidationResult{Targets: []definitions.ValidationTarget{{}}}},
						layout:     definitions.PrepareFactoryLayoutResult{Prepared: definitions.PreparedFactoryLayoutPayload{Canonical: []byte(`{"name":"alpha"}`)}},
						packaged:   definitions.ListBuiltInPackagedFactoriesResult{Entries: []definitions.BuiltInPackagedFactoryEntry{{Name: "alpha"}}},
						snapshot:   definitions.ResolveRuntimeSnapshotResult{Snapshot: definitions.RuntimeSnapshot{FactoryDir: "alpha"}},
					}
					disabled := definitions.UnimplementedService{}
					service := lifecycle.NewWithCatalogPackagesValidationDistributionAndAuthoring(
						nil, lifecycle.StubActivationGateway(), disabled, owner, owner, owner, owner, disabled, nil, disabled.ListEffectiveFactories,
					)
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
	case "validation":
		r := definitions.ValidateStructuralFactoryDefinitionRequest{Canonical: []byte(`{"name":"alpha"}`)}
		request, expected = r, owner.validation
		result, err = service.ValidateStructuralFactoryDefinition(ctx, r)
	case "authoring":
		r := definitions.PrepareFactoryLayoutRequest{Name: "alpha", Payload: []byte(`{"name":"alpha"}`)}
		request, expected = r, owner.layout
		result, err = service.PrepareFactoryLayout(ctx, r)
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
