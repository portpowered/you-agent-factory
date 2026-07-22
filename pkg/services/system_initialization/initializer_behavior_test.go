package systeminitialization

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

type fakeOperatorSettings struct {
	loadErr, ensureErr     error
	loadCalls, ensureCalls []string
}

func (fake *fakeOperatorSettings) LoadFileConfig(path string) (operatorsettings.FileConfig, error) {
	fake.loadCalls = append(fake.loadCalls, path)
	return operatorsettings.FileConfig{}, fake.loadErr
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

type packagedInstallCall struct {
	root        string
	definitions []factorydefinitions.PackagedDefinition
}

func newTestInitializer(
	t *testing.T,
	settings OperatorSettings,
	installer factorydefinitions.PackagedFactoryInstaller,
	definitions []factorydefinitions.PackagedDefinition,
) *Initializer {
	t.Helper()
	initializer, err := New(settings, installer, definitions, os.Stat)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return initializer
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

func TestInit_FreshHomeCreatesOperatorSystemConfig(t *testing.T) {
	settings := &fakeOperatorSettings{}
	installer := &fakePackagedInstaller{}
	homeDir := t.TempDir()
	result, err := newTestInitializer(t, settings, installer, nil).Initialize(t.Context(), Request{HomeDir: homeDir})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if result.SystemConfigOutcome != SystemConfigCreated || len(settings.ensureCalls) != 1 || len(settings.loadCalls) != 1 {
		t.Fatalf("result/settings = %#v, %#v, %#v", result, settings.ensureCalls, settings.loadCalls)
	}
	if len(installer.calls) != 1 || installer.calls[0].root != result.NamedFactoriesRoot {
		t.Fatalf("installer calls = %#v", installer.calls)
	}
}

func TestInit_ExistingConfigIsSkippedWithoutRewrite(t *testing.T) {
	homeDir := t.TempDir()
	settings := &fakeOperatorSettings{}
	installer := &fakePackagedInstaller{}
	initializer := newTestInitializer(t, settings, installer, nil)
	created, err := initializer.Initialize(t.Context(), Request{HomeDir: homeDir})
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
	result, err := initializer.Initialize(t.Context(), Request{HomeDir: homeDir})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	after, _ := os.ReadFile(configPath)
	if result.SystemConfigOutcome != SystemConfigSkipped || len(settings.ensureCalls) != 0 || string(after) != string(original) {
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
	initializer, err := New(&fakeOperatorSettings{}, &fakePackagedInstaller{}, nil, inspect)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = initializer.Initialize(t.Context(), Request{HomeDir: t.TempDir()})
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

	initializer, constructionErr := New(&fakeOperatorSettings{}, &fakePackagedInstaller{}, nil, inspect)
	if constructionErr != nil {
		t.Fatalf("New() error = %v", constructionErr)
	}
	_, err = initializer.Initialize(t.Context(), Request{HomeDir: homeDir})
	if !errors.Is(err, inspectErr) || !strings.Contains(err.Error(), "stat operator config") {
		t.Fatalf("Initialize() error = %v, want injected inspection failure", err)
	}
	if len(inspected) != 2 || filepath.Dir(inspected[1]) != inspected[0] {
		t.Fatalf("inspected paths = %#v, want parent then config", inspected)
	}
}

func TestInit_RejectsEmptyHomeDir(t *testing.T) {
	if _, err := newTestInitializer(t, &fakeOperatorSettings{}, &fakePackagedInstaller{}, nil).
		Initialize(t.Context(), Request{HomeDir: "  "}); err == nil {
		t.Fatal("Initialize(empty home) error = nil")
	}
}

func TestInit_FreshHomeMaterializesPackagedDefaultFactories(t *testing.T) {
	definitions := []factorydefinitions.PackagedDefinition{{Name: "@you/goal", JSON: []byte(`{}`)}}
	installer := &fakePackagedInstaller{results: []factorydefinitions.PackagedFactoryInstallResult{{
		Name: "@you/goal", FactoryDir: "goal", Outcome: factorydefinitions.PackagedFactoryInstallCreated,
	}}}
	result, err := newTestInitializer(t, &fakeOperatorSettings{}, installer, definitions).
		Initialize(t.Context(), Request{HomeDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(installer.calls) != 1 || len(installer.calls[0].definitions) != 1 ||
		len(result.PackagedFactories) != 1 || result.PackagedFactories[0].Outcome != PackagedFactoryCreated {
		t.Fatalf("calls/result = %#v, %#v", installer.calls, result)
	}
}

func TestInit_DoubleRunIsSuccessfulNoOp(t *testing.T) {
	homeDir := t.TempDir()
	settings := &fakeOperatorSettings{}
	installer := &fakePackagedInstaller{}
	first, err := newTestInitializer(t, settings, installer, nil).Initialize(t.Context(), Request{HomeDir: homeDir})
	if err != nil {
		t.Fatal(err)
	}
	second, err := newTestInitializer(t, settings, installer, nil).Initialize(t.Context(), Request{HomeDir: homeDir})
	if err != nil {
		t.Fatal(err)
	}
	if first.SystemConfigOutcome != SystemConfigCreated || second.SystemConfigOutcome != SystemConfigSkipped ||
		len(settings.ensureCalls) != 1 || len(installer.calls) != 2 {
		t.Fatalf("first/second/settings/install = %#v, %#v, %#v, %#v", first, second, settings, installer.calls)
	}
}

func TestInit_ConfigCreationFailureReportsActionableError(t *testing.T) {
	settings := &fakeOperatorSettings{ensureErr: errors.New("write denied")}
	_, err := newTestInitializer(t, settings, &fakePackagedInstaller{}, nil).Initialize(t.Context(), Request{HomeDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "create system config") || !strings.Contains(err.Error(), "write denied") {
		t.Fatalf("Initialize() error = %v", err)
	}
}

func TestInit_FactoryMaterializationFailureReportsActionableError(t *testing.T) {
	installer := &fakePackagedInstaller{err: errors.New("invalid packaged layout")}
	_, err := newTestInitializer(t, &fakeOperatorSettings{}, installer, nil).Initialize(t.Context(), Request{HomeDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "invalid packaged layout") {
		t.Fatalf("Initialize() error = %v", err)
	}
}

func TestNewRequiresInjectedServices(t *testing.T) {
	if initializer, err := New(nil, &fakePackagedInstaller{}, nil, os.Stat); err == nil || initializer != nil {
		t.Fatalf("New(nil Operator Settings) = (%#v, %v), want nil and error", initializer, err)
	}
	if initializer, err := New(&fakeOperatorSettings{}, nil, nil, os.Stat); err == nil || initializer != nil {
		t.Fatalf("New(nil packaged installer) = (%#v, %v), want nil and error", initializer, err)
	}
	if initializer, err := New(&fakeOperatorSettings{}, &fakePackagedInstaller{}, nil, nil); err == nil || initializer != nil || !strings.Contains(err.Error(), "inspect path edge is required") {
		t.Fatalf("New(nil inspect path) = (%#v, %v), want nil and inspect-path error", initializer, err)
	}
}
