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

func TestSnapshotsPortability_CaptureValidSourcePreservesIdentity(t *testing.T) {
	t.Parallel()

	portability, err := snapshotsportabilitywire.NewService()
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	canonical := []byte(`{
		"name": "source-name",
		"futureField": {"enabled": true},
		"resourceManifest": {
			"bundledFiles": [
				{"type": "DOC", "targetPath": "factory/docs/README.md", "content": {"inline": "hello", "encoding": "utf-8"}}
			]
		}
	}`)
	request := factorydefinitions.CaptureFactorySnapshotRequest{
		FactoryDir: "/factories/alpha",
		Canonical:  canonical,
		Name:       "alpha",
	}

	result, err := portability.CaptureFactorySnapshot(context.Background(), request)
	if err != nil {
		t.Fatalf("CaptureFactorySnapshot: %v", err)
	}
	if result.Snapshot == nil {
		t.Fatal("CaptureFactorySnapshot snapshot is nil")
	}

	// Public Definitions boundary success shape stays CaptureFactorySnapshotResult
	// with a detached FactorySnapshot — not peer storage/Recordings/Runtime types.
	var bounded factorydefinitions.CaptureFactorySnapshotResult = result
	if bounded.Snapshot == nil {
		t.Fatal("CaptureFactorySnapshotResult.Snapshot is nil")
	}

	var object map[string]any
	if decodeErr := result.Snapshot.Decode(&object); decodeErr != nil {
		t.Fatalf("CaptureFactorySnapshot decode: %v", decodeErr)
	}
	if object["name"] != "alpha" {
		t.Fatalf("captured name = %#v, want alpha", object["name"])
	}
	if object["factoryDirectory"] != "/factories/alpha" {
		t.Fatalf("captured factoryDirectory = %#v, want /factories/alpha", object["factoryDirectory"])
	}
	future, ok := object["futureField"].(map[string]any)
	if !ok || future["enabled"] != true {
		t.Fatalf("captured futureField = %#v, want preserved unknown identity-adjacent field", object["futureField"])
	}

	// Detached capture must not share mutable backing with the request payload.
	canonical[2] = 'X'
	var afterMutation map[string]any
	if decodeErr := result.Snapshot.Decode(&afterMutation); decodeErr != nil {
		t.Fatalf("CaptureFactorySnapshot decode after mutation: %v", decodeErr)
	}
	if afterMutation["name"] != "alpha" {
		t.Fatalf("captured snapshot mutated with request Canonical; name = %#v", afterMutation["name"])
	}
}

func TestSnapshotsPortability_PrepareValidPayloadYieldsPortableImportFacts(t *testing.T) {
	t.Parallel()

	portability, err := snapshotsportabilitywire.NewService()
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	payload := []byte(`{
		"name": "alpha",
		"factoryDirectory": "/factories/alpha",
		"futureField": {"enabled": true},
		"resourceManifest": {
			"bundledFiles": [
				{"type": "DOC", "targetPath": "factory/docs/README.md", "content": {"inline": "hello", "encoding": "utf-8"}}
			]
		}
	}`)
	request := factorydefinitions.PrepareFactorySnapshotImportRequest{Payload: payload}

	result, err := portability.PrepareFactorySnapshotImport(context.Background(), request)
	if err != nil {
		t.Fatalf("PrepareFactorySnapshotImport: %v", err)
	}
	if result.Snapshot == nil {
		t.Fatal("PrepareFactorySnapshotImport snapshot is nil")
	}

	// Public Definitions boundary success shape stays PrepareFactorySnapshotImportResult
	// with detached FactorySnapshot + PortableFactorySnapshotFacts.
	var bounded factorydefinitions.PrepareFactorySnapshotImportResult = result
	if bounded.Snapshot == nil {
		t.Fatal("PrepareFactorySnapshotImportResult.Snapshot is nil")
	}
	if bounded.Name != "alpha" {
		t.Fatalf("PrepareFactorySnapshotImportResult.Name = %q, want alpha", bounded.Name)
	}
	if bounded.Portable.FactoryDir != "/factories/alpha" {
		t.Fatalf("Portable.FactoryDir = %q, want /factories/alpha", bounded.Portable.FactoryDir)
	}
	if len(bounded.Portable.Assets) == 0 || bounded.Portable.Assets[0].TargetPath != "factory/docs/README.md" {
		t.Fatalf("Portable.Assets = %#v, want README asset fact", bounded.Portable.Assets)
	}

	var object map[string]any
	if decodeErr := result.Snapshot.Decode(&object); decodeErr != nil {
		t.Fatalf("PrepareFactorySnapshotImport decode: %v", decodeErr)
	}
	if object["name"] != "alpha" {
		t.Fatalf("prepared name = %#v, want alpha", object["name"])
	}
	if object["factoryDirectory"] != "/factories/alpha" {
		t.Fatalf("prepared factoryDirectory = %#v, want /factories/alpha", object["factoryDirectory"])
	}
	future, ok := object["futureField"].(map[string]any)
	if !ok || future["enabled"] != true {
		t.Fatalf("prepared futureField = %#v, want preserved unknown field", object["futureField"])
	}

	// Detached prepare must not share mutable backing with the request payload.
	payload[2] = 'X'
	var afterMutation map[string]any
	if decodeErr := result.Snapshot.Decode(&afterMutation); decodeErr != nil {
		t.Fatalf("PrepareFactorySnapshotImport decode after mutation: %v", decodeErr)
	}
	if afterMutation["name"] != "alpha" {
		t.Fatalf("prepared snapshot mutated with request Payload; name = %#v", afterMutation["name"])
	}
}

func TestSnapshotsPortability_PrepareInvalidPayloadTypedFailure(t *testing.T) {
	t.Parallel()

	portability, err := snapshotsportabilitywire.NewService()
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, invalidErr := portability.PrepareFactorySnapshotImport(
		context.Background(),
		factorydefinitions.PrepareFactorySnapshotImportRequest{Payload: []byte(`["not-object"]`)},
	)
	if !errors.Is(invalidErr, factorydefinitions.ErrInvalidFactorySnapshotPayload) {
		t.Fatalf(
			"PrepareFactorySnapshotImport invalid-payload error = %v, want ErrInvalidFactorySnapshotPayload",
			invalidErr,
		)
	}
	if errors.Is(invalidErr, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize) {
		t.Fatal("invalid prepare payload must not also match ErrUnsafeFactorySnapshotMaterialize")
	}

	_, emptyErr := portability.PrepareFactorySnapshotImport(
		context.Background(),
		factorydefinitions.PrepareFactorySnapshotImportRequest{Payload: []byte(`"string"`)},
	)
	if !errors.Is(emptyErr, factorydefinitions.ErrInvalidFactorySnapshotPayload) {
		t.Fatalf(
			"PrepareFactorySnapshotImport string payload error = %v, want ErrInvalidFactorySnapshotPayload",
			emptyErr,
		)
	}
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
