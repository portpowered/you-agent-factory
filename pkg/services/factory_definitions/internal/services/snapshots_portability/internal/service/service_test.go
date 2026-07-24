package service_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
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

func TestSnapshotsPortability_DetachedRoundTripPreservesPortableFacts(t *testing.T) {
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

	captured, err := portability.CaptureFactorySnapshot(
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

	imported, err := portability.PrepareFactorySnapshotImport(
		context.Background(),
		factorydefinitions.PrepareFactorySnapshotImportRequest{
			Payload: []byte(*captured.Snapshot),
		},
	)
	if err != nil {
		t.Fatalf("PrepareFactorySnapshotImport: %v", err)
	}
	if imported.Snapshot == nil || imported.Name != "alpha" {
		t.Fatalf("PrepareFactorySnapshotImport result = %#v, want alpha snapshot", imported)
	}
	if imported.Portable.FactoryDir != "/factories/alpha" {
		t.Fatalf("imported Portable.FactoryDir = %q, want /factories/alpha", imported.Portable.FactoryDir)
	}
	if len(imported.Portable.Assets) == 0 || imported.Portable.Assets[0].TargetPath != "factory/docs/README.md" {
		t.Fatalf("imported Portable.Assets = %#v, want README asset", imported.Portable.Assets)
	}

	materialized, err := portability.MaterializeFactorySnapshot(
		context.Background(),
		factorydefinitions.MaterializeFactorySnapshotRequest{
			TargetDir: "/factories/replay-alpha",
			Snapshot:  imported.Snapshot,
		},
	)
	if err != nil {
		t.Fatalf("MaterializeFactorySnapshot: %v", err)
	}

	// Public Definitions boundary success shape stays MaterializeFactorySnapshotResult
	// with Definitions-owned portable success facts — not Recordings/Runtime types.
	var bounded factorydefinitions.MaterializeFactorySnapshotResult = materialized
	if bounded.TargetDir != "/factories/replay-alpha" {
		t.Fatalf("MaterializeFactorySnapshotResult.TargetDir = %q, want /factories/replay-alpha", bounded.TargetDir)
	}
	if bounded.Portable.FactoryDir != "/factories/replay-alpha" {
		t.Fatalf("materialized Portable.FactoryDir = %q, want /factories/replay-alpha", bounded.Portable.FactoryDir)
	}
	if len(bounded.Portable.Assets) == 0 || bounded.Portable.Assets[0].TargetPath != "factory/docs/README.md" {
		t.Fatalf("materialized Portable.Assets = %#v, want README asset preserved across round trip", bounded.Portable.Assets)
	}

	// Replay-compatible identity facts survive capture → prepare → materialize.
	var capturedObject map[string]any
	if decodeErr := captured.Snapshot.Decode(&capturedObject); decodeErr != nil {
		t.Fatalf("captured decode: %v", decodeErr)
	}
	var importedObject map[string]any
	if decodeErr := imported.Snapshot.Decode(&importedObject); decodeErr != nil {
		t.Fatalf("imported decode: %v", decodeErr)
	}
	if capturedObject["name"] != "alpha" || importedObject["name"] != "alpha" {
		t.Fatalf("round-trip name facts: captured=%#v imported=%#v, want alpha", capturedObject["name"], importedObject["name"])
	}
	if capturedObject["factoryDirectory"] != "/factories/alpha" ||
		importedObject["factoryDirectory"] != "/factories/alpha" {
		t.Fatalf(
			"round-trip factoryDirectory facts: captured=%#v imported=%#v, want /factories/alpha",
			capturedObject["factoryDirectory"],
			importedObject["factoryDirectory"],
		)
	}
	future, ok := importedObject["futureField"].(map[string]any)
	if !ok || future["enabled"] != true {
		t.Fatalf("round-trip futureField = %#v, want preserved unknown field", importedObject["futureField"])
	}
}

func TestSnapshotsPortability_MaterializeUnsafeInputsTypedFailure(t *testing.T) {
	t.Parallel()

	portability, err := snapshotsportabilitywire.NewService()
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	validSnapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"name":             "alpha",
		"factoryDirectory": "/factories/alpha",
		"resourceManifest": map[string]any{
			"bundledFiles": []any{
				map[string]any{"type": "DOC", "targetPath": "factory/docs/README.md"},
			},
		},
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}

	assertUnsafe := func(t *testing.T, label string, request factorydefinitions.MaterializeFactorySnapshotRequest) {
		t.Helper()
		_, unsafeErr := portability.MaterializeFactorySnapshot(context.Background(), request)
		if !errors.Is(unsafeErr, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize) {
			t.Fatalf(
				"%s error = %v, want ErrUnsafeFactorySnapshotMaterialize",
				label,
				unsafeErr,
			)
		}
		if errors.Is(unsafeErr, factorydefinitions.ErrInvalidFactorySnapshotPayload) {
			t.Fatalf("%s must not also match ErrInvalidFactorySnapshotPayload", label)
		}
	}

	assertUnsafe(t, "empty TargetDir", factorydefinitions.MaterializeFactorySnapshotRequest{
		TargetDir: "",
		Snapshot:  validSnapshot,
	})
	assertUnsafe(t, "nil Snapshot", factorydefinitions.MaterializeFactorySnapshotRequest{
		TargetDir: "/factories/alpha",
		Snapshot:  nil,
	})
	assertUnsafe(t, "path escape TargetDir", factorydefinitions.MaterializeFactorySnapshotRequest{
		TargetDir: "../outside",
		Snapshot:  validSnapshot,
	})
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

// assertCTRDEFRootSnapshotSuccessVocabulary exercises the published CTR-DEF
// root snapshot success cases against a peer-facing factorydefinitions.Service
// only — cross-service callers must not import snapshots_portability.
func assertCTRDEFRootSnapshotSuccessVocabulary(
	t *testing.T,
	service factorydefinitions.Service,
) {
	t.Helper()

	payload := []byte(`{
		"name": "alpha",
		"factoryDirectory": "/factories/alpha",
		"resourceManifest": {
			"bundledFiles": [
				{"type": "DOC", "targetPath": "factory/docs/README.md", "content": {"inline": "hello", "encoding": "utf-8"}}
			]
		}
	}`)

	captured, err := service.CaptureFactorySnapshot(
		context.Background(),
		factorydefinitions.CaptureFactorySnapshotRequest{
			FactoryDir: "/factories/alpha",
			Canonical:  payload,
			Name:       "alpha",
		},
	)
	if err != nil {
		t.Fatalf("CaptureFactorySnapshot: %v", err)
	}
	if captured.Snapshot == nil {
		t.Fatal("CaptureFactorySnapshot snapshot is nil")
	}
	var capturedObject map[string]any
	if decodeErr := captured.Snapshot.Decode(&capturedObject); decodeErr != nil {
		t.Fatalf("CaptureFactorySnapshot decode: %v", decodeErr)
	}
	if capturedObject["name"] != "alpha" {
		t.Fatalf("CaptureFactorySnapshot name = %#v, want alpha", capturedObject["name"])
	}

	imported, err := service.PrepareFactorySnapshotImport(
		context.Background(),
		factorydefinitions.PrepareFactorySnapshotImportRequest{Payload: payload},
	)
	if err != nil {
		t.Fatalf("PrepareFactorySnapshotImport: %v", err)
	}
	if imported.Snapshot == nil || imported.Name != "alpha" {
		t.Fatalf("PrepareFactorySnapshotImport result = %#v, want alpha snapshot facts", imported)
	}
	if imported.Portable.FactoryDir != "/factories/alpha" || len(imported.Portable.Assets) == 0 {
		t.Fatalf("PrepareFactorySnapshotImport portable = %#v, want portable success facts", imported.Portable)
	}

	materialized, err := service.MaterializeFactorySnapshot(
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

// assertCTRDEFRootSnapshotTypedFailureVocabulary exercises the published
// CTR-DEF root typed-failure cases against a peer-facing
// factorydefinitions.Service only.
func assertCTRDEFRootSnapshotTypedFailureVocabulary(
	t *testing.T,
	service factorydefinitions.Service,
) {
	t.Helper()

	_, invalidErr := service.PrepareFactorySnapshotImport(
		context.Background(),
		factorydefinitions.PrepareFactorySnapshotImportRequest{Payload: []byte(`["not-object"]`)},
	)
	if !errors.Is(invalidErr, factorydefinitions.ErrInvalidFactorySnapshotPayload) {
		t.Fatalf(
			"PrepareFactorySnapshotImport invalid-payload error = %v, want %v",
			invalidErr,
			factorydefinitions.ErrInvalidFactorySnapshotPayload,
		)
	}

	_, captureInvalidErr := service.CaptureFactorySnapshot(
		context.Background(),
		factorydefinitions.CaptureFactorySnapshotRequest{Canonical: []byte(`"string"`)},
	)
	if !errors.Is(captureInvalidErr, factorydefinitions.ErrInvalidFactorySnapshotPayload) {
		t.Fatalf(
			"CaptureFactorySnapshot invalid-payload error = %v, want %v",
			captureInvalidErr,
			factorydefinitions.ErrInvalidFactorySnapshotPayload,
		)
	}

	_, unsafeErr := service.MaterializeFactorySnapshot(
		context.Background(),
		factorydefinitions.MaterializeFactorySnapshotRequest{
			TargetDir: "../outside",
			Snapshot:  nil,
		},
	)
	if !errors.Is(unsafeErr, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize) {
		t.Fatalf(
			"MaterializeFactorySnapshot unsafe error = %v, want %v",
			unsafeErr,
			factorydefinitions.ErrUnsafeFactorySnapshotMaterialize,
		)
	}
	if errors.Is(unsafeErr, factorydefinitions.ErrInvalidFactorySnapshotPayload) {
		t.Fatal("unsafe materialize must not also match ErrInvalidFactorySnapshotPayload")
	}
}

func TestRootService_SnapshotSlice_CTRDEFEquivalenceThroughPrivateOwner(t *testing.T) {
	t.Parallel()

	portability, err := snapshotsportabilitywire.NewService()
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	// Peer-facing handle stays typed as the public Definitions root Service.
	// Construction may use the private owner; cross-service callers must not.
	var root factorydefinitions.Service = rootSnapshotFacade{portability: portability}

	assertCTRDEFRootSnapshotSuccessVocabulary(t, root)
	assertCTRDEFRootSnapshotTypedFailureVocabulary(t, root)
}

func TestSnapshotsPortabilityPublicSurfaceRejectsForbiddenOwnershipLeaks(t *testing.T) {
	t.Parallel()

	// Service contract is snapshot/portability-only: no catalog, authoring,
	// compilation, validation, or distribution ownership methods.
	serviceType := reflect.TypeOf((*snapshotsportability.Service)(nil)).Elem()
	// reflect reports interface methods in lexicographic order.
	wantMethods := []string{
		"CaptureFactorySnapshot",
		"MaterializeFactorySnapshot",
		"PrepareFactorySnapshotImport",
	}
	gotMethods := make([]string, 0, serviceType.NumMethod())
	for i := 0; i < serviceType.NumMethod(); i++ {
		method := serviceType.Method(i)
		gotMethods = append(gotMethods, method.Name)
		assertSnapshotsPortabilitySurfaceTypesAllowed(t, "Service."+method.Name, method.Type)
	}
	if !reflect.DeepEqual(gotMethods, wantMethods) {
		t.Fatalf("Service methods = %v, want portability-only %v", gotMethods, wantMethods)
	}
	for _, forbidden := range []string{
		"ListNamedFactories",
		"DeleteNamedFactory",
		"SetCurrentNamedFactoryPointer",
		"PrepareFactoryLayout",
		"FlattenFactoryLayout",
		"ExpandFactoryLayout",
		"CreateNamedFactory",
		"ReplaceNamedFactory",
		"CompileEffectiveFactorySource",
		"ValidateStructuralFactoryDefinition",
		"ValidateEffectiveFactoryDefinition",
		"InstallPackagedFactory",
		"CreateFactoryScaffold",
		"ListBuiltInPackagedFactories",
		"ActivateNamedFactory",
		"Save",
	} {
		if _, ok := serviceType.MethodByName(forbidden); ok {
			t.Fatalf("Service exposes out-of-lease method %q", forbidden)
		}
	}

	// Construction takes no peer collaborators, Runtime/Petri types, or
	// Wire/root composition ownership — detached snapshot ops need no host
	// effects beyond Definitions-owned request/result vocabulary.
	ctorType := reflect.TypeOf(snapshotsportabilitywire.NewService)
	if ctorType.NumIn() != 0 {
		t.Fatalf("NewService inputs = %v, want zero injected peer/Wire collaborators", ctorType)
	}
	if ctorType.NumOut() != 2 ||
		ctorType.Out(0) != serviceType ||
		ctorType.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
		t.Fatalf("NewService outputs = %v, want (Service, error)", ctorType)
	}

	// Observable runtime proof: construct and complete capture → prepare →
	// materialize with no peer/Wire/root collaborator injected.
	portability, err := snapshotsportabilitywire.NewService()
	if err != nil {
		t.Fatalf("NewService without peer/Wire collaborators: %v", err)
	}
	payload := []byte(`{
		"name": "surface",
		"factoryDirectory": "/factories/surface",
		"resourceManifest": {
			"bundledFiles": [
				{"type": "DOC", "targetPath": "factory/docs/README.md", "content": {"inline": "hello", "encoding": "utf-8"}}
			]
		}
	}`)
	captured, err := portability.CaptureFactorySnapshot(
		context.Background(),
		factorydefinitions.CaptureFactorySnapshotRequest{
			FactoryDir: "/factories/surface",
			Canonical:  payload,
			Name:       "surface",
		},
	)
	if err != nil {
		t.Fatalf("CaptureFactorySnapshot through portability-only surface: %v", err)
	}
	prepared, err := portability.PrepareFactorySnapshotImport(
		context.Background(),
		factorydefinitions.PrepareFactorySnapshotImportRequest{Payload: payload},
	)
	if err != nil {
		t.Fatalf("PrepareFactorySnapshotImport through portability-only surface: %v", err)
	}
	materialized, err := portability.MaterializeFactorySnapshot(
		context.Background(),
		factorydefinitions.MaterializeFactorySnapshotRequest{
			TargetDir: "/factories/surface",
			Snapshot:  captured.Snapshot,
		},
	)
	if err != nil {
		t.Fatalf("MaterializeFactorySnapshot through portability-only surface: %v", err)
	}
	if prepared.Name != "surface" ||
		prepared.Portable.FactoryDir != "/factories/surface" ||
		len(prepared.Portable.Assets) == 0 ||
		materialized.Portable.FactoryDir != "/factories/surface" ||
		len(materialized.Portable.Assets) == 0 {
		t.Fatalf(
			"portability-only round trip prepared=%#v materialized=%#v",
			prepared,
			materialized,
		)
	}
}

func assertSnapshotsPortabilitySurfaceTypesAllowed(t *testing.T, path string, typ reflect.Type) {
	t.Helper()
	switch typ.Kind() {
	case reflect.Func:
		for i := 0; i < typ.NumIn(); i++ {
			assertSnapshotsPortabilitySurfaceTypesAllowed(t, path+".in", typ.In(i))
		}
		for i := 0; i < typ.NumOut(); i++ {
			assertSnapshotsPortabilitySurfaceTypesAllowed(t, path+".out", typ.Out(i))
		}
		return
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Chan, reflect.Map:
		if typ.Kind() == reflect.Map {
			assertSnapshotsPortabilitySurfaceTypesAllowed(t, path+".key", typ.Key())
		}
		assertSnapshotsPortabilitySurfaceTypesAllowed(t, path+".elem", typ.Elem())
		return
	case reflect.Interface:
		if typ == reflect.TypeOf((*error)(nil)).Elem() ||
			typ == reflect.TypeOf((*context.Context)(nil)).Elem() {
			return
		}
		if snapshotsPortabilitySurfacePackageAllowed(typ.PkgPath()) {
			return
		}
		t.Fatalf("%s exposes interface type %v outside Definitions-owned portability vocabulary", path, typ)
		return
	}

	pkg := typ.PkgPath()
	if snapshotsPortabilitySurfacePackageAllowed(pkg) {
		// Definitions-owned value types are opaque here; do not walk nested fields.
		return
	}
	for _, forbiddenPrefix := range []string{
		"github.com/portpowered/infinite-you/pkg/services/factory_runtime",
		"github.com/portpowered/infinite-you/pkg/services/factory_sessions",
		"github.com/portpowered/infinite-you/pkg/services/workers",
		"github.com/portpowered/infinite-you/pkg/services/automations",
		"github.com/portpowered/infinite-you/pkg/services/recordings",
		"github.com/portpowered/infinite-you/pkg/wire",
		"github.com/portpowered/infinite-you/pkg/root",
	} {
		if pkg == forbiddenPrefix || strings.HasPrefix(pkg, forbiddenPrefix+"/") {
			t.Fatalf("%s exposes type %v from forbidden ownership package %q", path, typ, pkg)
		}
	}
	t.Fatalf("%s exposes type %v from non-Definitions package %q", path, typ, pkg)
}

func snapshotsPortabilitySurfacePackageAllowed(pkg string) bool {
	const definitionsRoot = "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	const definitionsContracts = definitionsRoot + "/contracts"
	const snapshotsPortabilityPkg = definitionsRoot + "/internal/services/snapshots_portability"
	switch {
	case pkg == "", pkg == "context", pkg == definitionsRoot, pkg == snapshotsPortabilityPkg:
		return true
	case pkg == definitionsContracts, strings.HasPrefix(pkg, definitionsContracts+"/"):
		return true
	default:
		return false
	}
}
