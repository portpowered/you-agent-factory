package acceptance

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	factorypackages "github.com/portpowered/infinite-you/pkg/factory/packages"
)

func TestFreshInstall_EmptyHomeProducesDocumentedCustomerOutcome(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := session.Run(ctx, "config", "init")
	session.RequireSuccess(t, "fresh-install-config-init", result, err)

	configPath := defaultpaths.OperatorConfigPath(session.HomeDir)
	if _, statErr := os.Stat(configPath); statErr != nil {
		t.Fatalf("operator config %q: %v", configPath, statErr)
	}
	if _, loadErr := operatorconfig.LoadFileConfig(configPath); loadErr != nil {
		t.Fatalf("LoadFileConfig(%q): %v", configPath, loadErr)
	}

	output := result.Stdout
	if !strings.Contains(output, "Created system config at "+configPath) {
		t.Fatalf("stdout = %q, want created system config message for %q", output, configPath)
	}
	for _, name := range factorypackages.Names() {
		if !strings.Contains(output, "Created packaged factory "+name) {
			t.Fatalf("stdout = %q, want created packaged factory message for %q", output, name)
		}
	}

	namedFactoriesRoot := defaultpaths.NamedFactoriesRoot(session.HomeDir)
	for _, name := range factorypackages.Names() {
		factoryDir, mapErr := factoryconfig.MapNamedFactoryDir(namedFactoriesRoot, name)
		if mapErr != nil {
			t.Fatalf("MapNamedFactoryDir(%q): %v", name, mapErr)
		}
		if _, statErr := os.Stat(factoryDir); statErr != nil {
			t.Fatalf("packaged factory dir %q: %v", factoryDir, statErr)
		}
		if _, loadErr := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil); loadErr != nil {
			t.Fatalf("LoadRuntimeConfigFromFactoryDir(%q): %v", name, loadErr)
		}
	}
}

func TestMigratedInstall_ExistingConfigIsPreservedWithoutRewrite(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)

	configPath := defaultpaths.OperatorConfigPath(session.HomeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(config parent): %v", err)
	}
	original := []byte(`{
  "defaults": {
    "workerModelProvider": "codex",
    "workerModel": "gpt-5-codex"
  }
}`)
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := session.Run(ctx, "config", "init")
	session.RequireSuccess(t, "migrated-install-existing-config", result, err)

	if !strings.Contains(result.Stdout, "System config already present at "+configPath) {
		t.Fatalf("stdout = %q, want already-present system config message for %q", result.Stdout, configPath)
	}

	got, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("ReadFile(config): %v", readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("operator config changed on migrated install:\nwant:\n%s\ngot:\n%s", original, got)
	}
}

func TestMigratedInstall_LegacyNamedFactoryMovesToCanonicalRoot(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)
	definition, ok := factorypackages.Lookup("@you/goal")
	if !ok {
		t.Fatal("expected @you/goal packaged definition")
	}
	legacyDir, err := factoryconfig.PersistNamedFactory(defaultpaths.LegacyNamedFactoriesRoot(session.HomeDir), definition.Name, definition.JSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory(legacy): %v", err)
	}
	markerPath := filepath.Join(legacyDir, "customer-edit.txt")
	if err := os.WriteFile(markerPath, []byte("preserve this edit\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(customer edit): %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := session.Run(ctx, "config", "init")
	session.RequireSuccess(t, "migrated-install-legacy-named-factory", result, err)

	canonicalDir, err := factoryconfig.MapNamedFactoryDir(defaultpaths.NamedFactoriesRoot(session.HomeDir), definition.Name)
	if err != nil {
		t.Fatalf("MapNamedFactoryDir(canonical): %v", err)
	}
	if _, err := os.Stat(legacyDir); !os.IsNotExist(err) {
		t.Fatalf("legacy factory directory still exists: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(canonicalDir, "customer-edit.txt")); err != nil || string(got) != "preserve this edit\n" {
		t.Fatalf("migrated customer edit = %q, %v; want preserved content", got, err)
	}
}

func TestMigratedInstall_MaterializesMissingPackagedDefaultsWithoutCorruption(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	first, err := session.Run(ctx, "config", "init")
	session.RequireSuccess(t, "migrated-install-baseline", first, err)

	namedFactoriesRoot := defaultpaths.NamedFactoriesRoot(session.HomeDir)
	goalDir, mapErr := factoryconfig.MapNamedFactoryDir(namedFactoriesRoot, "@you/goal")
	if mapErr != nil {
		t.Fatalf("MapNamedFactoryDir(@you/goal): %v", mapErr)
	}
	editedWorker := filepath.Join(goalDir, "workers", "goal-executor", "AGENTS.md")
	editedBody := "customer edited goal worker body for migrated install\n"
	if err := os.WriteFile(editedWorker, []byte(editedBody), 0o644); err != nil {
		t.Fatalf("WriteFile(edited worker): %v", err)
	}

	ttsDir, mapErr := factoryconfig.MapNamedFactoryDir(namedFactoriesRoot, "@you/tts")
	if mapErr != nil {
		t.Fatalf("MapNamedFactoryDir(@you/tts): %v", mapErr)
	}
	if err := os.RemoveAll(ttsDir); err != nil {
		t.Fatalf("RemoveAll(@you/tts): %v", err)
	}

	second, err := session.Run(ctx, "config", "init")
	session.RequireSuccess(t, "migrated-install-partial-upgrade", second, err)

	output := second.Stdout
	if !strings.Contains(output, "System config already present at") {
		t.Fatalf("stdout = %q, want already-present system config message", output)
	}
	if !strings.Contains(output, "Created packaged factory @you/tts at "+ttsDir) {
		t.Fatalf("stdout = %q, want recreated @you/tts message for %q", output, ttsDir)
	}
	if !strings.Contains(output, "Packaged factory @you/goal already present at "+goalDir) {
		t.Fatalf("stdout = %q, want already-present @you/goal message for %q", output, goalDir)
	}

	gotBody, readErr := os.ReadFile(editedWorker)
	if readErr != nil {
		t.Fatalf("ReadFile(edited worker): %v", readErr)
	}
	if string(gotBody) != editedBody {
		t.Fatalf("edited worker changed:\nwant:\n%s\ngot:\n%s", editedBody, gotBody)
	}
	if _, loadErr := factoryconfig.LoadRuntimeConfigFromFactoryDir(ttsDir, nil); loadErr != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir(@you/tts): %v", loadErr)
	}
}

func TestMigratedInstall_JSONReportsSkippedAndCreatedOutcomes(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	first, err := session.Run(ctx, "config", "init")
	session.RequireSuccess(t, "migrated-install-json-baseline", first, err)

	namedFactoriesRoot := defaultpaths.NamedFactoriesRoot(session.HomeDir)
	ttsDir, mapErr := factoryconfig.MapNamedFactoryDir(namedFactoriesRoot, "@you/tts")
	if mapErr != nil {
		t.Fatalf("MapNamedFactoryDir(@you/tts): %v", mapErr)
	}
	if err := os.RemoveAll(ttsDir); err != nil {
		t.Fatalf("RemoveAll(@you/tts): %v", err)
	}

	second, err := session.Run(ctx, "--json", "config", "init")
	session.RequireSuccess(t, "migrated-install-json-upgrade", second, err)

	var payload struct {
		SystemConfigOutcome string `json:"systemConfigOutcome"`
		PackagedFactories   []struct {
			Name    string `json:"name"`
			Outcome string `json:"outcome"`
		} `json:"packagedFactories"`
	}
	if unmarshalErr := json.Unmarshal([]byte(strings.TrimSpace(second.Stdout)), &payload); unmarshalErr != nil {
		t.Fatalf("Unmarshal JSON stdout: %v\n%s", unmarshalErr, second.Stdout)
	}
	if payload.SystemConfigOutcome != "skipped" {
		t.Fatalf("systemConfigOutcome = %q, want skipped", payload.SystemConfigOutcome)
	}

	outcomes := make(map[string]string, len(payload.PackagedFactories))
	for _, factory := range payload.PackagedFactories {
		outcomes[factory.Name] = factory.Outcome
	}
	if outcomes["@you/tts"] != "created" {
		t.Fatalf("@you/tts outcome = %q, want created", outcomes["@you/tts"])
	}
	if outcomes["@you/goal"] != "skipped" {
		t.Fatalf("@you/goal outcome = %q, want skipped", outcomes["@you/goal"])
	}
}
