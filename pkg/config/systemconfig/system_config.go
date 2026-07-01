// Package systemconfig loads and persists backend-owned system identity fields
// in the shared operator config file under ~/.you-agent-factory/config.json.
package systemconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
)

const (
	// LocalBackendScopePrefix is the required prefix for generated local backend scopes.
	LocalBackendScopePrefix = "local-"
)

// Outcome reports whether a backend scope was reused from config or generated.
type Outcome string

const (
	OutcomeReused    Outcome = "reused"
	OutcomeGenerated Outcome = "generated"
)

var localBackendScopePattern = regexp.MustCompile(
	`^local-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`,
)

// ResolvedBackendScope is the effective backend scope after load or generation.
type ResolvedBackendScope struct {
	BackendScopeID string
	Outcome        Outcome
	ConfigPath     string
}

// DefaultConfigPath returns the canonical system config path below homeDir.
func DefaultConfigPath(homeDir string) string {
	return defaultpaths.OperatorConfigPath(homeDir)
}

// EnsureLocalBackendScope loads backendScopeID from configPath, generates
// local-<uuid> when missing, and persists a newly generated value before returning.
func EnsureLocalBackendScope(configPath string) (ResolvedBackendScope, error) {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return ResolvedBackendScope{}, fmt.Errorf("system config path is required to resolve backend scope")
	}

	existing, err := loadBackendScopeID(configPath)
	if err != nil {
		return ResolvedBackendScope{}, err
	}
	if trimmed := strings.TrimSpace(existing); trimmed != "" {
		return ResolvedBackendScope{
			BackendScopeID: trimmed,
			Outcome:        OutcomeReused,
			ConfigPath:     configPath,
		}, nil
	}

	generated := GenerateLocalBackendScopeID()
	if err := persistBackendScopeID(configPath, generated); err != nil {
		return ResolvedBackendScope{}, fmt.Errorf(
			"persist generated backend scope ID to system config %q: %w; local backends require a stable backendScopeID before exposing session identity",
			configPath,
			err,
		)
	}
	return ResolvedBackendScope{
		BackendScopeID: generated,
		Outcome:        OutcomeGenerated,
		ConfigPath:     configPath,
	}, nil
}

// GenerateLocalBackendScopeID returns a new local backend scope identifier.
func GenerateLocalBackendScopeID() string {
	return LocalBackendScopePrefix + uuid.NewString()
}

// IsLocalBackendScopeID reports whether value matches the local-<uuid> shape.
func IsLocalBackendScopeID(value string) bool {
	return localBackendScopePattern.MatchString(strings.TrimSpace(value))
}

// DiagnosticsLine returns a redacted diagnostic line for backend scope resolution.
func (r ResolvedBackendScope) DiagnosticsLine() string {
	return fmt.Sprintf(
		"backendScope outcome=%s backendScopeID=%s configPath=%q",
		r.Outcome,
		diagnosticsBackendScopeID(r.BackendScopeID),
		r.ConfigPath,
	)
}

func diagnosticsBackendScopeID(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "unset"
	}
	return trimmed
}

func loadBackendScopeID(configPath string) (string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read system config %q: %w", configPath, err)
	}
	var cfg struct {
		BackendScopeID string `json:"backendScopeID"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("parse system config %q: %w", configPath, err)
	}
	return strings.TrimSpace(cfg.BackendScopeID), nil
}

func persistBackendScopeID(configPath, backendScopeID string) error {
	backendScopeID = strings.TrimSpace(backendScopeID)
	if backendScopeID == "" {
		return fmt.Errorf("backend scope ID is required")
	}
	if !IsLocalBackendScopeID(backendScopeID) {
		return fmt.Errorf("backend scope ID %q is not a valid local backend scope", backendScopeID)
	}

	configMap, err := readConfigMap(configPath)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(backendScopeID)
	if err != nil {
		return fmt.Errorf("marshal backend scope ID: %w", err)
	}
	configMap["backendScopeID"] = encoded
	return writeConfigMap(configPath, configMap)
}

func readConfigMap(configPath string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]json.RawMessage{}, nil
		}
		return nil, fmt.Errorf("read system config %q: %w", configPath, err)
	}
	configMap := map[string]json.RawMessage{}
	if len(strings.TrimSpace(string(data))) == 0 {
		return configMap, nil
	}
	if err := json.Unmarshal(data, &configMap); err != nil {
		return nil, fmt.Errorf("parse system config %q: %w", configPath, err)
	}
	return configMap, nil
}

func writeConfigMap(configPath string, configMap map[string]json.RawMessage) error {
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create system config directory %q: %w", dir, err)
	}

	data, err := json.MarshalIndent(configMap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal system config: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, filepath.Base(configPath)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create system config temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write system config temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync system config temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close system config temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("set system config temp file permissions: %w", err)
	}
	if err := os.Rename(tmpPath, configPath); err != nil {
		return fmt.Errorf("replace system config with temp file: %w", err)
	}
	cleanupTemp = false
	return nil
}
