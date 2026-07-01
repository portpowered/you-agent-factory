package systemconfig

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

	first, err := EnsureLocalBackendScope(configPath)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope() error = %v", err)
	}
	if first.Outcome != OutcomeGenerated {
		t.Fatalf("outcome = %q, want %q", first.Outcome, OutcomeGenerated)
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

	second, err := EnsureLocalBackendScope(configPath)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope() second call error = %v", err)
	}
	if second.Outcome != OutcomeReused {
		t.Fatalf("second outcome = %q, want %q", second.Outcome, OutcomeReused)
	}
	if second.BackendScopeID != first.BackendScopeID {
		t.Fatalf("second backendScopeID = %q, want %q", second.BackendScopeID, first.BackendScopeID)
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

	resolved, err := EnsureLocalBackendScope(configPath)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope() error = %v", err)
	}
	if resolved.Outcome != OutcomeGenerated {
		t.Fatalf("outcome = %q, want %q", resolved.Outcome, OutcomeGenerated)
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

func TestEnsureLocalBackendScope_ReusesConfiguredScope(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := DefaultConfigPath(homeDir)
	existing := "local-11111111-2222-4333-8444-555555555555"
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{"backendScopeID":"`+existing+`"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	resolved, err := EnsureLocalBackendScope(configPath)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope() error = %v", err)
	}
	if resolved.Outcome != OutcomeReused {
		t.Fatalf("outcome = %q, want %q", resolved.Outcome, OutcomeReused)
	}
	if resolved.BackendScopeID != existing {
		t.Fatalf("backendScopeID = %q, want %q", resolved.BackendScopeID, existing)
	}
}

func TestEnsureLocalBackendScope_PersistFailureReturnsActionableError(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, "blocked")
	if err := os.MkdirAll(configDir, 0o555); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	configPath := filepath.Join(configDir, "config.json")

	_, err := EnsureLocalBackendScope(configPath)
	if err == nil {
		t.Fatal("expected persistence failure")
	}
	if !strings.Contains(err.Error(), configPath) {
		t.Fatalf("error = %q, want config path %q", err.Error(), configPath)
	}
	if !strings.Contains(err.Error(), "stable backendScopeID") {
		t.Fatalf("error = %q, want actionable backend scope message", err.Error())
	}
}

func TestEnsureLocalBackendScope_MalformedLocalScopeReturnsConfigError(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{"backendScopeID":"local-not-a-uuid"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := EnsureLocalBackendScope(configPath)
	if err == nil {
		t.Fatal("expected malformed local backend scope error")
	}
	if !strings.Contains(err.Error(), configPath) {
		t.Fatalf("error = %q, want config path %q", err.Error(), configPath)
	}
	if !strings.Contains(err.Error(), "malformed backendScopeID") {
		t.Fatalf("error = %q, want malformed backend scope message", err.Error())
	}
}

func TestEnsureLocalBackendScope_BlankConfiguredScopeGeneratesLocalUUID(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{"backendScopeID":"   "}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	resolved, err := EnsureLocalBackendScope(configPath)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope() error = %v", err)
	}
	if resolved.Outcome != OutcomeGenerated {
		t.Fatalf("outcome = %q, want %q", resolved.Outcome, OutcomeGenerated)
	}
	if !IsLocalBackendScopeID(resolved.BackendScopeID) {
		t.Fatalf("backendScopeID = %q, want local-<uuid>", resolved.BackendScopeID)
	}
}

func TestEnsureLocalBackendScope_ReusesExplicitNonLocalScope(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := DefaultConfigPath(homeDir)
	existing := "cloud-review-scope"
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{"backendScopeID":"`+existing+`"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	resolved, err := EnsureLocalBackendScope(configPath)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope() error = %v", err)
	}
	if resolved.Outcome != OutcomeReused {
		t.Fatalf("outcome = %q, want %q", resolved.Outcome, OutcomeReused)
	}
	if resolved.BackendScopeID != existing {
		t.Fatalf("backendScopeID = %q, want %q", resolved.BackendScopeID, existing)
	}
}

func TestEnsureLocalBackendScope_MalformedConfigNamesPath(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"backendScopeID":`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := EnsureLocalBackendScope(configPath)
	if err == nil {
		t.Fatal("expected malformed config error")
	}
	if !strings.Contains(err.Error(), configPath) {
		t.Fatalf("error = %q, want path %q", err.Error(), configPath)
	}
}

func TestResolvedBackendScope_DiagnosticsLineRedactsNothingForScopeID(t *testing.T) {
	t.Parallel()

	line := (ResolvedBackendScope{
		BackendScopeID: "local-11111111-2222-4333-8444-555555555555",
		Outcome:        OutcomeGenerated,
		ConfigPath:     "/tmp/config.json",
	}).DiagnosticsLine()
	if !strings.Contains(line, "outcome=generated") {
		t.Fatalf("line = %q, want generated outcome", line)
	}
	if !strings.Contains(line, "local-11111111-2222-4333-8444-555555555555") {
		t.Fatalf("line = %q, want backend scope id", line)
	}
	if strings.Contains(line, "workerModel") {
		t.Fatalf("line = %q, should not leak unrelated config fields", line)
	}
}

func TestEnsureLocalBackendScope_RequiresConfigPath(t *testing.T) {
	t.Parallel()

	_, err := EnsureLocalBackendScope("   ")
	if err == nil {
		t.Fatal("expected missing config path error")
	}
	if !strings.Contains(err.Error(), "system config path is required") {
		t.Fatalf("error = %q, want required path message", err.Error())
	}
}

func TestResolvedBackendScope_DiagnosticsLineUnsetScope(t *testing.T) {
	t.Parallel()

	line := (ResolvedBackendScope{
		BackendScopeID: "   ",
		Outcome:        OutcomeReused,
		ConfigPath:     "/tmp/config.json",
	}).DiagnosticsLine()
	if !strings.Contains(line, "backendScopeID=unset") {
		t.Fatalf("line = %q, want unset backend scope id", line)
	}
}

func TestReadConfigMap_EmptyFileReturnsEmptyMap(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte("  \n  "), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	configMap, err := readConfigMap(configPath)
	if err != nil {
		t.Fatalf("readConfigMap() error = %v", err)
	}
	if len(configMap) != 0 {
		t.Fatalf("configMap = %#v, want empty map", configMap)
	}
}

func TestReadConfigMap_InvalidJSONReturnsError(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"backendScopeID":`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := readConfigMap(configPath)
	if err == nil {
		t.Fatal("expected invalid JSON error")
	}
	if !strings.Contains(err.Error(), configPath) {
		t.Fatalf("error = %q, want path %q", err.Error(), configPath)
	}
}

func TestLoadBackendScopeID_ReadErrorIncludesPath(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{}`), 0o000); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(configPath, 0o600)
	})

	_, err := loadBackendScopeID(configPath)
	if err == nil {
		t.Fatal("expected read error")
	}
	if !strings.Contains(err.Error(), configPath) {
		t.Fatalf("error = %q, want path %q", err.Error(), configPath)
	}
}

func TestPersistBackendScopeID_RejectsEmptyAndInvalidValues(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.json")

	if err := persistBackendScopeID(configPath, "   "); err == nil {
		t.Fatal("expected empty backend scope error")
	} else if !strings.Contains(err.Error(), "backend scope ID is required") {
		t.Fatalf("error = %q, want required scope message", err.Error())
	}

	if err := persistBackendScopeID(configPath, "cloud-review-scope"); err == nil {
		t.Fatal("expected invalid local backend scope error")
	} else if !strings.Contains(err.Error(), "not a valid local backend scope") {
		t.Fatalf("error = %q, want invalid local scope message", err.Error())
	}
}

func TestWriteConfigMap_ReplaceFailureWhenPathIsDirectory(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.Mkdir(configPath, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	err := writeConfigMap(configPath, map[string]json.RawMessage{})
	if err == nil {
		t.Fatal("expected replace failure when config path is a directory")
	}
	if !strings.Contains(err.Error(), "replace system config") {
		t.Fatalf("error = %q, want replace failure message", err.Error())
	}
}
