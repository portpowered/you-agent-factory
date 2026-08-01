package wire_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/validationassert"
	"github.com/portpowered/infinite-you/pkg/platform/directoryreplace"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/inboxgitkeep"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionsinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
	factorydefinitiontestcomposition "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/testcomposition"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
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

func TestWireFoldPreservesCatalogResolveDeleteAndCurrentPointerThroughPublishedRoot(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	globalRoot := t.TempDir()
	alphaDir := writeFoldPreservationNamedFactory(t, projectRoot, "alpha")
	_ = writeFoldPreservationNamedFactory(t, projectRoot, "beta")
	_ = writeFoldPreservationNamedFactory(t, globalRoot, "alpha")
	service := newWireFoldPreservationService(t)
	ctx := context.Background()

	resolved, err := service.ResolveNamedFactory(
		ctx,
		factorydefinitions.ResolveNamedFactoryRequest{
			ProjectRoot: projectRoot,
			GlobalRoot:  globalRoot,
			Name:        "alpha",
		},
	)
	if err != nil {
		t.Fatalf("ResolveNamedFactory(alpha) error = %v", err)
	}
	if resolved.Resolution.FactoryDir != alphaDir ||
		resolved.Resolution.Source != factorydefinitions.NamedFactoryResolutionSourceProjectLocal {
		t.Fatalf("ResolveNamedFactory() = %#v, want project-local alpha at %q", resolved.Resolution, alphaDir)
	}

	if _, err := service.SetCurrentFactoryPointer(
		ctx,
		factorydefinitions.SetCurrentFactoryPointerRequest{RootDir: projectRoot, Name: "beta"},
	); err != nil {
		t.Fatalf("SetCurrentFactoryPointer(beta) error = %v", err)
	}
	pointer, err := service.GetCurrentFactoryPointer(
		ctx,
		factorydefinitions.GetCurrentFactoryPointerRequest{RootDir: projectRoot},
	)
	if err != nil {
		t.Fatalf("GetCurrentFactoryPointer() error = %v", err)
	}
	if pointer.Name != "beta" {
		t.Fatalf("GetCurrentFactoryPointer() = %#v, want beta current pointer", pointer)
	}

	emptyRoot := t.TempDir()
	_, missingPointerErr := service.GetCurrentFactoryPointer(
		ctx,
		factorydefinitions.GetCurrentFactoryPointerRequest{RootDir: emptyRoot},
	)
	if !errors.Is(missingPointerErr, factorydefinitions.ErrCurrentFactoryNotFound) {
		t.Fatalf(
			"GetCurrentFactoryPointer(missing) error = %v, want %v",
			missingPointerErr,
			factorydefinitions.ErrCurrentFactoryNotFound,
		)
	}

	deleted, err := service.DeleteNamedFactory(
		ctx,
		factorydefinitions.DeleteNamedFactoryRequest{RootDir: projectRoot, Name: "alpha"},
	)
	if err != nil {
		t.Fatalf("DeleteNamedFactory(alpha) error = %v", err)
	}
	if deleted.FactoryDir != alphaDir {
		t.Fatalf("DeleteNamedFactory() factoryDir = %q, want %q", deleted.FactoryDir, alphaDir)
	}

	listed, err := service.ListNamedFactories(
		ctx,
		factorydefinitions.ListNamedFactoriesRequest{RootDir: projectRoot},
	)
	if err != nil {
		t.Fatalf("ListNamedFactories() after delete error = %v", err)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].Name != "beta" {
		t.Fatalf("ListNamedFactories() after delete = %#v, want only beta", listed.Entries)
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

func TestWireFoldPreservesWorkerWorkstationCompatibilityThroughPublishedRoot(t *testing.T) {
	t.Parallel()

	service := newWireFoldPreservationService(t)
	ctx := context.Background()
	payload := []byte(`{
		"name":"alpha",
		"workTypes":[{"name":"story","states":[
			{"name":"queued","type":"INITIAL"},
			{"name":"done","type":"TERMINAL"},
			{"name":"failed","type":"FAILED"}
		]}],
		"workers":[{"name":"worker-a","type":"AGENT_WORKER"}],
		"workstations":[{
			"name":"process",
			"type":"INFERENCE_RUN",
			"worker":"worker-a",
			"operation":"TTS",
			"inputs":[{"workType":"story","state":"queued"}],
			"outputs":[{"workType":"story","state":"done"}],
			"onFailure":[{"workType":"story","state":"failed"}]
		}]
	}`)

	_, err := service.ValidateStructuralFactoryDefinition(
		ctx,
		factorydefinitions.ValidateStructuralFactoryDefinitionRequest{
			Canonical: payload,
			Profile:   factorydefinitions.ValidationProfileTopology,
		},
	)
	assertWireFoldValidationFailure(
		t,
		err,
		factorydefinitions.ValidationCodeWorkerWorkstationBehaviorCompatibility,
	)
}

func TestWireFoldPreservesRequiredToolsThroughPublishedRoot(t *testing.T) {
	t.Parallel()

	service := newWireFoldPreservationService(t, withRequiredToolChecker(foldRequiredToolChecker{
		"missing-tool": {
			FailureKind: factorydefinitions.RequiredToolFailureKindMissing,
			Err:         errors.New(`required tool "Portable helper" command "missing-tool" was not found on PATH`),
		},
	}))
	ctx := context.Background()
	payload := []byte(`{
		"name":"alpha",
		"workTypes":[{"name":"story","states":[
			{"name":"queued","type":"INITIAL"},
			{"name":"done","type":"TERMINAL"},
			{"name":"failed","type":"FAILED"}
		]}],
		"workers":[{"name":"worker-a"}],
		"workstations":[{
			"name":"process",
			"worker":"worker-a",
			"inputs":[{"workType":"story","state":"queued"}],
			"outputs":[{"workType":"story","state":"done"}],
			"onFailure":[{"workType":"story","state":"failed"}]
		}],
		"supportingFiles":{"requiredTools":[{"name":"Portable helper","command":"missing-tool"}]}
	}`)

	_, err := service.ValidateStructuralFactoryDefinition(
		ctx,
		factorydefinitions.ValidateStructuralFactoryDefinitionRequest{
			Canonical: payload,
			Profile:   factorydefinitions.ValidationProfileTopology,
		},
	)
	assertWireFoldValidationFailure(t, err, "factory.requiredTool.missing")
}

func TestWireFoldPreservesEffectiveValidationTypedFailureThroughPublishedRoot(t *testing.T) {
	t.Parallel()

	service := newWireFoldPreservationService(t)
	ctx := context.Background()
	payload := []byte(factoryfixtures.CrossPathValidAlphaFactoryJSON)

	_, err := service.ValidateEffectiveFactoryDefinition(
		ctx,
		factorydefinitions.ValidateEffectiveFactoryDefinitionRequest{
			Canonical: payload,
			Effective: factorydefinitions.EffectiveFactorySource{
				FactoryDir:      "/factories/alpha",
				RuntimeBaseDir:  "/factories/alpha",
				ContentIdentity: string(payload),
			},
		},
	)
	assertWireFoldValidationFailure(
		t,
		err,
		"work-type-handling-behavior-required-default",
	)
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
	packagedCatalog     factorydefinitions.PackagedFactoryCatalogOperations
	requiredToolChecker factorydefinitions.RequiredToolChecker
}

func withRequiredToolChecker(
	checker factorydefinitions.RequiredToolChecker,
) foldPreservationOption {
	return func(ports *foldPreservationPorts) {
		ports.requiredToolChecker = checker
	}
}

type foldRequiredToolChecker map[string]factorydefinitions.RequiredToolCheckResult

func (c foldRequiredToolChecker) Check(
	tool factorydefinitions.RequiredToolConfig,
) factorydefinitions.RequiredToolCheckResult {
	if result, ok := c[tool.Command]; ok {
		return result
	}
	return factorydefinitions.RequiredToolCheckResult{}
}

func newWireFoldPreservationService(t *testing.T, options ...foldPreservationOption) factorydefinitions.Service {
	t.Helper()

	ports := foldPreservationPorts{}
	for _, option := range options {
		option(&ports)
	}

	composition := newFoldPreservationComposition()
	validator := factorydefinitionswire.NewValidationOperations(nil)
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

	requiredToolChecker := ports.requiredToolChecker
	if requiredToolChecker == nil {
		requiredToolChecker = stubRequiredToolChecker{}
	}

	service, err := factorydefinitionswire.NewService(factorydefinitionswire.Dependencies{
		Validator:                     validator,
		DefinitionValidation:          validator,
		EffectiveDefinitionValidation: validator,
		Loader:                        loader,
		ApplySupportedFiles:           applySupportedFiles,
		ApplyStarterWork:              applyStarterWork,
		NamedPaths:                    namedPaths,
		NamedFactoryCatalogFileSystem: fileSystem,
		ListEffective:                 listEffective,
		PackagedCatalog:               packagedCatalog,
		PackagedInstaller: factorydefinitions.PackagedFactoryInstallationOperations{
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
		RequiredToolChecker:       requiredToolChecker,
		OrchestratorValidator:     stubOrchestratorValidator{},
		PortableFileSystem:        fileSystem,
		DirectoryReplacementStore: directoryreplace.Local{},
		Representation:            testRepresentation(),
		MapFactoryJSONForPersistence: func(payload []byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return validationentry.MapFactoryJSONForPersistence(payload, loader.LoadSourceFromCanonicalJSON)
		},
	})
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

func assertWireFoldValidationFailure(t *testing.T, err error, wantCodes ...string) {
	t.Helper()

	var validationFailure *factorydefinitions.FactoryDefinitionValidationFailure
	if !errors.As(err, &validationFailure) {
		t.Fatalf("error = %v, want FactoryDefinitionValidationFailure", err)
	}
	if !errors.Is(err, factorydefinitions.ErrFactoryDefinitionValidationFailed) {
		t.Fatalf("error = %v, want %v", err, factorydefinitions.ErrFactoryDefinitionValidationFailed)
	}
	if errors.Is(err, factorydefinitions.ErrInvalidFactoryDefinitionPayload) {
		t.Fatal("validation findings must not also match ErrInvalidFactoryDefinitionPayload")
	}
	if len(validationFailure.Validation.Targets) == 0 {
		t.Fatal("FactoryDefinitionValidationFailure must carry validation targets")
	}

	hasErrorFinding := false
	for _, target := range validationFailure.Validation.Targets {
		if target.Severity == factorydefinitions.ValidationSeverityError {
			hasErrorFinding = true
		}
		if strings.Contains(strings.ToLower(target.Code), "petri") ||
			strings.Contains(strings.ToLower(target.Message), "petri") {
			t.Fatalf("published validation findings must not use Petri vocabulary: %#v", target)
		}
	}
	if !hasErrorFinding {
		t.Fatal("FactoryDefinitionValidationFailure must carry at least one error-severity finding")
	}
	for _, code := range wantCodes {
		validationassert.HasDomainTargetCode(t, validationFailure.Validation.Targets, code)
	}
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
