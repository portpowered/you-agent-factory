package workflow

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
)

// settingsCommandRecorder records Settings root collaborator invocations during
// Initialize. It implements only the Bootstrap-injected OperatorSettings seam
// (LoadFileConfig / EnsureLocalBackendScope returning Settings root types) so
// production paths that bypass the collaborator or depend on Settings
// transitional packages would fail these behavioral proofs.
type settingsCommandRecorder struct {
	loadErr   error
	ensureErr error

	loadCalls   []string
	ensureCalls []string
}

func (recorder *settingsCommandRecorder) LoadFileConfig(path string) (operatorsettings.Config, error) {
	recorder.loadCalls = append(recorder.loadCalls, path)
	return operatorsettings.Config{}, recorder.loadErr
}

func (recorder *settingsCommandRecorder) EnsureLocalBackendScope(path string) (operatorsettings.ResolvedBackendScope, error) {
	recorder.ensureCalls = append(recorder.ensureCalls, path)
	if recorder.ensureErr == nil {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return operatorsettings.ResolvedBackendScope{}, err
		}
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			return operatorsettings.ResolvedBackendScope{}, err
		}
	}
	return operatorsettings.ResolvedBackendScope{}, recorder.ensureErr
}

// TestInitializeSettingsCommandConstructionThroughRootCollaborator proves
// Initialize derives the operator config path with Settings root DefaultConfigPath
// and routes load/ensure commands only through the injected Settings collaborator
// ports, with observable Bootstrap outcomes on create, skip, and failure paths.
func TestInitializeSettingsCommandConstructionThroughRootCollaborator(t *testing.T) {
	t.Parallel()

	type settingsCommandExpectation struct {
		wantConfigPath string
		wantLoadCalls  []string
		wantEnsure     []string
	}

	tests := []struct {
		name              string
		prepareHome       func(t *testing.T, homeDir, configPath string)
		settings          *settingsCommandRecorder
		wantOutcome       systeminitialization.SystemConfigOutcome
		wantErrPartial    bool
		wantSettingsCalls settingsCommandExpectation
	}{
		{
			name: "create path ensures then loads through root collaborator",
			settings: &settingsCommandRecorder{},
			wantOutcome: systeminitialization.SystemConfigCreated,
			wantSettingsCalls: settingsCommandExpectation{
				wantEnsure: []string{"<config>"},
				wantLoadCalls: []string{"<config>"},
			},
		},
		{
			name: "skip path loads existing config without ensure",
			prepareHome: func(t *testing.T, homeDir, configPath string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(configPath, []byte(`{"customer":"owned"}`), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			settings:    &settingsCommandRecorder{},
			wantOutcome: systeminitialization.SystemConfigSkipped,
			wantSettingsCalls: settingsCommandExpectation{
				wantLoadCalls: []string{"<config>"},
			},
		},
		{
			name: "ensure failure surfaces Bootstrap partial failure with rollback facts",
			settings: &settingsCommandRecorder{
				ensureErr: errors.New("ensure denied"),
			},
			wantErrPartial: true,
			wantSettingsCalls: settingsCommandExpectation{
				wantEnsure: []string{"<config>"},
			},
		},
		{
			name: "load failure on existing config surfaces Bootstrap partial failure",
			prepareHome: func(t *testing.T, homeDir, configPath string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(configPath, []byte(`{"customer":"owned"}`), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			settings: &settingsCommandRecorder{
				loadErr: errors.New("load denied"),
			},
			wantErrPartial: true,
			wantSettingsCalls: settingsCommandExpectation{
				wantLoadCalls: []string{"<config>"},
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			homeDir := t.TempDir()
			wantConfigPath := operatorsettings.DefaultConfigPath(homeDir)
			if test.prepareHome != nil {
				test.prepareHome(t, homeDir, wantConfigPath)
			}

			result, err := newTestInitializer(t, test.settings, &fakePackagedInstaller{}, nil).
				Initialize(t.Context(), systeminitialization.Request{HomeDir: homeDir})

			resolveConfigPath := func(paths []string) []string {
				resolved := make([]string, len(paths))
				for index, path := range paths {
					if path == "<config>" {
						resolved[index] = wantConfigPath
					} else {
						resolved[index] = path
					}
				}
				return resolved
			}

			if test.wantErrPartial {
				if !errors.Is(err, systeminitialization.ErrInitializePartialFailure) {
					t.Fatalf("Initialize() error = %v, want ErrInitializePartialFailure", err)
				}
				var partialFailure systeminitialization.InitializePartialFailure
				if !errors.As(err, &partialFailure) {
					t.Fatalf("Initialize() error = %T(%v), want InitializePartialFailure", err, err)
				}
				if len(partialFailure.Facts) == 0 ||
					partialFailure.Facts[len(partialFailure.Facts)-1].Step != systeminitialization.InitializeStepSystemConfig ||
					partialFailure.Facts[len(partialFailure.Facts)-1].Outcome != systeminitialization.RollbackStepUnresolved {
					t.Fatalf("Initialize() rollback facts = %#v, want unresolved system-config step", partialFailure.Facts)
				}
			} else if err != nil {
				t.Fatalf("Initialize() error = %v", err)
			}

			if !test.wantErrPartial {
				if result.ConfigPath != wantConfigPath {
					t.Fatalf("ConfigPath = %q, want Settings root DefaultConfigPath %q", result.ConfigPath, wantConfigPath)
				}
				if result.SystemConfigOutcome != test.wantOutcome {
					t.Fatalf("SystemConfigOutcome = %q, want %q", result.SystemConfigOutcome, test.wantOutcome)
				}
			}

			wantLoad := resolveConfigPath(test.wantSettingsCalls.wantLoadCalls)
			wantEnsure := resolveConfigPath(test.wantSettingsCalls.wantEnsure)
			if len(test.settings.loadCalls) != len(wantLoad) {
				t.Fatalf("LoadFileConfig calls = %#v, want %#v", test.settings.loadCalls, wantLoad)
			}
			for index, got := range test.settings.loadCalls {
				if got != wantLoad[index] {
					t.Fatalf("LoadFileConfig[%d] = %q, want %q", index, got, wantLoad[index])
				}
			}
			if len(test.settings.ensureCalls) != len(wantEnsure) {
				t.Fatalf("EnsureLocalBackendScope calls = %#v, want %#v", test.settings.ensureCalls, wantEnsure)
			}
			for index, got := range test.settings.ensureCalls {
				if got != wantEnsure[index] {
					t.Fatalf("EnsureLocalBackendScope[%d] = %q, want %q", index, got, wantEnsure[index])
				}
			}
		})
	}
}
