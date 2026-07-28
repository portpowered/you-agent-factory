package identityinputinventory

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

type renameFailingFileSystem struct{ operatorsettings.FileSystem }

func (f renameFailingFileSystem) Rename(_, _ string) error {
	return errors.New("injected rename failure")
}

type chmodRecordingFileSystem struct {
	operatorsettings.FileSystem
	mode fs.FileMode
}

func (f *chmodRecordingFileSystem) Chmod(path string, mode fs.FileMode) error {
	f.mode = mode
	return f.FileSystem.Chmod(path, mode)
}

func TestEnsureLocalBackendScope_GeneratesAndPersistsMissingScope(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)

	first, err := ensureTestBackendScope(configPath)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope() error = %v", err)
	}
	if first.Outcome != operatorsettings.BackendScopeOutcomeGenerated {
		t.Fatalf("outcome = %q, want %q", first.Outcome, operatorsettings.BackendScopeOutcomeGenerated)
	}
	if !operatorsettings.IsLocalBackendScopeID(first.BackendScopeID) {
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

	second, err := ensureTestBackendScope(configPath)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope() second call error = %v", err)
	}
	if second.Outcome != operatorsettings.BackendScopeOutcomeReused {
		t.Fatalf("second outcome = %q, want %q", second.Outcome, operatorsettings.BackendScopeOutcomeReused)
	}
	if second.BackendScopeID != first.BackendScopeID {
		t.Fatalf("second backendScopeID = %q, want %q", second.BackendScopeID, first.BackendScopeID)
	}
}

func TestEnsureLocalBackendScopeUsesInjectedIdentityGenerator(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	const generated = "12345678-1234-1234-1234-123456789abc"

	resolved, err := operatorsettings.EnsureLocalBackendScope(
		testFiles,
		testCreateTemp,
		func() string { return generated },
		decodeTestConfig,
		encodeTestConfig,
		configPath,
	)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope() error = %v", err)
	}
	if resolved.BackendScopeID != operatorsettings.LocalBackendScopePrefix+generated {
		t.Fatalf("BackendScopeID = %q, want injected identity", resolved.BackendScopeID)
	}
}

func TestEnsureLocalBackendScope_PreservesExistingSettings(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{
  "defaults": {
    "workerModelProvider": "codex",
    "workerModel": "gpt-5-codex"
  },
  "workerPresets": [{
    "id": "research",
    "modelProvider": "codex",
    "model": "gpt-5-mini",
    "reasoningEffort": "high"
  }]
}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	resolved, err := ensureTestBackendScope(configPath)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope() error = %v", err)
	}
	if resolved.Outcome != operatorsettings.BackendScopeOutcomeGenerated {
		t.Fatalf("outcome = %q, want %q", resolved.Outcome, operatorsettings.BackendScopeOutcomeGenerated)
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
	var presets []operatorsettings.WorkerPreset
	if err := json.Unmarshal(persisted["workerPresets"], &presets); err != nil {
		t.Fatalf("Unmarshal workerPresets: %v", err)
	}
	wantPreset := operatorsettings.WorkerPreset{ID: "research", ModelProvider: "CODEX", Model: "gpt-5-mini", ReasoningEffort: "high"}
	if len(presets) != 1 || presets[0] != wantPreset {
		t.Fatalf("workerPresets = %#v, want %#v", presets, []operatorsettings.WorkerPreset{wantPreset})
	}
}

func TestEnsureLocalBackendScope_WriteFailureLeavesOriginalDocument(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	original := []byte(`{"defaults":{"workerModelProvider":"codex"}}`)
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := operatorsettings.EnsureLocalBackendScope(
		renameFailingFileSystem{FileSystem: testFiles},
		testCreateTemp,
		testIDGenerator,
		decodeTestConfig,
		encodeTestConfig,
		configPath,
	)
	if err == nil || !strings.Contains(err.Error(), "injected rename failure") || !strings.Contains(err.Error(), "persist generated backend scope ID to system config") {
		t.Fatalf("EnsureLocalBackendScope() error = %v, want path-aware rename failure", err)
	}
	got, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("config after failed replace = %q, want original %q", got, original)
	}
}

func TestEnsureLocalBackendScope_RestrictsPersistedFilePermissions(t *testing.T) {
	files := &chmodRecordingFileSystem{FileSystem: testFiles}
	configPath := filepath.Join(t.TempDir(), "config.json")
	if _, err := operatorsettings.EnsureLocalBackendScope(
		files,
		testCreateTemp,
		testIDGenerator,
		decodeTestConfig,
		encodeTestConfig,
		configPath,
	); err != nil {
		t.Fatalf("EnsureLocalBackendScope() error = %v", err)
	}
	if files.mode.Perm() != 0o600 {
		t.Fatalf("persisted file permissions = %o, want 600", files.mode.Perm())
	}
}

func TestDeriveProviderBackendScopeID_DistinctForDifferentBoundaries(t *testing.T) {
	t.Parallel()

	first := operatorsettings.DeriveProviderBackendScopeID("codex", "account", "workspace-a")
	second := operatorsettings.DeriveProviderBackendScopeID("codex", "account", "workspace-b")
	third := operatorsettings.DeriveProviderBackendScopeID("claude", "account", "workspace-a")
	if first == second || first == third || second == third {
		t.Fatalf("provider scopes should differ: %q %q %q", first, second, third)
	}
	if strings.HasPrefix(first, operatorsettings.LocalBackendScopePrefix) {
		t.Fatalf("provider scope = %q, want non-local prefix", first)
	}
}

func TestResolvedBackendScope_DiagnosticsLine(t *testing.T) {
	t.Parallel()

	scope := operatorsettings.ResolvedBackendScope{
		BackendScopeID: "local-aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
		Outcome:        operatorsettings.BackendScopeOutcomeReused,
		ConfigPath:     "/tmp/config.json",
	}
	line := scope.DiagnosticsLine()
	if !strings.Contains(line, "outcome=reused") {
		t.Fatalf("diagnostics line = %q, want reused outcome", line)
	}
	if !strings.Contains(line, scope.BackendScopeID) {
		t.Fatalf("diagnostics line = %q, want backend scope id", line)
	}

	unset := operatorsettings.ResolvedBackendScope{Outcome: operatorsettings.BackendScopeOutcomeGenerated}.DiagnosticsLine()
	if !strings.Contains(unset, "backendScopeID=unset") {
		t.Fatalf("diagnostics line = %q, want unset backend scope", unset)
	}
}

func TestEnsureLocalBackendScope_ReusesPersistedScope(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	existing := "local-aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{"backendScopeID":"`+existing+`"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	resolved, err := ensureTestBackendScope(configPath)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope() error = %v", err)
	}
	if resolved.Outcome != operatorsettings.BackendScopeOutcomeReused {
		t.Fatalf("outcome = %q, want %q", resolved.Outcome, operatorsettings.BackendScopeOutcomeReused)
	}
	if resolved.BackendScopeID != existing {
		t.Fatalf("backendScopeID = %q, want %q", resolved.BackendScopeID, existing)
	}
}

func TestEnsureLocalBackendScope_RejectsEmptyConfigPath(t *testing.T) {
	t.Parallel()

	if _, err := ensureTestBackendScope("   "); err == nil {
		t.Fatal("expected error for empty config path")
	}
}

func TestEnsureLocalBackendScope_RejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{invalid`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := ensureTestBackendScope(configPath); err == nil {
		t.Fatal("expected parse error for invalid config JSON")
	}
}

func TestPersistBackendScopeID_RejectsInvalidScope(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)

	if err := persistBackendScopeID(testFiles, testCreateTemp, encodeTestConfig, configPath, operatorsettings.Config{BackendScopeID: "not-a-local-scope"}); err == nil {
		t.Fatal("expected error for invalid backend scope id")
	}
	if err := persistBackendScopeID(testFiles, testCreateTemp, encodeTestConfig, configPath, operatorsettings.Config{}); err == nil {
		t.Fatal("expected error for empty backend scope id")
	}
}

func TestGenerateLocalBackendScopeID_AndValidationHelpers(t *testing.T) {
	t.Parallel()

	generated := operatorsettings.GenerateLocalBackendScopeID(testIDGenerator)
	if !operatorsettings.IsLocalBackendScopeID(generated) {
		t.Fatalf("generated scope = %q, want local-<uuid>", generated)
	}
	if operatorsettings.IsLocalBackendScopeID("provider-codex-account-workspace") {
		t.Fatal("provider scope should not match local backend scope pattern")
	}
	if operatorsettings.IsLocalBackendScopeID("local-not-a-uuid") {
		t.Fatal("malformed local scope should be rejected")
	}
}

func TestDeriveProviderBackendScopeID_SanitizesEmptySegments(t *testing.T) {
	t.Parallel()

	scope := operatorsettings.DeriveProviderBackendScopeID(" ", "", "workspace")
	if !strings.Contains(scope, "unknown") {
		t.Fatalf("provider scope = %q, want unknown placeholders for empty segments", scope)
	}
}

func TestEnsureLocalBackendScope_RejectsWhitespaceConfig(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("   \n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := ensureTestBackendScope(configPath); err == nil || !strings.Contains(err.Error(), configPath) {
		t.Fatalf("EnsureLocalBackendScope() error = %v, want path-aware parse failure", err)
	}
}
