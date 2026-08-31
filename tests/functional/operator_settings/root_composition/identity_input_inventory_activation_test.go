package root_composition_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/root"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	identityActivationGeneratedUUID   = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	identityActivationExistingScope   = "local-11111111-1111-4111-8111-111111111111"
	operatorConfigFixturesRelativeDir = "pkg/services/operator_settings/testdata/fixtures"
)

// TestBackendScopeIdentityGeneratesThroughRootBuildProcessAfterLifecycle proves
// backend-scope identity resolution and persistence activate through public
// Operator Settings surfaces after runtime lifecycle on the shared process.
func TestBackendScopeIdentityGeneratesThroughRootBuildProcessAfterLifecycle(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	fixture := ensureSharedOperatorSettingsFixture(t)
	constructionIDCalls := fixture.constructionEffectSnapshot().operatorIDCalls
	fixture.withOperatorSettingsRoute(
		t,
		"generated backend scope",
		homeDir,
		homeDir,
		identityActivationGeneratedUUID,
		nil,
		func(_ *operatorSettingsEffectRoute) {
			runOperatorSettingsLifecycleInitialization(t, fixture.process, homeDir)
			if got := fixture.router.operatorIDCalls.Load(); got <= constructionIDCalls {
				t.Fatalf("IDGenerator calls after lifecycle = %d, want > construction count %d", got, constructionIDCalls)
			}

			wantScope := operatorsettings.LocalBackendScopePrefix + identityActivationGeneratedUUID
			if got := readBackendScopeIDFromHome(t, homeDir); got != wantScope {
				t.Fatalf("backendScopeID = %q, want generated scope %q", got, wantScope)
			}
		},
	)
}

// TestBackendScopeIdentityReusesExistingScopeThroughRootBuildProcessAfterLifecycle
// proves persisted backend-scope identity is reused through public Operator
// Settings surfaces after runtime lifecycle without regenerating scope IDs.
func TestBackendScopeIdentityReusesExistingScopeThroughRootBuildProcessAfterLifecycle(t *testing.T) {
	t.Parallel()

	homeDir := writeOperatorConfigWithBackendScope(t, identityActivationExistingScope)
	fixture := ensureSharedOperatorSettingsFixture(t)
	fixture.withOperatorSettingsRoute(
		t,
		"existing backend scope",
		homeDir,
		homeDir,
		identityActivationExistingScope,
		nil,
		func(_ *operatorSettingsEffectRoute) {
			beforeID := fixture.router.operatorIDCalls.Load()
			runOperatorSettingsLifecycleInitialization(t, fixture.process, homeDir)

			if got := fixture.router.operatorIDCalls.Load() - beforeID; got != 0 {
				t.Fatalf("IDGenerator calls after lifecycle with existing scope = %d, want 0", got)
			}
			if got := readBackendScopeIDFromHome(t, homeDir); got != identityActivationExistingScope {
				t.Fatalf("backendScopeID = %q, want reused scope %q", got, identityActivationExistingScope)
			}
		},
	)
}

// TestOperatorInputInventoryActivatesThroughRootBuildProcessAfterLifecycle proves
// the published input inventory index and LoadFileConfig loader paths activate
// through public Operator Settings surfaces after runtime lifecycle on a process
// composed only via the canonical process construction.
func TestOperatorInputInventoryActivatesThroughRootBuildProcessAfterLifecycle(t *testing.T) {
	t.Parallel()

	homeDir := writeOperatorConfigWithBackendScope(t, identityActivationExistingScope)
	fixture := ensureSharedOperatorSettingsFixture(t)
	fixture.withOperatorSettingsRoute(
		t,
		"input inventory",
		homeDir,
		homeDir,
		identityActivationExistingScope,
		nil,
		func(_ *operatorSettingsEffectRoute) {
			providersRoot, err := providerswire.NewService()
			if err != nil {
				t.Fatalf("providerswire.NewService() error = %v", err)
			}
			settingsRoot, err := settingswire.NewServiceFromHomePorts(
				fixture.router,
				globalconfigmapping.Decode,
				providersRoot,
				fixture.router.generateOperatorID,
				logging.NoopLogger{},
			)
			if err != nil {
				t.Fatalf("NewServiceFromHomePorts() error = %v", err)
			}
			inventory := settingsRoot.ProjectInputInventory()
			if inventory.FormatVersion != operatorsettings.InputInventoryFormatVersion {
				t.Fatalf("inventory formatVersion = %q, want %q", inventory.FormatVersion, operatorsettings.InputInventoryFormatVersion)
			}
			if len(inventory.Cases) == 0 {
				t.Fatal("ProjectInputInventory() returned no cases after lifecycle")
			}

			inputCase, ok := findInputInventoryCase(inventory.Cases, "valid-load-defaults")
			if !ok {
				t.Fatal("ProjectInputInventory() missing valid-load-defaults case")
			}
			if inputCase.Entrypoint != "LoadFileConfig" || inputCase.Outcome != "accept" {
				t.Fatalf("valid-load-defaults case = %#v, want LoadFileConfig accept", inputCase)
			}

			configPath := writeOperatorSettingsFixtureToTemp(t, homeDir, inputCase.Fixture)
			loaded, err := operatorsettings.LoadFileConfig(
				fixture.router,
				globalconfigmapping.Decode,
				configPath,
			)
			if err != nil {
				t.Fatalf("LoadFileConfig() after lifecycle = %v", err)
			}
			if inputCase.ExpectedConfig == nil {
				t.Fatal("valid-load-defaults case missing expectedConfig")
			}
			if loaded.Defaults.WorkerModelProvider != inputCase.ExpectedConfig.Defaults.WorkerModelProvider ||
				loaded.Defaults.WorkerModel != inputCase.ExpectedConfig.Defaults.WorkerModel {
				t.Fatalf(
					"LoadFileConfig() defaults = %#v, want %#v from input inventory",
					loaded.Defaults,
					inputCase.ExpectedConfig.Defaults,
				)
			}
		},
	)
}

func runOperatorSettingsLifecycleInitialization(t *testing.T, process support.Process, homeDir string) {
	t.Helper()

	missingFactory := filepath.Join(homeDir, "missing-lifecycle-factory.json")
	err := process.Execute(root.Input{
		Args: []string{"you", "run", "--factory", missingFactory},
		Env: append(
			os.Environ(),
			"HOME="+homeDir,
			"USERPROFILE="+homeDir,
		),
		Context:          t.Context(),
		WorkingDirectory: homeDir,
	})
	if err == nil || !strings.Contains(err.Error(), filepath.Base(missingFactory)) {
		t.Fatalf("Process.Execute(run missing Factory) error = %v, want missing-factory failure", err)
	}
}

func writeOperatorConfigWithBackendScope(t *testing.T, scopeID string) string {
	t.Helper()

	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir operator config directory: %v", err)
	}
	config := []byte(`{
  "backendScopeID": "` + scopeID + `",
  "defaults": {
    "workerModelProvider": "codex",
    "workerModel": "configured-model"
  }
}`)
	configPath := filepath.Join(configDir, "config.json")
	if err := os.WriteFile(configPath, config, 0o600); err != nil {
		t.Fatalf("write operator config: %v", err)
	}
	return homeDir
}

func readBackendScopeIDFromHome(t *testing.T, homeDir string) string {
	t.Helper()

	configPath := operatorsettings.DefaultConfigPath(homeDir)
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read operator config at %q: %v", configPath, err)
	}
	var document struct {
		BackendScopeID string `json:"backendScopeID"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode operator config: %v\ncontent:\n%s", err, raw)
	}
	return document.BackendScopeID
}

func findInputInventoryCase(cases []operatorsettings.InputCase, id string) (operatorsettings.InputCase, bool) {
	for _, inputCase := range cases {
		if inputCase.ID == id {
			return inputCase, true
		}
	}
	return operatorsettings.InputCase{}, false
}

func writeOperatorSettingsFixtureToTemp(t *testing.T, directory, rel string) string {
	t.Helper()

	fixturePath := testutil.MustRepoPath(t, filepath.ToSlash(filepath.Join(operatorConfigFixturesRelativeDir, rel)))
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixturePath, err)
	}
	configPath := filepath.Join(directory, "input-inventory-config.json")
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write fixture config: %v", err)
	}
	return configPath
}
