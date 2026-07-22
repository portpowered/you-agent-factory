package operatorsettings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// LocalBackendScopePrefix is the required prefix for generated local backend scopes.
	LocalBackendScopePrefix = "local-"
)

// BackendScopeOutcome reports whether a backend scope was reused or generated.
type BackendScopeOutcome string

const (
	BackendScopeOutcomeReused    BackendScopeOutcome = "reused"
	BackendScopeOutcomeGenerated BackendScopeOutcome = "generated"
)

var localBackendScopePattern = regexp.MustCompile(
	`^local-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`,
)

// ResolvedBackendScope is the effective backend scope after load or generation.
type ResolvedBackendScope struct {
	BackendScopeID string
	Outcome        BackendScopeOutcome
	ConfigPath     string
}

// EnsureLocalBackendScope loads backendScopeID from configPath, generates
// local-<uuid> when missing, and persists a newly generated value before returning.
func EnsureLocalBackendScope(files FileSystem, createTemp CreateTemporaryFile, generateID IDGenerator, configPath string) (ResolvedBackendScope, error) {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return ResolvedBackendScope{}, fmt.Errorf("system config path is required to resolve backend scope")
	}

	existing, err := loadBackendScopeID(files, configPath)
	if err != nil {
		return ResolvedBackendScope{}, err
	}
	if trimmed := strings.TrimSpace(existing); trimmed != "" {
		return ResolvedBackendScope{
			BackendScopeID: trimmed,
			Outcome:        BackendScopeOutcomeReused,
			ConfigPath:     configPath,
		}, nil
	}

	generated := GenerateLocalBackendScopeID(generateID)
	if err := persistBackendScopeID(files, createTemp, configPath, generated); err != nil {
		return ResolvedBackendScope{}, fmt.Errorf(
			"persist generated backend scope ID to system config %q: %w; local backends require a stable backendScopeID before exposing session identity",
			configPath,
			err,
		)
	}
	return ResolvedBackendScope{
		BackendScopeID: generated,
		Outcome:        BackendScopeOutcomeGenerated,
		ConfigPath:     configPath,
	}, nil
}

// GenerateLocalBackendScopeID returns a new local backend scope identifier.
func GenerateLocalBackendScopeID(generateID IDGenerator) string {
	return LocalBackendScopePrefix + generateID()
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

func loadBackendScopeID(files FileSystem, configPath string) (string, error) {
	data, err := files.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read system config %q: %w", configPath, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return "", nil
	}
	var cfg struct {
		BackendScopeID string `json:"backendScopeID"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("parse system config %q: %w", configPath, err)
	}
	return strings.TrimSpace(cfg.BackendScopeID), nil
}

func persistBackendScopeID(files FileSystem, createTemp CreateTemporaryFile, configPath, backendScopeID string) error {
	backendScopeID = strings.TrimSpace(backendScopeID)
	if backendScopeID == "" {
		return fmt.Errorf("backend scope ID is required")
	}
	if !IsLocalBackendScopeID(backendScopeID) {
		return fmt.Errorf("backend scope ID %q is not a valid local backend scope", backendScopeID)
	}

	configMap, err := readConfigMap(files, configPath)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(backendScopeID)
	if err != nil {
		return fmt.Errorf("marshal backend scope ID: %w", err)
	}
	configMap["backendScopeID"] = encoded
	return writeConfigMap(files, createTemp, configPath, configMap)
}

func readConfigMap(files FileSystem, configPath string) (map[string]json.RawMessage, error) {
	data, err := files.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
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

func writeConfigMap(files FileSystem, createTemp CreateTemporaryFile, configPath string, configMap map[string]json.RawMessage) error {
	dir := filepath.Dir(configPath)
	if err := files.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create system config directory %q: %w", dir, err)
	}

	data, err := json.MarshalIndent(configMap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal system config: %w", err)
	}
	data = append(data, '\n')

	tmp, err := createTemp(dir, filepath.Base(configPath)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create system config temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = files.Remove(tmpPath)
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
	if err := files.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("set system config temp file permissions: %w", err)
	}
	if err := files.Rename(tmpPath, configPath); err != nil {
		return fmt.Errorf("replace system config with temp file: %w", err)
	}
	cleanupTemp = false
	return nil
}
