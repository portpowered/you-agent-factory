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
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type configInitResult struct {
	ConfigPath        string                  `json:"configPath"`
	PackagedFactories []packagedFactoryResult `json:"packagedFactories"`
}

type packagedFactoryResult struct {
	Name       string `json:"name"`
	FactoryDir string `json:"factoryDirectory"`
}

var packagedFactoryNames = []string{
	factorydefinitions.PackagedDeepResearchFactoryName,
	factorydefinitions.PackagedFusionFactoryName,
	factorydefinitions.PackagedGoalFactoryName,
	factorydefinitions.PackagedQuorumFactoryName,
	factorydefinitions.PackagedReviewFactoryName,
	factorydefinitions.PackagedSubagentFactoryName,
	factorydefinitions.PackagedTTSFactoryName,
}

func TestFreshInstall_EmptyHomeProducesDocumentedCustomerOutcome(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := session.Run(ctx, "config", "init")
	session.RequireSuccess(t, "fresh-install-config-init", result, err)

	configPath := filepath.Join(session.HomeDir, ".you-agent-factory", "config.json")
	if _, statErr := os.Stat(configPath); statErr != nil {
		t.Fatalf("operator config %q: %v", configPath, statErr)
	}
	configData, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("read operator config %q: %v", configPath, readErr)
	}
	var configDocument map[string]any
	if decodeErr := json.Unmarshal(configData, &configDocument); decodeErr != nil {
		t.Fatalf("decode operator config %q: %v", configPath, decodeErr)
	}

	output := result.Stdout
	if !strings.Contains(output, "Created system config at "+configPath) {
		t.Fatalf("stdout = %q, want created system config message for %q", output, configPath)
	}
	for _, name := range packagedFactoryNames {
		if !strings.Contains(output, "Created packaged factory "+name) {
			t.Fatalf("stdout = %q, want created packaged factory message for %q", output, name)
		}
	}

	jsonResult, jsonErr := session.Run(ctx, "--json", "config", "init")
	session.RequireSuccess(t, "fresh-install-config-init-json", jsonResult, jsonErr)
	initResult := decodeConfigInitResult(t, jsonResult.Stdout)
	if len(initResult.PackagedFactories) != len(packagedFactoryNames) {
		t.Fatalf("packaged Factories = %#v, want %d entries", initResult.PackagedFactories, len(packagedFactoryNames))
	}
	for _, factory := range initResult.PackagedFactories {
		if _, statErr := os.Stat(factory.FactoryDir); statErr != nil {
			t.Fatalf("packaged factory dir %q: %v", factory.FactoryDir, statErr)
		}
		if _, loadErr := support.LoadedFactory(t, factory.FactoryDir); loadErr != nil {
			t.Fatalf("LoadRuntimeConfigFromFactoryDir(%q): %v", factory.Name, loadErr)
		}
	}
}

func TestMigratedInstall_ExistingConfigIsPreservedWithoutRewrite(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	initial, err := session.Run(ctx, "--json", "config", "init")
	session.RequireSuccess(t, "migrated-install-initial-config", initial, err)
	configPath := decodeConfigInitResult(t, initial.Stdout).ConfigPath
	original := []byte(`{
  "defaults": {
    "workerModelProvider": "codex",
    "workerModel": "gpt-5-codex"
  }
}`)
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}

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

func decodeConfigInitResult(t *testing.T, output string) configInitResult {
	t.Helper()
	var result configInitResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode config init JSON: %v\nstdout:\n%s", err, output)
	}
	if result.ConfigPath == "" {
		t.Fatalf("config init JSON omitted configPath: %s", output)
	}
	return result
}

func TestMigratedInstall_MaterializesMissingPackagedDefaultsWithoutCorruption(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	first, err := session.Run(ctx, "--json", "config", "init")
	session.RequireSuccess(t, "migrated-install-baseline", first, err)
	initial := decodeConfigInitResult(t, first.Stdout)
	goalDir := packagedFactoryDir(t, initial, "@you/goal")
	editedWorker := filepath.Join(goalDir, "workers", "goal-executor", "AGENTS.md")
	editedBody := "customer edited goal worker body for migrated install\n"
	if err := os.WriteFile(editedWorker, []byte(editedBody), 0o644); err != nil {
		t.Fatalf("WriteFile(edited worker): %v", err)
	}

	ttsDir := packagedFactoryDir(t, initial, "@you/tts")
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
	if _, loadErr := support.LoadedFactory(t, ttsDir); loadErr != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir(@you/tts): %v", loadErr)
	}
}

func TestMigratedInstall_JSONReportsSkippedAndCreatedOutcomes(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	first, err := session.Run(ctx, "--json", "config", "init")
	session.RequireSuccess(t, "migrated-install-json-baseline", first, err)
	ttsDir := packagedFactoryDir(t, decodeConfigInitResult(t, first.Stdout), "@you/tts")
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

func packagedFactoryDir(t *testing.T, result configInitResult, name string) string {
	t.Helper()
	for _, factory := range result.PackagedFactories {
		if factory.Name == name {
			return factory.FactoryDir
		}
	}
	t.Fatalf("config init result omitted packaged Factory %q: %#v", name, result.PackagedFactories)
	return ""
}
