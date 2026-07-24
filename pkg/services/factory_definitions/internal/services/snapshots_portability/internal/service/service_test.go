package service_test

import (
	"context"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	snapshotsportability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability"
	snapshotsportabilitywire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/wire"
)

// rootSnapshotFacade proves the published Definitions root Service snapshot
// surface can succeed through the private snapshots_portability owner rather
// than a second competing authority. UnimplementedService covers the other
// CTR-DEF slices; the core read/save methods are stubbed so the facade stays
// assignable to factorydefinitions.Service.
type rootSnapshotFacade struct {
	factorydefinitions.UnimplementedService
	portability snapshotsportability.Service
}

func (rootSnapshotFacade) ActivateNamedFactory(context.Context, string) error {
	return nil
}

func (rootSnapshotFacade) Save(
	context.Context,
	string,
	factorydefinitions.SaveMode,
	factorydefinitions.EditableFactory,
) (factorydefinitions.EditableFactory, error) {
	return factorydefinitions.EditableFactory{}, nil
}

func (rootSnapshotFacade) GetCurrentNamedFactory(
	context.Context,
) (*factorydefinitions.FactorySnapshot, error) {
	return nil, factorydefinitions.ErrCurrentFactoryNotFound
}

func (rootSnapshotFacade) GetCurrentFactoryForSession(
	context.Context,
	string,
) (factorydefinitions.EditableFactory, error) {
	return factorydefinitions.EditableFactory{}, factorydefinitions.ErrCurrentFactoryNotFound
}

func (rootSnapshotFacade) CurrentFactoryDefinitionVersionAtRoot(
	string,
	string,
) (factorydefinitions.FactoryVersion, error) {
	return factorydefinitions.FactoryVersion{}, factorydefinitions.ErrCurrentFactoryNotFound
}

func (f rootSnapshotFacade) CaptureFactorySnapshot(
	ctx context.Context,
	request factorydefinitions.CaptureFactorySnapshotRequest,
) (factorydefinitions.CaptureFactorySnapshotResult, error) {
	return f.portability.CaptureFactorySnapshot(ctx, request)
}

func (f rootSnapshotFacade) PrepareFactorySnapshotImport(
	ctx context.Context,
	request factorydefinitions.PrepareFactorySnapshotImportRequest,
) (factorydefinitions.PrepareFactorySnapshotImportResult, error) {
	return f.portability.PrepareFactorySnapshotImport(ctx, request)
}

func (f rootSnapshotFacade) MaterializeFactorySnapshot(
	ctx context.Context,
	request factorydefinitions.MaterializeFactorySnapshotRequest,
) (factorydefinitions.MaterializeFactorySnapshotResult, error) {
	return f.portability.MaterializeFactorySnapshot(ctx, request)
}

func TestSnapshotsPortability_OwnsRootSnapshotSurfaceSuccess(t *testing.T) {
	t.Parallel()

	portability, err := snapshotsportabilitywire.NewService()
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	var root factorydefinitions.Service = rootSnapshotFacade{portability: portability}

	canonical := []byte(`{
		"name": "alpha",
		"resourceManifest": {
			"bundledFiles": [
				{"type": "DOC", "targetPath": "factory/docs/README.md", "content": {"inline": "hello", "encoding": "utf-8"}}
			]
		}
	}`)
	captured, err := root.CaptureFactorySnapshot(
		context.Background(),
		factorydefinitions.CaptureFactorySnapshotRequest{
			FactoryDir: "/factories/alpha",
			Canonical:  canonical,
			Name:       "alpha",
		},
	)
	if err != nil {
		t.Fatalf("CaptureFactorySnapshot: %v", err)
	}
	if captured.Snapshot == nil {
		t.Fatal("CaptureFactorySnapshot snapshot is nil")
	}

	imported, err := root.PrepareFactorySnapshotImport(
		context.Background(),
		factorydefinitions.PrepareFactorySnapshotImportRequest{
			Payload: []byte(*captured.Snapshot),
		},
	)
	if err != nil {
		t.Fatalf("PrepareFactorySnapshotImport: %v", err)
	}
	if imported.Snapshot == nil || imported.Name != "alpha" {
		t.Fatalf("PrepareFactorySnapshotImport result = %#v, want alpha snapshot facts", imported)
	}
	if imported.Portable.FactoryDir != "/factories/alpha" {
		t.Fatalf("PrepareFactorySnapshotImport factoryDir = %q, want /factories/alpha", imported.Portable.FactoryDir)
	}
	if len(imported.Portable.Assets) == 0 || imported.Portable.Assets[0].TargetPath != "factory/docs/README.md" {
		t.Fatalf("PrepareFactorySnapshotImport assets = %#v, want README asset", imported.Portable.Assets)
	}

	materialized, err := root.MaterializeFactorySnapshot(
		context.Background(),
		factorydefinitions.MaterializeFactorySnapshotRequest{
			TargetDir: "/factories/alpha",
			Snapshot:  imported.Snapshot,
		},
	)
	if err != nil {
		t.Fatalf("MaterializeFactorySnapshot: %v", err)
	}
	if materialized.TargetDir != "/factories/alpha" ||
		materialized.Portable.FactoryDir != "/factories/alpha" ||
		len(materialized.Portable.Assets) == 0 {
		t.Fatalf("MaterializeFactorySnapshot result = %#v, want portable success facts", materialized)
	}
}

func TestSnapshotsPortability_TypedFailuresStayDistinct(t *testing.T) {
	t.Parallel()

	portability, err := snapshotsportabilitywire.NewService()
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, invalidErr := portability.CaptureFactorySnapshot(
		context.Background(),
		factorydefinitions.CaptureFactorySnapshotRequest{Canonical: []byte(`"string"`)},
	)
	if !errors.Is(invalidErr, factorydefinitions.ErrInvalidFactorySnapshotPayload) {
		t.Fatalf("CaptureFactorySnapshot invalid error = %v, want ErrInvalidFactorySnapshotPayload", invalidErr)
	}

	_, unsafeErr := portability.MaterializeFactorySnapshot(
		context.Background(),
		factorydefinitions.MaterializeFactorySnapshotRequest{
			TargetDir: "../outside",
			Snapshot:  nil,
		},
	)
	if !errors.Is(unsafeErr, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize) {
		t.Fatalf("MaterializeFactorySnapshot unsafe error = %v, want ErrUnsafeFactorySnapshotMaterialize", unsafeErr)
	}
	if errors.Is(unsafeErr, factorydefinitions.ErrInvalidFactorySnapshotPayload) {
		t.Fatal("unsafe materialize must not also match ErrInvalidFactorySnapshotPayload")
	}
}
