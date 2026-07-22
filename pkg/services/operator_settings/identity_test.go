package operatorsettings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureLocalBackendScope_GeneratesAndPersistsMissingScope(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := DefaultConfigPath(homeDir)

	first, err := EnsureLocalBackendScope(testFiles, testCreateTemp, testIDGenerator, configPath)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope() error = %v", err)
	}
	if first.Outcome != BackendScopeOutcomeGenerated {
		t.Fatalf("outcome = %q, want %q", first.Outcome, BackendScopeOutcomeGenerated)
	}
	if !IsLocalBackendScopeID(first.BackendScopeID) {
		t.Fatalf("backendScopeID = %q, want local-<uuid>", first.BackendScopeID)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var persisted struct {
		BackendScopeID string `json:"backendScopeID"`
	}
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if persisted.BackendScopeID != first.BackendScopeID {
		t.Fatalf("persisted backendScopeID = %q, want %q", persisted.BackendScopeID, first.BackendScopeID)
	}

	second, err := EnsureLocalBackendScope(testFiles, testCreateTemp, testIDGenerator, configPath)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope() second call error = %v", err)
	}
	if second.Outcome != BackendScopeOutcomeReused {
		t.Fatalf("second outcome = %q, want %q", second.Outcome, BackendScopeOutcomeReused)
	}
	if second.BackendScopeID != first.BackendScopeID {
		t.Fatalf("second backendScopeID = %q, want %q", second.BackendScopeID, first.BackendScopeID)
	}
}

func TestEnsureLocalBackendScopeUsesInjectedIdentityGenerator(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	const generated = "12345678-1234-1234-1234-123456789abc"

	resolved, err := EnsureLocalBackendScope(
		testFiles,
		testCreateTemp,
		func() string { return generated },
		configPath,
	)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope() error = %v", err)
	}
	if resolved.BackendScopeID != LocalBackendScopePrefix+generated {
		t.Fatalf("BackendScopeID = %q, want injected identity", resolved.BackendScopeID)
	}
}

func TestEnsureLocalBackendScope_PreservesExistingDefaults(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{
  "defaults": {
    "workerModelProvider": "codex",
    "workerModel": "gpt-5-codex"
  }
}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	resolved, err := EnsureLocalBackendScope(testFiles, testCreateTemp, testIDGenerator, configPath)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope() error = %v", err)
	}
	if resolved.Outcome != BackendScopeOutcomeGenerated {
		t.Fatalf("outcome = %q, want %q", resolved.Outcome, BackendScopeOutcomeGenerated)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var persisted map[string]json.RawMessage
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	defaultsRaw, ok := persisted["defaults"]
	if !ok {
		t.Fatal("expected defaults block to remain in system config")
	}
	var defaults struct {
		WorkerModelProvider string `json:"workerModelProvider"`
		WorkerModel         string `json:"workerModel"`
	}
	if err := json.Unmarshal(defaultsRaw, &defaults); err != nil {
		t.Fatalf("Unmarshal defaults: %v", err)
	}
	if defaults.WorkerModelProvider != "codex" || defaults.WorkerModel != "gpt-5-codex" {
		t.Fatalf("defaults = %#v, want codex/gpt-5-codex preserved", defaults)
	}
}

func TestDeriveProviderBackendScopeID_DistinctForDifferentBoundaries(t *testing.T) {
	t.Parallel()

	first := DeriveProviderBackendScopeID("codex", "account", "workspace-a")
	second := DeriveProviderBackendScopeID("codex", "account", "workspace-b")
	third := DeriveProviderBackendScopeID("claude", "account", "workspace-a")
	if first == second || first == third || second == third {
		t.Fatalf("provider scopes should differ: %q %q %q", first, second, third)
	}
	if strings.HasPrefix(first, LocalBackendScopePrefix) {
		t.Fatalf("provider scope = %q, want non-local prefix", first)
	}
}

func TestResolvedBackendScope_DiagnosticsLine(t *testing.T) {
	t.Parallel()

	scope := ResolvedBackendScope{
		BackendScopeID: "local-aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
		Outcome:        BackendScopeOutcomeReused,
		ConfigPath:     "/tmp/config.json",
	}
	line := scope.DiagnosticsLine()
	if !strings.Contains(line, "outcome=reused") {
		t.Fatalf("diagnostics line = %q, want reused outcome", line)
	}
	if !strings.Contains(line, scope.BackendScopeID) {
		t.Fatalf("diagnostics line = %q, want backend scope id", line)
	}

	unset := ResolvedBackendScope{Outcome: BackendScopeOutcomeGenerated}.DiagnosticsLine()
	if !strings.Contains(unset, "backendScopeID=unset") {
		t.Fatalf("diagnostics line = %q, want unset backend scope", unset)
	}
}

func TestEnsureLocalBackendScope_ReusesPersistedScope(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := DefaultConfigPath(homeDir)
	existing := "local-aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{"backendScopeID":"`+existing+`"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	resolved, err := EnsureLocalBackendScope(testFiles, testCreateTemp, testIDGenerator, configPath)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope() error = %v", err)
	}
	if resolved.Outcome != BackendScopeOutcomeReused {
		t.Fatalf("outcome = %q, want %q", resolved.Outcome, BackendScopeOutcomeReused)
	}
	if resolved.BackendScopeID != existing {
		t.Fatalf("backendScopeID = %q, want %q", resolved.BackendScopeID, existing)
	}
}

func TestEnsureLocalBackendScope_RejectsEmptyConfigPath(t *testing.T) {
	t.Parallel()

	if _, err := EnsureLocalBackendScope(testFiles, testCreateTemp, testIDGenerator, "   "); err == nil {
		t.Fatal("expected error for empty config path")
	}
}

func TestEnsureLocalBackendScope_RejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{invalid`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := EnsureLocalBackendScope(testFiles, testCreateTemp, testIDGenerator, configPath); err == nil {
		t.Fatal("expected parse error for invalid config JSON")
	}
}

func TestPersistBackendScopeID_RejectsInvalidScope(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := DefaultConfigPath(homeDir)

	if err := persistBackendScopeID(testFiles, testCreateTemp, configPath, "not-a-local-scope"); err == nil {
		t.Fatal("expected error for invalid backend scope id")
	}
	if err := persistBackendScopeID(testFiles, testCreateTemp, configPath, ""); err == nil {
		t.Fatal("expected error for empty backend scope id")
	}
}

func TestGenerateLocalBackendScopeID_AndValidationHelpers(t *testing.T) {
	t.Parallel()

	generated := GenerateLocalBackendScopeID(testIDGenerator)
	if !IsLocalBackendScopeID(generated) {
		t.Fatalf("generated scope = %q, want local-<uuid>", generated)
	}
	if IsLocalBackendScopeID("provider-codex-account-workspace") {
		t.Fatal("provider scope should not match local backend scope pattern")
	}
	if IsLocalBackendScopeID("local-not-a-uuid") {
		t.Fatal("malformed local scope should be rejected")
	}
}

func TestDeriveProviderBackendScopeID_SanitizesEmptySegments(t *testing.T) {
	t.Parallel()

	scope := DeriveProviderBackendScopeID(" ", "", "workspace")
	if !strings.Contains(scope, "unknown") {
		t.Fatalf("provider scope = %q, want unknown placeholders for empty segments", scope)
	}
}

func TestEnsureLocalBackendScope_TreatsWhitespaceConfigAsMissingScope(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("   \n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	resolved, err := EnsureLocalBackendScope(testFiles, testCreateTemp, testIDGenerator, configPath)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope() error = %v", err)
	}
	if resolved.Outcome != BackendScopeOutcomeGenerated {
		t.Fatalf("outcome = %q, want %q", resolved.Outcome, BackendScopeOutcomeGenerated)
	}
	if !IsLocalBackendScopeID(resolved.BackendScopeID) {
		t.Fatalf("backendScopeID = %q, want generated local scope", resolved.BackendScopeID)
	}
}
