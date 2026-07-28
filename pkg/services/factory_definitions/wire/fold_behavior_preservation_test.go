package wire_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/pkg/platform/directoryreplace"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/inboxgitkeep"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionsinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
	factorydefinitiontestcomposition "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/testcomposition"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

// Fold-behavior preservation tests construct Factory Definitions exclusively
// through factory_definitions/wire and exercise catalog, authoring, validate,
// snapshot/portability, and distribute surfaces on the published Service root
// after the internal composed-root relocation.

func TestWireFoldPreservesCatalogThroughPublishedRoot(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	alphaDir := writeFoldPreservationNamedFactory(t, rootDir, "alpha")
	service := newWireFoldPreservationService(t)
	ctx := context.Background()

	listed, err := service.ListNamedFactories(
		ctx,
		factorydefinitions.ListNamedFactoriesRequest{RootDir: rootDir},
	)
	if err != nil {
		t.Fatalf("ListNamedFactories() error = %v", err)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].Name != "alpha" {
		t.Fatalf("ListNamedFactories() = %#v, want one alpha entry", listed.Entries)
	}

	got, err := service.GetNamedFactory(
		ctx,
		factorydefinitions.GetNamedFactoryRequest{RootDir: rootDir, Name: "alpha"},
	)
	if err != nil {
		t.Fatalf("GetNamedFactory(alpha) error = %v", err)
	}
	if got.Entry.FactoryDir != alphaDir {
		t.Fatalf("GetNamedFactory() factoryDir = %q, want %q", got.Entry.FactoryDir, alphaDir)
	}

	_, invalidErr := service.GetNamedFactory(
		ctx,
		factorydefinitions.GetNamedFactoryRequest{RootDir: rootDir, Name: "../evil"},
	)
	if !errors.Is(invalidErr, factorydefinitions.ErrInvalidNamedFactoryName) {
		t.Fatalf(
			"GetNamedFactory(invalid-name) error = %v, want %v",
			invalidErr,
			factorydefinitions.ErrInvalidNamedFactoryName,
		)
	}

	_, missingErr := service.GetNamedFactory(
		ctx,
		factorydefinitions.GetNamedFactoryRequest{RootDir: rootDir, Name: "missing"},
	)
	if !errors.Is(missingErr, factorydefinitions.ErrNamedFactoryNotFound) {
		t.Fatalf(
			"GetNamedFactory(missing) error = %v, want %v",
			missingErr,
			factorydefinitions.ErrNamedFactoryNotFound,
		)
	}
	if errors.Is(missingErr, factorydefinitions.ErrInvalidNamedFactoryName) {
		t.Fatal("missing named factory must not also match invalid-name sentinel")
	}
}

func TestWireFoldPreservesAuthoringThroughPublishedRoot(t *testing.T) {
	t.Parallel()

	service := newWireFoldPreservationService(t)
	ctx := context.Background()
	payload := []byte(factoryfixtures.CrossPathValidAlphaFactoryJSON)

	prepared, err := service.PrepareFactoryLayout(
		ctx,
		factorydefinitions.PrepareFactoryLayoutRequest{Name: "alpha", Payload: payload},
	)
	if err != nil {
		t.Fatalf("PrepareFactoryLayout(valid) error = %v", err)
	}
	if len(prepared.Prepared.Canonical) == 0 {
		t.Fatal("PrepareFactoryLayout() returned empty canonical payload")
	}

	_, malformedErr := service.PrepareFactoryLayout(
		ctx,
		factorydefinitions.PrepareFactoryLayoutRequest{Name: "alpha", Payload: []byte("{")},
	)
	if !errors.Is(malformedErr, factorydefinitions.ErrInvalidNamedFactory) {
		t.Fatalf(
			"PrepareFactoryLayout(malformed) error = %v, want %v",
			malformedErr,
			factorydefinitions.ErrInvalidNamedFactory,
		)
	}
}

func TestWireFoldPreservesValidateThroughPublishedRoot(t *testing.T) {
	t.Parallel()

	service := newWireFoldPreservationService(t)
	ctx := context.Background()
	validPayload := []byte(factoryfixtures.CrossPathValidAlphaFactoryJSON)
	validEffectivePayload := foldPreservationEffectivePayload(t)

	structural, err := service.ValidateStructuralFactoryDefinition(
		ctx,
		factorydefinitions.ValidateStructuralFactoryDefinitionRequest{
			Canonical: validPayload,
			Profile:   factorydefinitions.ValidationProfileTopology,
		},
	)
	if err != nil {
		t.Fatalf("ValidateStructuralFactoryDefinition(valid) error = %v", err)
	}
	if structural.Validation.HasBlockingTargets() {
		t.Fatalf(
			"ValidateStructuralFactoryDefinition(valid) findings = %#v, want none",
			structural.Validation,
		)
	}

	effective, err := service.ValidateEffectiveFactoryDefinition(
		ctx,
		factorydefinitions.ValidateEffectiveFactoryDefinitionRequest{
			Canonical: validEffectivePayload,
			Effective: factorydefinitions.EffectiveFactorySource{
				FactoryDir:      "/factories/alpha",
				RuntimeBaseDir:  "/factories/alpha",
				ContentIdentity: string(validEffectivePayload),
			},
		},
	)
	if err != nil {
		t.Fatalf("ValidateEffectiveFactoryDefinition(valid) error = %v", err)
	}
	if effective.Validation.HasBlockingTargets() {
		t.Fatalf(
			"ValidateEffectiveFactoryDefinition(valid) findings = %#v, want none",
			effective.Validation,
		)
	}

	_, invalidErr := service.ValidateStructuralFactoryDefinition(
		ctx,
		factorydefinitions.ValidateStructuralFactoryDefinitionRequest{Canonical: []byte("{")},
	)
	if !errors.Is(invalidErr, factorydefinitions.ErrInvalidFactoryDefinitionPayload) {
		t.Fatalf(
			"ValidateStructuralFactoryDefinition(invalid-payload) error = %v, want %v",
			invalidErr,
			factorydefinitions.ErrInvalidFactoryDefinitionPayload,
		)
	}
}

func TestWireFoldPreservesSnapshotPortabilityThroughPublishedRoot(t *testing.T) {
	t.Parallel()

	service := newWireFoldPreservationService(t)
	ctx := context.Background()
	validCanonical := []byte(factoryfixtures.CrossPathValidAlphaFactoryJSON)
	factoryDir := "/factories/alpha"

	captured, err := service.CaptureFactorySnapshot(
		ctx,
		factorydefinitions.CaptureFactorySnapshotRequest{
			FactoryDir: factoryDir,
			Canonical:  validCanonical,
			Name:       "alpha",
		},
	)
	if err != nil {
		t.Fatalf("CaptureFactorySnapshot(valid) error = %v", err)
	}
	if captured.Snapshot == nil {
		t.Fatal("CaptureFactorySnapshot() snapshot is nil")
	}

	targetDir := t.TempDir()
	materialized, err := service.MaterializeFactorySnapshot(
		ctx,
		factorydefinitions.MaterializeFactorySnapshotRequest{
			TargetDir: targetDir,
			Snapshot:  captured.Snapshot,
		},
	)
	if err != nil {
		t.Fatalf("MaterializeFactorySnapshot(valid) error = %v", err)
	}
	if materialized.TargetDir != targetDir || materialized.Portable.FactoryDir == "" {
		t.Fatalf("MaterializeFactorySnapshot() = %#v, want portable success facts", materialized)
	}

	_, invalidErr := service.PrepareFactorySnapshotImport(
		ctx,
		factorydefinitions.PrepareFactorySnapshotImportRequest{Payload: []byte(`["not-object"]`)},
	)
	if !errors.Is(invalidErr, factorydefinitions.ErrInvalidFactorySnapshotPayload) {
		t.Fatalf(
			"PrepareFactorySnapshotImport(invalid) error = %v, want %v",
			invalidErr,
			factorydefinitions.ErrInvalidFactorySnapshotPayload,
		)
	}

	_, unsafeErr := service.MaterializeFactorySnapshot(
		ctx,
		factorydefinitions.MaterializeFactorySnapshotRequest{},
	)
	if !errors.Is(unsafeErr, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize) {
		t.Fatalf(
			"MaterializeFactorySnapshot(unsafe) error = %v, want %v",
			unsafeErr,
			factorydefinitions.ErrUnsafeFactorySnapshotMaterialize,
		)
	}
}

func TestWireFoldPreservesDistributeThroughPublishedRoot(t *testing.T) {
	t.Parallel()

	goalJSON, err := json.Marshal(map[string]string{"name": "goal", "project": "builtin-goal"})
	if err != nil {
		t.Fatalf("marshal goal factory: %v", err)
	}
	packagedCatalog, err := factorydefinitionsinternal.NewPackagedFactoryCatalog([]factorydefinitions.PackagedDefinition{{
		Name:    "@you/goal",
		Project: "builtin-goal",
		JSON:    goalJSON,
		Formats: []factorydefinitions.PackagedFactoryFormat{
			factorydefinitions.PackagedFactoryFormatJSON,
		},
	}})
	if err != nil {
		t.Fatalf("NewPackagedFactoryCatalog() error = %v", err)
	}

	service := newWireFoldPreservationService(t, withPackagedCatalog(packagedCatalog))
	ctx := context.Background()

	listed, err := service.ListBuiltInPackagedFactories(
		ctx,
		factorydefinitions.ListBuiltInPackagedFactoriesRequest{},
	)
	if err != nil {
		t.Fatalf("ListBuiltInPackagedFactories() error = %v", err)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].Name != "@you/goal" {
		t.Fatalf("ListBuiltInPackagedFactories() = %#v, want @you/goal entry", listed.Entries)
	}

	installed, err := service.InstallPackagedFactory(
		ctx,
		factorydefinitions.InstallPackagedFactoryRequest{
			RootDir: t.TempDir(),
			Name:    "@you/goal",
		},
	)
	if err != nil {
		t.Fatalf("InstallPackagedFactory() error = %v", err)
	}
	if installed.Definition.Name == "" || installed.Definition.FactoryDir == "" {
		t.Fatalf("InstallPackagedFactory() = %#v, want populated aggregate facts", installed.Definition)
	}

	_, unknownErr := service.InstallPackagedFactory(
		ctx,
		factorydefinitions.InstallPackagedFactoryRequest{
			RootDir: t.TempDir(),
			Name:    "@you/missing",
		},
	)
	if !errors.Is(unknownErr, factorydefinitions.ErrUnknownPackagedFactoryIdentity) {
		t.Fatalf(
			"InstallPackagedFactory(missing) error = %v, want %v",
			unknownErr,
			factorydefinitions.ErrUnknownPackagedFactoryIdentity,
		)
	}
}

type foldPreservationOption func(*foldPreservationPorts)

func withPackagedCatalog(
	catalog factorydefinitions.PackagedFactoryCatalogOperations,
) foldPreservationOption {
	return func(ports *foldPreservationPorts) {
		ports.packagedCatalog = catalog
	}
}

type foldPreservationPorts struct {
	packagedCatalog factorydefinitions.PackagedFactoryCatalogOperations
}

func newWireFoldPreservationService(t *testing.T, options ...foldPreservationOption) factorydefinitions.Service {
	t.Helper()

	ports := foldPreservationPorts{}
	for _, option := range options {
		option(&ports)
	}

	composition := newFoldPreservationComposition()
	validator := factoryvalidation.New(nil)
	mapInput := func(payload []byte) (factorydefinitions.DefinitionValidationRequest, error) {
		return validationentry.MapFactoryJSONForPersistence(payload, composition.LoadCanonicalJSON)
	}
	persistence := composition.Persistence(validator, mapInput)
	loader := composition.Loader()
	namedPaths := composition.NamedPaths()
	fileSystem := platformfilesystem.Local{}

	packagedDefinitions := []factorydefinitions.PackagedDefinition{{
		Name:    "@you/fold-preservation",
		Project: "fold-preservation",
		JSON:    []byte(factoryfixtures.CrossPathValidAlphaFactoryJSON),
		Formats: []factorydefinitions.PackagedFactoryFormat{
			factorydefinitions.PackagedFactoryFormatJSON,
		},
	}}
	packagedCatalog := ports.packagedCatalog
	if packagedCatalog.List == nil || packagedCatalog.Resolve == nil {
		var err error
		packagedCatalog, err = factorydefinitionsinternal.NewPackagedFactoryCatalog(packagedDefinitions)
		if err != nil {
			t.Fatalf("NewPackagedFactoryCatalog() error = %v", err)
		}
	}

	discovery, err := factorydefinitionsinternal.NewEffectiveCatalogDiscovery(
		composition.NamedFactoryCatalog().ListNamedFactories,
		os.ReadFile,
		packagedDefinitions,
	)
	if err != nil {
		t.Fatalf("NewEffectiveCatalogDiscovery() error = %v", err)
	}
	listEffective, err := factorydefinitionsinternal.NewEffectiveCatalog(
		discovery,
		factorydefinitionswire.EffectiveFactoryDefinitionNormalizerFromMapper(),
	)
	if err != nil {
		t.Fatalf("NewEffectiveCatalog() error = %v", err)
	}

	applySupportedFiles, applyStarterWork, _ := factorydefinitiontestcomposition.PortableOperations(fileSystem)

	service, err := factorydefinitionswire.NewService(
		stubSessionHost{},
		wireStubActivationGateway{},
		validator,
		persistence,
		loader,
		applySupportedFiles,
		applyStarterWork,
		namedPaths,
		fileSystem,
		factorydefinitionswire.StaticClock(time.Unix(0, 0)),
		fileSystem,
		listEffective,
		packagedCatalog,
		factorydefinitions.PackagedFactoryInstallationOperations{
			Install: func(
				_ context.Context,
				params factorydefinitions.PackagedFactoryInstallParams,
			) (factorydefinitions.PackagedFactoryInstallResult, error) {
				factoryDir := filepath.Join(
					strings.TrimSpace(params.NamedFactoriesRoot),
					strings.TrimPrefix(params.Definition.Name, "@you/"),
				)
				return factorydefinitions.PackagedFactoryInstallResult{
					Name:       params.Definition.Name,
					FactoryDir: factoryDir,
					Outcome:    factorydefinitions.PackagedFactoryInstallCreated,
					Format:     params.Format,
				}, nil
			},
		},
		stubRequiredToolChecker{},
		stubOrchestratorValidator{},
		fileSystem,
		directoryreplace.Local{},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var root factorydefinitions.Service = service
	if root == nil {
		t.Fatal("constructed value is not assignable to factorydefinitions.Service")
	}
	return root
}

func newFoldPreservationComposition() factorydefinitiontestcomposition.Composition {
	mapper := factorymapping.NewFactoryConfigMapper()
	fileSystem := platformfilesystem.Local{}
	var composition factorydefinitiontestcomposition.Composition
	composition = factorydefinitiontestcomposition.New(factorydefinitiontestcomposition.Representation{
		DecodeFactory:     factorymapping.ExpandFactoryConfigForRuntimeLoad,
		DecodeAuthored:    mapper.Expand,
		EncodeFactory:     factorymapping.MarshalCanonicalFactoryConfig,
		NormalizeAuthored: authoredmapping.AuthoredFactoryConfigForExpandedLayout,
		ParseWorker:       authoredmapping.ParseWorkerConfig,
		ParseWorkstation:  authoredmapping.ParseWorkstationConfig,
		ParseBody:         authoredmapping.ParseAgentsBody,
		RenderWorker:      authoredmapping.RenderWorkerAgentsMarkdown,
		RenderWorkstation: authoredmapping.RenderWorkstationAgentsMarkdown,
		RenderBody:        authoredmapping.RenderAgentsBody,
		SafeLayoutSegment: authoredmapping.SafeFactoryLayoutSegment,
		SafePromptPath:    authoredmapping.SafePromptFilePath,
		MapPersistence: func(payload []byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return validationentry.MapFactoryJSONForPersistence(payload, composition.LoadCanonicalJSON)
		},
	}, fileSystem, directoryreplace.Local{}, factorydefinitiontestcomposition.Effects{
		Loading:             fileSystem,
		AuthoredReader:      fileSystem,
		AuthoredWriter:      fileSystem,
		Persistence:         fileSystem,
		NamedPaths:          fileSystem,
		NamedFactoryCatalog: fileSystem,
		InboxSentinels:      inboxgitkeep.NewLocal(fileSystem),
	})
	return composition
}

func foldPreservationEffectivePayload(t *testing.T) []byte {
	t.Helper()

	factory, err := factoryfixtures.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}
	if factory.WorkTypes == nil || len(*factory.WorkTypes) < 1 {
		t.Fatal("expected alpha fixture work types")
	}
	defaultBehavior := factoryapi.WorkTypeHandlingBehaviorDefault
	(*factory.WorkTypes)[0].HandlingBehavior = &[]factoryapi.WorkTypeHandlingBehavior{defaultBehavior}

	payload, err := json.Marshal(factory)
	if err != nil {
		t.Fatalf("Marshal(alpha factory): %v", err)
	}
	return payload
}

func writeFoldPreservationNamedFactory(t *testing.T, rootDir, name string) string {
	t.Helper()

	factoryDir := filepath.Join(rootDir, filepath.FromSlash(name))
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", name, err)
	}
	if err := os.WriteFile(
		filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile),
		[]byte(`{}`),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(%s/%s): %v", name, factorydefinitions.FactoryConfigFile, err)
	}
	return factoryDir
}
