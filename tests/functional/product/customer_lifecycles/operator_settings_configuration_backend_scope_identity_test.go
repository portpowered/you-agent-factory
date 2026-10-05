package customer_lifecycles_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/portpowered/infinite-you/pkg/root"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	identityActivationGeneratedUUID = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	identityActivationExistingScope = "local-11111111-1111-4111-8111-111111111111"
)

// TestCLIInitializationPersistsBackendScopeIdentity proves
// backend-scope identity resolution and persistence activate through public
// Operator Settings surfaces after runtime lifecycle on the shared process.
func testOperatorsettingsconfigurationCLIInitializationPersistsBackendScopeIdentity(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	fixture := ensureSharedOperatorSettingsFixture(t)
	fixture.withOperatorSettingsRoute(
		t,
		"generated backend scope",
		homeDir,
		homeDir,
		identityActivationGeneratedUUID,
		nil,
		func(_ *operatorSettingsEffectRoute) {
			runOperatorSettingsLifecycleInitialization(t, fixture.process, homeDir)

			got := readBackendScopeIDFromHome(t, homeDir)

			if _, err := uuid.Parse(strings.TrimPrefix(got, operatorsettings.LocalBackendScopePrefix)); err != nil || !strings.HasPrefix(got, operatorsettings.LocalBackendScopePrefix) {
				t.Fatalf("backendScopeID = %q, want a valid local UUID", got)
			}

		},
	)
}

// TestCLIInitializationReusesBackendScopeIdentity
// proves persisted backend-scope identity is reused through public Operator
// Settings surfaces after runtime lifecycle without regenerating scope IDs.
func testOperatorsettingsconfigurationCLIInitializationReusesBackendScopeIdentity(t *testing.T) {
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
			runOperatorSettingsLifecycleInitialization(t, fixture.process, homeDir)

			if got := readBackendScopeIDFromHome(t, homeDir); got != identityActivationExistingScope {
				t.Fatalf("backendScopeID = %q, want reused scope %q", got, identityActivationExistingScope)
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

	configPath := filepath.Join(homeDir, ".you-agent-factory", "config.json")
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
