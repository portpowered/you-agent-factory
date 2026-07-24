package factorydefinition

import (
	"context"
	"errors"
	"testing"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestService_PromotedUnimplementedRootSlices(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := New(stubDefinitionHost{})

	if _, err := svc.ListNamedFactories(ctx, factoryroot.ListNamedFactoriesRequest{}); err == nil {
		t.Fatal("promoted ListNamedFactories: expected collaborator-required error")
	}
	if _, err := svc.GetNamedFactory(ctx, factoryroot.GetNamedFactoryRequest{Name: "missing"}); !errors.Is(err, factoryroot.ErrNamedFactoryNotFound) {
		t.Fatalf("promoted GetNamedFactory: got %v", err)
	}
	if _, err := svc.GetCurrentFactoryPointer(ctx, factoryroot.GetCurrentFactoryPointerRequest{}); !errors.Is(err, factoryroot.ErrCurrentFactoryNotFound) {
		t.Fatalf("promoted GetCurrentFactoryPointer: got %v", err)
	}
	if _, err := svc.SetCurrentFactoryPointer(ctx, factoryroot.SetCurrentFactoryPointerRequest{Name: "alpha"}); !errors.Is(err, factoryroot.ErrNamedFactoryNotFound) {
		t.Fatalf("promoted SetCurrentFactoryPointer: got %v", err)
	}
	if _, err := svc.PrepareFactoryLayout(ctx, factoryroot.PrepareFactoryLayoutRequest{}); !errors.Is(err, factoryroot.ErrMalformedFactoryLayoutPayload) {
		t.Fatalf("promoted PrepareFactoryLayout: got %v", err)
	}
	if _, err := svc.CompileEffectiveFactorySource(ctx, factoryroot.CompileEffectiveFactorySourceRequest{}); !errors.Is(err, factoryroot.ErrInvalidAuthoredFactorySource) {
		t.Fatalf("promoted CompileEffectiveFactorySource: got %v", err)
	}
	if _, err := svc.ValidateStructuralFactoryDefinition(ctx, factoryroot.ValidateStructuralFactoryDefinitionRequest{}); !errors.Is(err, factoryroot.ErrInvalidFactoryDefinitionPayload) {
		t.Fatalf("promoted ValidateStructuralFactoryDefinition: got %v", err)
	}
	if _, err := svc.CaptureFactorySnapshot(ctx, factoryroot.CaptureFactorySnapshotRequest{}); !errors.Is(err, factoryroot.ErrInvalidFactorySnapshotPayload) {
		t.Fatalf("promoted CaptureFactorySnapshot: got %v", err)
	}
	if _, err := svc.InstallPackagedFactory(ctx, factoryroot.InstallPackagedFactoryRequest{Name: "@you/missing"}); !errors.Is(err, factoryroot.ErrUnknownPackagedFactoryIdentity) {
		t.Fatalf("promoted InstallPackagedFactory: got %v", err)
	}
	if _, err := svc.CreateFactoryScaffold(ctx, factoryroot.CreateFactoryScaffoldRequest{}); !errors.Is(err, factoryroot.ErrFactoryDistributeFailed) {
		t.Fatalf("promoted CreateFactoryScaffold: got %v", err)
	}
}
