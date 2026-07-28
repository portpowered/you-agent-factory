package wire_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
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
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

// Wire lifecycle behavior proofs construct Factory Definitions exclusively
// through factory_definitions/wire and exercise Activate, Save, GetCurrent*,
// and version surfaces on the published Service root after the internal
// lifecycle-host fold.

func TestWireLifecycleBehavior_ActivateNamedFactorySuccessAndIdleRejection(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	composition := newWireLifecycleComposition()
	validator := factoryvalidation.New(nil)
	if _, err := composition.PersistNamedFactory(
		rootDir,
		"named-target",
		wireLifecycleNamedFactoryPayload(t, "named-target"),
		validator,
	); err != nil {
		t.Fatalf("PersistNamedFactory(named-target): %v", err)
	}

	session := &factorydefinitions.DefinitionSession{
		ID:         "session-alpha",
		FactoryDir: rootDir,
		FolderPath: rootDir,
	}
	gateway := &wireLifecycleTrackingGateway{
		runSessionID:         "session-alpha",
		sessionForActivation: session,
		persistRoot:          rootDir,
		folderPath:           rootDir,
		idleNamedErr:         errors.New("runtime busy"),
	}
	service := newWireLifecycleBehaviorService(
		t,
		withWireLifecycleSessionHost(wireLifecycleSessionHost{persistRootDir: rootDir}),
		withWireLifecycleActivationGateway(gateway),
	)
	ctx := context.Background()

	if err := service.ActivateNamedFactory(ctx, "named-target"); err == nil {
		t.Fatal("ActivateNamedFactory() error = nil, want idle rejection")
	}
	if gateway.swapCalls.Load() != 0 {
		t.Fatalf("SwapPersistedNamedFactoryRuntime calls = %d, want 0 before idle gate passes", gateway.swapCalls.Load())
	}

	gateway.idleNamedErr = nil
	if err := service.ActivateNamedFactory(ctx, "named-target"); err != nil {
		t.Fatalf("ActivateNamedFactory after idle cleared: %v", err)
	}
	if gateway.swapCalls.Load() != 1 {
		t.Fatalf("SwapPersistedNamedFactoryRuntime calls = %d, want 1", gateway.swapCalls.Load())
	}
	if gateway.swappedName != "named-target" {
		t.Fatalf("swapped name = %q, want named-target", gateway.swappedName)
	}
}

func TestWireLifecycleBehavior_GetCurrentNamedFactorySuccessAndNotFound(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	composition := newWireLifecycleComposition()
	validator := factoryvalidation.New(nil)
	namedPaths := composition.NamedPaths()
	if _, err := composition.PersistNamedFactory(
		rootDir,
		"alpha",
		wireLifecycleNamedFactoryPayload(t, "alpha"),
		validator,
	); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := namedPaths.WriteCurrentPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentPointer(alpha): %v", err)
	}

	service := newWireLifecycleBehaviorService(
		t,
		withWireLifecycleSessionHost(wireLifecycleSessionHost{persistRootDir: rootDir}),
	)
	ctx := context.Background()

	current, err := service.GetCurrentNamedFactory(ctx)
	if err != nil {
		t.Fatalf("GetCurrentNamedFactory() error = %v", err)
	}
	mapped, err := wireLifecycleFactorySnapshotToAPI(current)
	if err != nil {
		t.Fatalf("map GetCurrentNamedFactory result: %v", err)
	}
	if mapped.Name != "alpha" {
		t.Fatalf("current factory name = %q, want alpha", mapped.Name)
	}
	if mapped.Id == nil || *mapped.Id != "alpha" {
		t.Fatalf("current factory id = %#v, want alpha", mapped.Id)
	}

	emptyRoot := t.TempDir()
	emptyService := newWireLifecycleBehaviorService(
		t,
		withWireLifecycleSessionHost(wireLifecycleSessionHost{persistRootDir: emptyRoot}),
	)
	_, notFoundErr := emptyService.GetCurrentNamedFactory(ctx)
	if !errors.Is(notFoundErr, factorydefinitions.ErrCurrentFactoryNotFound) {
		t.Fatalf(
			"GetCurrentNamedFactory(empty) error = %v, want %v",
			notFoundErr,
			factorydefinitions.ErrCurrentFactoryNotFound,
		)
	}
}

func TestWireLifecycleBehavior_GetCurrentFactoryForSessionIncludesVersion(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	composition := newWireLifecycleComposition()
	validator := factoryvalidation.New(nil)
	namedPaths := composition.NamedPaths()
	versionTime := time.Date(2026, time.June, 1, 10, 0, 0, 0, time.UTC)
	payload := wireLifecycleNamedFactoryPayload(t, "alpha")
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal payload: %v", err)
	}
	decoded["version"] = map[string]any{
		"logical":  float64(17),
		"physical": versionTime.Format(time.RFC3339Nano),
	}
	versioned, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("Marshal versioned payload: %v", err)
	}
	if _, err := composition.PersistNamedFactory(rootDir, "alpha", versioned, validator); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := namedPaths.WriteCurrentPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentPointer(alpha): %v", err)
	}

	factoryDir, err := namedPaths.ResolveExistingDir(rootDir, "alpha")
	if err != nil {
		t.Fatalf("ResolveExistingDir(alpha): %v", err)
	}
	runtimeCfg, err := composition.Loader().LoadRuntimeSource(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeSource(alpha): %v", err)
	}

	const sessionID = "session-alpha"
	service := newWireLifecycleBehaviorService(
		t,
		withWireLifecycleSessionHost(wireLifecycleSessionHost{
			persistRootDir: rootDir,
			session: &factorydefinitions.DefinitionSession{
				ID:         sessionID,
				FactoryDir: rootDir,
				FolderPath: rootDir,
				IsDefault:  true,
			},
			sessionRuntime:     runtimeCfg,
			sessionPersistRoot: rootDir,
		}),
	)

	got, err := service.GetCurrentFactoryForSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetCurrentFactoryForSession() error = %v", err)
	}
	if got.Name != "alpha" {
		t.Fatalf("factory name = %q, want alpha", got.Name)
	}
	if got.Version == nil || got.Version.Logical != 17 || !got.Version.Physical.Equal(versionTime) {
		t.Fatalf("factory version = %#v, want logical=17 physical=%s", got.Version, versionTime)
	}
}

func TestWireLifecycleBehavior_SaveRejectsStaleVersion(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	initialPath := filepath.Join(rootDir, factorydefinitions.FactoryConfigFile)
	currentVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  5,
		Physical: time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC),
	}
	initial := []byte(`{"name":"root","id":"root-runtime","version":{"logical":"5","physical":"2026-05-31T12:00:00Z"},"workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],"workers":[{"name":"worker-a","type":"MODEL_WORKER","body":"initial worker"}],"workstations":[{"name":"process","worker":"worker-a","type":"MODEL_WORKSTATION","body":"initial workstation","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}]}]}`)
	if err := os.WriteFile(initialPath, initial, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	current := factoryapi.Factory{
		Name:    apisurface.DefaultCurrentFactoryName,
		Id:      wireLifecycleStringPointer("root-runtime"),
		Version: &currentVersion,
	}
	gateway := &wireLifecycleTrackingGateway{
		saveNow:      time.Date(2026, 5, 31, 12, 0, 1, 0, time.UTC),
		runSessionID: factorysessions.DefaultSessionID,
	}
	service := newWireLifecycleBehaviorService(
		t,
		withWireLifecycleSessionHost(wireLifecycleSessionHost{
			persistRootDir:  rootDir,
			currentSnapshot: wireLifecycleMustFactorySnapshot(t, current),
		}),
		withWireLifecycleActivationGateway(gateway),
	)

	staleVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  4,
		Physical: currentVersion.Physical.Add(time.Second),
	}
	replacement := factoryapi.Factory{
		Name:    apisurface.DefaultCurrentFactoryName,
		Id:      wireLifecycleStringPointer("root-runtime"),
		Version: &staleVersion,
		WorkTypes: &[]factoryapi.WorkType{{
			Name: "story",
			States: []factoryapi.WorkState{
				{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
				{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
				{Name: "failed", Type: factoryapi.WorkStateTypeFAILED},
			},
		}},
		Workers: &[]factoryapi.Worker{{
			Name: "planner",
			Type: wireLifecycleWorkerTypeModel(),
			Body: wireLifecycleStringPointer("You are the planner."),
		}},
		Workstations: &[]factoryapi.Workstation{{
			Name:   "plan-task",
			Worker: "planner",
			Type:   wireLifecycleWorkstationTypeModel(),
			Body:   wireLifecycleStringPointer("Plan the story."),
			Inputs: []factoryapi.WorkstationIO{{WorkType: "story", State: "init"}},
			Outputs: &[]factoryapi.WorkstationIO{
				{WorkType: "story", State: "complete"},
			},
			OnFailure: &[]factoryapi.WorkstationIO{
				{WorkType: "story", State: "failed"},
			},
		}},
	}

	_, err := service.Save(
		context.Background(),
		factorysessions.DefaultSessionID,
		factorydefinitions.SaveModeReplaceCurrent,
		wireLifecycleEditableFactoryForTest(t, replacement),
	)
	if !errors.Is(err, factorydefinitions.ErrFactoryVersionStale) {
		t.Fatalf("Save() error = %v, want %v", err, factorydefinitions.ErrFactoryVersionStale)
	}

	factoryJSON, err := os.ReadFile(initialPath)
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	if string(factoryJSON) != string(initial) {
		t.Fatalf("factory.json changed after stale save, got %s", factoryJSON)
	}
}

func TestWireLifecycleBehavior_CurrentFactoryDefinitionVersionAtRoot(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	composition := newWireLifecycleComposition()
	validator := factoryvalidation.New(nil)
	versionTime := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	payload := wireLifecycleNamedFactoryPayload(t, "alpha")
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal payload: %v", err)
	}
	decoded["version"] = map[string]any{
		"logical":  float64(23),
		"physical": versionTime.Format(time.RFC3339Nano),
	}
	versioned, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("Marshal versioned payload: %v", err)
	}
	if _, err := composition.PersistNamedFactory(rootDir, "alpha", versioned, validator); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}

	service := newWireLifecycleBehaviorService(
		t,
		withWireLifecycleSessionHost(wireLifecycleSessionHost{persistRootDir: rootDir}),
	)

	got, err := service.CurrentFactoryDefinitionVersionAtRoot(rootDir, "alpha")
	if err != nil {
		t.Fatalf("CurrentFactoryDefinitionVersionAtRoot() error = %v", err)
	}
	if got.Logical != 23 || !got.Physical.Equal(versionTime) {
		t.Fatalf("version = %#v, want logical=23 physical=%s", got, versionTime)
	}

	_, missingErr := service.CurrentFactoryDefinitionVersionAtRoot(rootDir, "missing")
	if !errors.Is(missingErr, factorydefinitions.ErrNamedFactoryNotFound) {
		t.Fatalf(
			"CurrentFactoryDefinitionVersionAtRoot(missing) error = %v, want %v",
			missingErr,
			factorydefinitions.ErrNamedFactoryNotFound,
		)
	}
}

func TestWireLifecycleBehavior_ValidateCollaboratesThroughComposedRoot(t *testing.T) {
	t.Parallel()

	service := newWireLifecycleBehaviorService(t)
	ctx := context.Background()
	validPayload := []byte(factoryfixtures.CrossPathValidAlphaFactoryJSON)

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

	_, invalidErr := service.ValidateStructuralFactoryDefinition(
		ctx,
		factorydefinitions.ValidateStructuralFactoryDefinitionRequest{Canonical: []byte("{")},
	)
	if !errors.Is(invalidErr, factorydefinitions.ErrInvalidFactoryDefinitionPayload) {
		t.Fatalf(
			"ValidateStructuralFactoryDefinition(invalid) error = %v, want %v",
			invalidErr,
			factorydefinitions.ErrInvalidFactoryDefinitionPayload,
		)
	}
}

type wireLifecycleBehaviorConfig struct {
	sessionHost       factorydefinitions.SessionHost
	activationGateway factorydefinitions.DefinitionActivationGateway
}

type wireLifecycleBehaviorOption func(*wireLifecycleBehaviorConfig)

func withWireLifecycleSessionHost(host factorydefinitions.SessionHost) wireLifecycleBehaviorOption {
	return func(cfg *wireLifecycleBehaviorConfig) {
		cfg.sessionHost = host
	}
}

func withWireLifecycleActivationGateway(
	gateway factorydefinitions.DefinitionActivationGateway,
) wireLifecycleBehaviorOption {
	return func(cfg *wireLifecycleBehaviorConfig) {
		cfg.activationGateway = gateway
	}
}

func newWireLifecycleBehaviorService(
	t *testing.T,
	options ...wireLifecycleBehaviorOption,
) factorydefinitions.Service {
	t.Helper()

	cfg := wireLifecycleBehaviorConfig{}
	for _, option := range options {
		option(&cfg)
	}
	if cfg.sessionHost == nil {
		cfg.sessionHost = wireLifecycleSessionHost{}
	}
	if cfg.activationGateway == nil {
		cfg.activationGateway = wireStubActivationGateway{}
	}

	composition := newWireLifecycleComposition()
	validator := factoryvalidation.New(nil)
	mapInput := func(payload []byte) (factorydefinitions.DefinitionValidationRequest, error) {
		return validationentry.MapFactoryJSONForPersistence(payload, composition.LoadCanonicalJSON)
	}
	persistence := composition.Persistence(validator, mapInput)
	loader := composition.Loader()
	namedPaths := composition.NamedPaths()
	fileSystem := platformfilesystem.Local{}
	applySupportedFiles, applyStarterWork, _ := factorydefinitiontestcomposition.PortableOperations(fileSystem)

	packagedCatalog, err := factorydefinitionswire.NewService(
		cfg.sessionHost,
		cfg.activationGateway,
		validator,
		persistence,
		loader,
		applySupportedFiles,
		applyStarterWork,
		namedPaths,
		fileSystem,
		factorydefinitionswire.StaticClock(time.Unix(0, 0)),
		fileSystem,
		wireLifecycleListEffective(composition),
		wireLifecyclePackagedCatalog(t),
		factorydefinitions.PackagedFactoryInstallationOperations{
			Install: func(
				context.Context,
				factorydefinitions.PackagedFactoryInstallParams,
			) (factorydefinitions.PackagedFactoryInstallResult, error) {
				return factorydefinitions.PackagedFactoryInstallResult{}, nil
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
	if packagedCatalog == nil {
		t.Fatal("NewService() returned nil service")
	}
	var root factorydefinitions.Service = packagedCatalog
	if root == nil {
		t.Fatal("constructed value is not assignable to factorydefinitions.Service")
	}
	return root
}

func newWireLifecycleComposition() factorydefinitiontestcomposition.Composition {
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

func wireLifecycleListEffective(
	composition factorydefinitiontestcomposition.Composition,
) factorydefinitions.EffectiveFactoryCatalogOperation {
	return func(
		context.Context,
		factorydefinitions.ListEffectiveFactoriesRequest,
	) (factorydefinitions.ListEffectiveFactoriesResult, error) {
		return factorydefinitions.ListEffectiveFactoriesResult{}, nil
	}
}

func wireLifecyclePackagedCatalog(t *testing.T) factorydefinitions.PackagedFactoryCatalogOperations {
	t.Helper()

	catalog, err := factorydefinitionsinternal.NewPackagedFactoryCatalog([]factorydefinitions.PackagedDefinition{{
		Name:    "@you/wire-lifecycle",
		Project: "wire-lifecycle",
		JSON:    []byte(factoryfixtures.CrossPathValidAlphaFactoryJSON),
		Formats: []factorydefinitions.PackagedFactoryFormat{
			factorydefinitions.PackagedFactoryFormatJSON,
		},
	}})
	if err != nil {
		t.Fatalf("NewPackagedFactoryCatalog() error = %v", err)
	}
	return catalog
}

type wireLifecycleSessionHost struct {
	persistRootDir     string
	session            *factorydefinitions.DefinitionSession
	sessionRuntime     factorydefinitions.LoadedFactorySource
	sessionPersistRoot string
	requireSessionErr  error
	sessionRuntimeErr  error
	currentSnapshot    *factorydefinitions.FactorySnapshot
}

func (h wireLifecycleSessionHost) PersistRootDir() string { return h.persistRootDir }
func (h wireLifecycleSessionHost) WorkstationLoader() factorydefinitions.WorkstationLoader {
	return nil
}
func (h wireLifecycleSessionHost) CurrentRuntimeConfig() factorydefinitions.LoadedFactorySource {
	return nil
}
func (h wireLifecycleSessionHost) WorkflowID() string { return "" }
func (h wireLifecycleSessionHost) RequireSession(sessionID string) (*factorydefinitions.DefinitionSession, error) {
	if h.requireSessionErr != nil {
		return nil, h.requireSessionErr
	}
	if h.session != nil {
		return h.session, nil
	}
	return &factorydefinitions.DefinitionSession{
		ID:         sessionID,
		FactoryDir: h.persistRootDir,
		FolderPath: h.persistRootDir,
		IsDefault:  true,
	}, nil
}
func (h wireLifecycleSessionHost) SessionRuntimeConfig(string) (factorydefinitions.LoadedFactorySource, error) {
	if h.sessionRuntimeErr != nil {
		return nil, h.sessionRuntimeErr
	}
	return h.sessionRuntime, nil
}
func (h wireLifecycleSessionHost) SessionFactoryPersistRoot(*factorydefinitions.DefinitionSession) string {
	if h.sessionPersistRoot != "" {
		return h.sessionPersistRoot
	}
	return h.persistRootDir
}
func (h wireLifecycleSessionHost) ValidateEditableFactorySnapshot(context.Context, *factorydefinitions.FactorySnapshot) error {
	return nil
}
func (h wireLifecycleSessionHost) GetCurrentFactorySnapshotForSession(context.Context, string) (*factorydefinitions.FactorySnapshot, error) {
	if h.currentSnapshot == nil {
		return nil, errors.New("current factory snapshot is required")
	}
	return h.currentSnapshot, nil
}
func (h wireLifecycleSessionHost) ReplaceFactoryLayoutAtDir(
	string,
	*factorydefinitions.PreparedFactoryLayoutPayload,
) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	return nil, nil
}

type wireLifecycleTrackingGateway struct {
	wireStubActivationGateway

	runSessionID         string
	sessionForActivation *factorydefinitions.DefinitionSession
	persistRoot          string
	folderPath           string
	idleNamedErr         error
	saveNow              time.Time
	swapCalls            atomic.Int32
	swappedName          string
}

func (g *wireLifecycleTrackingGateway) RunSessionID() string {
	if g.runSessionID != "" {
		return g.runSessionID
	}
	return g.wireStubActivationGateway.RunSessionID()
}

func (g *wireLifecycleTrackingGateway) SessionForActivation(string) *factorydefinitions.DefinitionSession {
	return g.sessionForActivation
}

func (g *wireLifecycleTrackingGateway) NamedFactoryActivationPaths(*factorydefinitions.DefinitionSession) (string, string) {
	return g.persistRoot, g.folderPath
}

func (g *wireLifecycleTrackingGateway) SaveNow() time.Time {
	if !g.saveNow.IsZero() {
		return g.saveNow
	}
	return g.wireStubActivationGateway.SaveNow()
}

func (g *wireLifecycleTrackingGateway) RequireIdleBeforeNamedFactoryActivation(
	context.Context,
	string,
	*factorydefinitions.DefinitionSession,
) error {
	return g.idleNamedErr
}

func (g *wireLifecycleTrackingGateway) SwapPersistedNamedFactoryRuntime(
	_ context.Context,
	_ string,
	_ *factorydefinitions.DefinitionSession,
	_ string,
	_ string,
	_ string,
	name string,
) error {
	g.swapCalls.Add(1)
	g.swappedName = name
	return nil
}

func wireLifecycleNamedFactoryPayload(t *testing.T, project string) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"name": project,
		"id":   project,
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":          "worker-a",
			"type":          "MODEL_WORKER",
			"modelProvider": "CODEX",
			"model":         "gpt-5-codex",
			"body":          "You are worker " + project + ".",
		}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			"type":      "MODEL_WORKSTATION",
			"body":      "Do the " + project + " work.",
		}},
	})
	if err != nil {
		t.Fatalf("Marshal named factory payload: %v", err)
	}
	return payload
}

func wireLifecycleEditableFactoryForTest(t *testing.T, request factoryapi.Factory) factorydefinitions.EditableFactory {
	t.Helper()
	snapshot, err := factorydefinitions.NewFactorySnapshot(request)
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	var version *factorydefinitions.FactoryVersion
	if request.Version != nil {
		version = &factorydefinitions.FactoryVersion{
			Logical:  request.Version.Logical.Int64(),
			Physical: request.Version.Physical.UTC(),
		}
	}
	return factorydefinitions.EditableFactory{
		Name:     string(request.Name),
		Snapshot: snapshot,
		Version:  version,
	}
}

func wireLifecycleMustFactorySnapshot(t *testing.T, factory factoryapi.Factory) *factorydefinitions.FactorySnapshot {
	t.Helper()
	snapshot, err := factorydefinitions.NewFactorySnapshot(factory)
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	return snapshot
}

func wireLifecycleFactorySnapshotToAPI(snapshot *factorydefinitions.FactorySnapshot) (factoryapi.Factory, error) {
	if snapshot == nil {
		return factoryapi.Factory{}, errors.New("factory snapshot is required")
	}
	var factory factoryapi.Factory
	if err := snapshot.Decode(&factory); err != nil {
		return factoryapi.Factory{}, err
	}
	return factory, nil
}

func wireLifecycleStringPointer(value string) *string { return &value }

func wireLifecycleWorkerTypeModel() *factoryapi.WorkerType {
	value := factoryapi.WorkerTypeModelWorker
	return &value
}

func wireLifecycleWorkstationTypeModel() *factoryapi.WorkstationType {
	value := factoryapi.WorkstationTypeModelWorkstation
	return &value
}
