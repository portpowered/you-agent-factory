package workflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
)

type fakeOperatorSettings struct {
	loadErr, ensureErr     error
	loadCalls, ensureCalls []string
}

type localMigrationFileSystem struct{}

func (localMigrationFileSystem) Stat(path string) (os.FileInfo, error)      { return os.Stat(path) }
func (localMigrationFileSystem) ReadFile(path string) ([]byte, error)       { return os.ReadFile(path) }
func (localMigrationFileSystem) ReadDir(path string) ([]os.DirEntry, error) { return os.ReadDir(path) }
func (localMigrationFileSystem) MkdirAll(path string, mode os.FileMode) error {
	return os.MkdirAll(path, mode)
}
func (localMigrationFileSystem) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (fake *fakeOperatorSettings) LoadFileConfig(path string) (operatorsettings.Config, error) {
	fake.loadCalls = append(fake.loadCalls, path)
	return operatorsettings.Config{}, fake.loadErr
}

func (fake *fakeOperatorSettings) EnsureLocalBackendScope(path string) (operatorsettings.ResolvedBackendScope, error) {
	fake.ensureCalls = append(fake.ensureCalls, path)
	if fake.ensureErr == nil {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return operatorsettings.ResolvedBackendScope{}, err
		}
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			return operatorsettings.ResolvedBackendScope{}, err
		}
	}
	return operatorsettings.ResolvedBackendScope{}, fake.ensureErr
}

type fakePackagedInstaller struct {
	calls   []packagedInstallCall
	results []factorydefinitions.PackagedFactoryInstallResult
	err     error
}

type failingPackagedCatalog struct {
	err error
}

func (catalog failingPackagedCatalog) ListBuiltInPackagedFactories(
	context.Context,
	factorydefinitions.ListBuiltInPackagedFactoriesRequest,
) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
	return factorydefinitions.ListBuiltInPackagedFactoriesResult{}, catalog.err
}

func (failingPackagedCatalog) ResolveBuiltInPackagedFactory(
	context.Context,
	factorydefinitions.ResolveBuiltInPackagedFactoryRequest,
) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
	return factorydefinitions.ResolveBuiltInPackagedFactoryResult{}, errors.New("unexpected resolve")
}

type packagedInstallCall struct {
	root        string
	definitions []factorydefinitions.PackagedDefinition
}

func newTestInitializer(
	t *testing.T,
	settings systeminitialization.OperatorSettings,
	installer factorydefinitions.PackagedFactoryInstaller,
	definitions []factorydefinitions.PackagedDefinition,
) *Initializer {
	t.Helper()
	catalog := newTestPackagedCatalog(definitions)
	initializer, err := New(settings, catalog, installer, os.Stat, localMigrationFileSystem{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return initializer
}

func newTestPackagedCatalog(
	definitions []factorydefinitions.PackagedDefinition,
) factorydefinitions.PackagedFactoryCatalogOperations {
	cloned := append([]factorydefinitions.PackagedDefinition(nil), definitions...)
	sort.Slice(cloned, func(i, j int) bool { return cloned[i].Name < cloned[j].Name })
	return factorydefinitions.PackagedFactoryCatalogOperations{
		List: func(
			context.Context,
			factorydefinitions.ListBuiltInPackagedFactoriesRequest,
		) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
			entries := make([]factorydefinitions.BuiltInPackagedFactoryEntry, len(cloned))
			for index, definition := range cloned {
				entries[index] = factorydefinitions.BuiltInPackagedFactoryEntry{
					Name: definition.Name, Project: definition.Project,
					Formats: append([]factorydefinitions.PackagedFactoryFormat(nil), definition.Formats...),
				}
			}
			return factorydefinitions.ListBuiltInPackagedFactoriesResult{Entries: entries}, nil
		},
		Resolve: func(
			_ context.Context,
			request factorydefinitions.ResolveBuiltInPackagedFactoryRequest,
		) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
			for _, definition := range cloned {
				if definition.Name == request.Name {
					return factorydefinitions.ResolveBuiltInPackagedFactoryResult{
						Definition: definition,
						Formats:    append([]factorydefinitions.PackagedFactoryFormat(nil), definition.Formats...),
					}, nil
				}
			}
			return factorydefinitions.ResolveBuiltInPackagedFactoryResult{},
				factorydefinitions.ErrUnknownPackagedFactoryIdentity
		},
	}
}

func (fake *fakePackagedInstaller) EnsurePackagedFactories(
	_ context.Context,
	root string,
	definitions []factorydefinitions.PackagedDefinition,
) ([]factorydefinitions.PackagedFactoryInstallResult, error) {
	fake.calls = append(fake.calls, packagedInstallCall{
		root:        root,
		definitions: append([]factorydefinitions.PackagedDefinition(nil), definitions...),
	})
	return append([]factorydefinitions.PackagedFactoryInstallResult(nil), fake.results...), fake.err
}

func TestInitializeFreshHomeReturnsTypedCreatedResultsThroughPeerRoots(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	definitions := []factorydefinitions.PackagedDefinition{{
		Name:    "@you/goal",
		JSON:    []byte(`{}`),
		Formats: []factorydefinitions.PackagedFactoryFormat{factorydefinitions.PackagedFactoryFormatJSON},
	}}
	settings := &fakeOperatorSettings{}
	installer := &fakePackagedInstaller{results: []factorydefinitions.PackagedFactoryInstallResult{{
		Name:       "@you/goal",
		FactoryDir: "goal",
		Outcome:    factorydefinitions.PackagedFactoryInstallCreated,
	}}}

	result, err := newTestInitializer(t, settings, installer, definitions).
		Initialize(t.Context(), systeminitialization.Request{HomeDir: homeDir})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	wantConfigPath := operatorsettings.DefaultConfigPath(homeDir)
	wantFactoriesRoot := factorydefinitions.NamedFactoriesRoot(homeDir)
	if result.HomeDir != homeDir ||
		result.ConfigPath != wantConfigPath ||
		result.NamedFactoriesRoot != wantFactoriesRoot ||
		result.SystemConfigOutcome != systeminitialization.SystemConfigCreated {
		t.Fatalf("Initialize() result = %#v, want typed created-path summary", result)
	}
	if len(result.PackagedFactories) != 1 ||
		result.PackagedFactories[0].Name != "@you/goal" ||
		result.PackagedFactories[0].FactoryDir != "goal" ||
		result.PackagedFactories[0].Outcome != systeminitialization.PackagedFactoryCreated {
		t.Fatalf("PackagedFactories = %#v, want one created packaged Factory", result.PackagedFactories)
	}
	if len(settings.ensureCalls) != 1 || settings.ensureCalls[0] != wantConfigPath {
		t.Fatalf("Operator Settings ensureCalls = %#v, want [%q]", settings.ensureCalls, wantConfigPath)
	}
	if len(settings.loadCalls) != 1 || settings.loadCalls[0] != wantConfigPath {
		t.Fatalf("Operator Settings loadCalls = %#v, want [%q]", settings.loadCalls, wantConfigPath)
	}
	if len(installer.calls) != 1 ||
		installer.calls[0].root != wantFactoriesRoot ||
		len(installer.calls[0].definitions) != 1 ||
		installer.calls[0].definitions[0].Name != "@you/goal" {
		t.Fatalf("Factory Definitions installer calls = %#v, want one install at %q", installer.calls, wantFactoriesRoot)
	}
}

func TestInitializeRepeatInvocationReportsSkippedOutcomesForSystemConfigAndPackagedFactories(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	definitions := []factorydefinitions.PackagedDefinition{{
		Name:    "@you/goal",
		JSON:    []byte(`{}`),
		Formats: []factorydefinitions.PackagedFactoryFormat{factorydefinitions.PackagedFactoryFormatJSON},
	}}
	settings := &fakeOperatorSettings{}
	installer := &repeatAwarePackagedInstaller{}
	initializer := newTestInitializer(t, settings, installer, definitions)

	first, err := initializer.Initialize(t.Context(), systeminitialization.Request{HomeDir: homeDir})
	if err != nil {
		t.Fatalf("first Initialize() error = %v", err)
	}
	configAfterFirst, err := os.ReadFile(first.ConfigPath)
	if err != nil {
		t.Fatalf("read config after first Initialize() = %v", err)
	}

	second, err := initializer.Initialize(t.Context(), systeminitialization.Request{HomeDir: homeDir})
	if err != nil {
		t.Fatalf("second Initialize() error = %v", err)
	}
	configAfterSecond, err := os.ReadFile(first.ConfigPath)
	if err != nil {
		t.Fatalf("read config after second Initialize() = %v", err)
	}

	if first.SystemConfigOutcome != systeminitialization.SystemConfigCreated ||
		second.SystemConfigOutcome != systeminitialization.SystemConfigSkipped {
		t.Fatalf("system config outcomes = %#v then %#v, want created then skipped", first, second)
	}
	if len(first.PackagedFactories) != 1 ||
		first.PackagedFactories[0].Outcome != systeminitialization.PackagedFactoryCreated ||
		len(second.PackagedFactories) != 1 ||
		second.PackagedFactories[0].Name != "@you/goal" ||
		second.PackagedFactories[0].Outcome != systeminitialization.PackagedFactorySkipped {
		t.Fatalf(
			"packaged factory outcomes = %#v then %#v, want created then skipped",
			first.PackagedFactories,
			second.PackagedFactories,
		)
	}
	if string(configAfterFirst) != string(configAfterSecond) {
		t.Fatalf("operator config changed on repeat: before %q after %q", configAfterFirst, configAfterSecond)
	}
	if len(settings.ensureCalls) != 1 {
		t.Fatalf("Operator Settings ensureCalls = %#v, want one create on first run only", settings.ensureCalls)
	}
	if len(settings.loadCalls) != 2 {
		t.Fatalf("Operator Settings loadCalls = %#v, want load on both invocations", settings.loadCalls)
	}
	if installer.calls != 2 {
		t.Fatalf("packaged installer calls = %d, want one per invocation", installer.calls)
	}
}

type repeatAwarePackagedInstaller struct {
	calls int
}

func (installer *repeatAwarePackagedInstaller) EnsurePackagedFactories(
	_ context.Context,
	root string,
	definitions []factorydefinitions.PackagedDefinition,
) ([]factorydefinitions.PackagedFactoryInstallResult, error) {
	installer.calls++
	outcome := factorydefinitions.PackagedFactoryInstallCreated
	if installer.calls > 1 {
		outcome = factorydefinitions.PackagedFactoryInstallSkipped
	}
	results := make([]factorydefinitions.PackagedFactoryInstallResult, 0, len(definitions))
	for _, definition := range definitions {
		results = append(results, factorydefinitions.PackagedFactoryInstallResult{
			Name:       definition.Name,
			FactoryDir: strings.TrimPrefix(definition.Name, "@you/"),
			Outcome:    outcome,
		})
	}
	_ = root
	return results, nil
}

func TestInit_FreshHomeCreatesOperatorSystemConfig(t *testing.T) {
	settings := &fakeOperatorSettings{}
	installer := &fakePackagedInstaller{}
	homeDir := t.TempDir()
	result, err := newTestInitializer(t, settings, installer, nil).Initialize(t.Context(), systeminitialization.Request{HomeDir: homeDir})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if result.SystemConfigOutcome != systeminitialization.SystemConfigCreated || len(settings.ensureCalls) != 1 || len(settings.loadCalls) != 1 {
		t.Fatalf("result/settings = %#v, %#v, %#v", result, settings.ensureCalls, settings.loadCalls)
	}
	if len(installer.calls) != 1 || installer.calls[0].root != result.NamedFactoriesRoot {
		t.Fatalf("installer calls = %#v", installer.calls)
	}
}

func TestInitializeMigratesLegacyFactoriesBeforePackagedInstallation(t *testing.T) {
	homeDir := t.TempDir()
	legacyDir := filepath.Join(factorydefinitions.LegacyNamedFactoriesRoot(homeDir), "@you", "goal")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := []byte("customer-owned\n")
	if err := os.WriteFile(filepath.Join(legacyDir, "customer-edit.txt"), marker, 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := newTestInitializer(t, &fakeOperatorSettings{}, &fakePackagedInstaller{}, nil).Initialize(t.Context(), systeminitialization.Request{HomeDir: homeDir})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	canonicalDir := filepath.Join(result.NamedFactoriesRoot, "@you", "goal")
	got, err := os.ReadFile(filepath.Join(canonicalDir, "customer-edit.txt"))
	if err != nil || string(got) != string(marker) {
		t.Fatalf("migrated customer edit = %q, %v", got, err)
	}
	if _, err := os.Stat(legacyDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy directory remains after migration: %v", err)
	}
}

func TestInitializeLegacyFactoryConflictPreservesBothCopies(t *testing.T) {
	homeDir := t.TempDir()
	legacyDir := filepath.Join(factorydefinitions.LegacyNamedFactoriesRoot(homeDir), "customer")
	canonicalDir := filepath.Join(homeDir, ".you-agent-factory", "factories", "customer")
	for _, dir := range []string{legacyDir, canonicalDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	_, err := newTestInitializer(t, &fakeOperatorSettings{}, &fakePackagedInstaller{}, nil).Initialize(t.Context(), systeminitialization.Request{HomeDir: homeDir})
	if err == nil || !strings.Contains(err.Error(), "without overwriting") {
		t.Fatalf("Initialize() conflict error = %v", err)
	}
	for _, dir := range []string{legacyDir, canonicalDir} {
		if _, statErr := os.Stat(dir); statErr != nil {
			t.Fatalf("conflict changed %s: %v", dir, statErr)
		}
	}
}

func TestInitializeRejectsInvalidLegacyCurrentFactoryPointer(t *testing.T) {
	homeDir := t.TempDir()
	legacyRoot := factorydefinitions.LegacyNamedFactoriesRoot(homeDir)
	if err := os.MkdirAll(legacyRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, factorydefinitions.CurrentFactoryPointerFile), []byte("../outside-root\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := newTestInitializer(t, &fakeOperatorSettings{}, &fakePackagedInstaller{}, nil).Initialize(t.Context(), systeminitialization.Request{HomeDir: homeDir})
	if err == nil || !strings.Contains(err.Error(), "list legacy global Factories") {
		t.Fatalf("Initialize() error = %v, want legacy inventory guidance", err)
	}
}

func TestInit_ExistingConfigIsSkippedWithoutRewrite(t *testing.T) {
	homeDir := t.TempDir()
	settings := &fakeOperatorSettings{}
	installer := &fakePackagedInstaller{}
	initializer := newTestInitializer(t, settings, installer, nil)
	created, err := initializer.Initialize(t.Context(), systeminitialization.Request{HomeDir: homeDir})
	if err != nil {
		t.Fatalf("Initialize() setup error = %v", err)
	}
	configPath := created.ConfigPath
	original := []byte(`{"customer":"owned"}`)
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	settings.loadCalls = nil
	settings.ensureCalls = nil
	installer.calls = nil
	result, err := initializer.Initialize(t.Context(), systeminitialization.Request{HomeDir: homeDir})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	after, _ := os.ReadFile(configPath)
	if result.SystemConfigOutcome != systeminitialization.SystemConfigSkipped || len(settings.ensureCalls) != 0 || string(after) != string(original) {
		t.Fatalf("result/ensure/content = %#v, %#v, %q", result, settings.ensureCalls, after)
	}
}

func TestInit_RejectsSystemConfigParentThatIsAFile(t *testing.T) {
	occupiedPath := filepath.Join(t.TempDir(), "occupied")
	if err := os.WriteFile(occupiedPath, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	occupied, err := os.Stat(occupiedPath)
	if err != nil {
		t.Fatal(err)
	}
	inspect := func(string) (os.FileInfo, error) { return occupied, nil }
	catalog := newTestPackagedCatalog(nil)
	initializer, err := New(&fakeOperatorSettings{}, catalog, &fakePackagedInstaller{}, inspect, localMigrationFileSystem{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = initializer.Initialize(t.Context(), systeminitialization.Request{HomeDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "exists but is not a directory") {
		t.Fatalf("Initialize() error = %v, want actionable parent-file error", err)
	}
}

func TestInit_PropagatesInjectedConfigInspectionFailure(t *testing.T) {
	homeDir := t.TempDir()
	directoryInfo, err := os.Stat(homeDir)
	if err != nil {
		t.Fatal(err)
	}
	inspected := []string{}
	inspectErr := errors.New("inspection denied")
	inspect := func(path string) (os.FileInfo, error) {
		inspected = append(inspected, path)
		if len(inspected) == 1 {
			return directoryInfo, nil
		}
		return nil, inspectErr
	}

	catalog := newTestPackagedCatalog(nil)
	initializer, constructionErr := New(&fakeOperatorSettings{}, catalog, &fakePackagedInstaller{}, inspect, localMigrationFileSystem{})
	if constructionErr != nil {
		t.Fatalf("New() error = %v", constructionErr)
	}
	_, err = initializer.Initialize(t.Context(), systeminitialization.Request{HomeDir: homeDir})
	if !errors.Is(err, inspectErr) || !strings.Contains(err.Error(), "stat operator config") {
		t.Fatalf("Initialize() error = %v, want injected inspection failure", err)
	}
	if len(inspected) != 2 || filepath.Dir(inspected[1]) != inspected[0] {
		t.Fatalf("inspected paths = %#v, want parent then config", inspected)
	}
}

func TestInit_RejectsEmptyHomeDir(t *testing.T) {
	_, err := newTestInitializer(t, &fakeOperatorSettings{}, &fakePackagedInstaller{}, nil).
		Initialize(t.Context(), systeminitialization.Request{HomeDir: "  "})
	if err == nil {
		t.Fatal("Initialize(empty home) error = nil")
	}
	if !errors.Is(err, systeminitialization.ErrMissingHomeDir) {
		t.Fatalf("Initialize(empty home) error = %v, want ErrMissingHomeDir", err)
	}
	var partialFailure systeminitialization.InitializePartialFailure
	if errors.As(err, &partialFailure) {
		t.Fatalf("Initialize(empty home) error = %v, want no rollback facts", err)
	}
}

func TestInit_FreshHomeMaterializesPackagedDefaultFactories(t *testing.T) {
	definitions := []factorydefinitions.PackagedDefinition{{
		Name: "@you/goal", JSON: []byte(`{}`),
		Formats: []factorydefinitions.PackagedFactoryFormat{
			factorydefinitions.PackagedFactoryFormatJSON,
		},
	}}
	installer := &fakePackagedInstaller{results: []factorydefinitions.PackagedFactoryInstallResult{{
		Name: "@you/goal", FactoryDir: "goal", Outcome: factorydefinitions.PackagedFactoryInstallCreated,
	}}}
	result, err := newTestInitializer(t, &fakeOperatorSettings{}, installer, definitions).
		Initialize(t.Context(), systeminitialization.Request{HomeDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(installer.calls) != 1 || len(installer.calls[0].definitions) != 1 ||
		len(result.PackagedFactories) != 1 || result.PackagedFactories[0].Outcome != systeminitialization.PackagedFactoryCreated {
		t.Fatalf("calls/result = %#v, %#v", installer.calls, result)
	}
}

func TestInit_DoubleRunIsSuccessfulNoOp(t *testing.T) {
	homeDir := t.TempDir()
	settings := &fakeOperatorSettings{}
	installer := &fakePackagedInstaller{}
	first, err := newTestInitializer(t, settings, installer, nil).Initialize(t.Context(), systeminitialization.Request{HomeDir: homeDir})
	if err != nil {
		t.Fatal(err)
	}
	second, err := newTestInitializer(t, settings, installer, nil).Initialize(t.Context(), systeminitialization.Request{HomeDir: homeDir})
	if err != nil {
		t.Fatal(err)
	}
	if first.SystemConfigOutcome != systeminitialization.SystemConfigCreated || second.SystemConfigOutcome != systeminitialization.SystemConfigSkipped ||
		len(settings.ensureCalls) != 1 || len(installer.calls) != 2 {
		t.Fatalf("first/second/settings/install = %#v, %#v, %#v, %#v", first, second, settings, installer.calls)
	}
}

func TestInit_ConfigCreationFailureReportsActionableError(t *testing.T) {
	settings := &fakeOperatorSettings{ensureErr: errors.New("write denied")}
	_, err := newTestInitializer(t, settings, &fakePackagedInstaller{}, nil).Initialize(t.Context(), systeminitialization.Request{HomeDir: t.TempDir()})
	if err == nil {
		t.Fatalf("Initialize() error = nil")
	}
	if !errors.Is(err, systeminitialization.ErrInitializePartialFailure) {
		t.Fatalf("Initialize() error = %v, want ErrInitializePartialFailure", err)
	}
	var partialFailure systeminitialization.InitializePartialFailure
	if !errors.As(err, &partialFailure) {
		t.Fatalf("Initialize() error = %T(%v), want InitializePartialFailure", err, err)
	}
	if !strings.Contains(partialFailure.Cause.Error(), "create system config") ||
		!strings.Contains(partialFailure.Cause.Error(), "write denied") {
		t.Fatalf("Initialize() cause = %v, want actionable create-system-config failure", partialFailure.Cause)
	}
	if len(partialFailure.Facts) != 2 ||
		partialFailure.Facts[0].Step != systeminitialization.InitializeStepLegacyMigration ||
		partialFailure.Facts[0].Outcome != systeminitialization.RollbackStepCompleted ||
		partialFailure.Facts[1].Step != systeminitialization.InitializeStepSystemConfig ||
		partialFailure.Facts[1].Outcome != systeminitialization.RollbackStepUnresolved {
		t.Fatalf("Initialize() rollback facts = %#v", partialFailure.Facts)
	}
}

func TestInit_FactoryMaterializationFailureReportsActionableError(t *testing.T) {
	installErr := errors.New("invalid packaged layout")
	installer := &fakePackagedInstaller{err: installErr}
	_, err := newTestInitializer(t, &fakeOperatorSettings{}, installer, nil).Initialize(t.Context(), systeminitialization.Request{HomeDir: t.TempDir()})
	if err == nil {
		t.Fatalf("Initialize() error = nil")
	}
	if !errors.Is(err, systeminitialization.ErrInitializePartialFailure) {
		t.Fatalf("Initialize() error = %v, want ErrInitializePartialFailure", err)
	}
	if !errors.Is(err, installErr) {
		t.Fatalf("Initialize() error = %v, want wrapped install cause", err)
	}
	var partialFailure systeminitialization.InitializePartialFailure
	if !errors.As(err, &partialFailure) {
		t.Fatalf("Initialize() error = %T(%v), want InitializePartialFailure", err, err)
	}
	if len(partialFailure.Facts) != 3 ||
		partialFailure.Facts[0].Step != systeminitialization.InitializeStepLegacyMigration ||
		partialFailure.Facts[0].Outcome != systeminitialization.RollbackStepCompleted ||
		partialFailure.Facts[1].Step != systeminitialization.InitializeStepSystemConfig ||
		partialFailure.Facts[1].Outcome != systeminitialization.RollbackStepRolledBackOrPreserved ||
		partialFailure.Facts[2].Step != systeminitialization.InitializeStepPackagedFactories ||
		partialFailure.Facts[2].Outcome != systeminitialization.RollbackStepUnresolved {
		t.Fatalf("Initialize() rollback facts = %#v", partialFailure.Facts)
	}
}

func TestInitializePackagedFactoryFailureAfterSkippedSystemConfigReportsRollbackFacts(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(`{"customer":"owned"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	installErr := errors.New("packaged factory install failed")
	_, err := newTestInitializer(
		t,
		&fakeOperatorSettings{},
		&fakePackagedInstaller{err: installErr},
		nil,
	).Initialize(t.Context(), systeminitialization.Request{HomeDir: homeDir})
	if !errors.Is(err, systeminitialization.ErrInitializePartialFailure) {
		t.Fatalf("Initialize() error = %v, want ErrInitializePartialFailure", err)
	}
	if !errors.Is(err, installErr) {
		t.Fatalf("Initialize() error = %v, want wrapped install cause", err)
	}
	var partialFailure systeminitialization.InitializePartialFailure
	if !errors.As(err, &partialFailure) {
		t.Fatalf("Initialize() error = %T(%v), want InitializePartialFailure", err, err)
	}
	if len(partialFailure.Facts) != 3 ||
		partialFailure.Facts[1].Step != systeminitialization.InitializeStepSystemConfig ||
		partialFailure.Facts[1].Outcome != systeminitialization.RollbackStepRolledBackOrPreserved {
		t.Fatalf("Initialize() rollback facts = %#v", partialFailure.Facts)
	}
}

func TestInitializeValidationAndCancellationFailuresDoNotInventRollbackFacts(t *testing.T) {
	t.Parallel()

	_, validationErr := newTestInitializer(t, &fakeOperatorSettings{}, &fakePackagedInstaller{}, nil).
		Initialize(t.Context(), systeminitialization.Request{HomeDir: "  "})
	if !errors.Is(validationErr, systeminitialization.ErrMissingHomeDir) {
		t.Fatalf("validation error = %v, want ErrMissingHomeDir", validationErr)
	}
	var validationPartialFailure systeminitialization.InitializePartialFailure
	if errors.As(validationErr, &validationPartialFailure) {
		t.Fatalf("validation error = %v, want no rollback facts", validationErr)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, cancellationErr := newTestInitializer(t, &fakeOperatorSettings{}, &fakePackagedInstaller{}, nil).
		Initialize(ctx, systeminitialization.Request{HomeDir: t.TempDir()})
	if !errors.Is(cancellationErr, systeminitialization.ErrInitializeCancelled) {
		t.Fatalf("cancellation error = %v, want ErrInitializeCancelled", cancellationErr)
	}
	var cancellationPartialFailure systeminitialization.InitializePartialFailure
	if errors.As(cancellationErr, &cancellationPartialFailure) {
		t.Fatalf("cancellation error = %v, want no rollback facts", cancellationErr)
	}
}

func TestInitializeCatalogFailureReturnsBeforeInstallationOrConfigMutation(t *testing.T) {
	settings := &fakeOperatorSettings{}
	installer := &fakePackagedInstaller{}
	catalogErr := errors.New("embedded manifest invalid")
	initializer, err := New(
		settings,
		factorydefinitions.PackagedFactoryCatalogOperations{
			List:    failingPackagedCatalog{err: catalogErr}.ListBuiltInPackagedFactories,
			Resolve: failingPackagedCatalog{}.ResolveBuiltInPackagedFactory,
		},
		installer,
		os.Stat,
		localMigrationFileSystem{},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = initializer.Initialize(t.Context(), systeminitialization.Request{HomeDir: t.TempDir()})
	if !errors.Is(err, catalogErr) {
		t.Fatalf("Initialize() error = %v", err)
	}
	if len(settings.ensureCalls) != 0 || len(settings.loadCalls) != 0 ||
		len(installer.calls) != 0 {
		t.Fatalf(
			"catalog failure caused side effects: settings=%#v installer=%#v",
			settings,
			installer.calls,
		)
	}
}

func TestNewRequiresInjectedServices(t *testing.T) {
	catalog := newTestPackagedCatalog(nil)
	if initializer, err := New(nil, catalog, &fakePackagedInstaller{}, os.Stat, localMigrationFileSystem{}); err == nil || initializer != nil {
		t.Fatalf("New(nil Operator Settings) = (%#v, %v), want nil and error", initializer, err)
	}
	if initializer, err := New(&fakeOperatorSettings{}, catalog, nil, os.Stat, localMigrationFileSystem{}); err == nil || initializer != nil {
		t.Fatalf("New(nil packaged installer) = (%#v, %v), want nil and error", initializer, err)
	}
	if initializer, err := New(&fakeOperatorSettings{}, factorydefinitions.PackagedFactoryCatalogOperations{}, &fakePackagedInstaller{}, os.Stat, localMigrationFileSystem{}); err == nil || initializer != nil {
		t.Fatalf("New(nil packaged catalog) = (%#v, %v), want nil and error", initializer, err)
	}
	if initializer, err := New(&fakeOperatorSettings{}, catalog, &fakePackagedInstaller{}, nil, localMigrationFileSystem{}); err == nil || initializer != nil || !strings.Contains(err.Error(), "inspect path edge is required") {
		t.Fatalf("New(nil inspect path) = (%#v, %v), want nil and inspect-path error", initializer, err)
	}
}
