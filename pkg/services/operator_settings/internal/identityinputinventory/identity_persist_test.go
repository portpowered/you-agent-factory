package identityinputinventory

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// persistBackendScopeID is retained here as a test-only oracle for the
// inventory cases that describe the private persistence contract. Production
// callers reach this behavior through Service.EnsureLocalBackendScope.
func persistBackendScopeID(
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	encode operatorsettings.ConfigEncoder,
	configPath string,
	config operatorsettings.Config,
) error {
	backendScopeID := strings.TrimSpace(config.BackendScopeID)
	if backendScopeID == "" {
		return fmt.Errorf("backend scope ID is required")
	}
	if !operatorsettings.IsLocalBackendScopeID(backendScopeID) {
		return fmt.Errorf("backend scope ID %q is not a valid local backend scope", backendScopeID)
	}
	if encode == nil {
		return fmt.Errorf("global config encoder is required")
	}
	data, err := encode(config)
	if err != nil {
		return err
	}
	dir := filepath.Dir(configPath)
	if err := files.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create system config directory %q: %w", dir, err)
	}
	tmp, err := createTemp(dir, filepath.Base(configPath)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create system config temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = files.Remove(tmpPath)
		}
	}()
	if written, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write system config temp file: %w", err)
	} else if written != len(data) {
		_ = tmp.Close()
		return fmt.Errorf("write system config temp file: short write: wrote %d of %d bytes", written, len(data))
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync system config temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close system config temp file: %w", err)
	}
	if err := files.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("set system config temp file permissions: %w", err)
	}
	if err := files.Rename(tmpPath, configPath); err != nil {
		return fmt.Errorf("replace system config with temp file: %w", err)
	}
	cleanup = false
	return nil
}

func TestIndexedPersistScopeCases_MatchProductionLoader(t *testing.T) {
	cases := []struct {
		name           string
		scopeID        string
		wantErrorParts []string
	}{
		{name: "valid-local-scope", scopeID: "local-aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"},
		{name: "invalid-empty-scope", wantErrorParts: []string{"backend scope ID is required"}},
		{name: "invalid-non-local-scope", scopeID: "not-a-local-scope", wantErrorParts: []string{"not a valid local backend scope"}},
		{name: "invalid-provider-scope", scopeID: "provider-codex-account-workspace", wantErrorParts: []string{"not a valid local backend scope"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			runPersistScopeCase(t, testCase.scopeID, testCase.wantErrorParts)
		})
	}
}

func runPersistScopeCase(t *testing.T, scopeID string, wantErrorParts []string) {
	t.Helper()

	configPath := operatorsettings.DefaultConfigPath(t.TempDir())
	err := persistBackendScopeID(testFiles, testCreateTemp, encodeTestConfig, configPath, operatorsettings.Config{BackendScopeID: scopeID})
	if len(wantErrorParts) == 0 {
		if err != nil {
			t.Fatalf("persistBackendScopeID() error = %v, want accept", err)
		}
		return
	}
	if err == nil {
		t.Fatal("persistBackendScopeID() error = nil, want reject")
	}
	for _, fragment := range wantErrorParts {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error = %q, want fragment %q", err.Error(), fragment)
		}
	}
}
